// Package httpapi exposes ToolHub's loopback-only dashboard and management API.
package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"net/url"
	"strings"

	"Lib/utils/toolhub/internal/supervisor"
)

const maximumRequestBytes = 64 << 10

// Server translates HTTP actions into narrow supervisor operations.
type Server struct {
	manager *supervisor.Manager
	web     fs.FS
}

type startRequest struct {
	Inputs map[string]string `json:"inputs"`
}

// NewHandler composes API routes before the static fallback so unknown API
// paths cannot accidentally return index.html with a successful status.
func NewHandler(manager *supervisor.Manager, web fs.FS) http.Handler {
	server := &Server{manager: manager, web: web}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/tools", server.listTools)
	mux.HandleFunc("GET /api/tools/{id}/logs", server.getLogs)
	mux.HandleFunc("POST /api/tools/{id}/start", server.startTool)
	mux.HandleFunc("POST /api/tools/{id}/stop", server.stopTool)
	mux.Handle("GET /", http.FileServer(http.FS(web)))
	return server.securityHeaders(mux)
}

// listTools returns one consistent snapshot and never triggers a slow network
// health check in the request path; the background monitor owns probing.
func (server *Server) listTools(writer http.ResponseWriter, _ *http.Request) {
	writeJSON(writer, http.StatusOK, map[string]any{"tools": server.manager.List()})
}

// getLogs exposes only the bounded in-memory tail retained by the supervisor.
func (server *Server) getLogs(writer http.ResponseWriter, request *http.Request) {
	logs, err := server.manager.Logs(request.PathValue("id"))
	if err != nil {
		writeManagerError(writer, err)
		return
	}
	writeJSON(writer, http.StatusOK, logs)
}

// startTool validates the browser boundary and structured task inputs before
// handing asynchronous startup to the supervisor.
func (server *Server) startTool(writer http.ResponseWriter, request *http.Request) {
	if !validMutationRequest(request) {
		writeJSON(writer, http.StatusForbidden, map[string]string{"error": "请求来源无效"})
		return
	}
	var payload startRequest
	if err := decodeJSON(writer, request, &payload); err != nil {
		writeJSON(writer, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if err := server.manager.Start(request.PathValue("id"), payload.Inputs); err != nil {
		writeManagerError(writer, err)
		return
	}
	writeJSON(writer, http.StatusAccepted, map[string]string{"status": supervisor.StatusStarting})
}

// stopTool is idempotent for already-stopped owned tools but refuses external
// processes so a port collision can never become an accidental kill action.
func (server *Server) stopTool(writer http.ResponseWriter, request *http.Request) {
	if !validMutationRequest(request) {
		writeJSON(writer, http.StatusForbidden, map[string]string{"error": "请求来源无效"})
		return
	}
	if err := decodeJSON(writer, request, &struct{}{}); err != nil {
		writeJSON(writer, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if err := server.manager.Stop(request.PathValue("id")); err != nil {
		writeManagerError(writer, err)
		return
	}
	writeJSON(writer, http.StatusAccepted, map[string]string{"status": supervisor.StatusStopping})
}

// validMutationRequest blocks cross-site browser POSTs against localhost while
// retaining support for direct local API clients that do not send Origin.
func validMutationRequest(request *http.Request) bool {
	if !strings.HasPrefix(request.Header.Get("Content-Type"), "application/json") {
		return false
	}
	origin := request.Header.Get("Origin")
	if origin == "" {
		return true
	}
	parsed, err := url.Parse(origin)
	return err == nil && parsed.Scheme == "http" && parsed.Host == request.Host
}

// decodeJSON enforces a small body limit and rejects unknown fields so client
// and server versions cannot silently disagree about an action.
func decodeJSON(writer http.ResponseWriter, request *http.Request, target any) error {
	reader := http.MaxBytesReader(writer, request.Body, maximumRequestBytes)
	decoder := json.NewDecoder(reader)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("请求内容无效：%w", err)
	}
	return nil
}

// writeManagerError maps domain failures to recovery-friendly HTTP semantics.
func writeManagerError(writer http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	message := err.Error()
	switch {
	case errors.Is(err, supervisor.ErrNotFound):
		status = http.StatusNotFound
	case errors.Is(err, supervisor.ErrAlreadyActive):
		status, message = http.StatusConflict, "工具已经在运行或正在切换状态"
	case errors.Is(err, supervisor.ErrNotOwned):
		status, message = http.StatusConflict, "该实例不是由 ToolHub 启动，不能从这里停止"
	}
	writeJSON(writer, status, map[string]string{"error": message})
}

// writeJSON centralizes content type and prevents accidental HTML error bodies
// from complicating frontend recovery.
func writeJSON(writer http.ResponseWriter, status int, payload any) {
	writer.Header().Set("Content-Type", "application/json; charset=utf-8")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(payload)
}

// securityHeaders protect local tools from framing and keep the dashboard free
// of third-party scripts, fonts, images, and network calls.
func (server *Server) securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Security-Policy", "default-src 'self'; connect-src 'self'; img-src 'self'; style-src 'self'; script-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'")
		writer.Header().Set("Referrer-Policy", "no-referrer")
		writer.Header().Set("X-Content-Type-Options", "nosniff")
		writer.Header().Set("X-Frame-Options", "DENY")
		next.ServeHTTP(writer, request)
	})
}
