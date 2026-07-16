package main

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func testApplication(t *testing.T, runner commandRunner) *application {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return newApplicationWithDataDirectory(runner, logger, t.TempDir())
}

// TestPlaybackAPI locks down the session-oriented JSON contract used by the
// page, including legacy fallback on installations without history artifacts.
func TestPlaybackAPI(t *testing.T) {
	runner := &fakeCommandRunner{outputs: map[string]string{lastFileKey: "https://example.com/movie.mp4", lastPositionKey: "42"}}
	request := httptest.NewRequest(http.MethodGet, "/api/playback", nil)
	response := httptest.NewRecorder()

	testApplication(t, runner).routes().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"sessions":[{"id":"latest"`) || !strings.Contains(response.Body.String(), `"name":"movie.mp4"`) {
		t.Fatalf("unexpected body: %s", response.Body.String())
	}
}

func TestResumeAPIRejectsCrossOriginRequest(t *testing.T) {
	runner := &fakeCommandRunner{outputs: map[string]string{lastFileKey: "https://example.com/movie.mp4"}}
	request := httptest.NewRequest(http.MethodPost, "/api/resume", strings.NewReader(`{"sessionId":"latest"}`))
	request.Header.Set("Origin", "https://untrusted.example")
	request.Header.Set("Content-Type", jsonContentType)
	response := httptest.NewRecorder()

	testApplication(t, runner).routes().ServeHTTP(response, request)
	if response.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusForbidden)
	}
	if len(runner.started) != 0 {
		t.Fatalf("cross-origin request started IINA: %#v", runner.started)
	}
}

func TestResumeAPIRejectsUnknownSession(t *testing.T) {
	runner := &fakeCommandRunner{outputs: map[string]string{lastFileKey: "https://example.com/movie.mp4"}}
	request := httptest.NewRequest(http.MethodPost, "/api/resume", strings.NewReader(`{"sessionId":"unknown"}`))
	request.Host = "127.0.0.1:17845"
	request.Header.Set("Origin", "http://127.0.0.1:17845")
	request.Header.Set("Content-Type", jsonContentType)
	response := httptest.NewRecorder()

	testApplication(t, runner).routes().ServeHTTP(response, request)
	if response.Code != http.StatusConflict {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	if len(runner.started) != 0 {
		t.Fatalf("unknown session started IINA: %#v", runner.started)
	}
}

func TestStaticPageIncludesSecurityHeaders(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	response := httptest.NewRecorder()
	testApplication(t, &fakeCommandRunner{}).routes().ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d", response.Code)
	}
	if response.Header().Get("Content-Security-Policy") == "" {
		t.Fatal("Content-Security-Policy header is missing")
	}
	if response.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("static assets may remain stale across local upgrades")
	}
}
