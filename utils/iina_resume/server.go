package main

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"io/fs"
	"log/slog"
	"net/http"
	"time"
)

const (
	apiTimeout      = 3 * time.Second
	jsonContentType = "application/json"
)

//go:embed web/*
var webAssets embed.FS

type application struct {
	iina   *iinaService
	logger *slog.Logger
}

// newApplication composes domain and transport dependencies without exposing
// concrete command execution to HTTP handlers.
func newApplication(runner commandRunner, logger *slog.Logger) *application {
	return newApplicationWithDataDirectory(runner, logger, defaultIINADataDirectory())
}

// newApplicationWithDataDirectory is the test seam for IINA's on-disk state;
// production callers always use the standard Application Support directory.
func newApplicationWithDataDirectory(runner commandRunner, logger *slog.Logger, dataDirectory string) *application {
	return &application{iina: newIINAService(runner, dataDirectory), logger: logger}
}

// routes exposes the smallest useful surface. The server binds to loopback,
// while the POST endpoint additionally rejects cross-origin browser requests
// so an unrelated website cannot silently launch a local video.
func (app *application) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/playback", app.handlePlayback)
	mux.HandleFunc("POST /api/resume", app.handleResume)
	mux.Handle("/", app.staticHandler())
	return app.securityHeaders(mux)
}

func (app *application) staticHandler() http.Handler {
	// fs.Sub removes the internal `web/` prefix so browser paths remain clean
	// and independent of the Go embedding layout.
	webRoot, err := fs.Sub(webAssets, "web")
	if err != nil {
		panic("embedded web assets are unavailable: " + err.Error())
	}
	return http.FileServer(http.FS(webRoot))
}

// handlePlayback returns recent shutdown batches. Absence is a valid empty
// state so the page can explain how to create the first IINA record.
func (app *application) handlePlayback(writer http.ResponseWriter, request *http.Request) {
	ctx, cancel := context.WithTimeout(request.Context(), apiTimeout)
	defer cancel()

	sessions, err := app.iina.RecentSessions(ctx)
	if errors.Is(err, errNoPlaybackHistory) {
		writeJSON(writer, http.StatusOK, map[string]any{"sessions": []playbackSession{}})
		return
	}
	if err != nil {
		app.writeError(writer, http.StatusInternalServerError, "无法读取 IINA 播放记录", err)
		return
	}
	writeJSON(writer, http.StatusOK, map[string]any{"sessions": sessions})
}

type resumeRequest struct {
	SessionID string `json:"sessionId"`
}

// handleResume accepts only a session ID. The backend re-reads IINA's catalog,
// eliminating arbitrary-command and arbitrary-file inputs from the browser.
func (app *application) handleResume(writer http.ResponseWriter, request *http.Request) {
	if !sameOrigin(request) || request.Header.Get("Content-Type") != jsonContentType {
		writeJSON(writer, http.StatusForbidden, map[string]string{"error": "请求来源无效"})
		return
	}
	var input resumeRequest
	if err := json.NewDecoder(http.MaxBytesReader(writer, request.Body, 1024)).Decode(&input); err != nil || input.SessionID == "" {
		writeJSON(writer, http.StatusBadRequest, map[string]string{"error": "会话标识无效"})
		return
	}
	ctx, cancel := context.WithTimeout(request.Context(), apiTimeout)
	defer cancel()
	session, err := app.iina.ResumeSession(ctx, input.SessionID)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, errNoPlaybackHistory) || errors.Is(err, errSourceUnavailable) || errors.Is(err, errSessionNotFound) {
			status = http.StatusConflict
		}
		app.writeError(writer, status, userFacingError(err), err)
		return
	}
	writeJSON(writer, http.StatusOK, map[string]any{"session": session})
}

func (app *application) writeError(writer http.ResponseWriter, status int, message string, cause error) {
	// Logs retain the diagnostic cause, while clients only receive stable text
	// that does not expose local paths or command details.
	app.logger.Warn("request failed", "status", status, "error", cause)
	writeJSON(writer, status, map[string]string{"error": message})
}

func userFacingError(err error) string {
	// Domain errors are intentionally mapped here, keeping presentation language
	// out of the IINA integration layer.
	if errors.Is(err, errNoPlaybackHistory) {
		return "还没有找到上次播放的视频"
	}
	if errors.Is(err, errSourceUnavailable) {
		return "该会话中的视频文件均已移动，或所在磁盘尚未连接"
	}
	if errors.Is(err, errSessionNotFound) {
		return "该会话已经变化，请刷新页面后重试"
	}
	return "未能启动 IINA，请稍后重试"
}

func sameOrigin(request *http.Request) bool {
	// Browsers supply Origin for fetch POSTs. Requiring an exact host match blocks
	// drive-by requests from unrelated pages even though the service is local.
	origin := request.Header.Get("Origin")
	return origin == "http://"+request.Host || origin == "https://"+request.Host
}

func writeJSON(writer http.ResponseWriter, status int, payload any) {
	// Every API response shares one encoding path, preventing divergent content
	// types and status ordering across handlers.
	writer.Header().Set("Content-Type", jsonContentType+"; charset=utf-8")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(payload)
}

func (app *application) securityHeaders(next http.Handler) http.Handler {
	// Assets are fully embedded, so a self-only CSP is both sufficient and a
	// useful guard against accidental external dependencies introduced later.
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Cache-Control", "no-store")
		writer.Header().Set("Content-Security-Policy", "default-src 'self'; style-src 'self'; script-src 'self'; connect-src 'self'; img-src 'self' data:")
		writer.Header().Set("Referrer-Policy", "no-referrer")
		writer.Header().Set("X-Content-Type-Options", "nosniff")
		next.ServeHTTP(writer, request)
	})
}
