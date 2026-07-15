package main

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func testApplication(runner commandRunner) *application {
	// Tests discard logs because assertions target the HTTP contract; production
	// logging remains wired through the same application constructor.
	return newApplication(runner, slog.New(slog.NewTextHandler(io.Discard, nil)))
}

// TestPlaybackAPI exercises the HTTP contract independently of macOS defaults.
// This catches accidental JSON field or empty-state changes used by the page.
func TestPlaybackAPI(t *testing.T) {
	runner := &fakeCommandRunner{outputs: map[string]string{lastFileKey: "https://example.com/movie.mp4", lastPositionKey: "42"}}
	request := httptest.NewRequest(http.MethodGet, "/api/playback", nil)
	response := httptest.NewRecorder()

	testApplication(runner).routes().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"name":"movie.mp4"`) {
		t.Fatalf("unexpected body: %s", response.Body.String())
	}
}

func TestResumeAPIRejectsCrossOriginRequest(t *testing.T) {
	runner := &fakeCommandRunner{outputs: map[string]string{lastFileKey: "https://example.com/movie.mp4"}}
	request := httptest.NewRequest(http.MethodPost, "/api/resume", strings.NewReader("{}"))
	request.Header.Set("Origin", "https://untrusted.example")
	request.Header.Set("Content-Type", jsonContentType)
	response := httptest.NewRecorder()

	testApplication(runner).routes().ServeHTTP(response, request)
	if response.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusForbidden)
	}
	if len(runner.started) != 0 {
		t.Fatalf("cross-origin request started IINA: %#v", runner.started)
	}
}

func TestStaticPageIncludesSecurityHeaders(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	response := httptest.NewRecorder()
	testApplication(&fakeCommandRunner{}).routes().ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d", response.Code)
	}
	if response.Header().Get("Content-Security-Policy") == "" {
		t.Fatal("Content-Security-Policy header is missing")
	}
}
