package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

type fakeVideoResolver struct {
	video VideoInfo
	err   error
}

func (fake *fakeVideoResolver) Resolve(context.Context, *url.URL) (VideoInfo, error) {
	return fake.video, fake.err
}

type fakeMediaOpener struct {
	body []byte
	err  error
}

func (fake *fakeMediaOpener) OpenMedia(context.Context, VideoInfo) (*http.Response, error) {
	if fake.err != nil {
		return nil, fake.err
	}
	return &http.Response{
		StatusCode:    http.StatusOK,
		Header:        http.Header{"Content-Type": []string{"video/mp4"}},
		Body:          io.NopCloser(bytes.NewReader(fake.body)),
		ContentLength: int64(len(fake.body)),
	}, nil
}

func testWebApplication(video VideoInfo, body []byte) *webApplication {
	return &webApplication{
		resolver:   &fakeVideoResolver{video: video},
		downloader: &fakeMediaOpener{body: body},
		tickets:    newDownloadTicketStore(),
	}
}

// TestWebResolveAndDownload covers the browser's complete application contract:
// same-origin JSON resolution, opaque ticket creation, attachment headers, and
// direct media streaming without exposing the upstream URL.
func TestWebResolveAndDownload(t *testing.T) {
	video := VideoInfo{ID: "123", Author: "周期", Title: "测试作品", MediaURL: "https://aweme.snssdk.com/aweme/v1/play/?video_id=test"}
	app := testWebApplication(video, []byte("video-body"))
	resolve := httptest.NewRequest(http.MethodPost, "/api/resolve", strings.NewReader(`{"shareText":"https://v.douyin.com/example/"}`))
	resolve.Host = "127.0.0.1:17846"
	resolve.Header.Set("Origin", "http://127.0.0.1:17846")
	resolve.Header.Set("Content-Type", "application/json")
	resolveRecorder := httptest.NewRecorder()
	app.routes().ServeHTTP(resolveRecorder, resolve)

	if resolveRecorder.Code != http.StatusOK {
		t.Fatalf("resolve status = %d, body = %s", resolveRecorder.Code, resolveRecorder.Body.String())
	}
	var payload resolveResponse
	if err := json.NewDecoder(resolveRecorder.Body).Decode(&payload); err != nil {
		t.Fatalf("decode resolve response: %v", err)
	}
	if payload.Author != "周期" || payload.DownloadURL == "" || strings.Contains(resolveRecorder.Body.String(), video.MediaURL) {
		t.Fatalf("unexpected resolve response: %#v", payload)
	}

	download := httptest.NewRequest(http.MethodGet, payload.DownloadURL, nil)
	downloadResponse := httptest.NewRecorder()
	app.routes().ServeHTTP(downloadResponse, download)
	if downloadResponse.Code != http.StatusOK || downloadResponse.Body.String() != "video-body" {
		t.Fatalf("download status = %d, body = %q", downloadResponse.Code, downloadResponse.Body.String())
	}
	if disposition := downloadResponse.Header().Get("Content-Disposition"); !strings.Contains(disposition, "attachment") {
		t.Fatalf("Content-Disposition = %q", disposition)
	}
}

func TestWebResolveRejectsCrossOriginRequest(t *testing.T) {
	app := testWebApplication(VideoInfo{}, nil)
	request := httptest.NewRequest(http.MethodPost, "/api/resolve", strings.NewReader(`{"shareText":"https://v.douyin.com/example/"}`))
	request.Host = "127.0.0.1:17846"
	request.Header.Set("Origin", "https://untrusted.example")
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()

	app.routes().ServeHTTP(response, request)
	if response.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusForbidden)
	}
}

// TestEmbeddedPagePreflight enforces two taste-skill shipping constraints that
// are cheap to verify mechanically: no visible em/en dashes and no inline assets
// that would require weakening the page's strict content security policy.
func TestEmbeddedPagePreflight(t *testing.T) {
	page, err := embeddedWebAssets.ReadFile("web/index.html")
	if err != nil {
		t.Fatalf("ReadFile(index.html) error = %v", err)
	}
	pageText := string(page)
	if strings.ContainsAny(pageText, "—–") {
		t.Fatal("page contains a forbidden em-dash or en-dash")
	}
	if strings.Contains(pageText, "<style") || strings.Contains(pageText, "<script>") {
		t.Fatal("page contains inline style or script")
	}

	request := httptest.NewRequest(http.MethodGet, "/", nil)
	response := httptest.NewRecorder()
	testWebApplication(VideoInfo{}, nil).routes().ServeHTTP(response, request)
	if response.Code != http.StatusOK || response.Header().Get("Content-Security-Policy") == "" {
		t.Fatalf("static response status = %d, CSP = %q", response.Code, response.Header().Get("Content-Security-Policy"))
	}
}
