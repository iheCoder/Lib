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

type videoResolver interface {
	Resolve(context.Context, *url.URL) (VideoInfo, error)
}

type mediaOpener interface {
	OpenMedia(context.Context, VideoInfo) (*http.Response, error)
}

type webApplication struct {
	resolver   videoResolver
	downloader mediaOpener
	tickets    *downloadTicketStore
}

type resolveRequest struct {
	ShareText string `json:"shareText"`
}

type resolveResponse struct {
	ID          string `json:"id"`
	Author      string `json:"author"`
	Title       string `json:"title"`
	Filename    string `json:"filename"`
	DownloadURL string `json:"downloadUrl"`
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
	video, err := app.resolver.Resolve(request.Context(), shareURL)
	if err != nil {
		writeAPIError(writer, http.StatusBadGateway, err.Error())
		return
	}
	app.writeResolvedVideo(writer, video)
}

// writeResolvedVideo issues the ticket only after the complete resolve stage
// succeeds, preventing unusable entries from accumulating after upstream errors.
func (app *webApplication) writeResolvedVideo(writer http.ResponseWriter, video VideoInfo) {
	ticket, err := app.tickets.Issue(video)
	if err != nil {
		writeAPIError(writer, http.StatusInternalServerError, "无法创建下载任务")
		return
	}
	writeJSONResponse(writer, http.StatusOK, resolveResponse{
		ID:          video.ID,
		Author:      video.Author,
		Title:       video.Title,
		Filename:    buildFilename(video),
		DownloadURL: "/api/download/" + ticket,
	})
}

// handleDownload validates a short-lived ticket, opens the trusted upstream
// stream, and proxies bytes directly to the browser. Neither the server nor the
// browser needs to buffer the complete video before the save dialog starts.
func (app *webApplication) handleDownload(writer http.ResponseWriter, request *http.Request) {
	video, exists := app.tickets.Get(request.PathValue("ticket"))
	if !exists {
		http.Error(writer, "下载链接已失效，请重新解析", http.StatusNotFound)
		return
	}
	response, err := app.downloader.OpenMedia(request.Context(), video)
	if err != nil {
		http.Error(writer, "无法连接视频源，请重新解析", http.StatusBadGateway)
		return
	}
	defer response.Body.Close()

	writer.Header().Set("Content-Type", response.Header.Get("Content-Type"))
	writer.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": buildFilename(video)}))
	if response.ContentLength >= 0 {
		writer.Header().Set("Content-Length", strconv.FormatInt(response.ContentLength, 10))
	}
	writer.WriteHeader(http.StatusOK)
	_, _ = io.CopyBuffer(writer, response.Body, make([]byte, downloadBufferSize))
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
