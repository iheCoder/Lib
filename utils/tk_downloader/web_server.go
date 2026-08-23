package main

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"mime"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	defaultWebAddress  = "127.0.0.1:17846"
	maximumAPIRequest  = 16 << 10
	webShutdownTimeout = 5 * time.Second
)

//go:embed web/*
var embeddedWebAssets embed.FS

type workResolver interface {
	Resolve(context.Context, *url.URL) (WorkInfo, error)
}

type mediaOpener interface {
	OpenAsset(context.Context, MediaAsset) (*http.Response, error)
}

type webApplication struct {
	resolver   workResolver
	downloader mediaOpener
	tickets    *downloadTicketStore
}

type resolveRequest struct {
	ShareText string `json:"shareText"`
}

type resolveResponse struct {
	ID          string          `json:"id"`
	Author      string          `json:"author"`
	Title       string          `json:"title"`
	Filename    string          `json:"filename"`
	DownloadURL string          `json:"downloadUrl"`
	Kind        WorkKind        `json:"kind"`
	AssetCount  int             `json:"assetCount"`
	Assets      []resolvedAsset `json:"assets,omitempty"`
}

type resolvedAsset struct {
	Index       int    `json:"index"`
	Filename    string `json:"filename"`
	PreviewURL  string `json:"previewUrl"`
	DownloadURL string `json:"downloadUrl"`
	Width       int    `json:"width,omitempty"`
	Height      int    `json:"height,omitempty"`
}

// newWebApplication composes the production dependencies behind narrow
// interfaces. Tests can replace upstream Douyin access without changing handler
// behavior or weakening outbound URL validation in the real server.
func newWebApplication() *webApplication {
	return &webApplication{
		resolver:   newResolver(),
		downloader: newDownloader(),
		tickets:    newDownloadTicketStore(),
	}
}

// routes exposes one resolution action and one ticketed media stream. Static
// assets are embedded into the binary, so the web mode has no runtime file-path
// dependency and can be launched from any working directory.
func (app *webApplication) routes() http.Handler {
	webRoot, err := fs.Sub(embeddedWebAssets, "web")
	if err != nil {
		panic("embedded web assets are unavailable: " + err.Error())
	}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/resolve", app.handleResolve)
	mux.HandleFunc("GET /api/preview/{ticket}", app.handlePreview)
	mux.HandleFunc("GET /api/download/{ticket}", app.handleDownload)
	mux.Handle("GET /", http.FileServer(http.FS(webRoot)))
	return app.securityHeaders(mux)
}

// handleResolve validates the browser boundary before contacting Douyin. The
// response contains display metadata and an opaque local ticket, never the
// upstream media URL itself.
func (app *webApplication) handleResolve(writer http.ResponseWriter, request *http.Request) {
	if !isJSONRequest(request) || !isSameOrigin(request) {
		writeAPIError(writer, http.StatusForbidden, "请求来源无效")
		return
	}
	var input resolveRequest
	reader := http.MaxBytesReader(writer, request.Body, maximumAPIRequest)
	if err := json.NewDecoder(reader).Decode(&input); err != nil || strings.TrimSpace(input.ShareText) == "" {
		writeAPIError(writer, http.StatusBadRequest, "请粘贴有效的抖音分享文本或链接")
		return
	}
	shareURL, err := extractShareURL(input.ShareText)
	if err != nil {
		writeAPIError(writer, http.StatusBadRequest, err.Error())
		return
	}
	work, err := app.resolver.Resolve(request.Context(), shareURL)
	if err != nil {
		writeAPIError(writer, http.StatusBadGateway, err.Error())
		return
	}
	app.writeResolvedWork(writer, work)
}

// writeResolvedWork issues the ticket only after the complete resolve stage
// succeeds, preventing unusable entries from accumulating after upstream errors.
func (app *webApplication) writeResolvedWork(writer http.ResponseWriter, work WorkInfo) {
	if err := work.Validate(); err != nil {
		writeAPIError(writer, http.StatusBadGateway, err.Error())
		return
	}
	response := resolveResponse{
		ID:         work.ID,
		Author:     work.Author,
		Title:      work.Title,
		Kind:       work.Kind,
		AssetCount: len(work.Assets),
	}
	if err := app.attachMediaTickets(work, &response); err != nil {
		writeAPIError(writer, http.StatusInternalServerError, "无法创建下载任务")
		return
	}
	writeJSONResponse(writer, http.StatusOK, response)
}

// attachMediaTickets creates a single video action or one preview/download pair
// per image. Upstream URLs remain server-side in both response shapes.
func (app *webApplication) attachMediaTickets(work WorkInfo, response *resolveResponse) error {
	if !work.IsImagePost() {
		ticket, err := app.tickets.Issue(work, 0)
		if err != nil {
			return err
		}
		response.Filename = buildDownloadFilename(work)
		response.DownloadURL = "/api/download/" + ticket
		return nil
	}
	for index, asset := range work.Assets {
		ticket, err := app.tickets.Issue(work, index)
		if err != nil {
			return err
		}
		response.Assets = append(response.Assets, resolvedAsset{
			Index: index + 1, Filename: buildIndividualImageFilename(work, index, asset),
			PreviewURL: "/api/preview/" + ticket, DownloadURL: "/api/download/" + ticket,
			Width: asset.Width, Height: asset.Height,
		})
	}
	return nil
}

// handlePreview serves a same-origin inline image. The route is never returned
// for videos, but the same ticket validation still protects accidental calls.
func (app *webApplication) handlePreview(writer http.ResponseWriter, request *http.Request) {
	app.serveTicketMedia(writer, request, false)
}

// handleDownload serves one attachment. Image posts therefore remain useful
// when one CDN object expires without forcing the user to download a full set.
func (app *webApplication) handleDownload(writer http.ResponseWriter, request *http.Request) {
	app.serveTicketMedia(writer, request, true)
}

// serveTicketMedia is the shared capability boundary for previews and saves.
// It reopens and revalidates the upstream asset on every browser request.
func (app *webApplication) serveTicketMedia(writer http.ResponseWriter, request *http.Request, attachment bool) {
	grant, exists := app.tickets.Get(request.PathValue("ticket"))
	if !exists {
		http.Error(writer, "媒体链接已失效，请重新解析", http.StatusNotFound)
		return
	}
	asset, exists := grant.Asset()
	if !exists {
		http.Error(writer, "媒体任务无效，请重新解析", http.StatusNotFound)
		return
	}
	response, err := app.downloader.OpenAsset(request.Context(), asset)
	if err != nil {
		http.Error(writer, "无法连接媒体源，请重新解析", http.StatusBadGateway)
		return
	}
	defer response.Body.Close()
	copyMediaHeaders(writer, response, grant, asset, attachment)
	writer.WriteHeader(http.StatusOK)
	_, _ = io.CopyBuffer(writer, response.Body, make([]byte, downloadBufferSize))
}

// copyMediaHeaders preserves type and length while adding attachment metadata
// only for explicit downloads, allowing preview responses to render in <img>.
func copyMediaHeaders(writer http.ResponseWriter, response *http.Response, grant mediaGrant, asset MediaAsset, attachment bool) {
	writer.Header().Set("Content-Type", response.Header.Get("Content-Type"))
	if attachment {
		filename := buildDownloadFilename(grant.Work)
		if grant.Work.IsImagePost() {
			filename = buildIndividualImageFilename(grant.Work, grant.AssetIndex, asset)
		}
		writer.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": filename}))
	}
	if response.ContentLength >= 0 {
		writer.Header().Set("Content-Length", strconv.FormatInt(response.ContentLength, 10))
	}
}

// isJSONRequest parses parameters such as charset instead of comparing the raw
// header string, while still rejecting browser form posts that can bypass CORS
// preflight and trigger local-network actions from another site.
func isJSONRequest(request *http.Request) bool {
	mediaType, _, err := mime.ParseMediaType(request.Header.Get("Content-Type"))
	return err == nil && mediaType == "application/json"
}

// isSameOrigin accepts command-line clients with no Origin header, but browser
// requests must match the local server host exactly. JSON content type plus this
// check closes the normal cross-site request path to the loopback service.
func isSameOrigin(request *http.Request) bool {
	origin := request.Header.Get("Origin")
	if origin == "" {
		return true
	}
	parsed, err := url.Parse(origin)
	return err == nil && parsed.Host == request.Host && (parsed.Scheme == "http" || parsed.Scheme == "https")
}

func writeAPIError(writer http.ResponseWriter, status int, message string) {
	writeJSONResponse(writer, status, map[string]string{"error": message})
}

// writeJSONResponse is the only JSON output path, keeping content type and
// status ordering consistent across successful and failed API responses.
func writeJSONResponse(writer http.ResponseWriter, status int, payload any) {
	writer.Header().Set("Content-Type", "application/json; charset=utf-8")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(payload)
}

// securityHeaders locks the embedded page to its own assets and API. The page
// intentionally has no third-party scripts, font requests, or analytics.
func (app *webApplication) securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Cache-Control", "no-store")
		writer.Header().Set("Content-Security-Policy", "default-src 'self'; img-src 'self'; style-src 'self'; script-src 'self'; connect-src 'self'; object-src 'none'; base-uri 'none'; frame-ancestors 'none'")
		writer.Header().Set("Referrer-Policy", "no-referrer")
		writer.Header().Set("X-Content-Type-Options", "nosniff")
		writer.Header().Set("X-Frame-Options", "DENY")
		next.ServeHTTP(writer, request)
	})
}

// runWebServer owns the HTTP lifecycle and performs a bounded shutdown when the
// process context is cancelled. In-flight downloads receive the grace window
// before the server closes their connections.
func runWebServer(ctx context.Context, address string, output io.Writer) error {
	server := &http.Server{
		Addr:              address,
		Handler:           newWebApplication().routes(),
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	serverErrors := make(chan error, 1)
	go func() { serverErrors <- server.ListenAndServe() }()
	fmt.Fprintf(output, "网页端已启动: http://%s\n按 Ctrl+C 停止服务\n", address)
	select {
	case err := <-serverErrors:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		shutdownContext, cancel := context.WithTimeout(context.Background(), webShutdownTimeout)
		defer cancel()
		return server.Shutdown(shutdownContext)
	}
}
