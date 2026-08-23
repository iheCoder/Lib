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

type fakeWorkResolver struct {
	work WorkInfo
	err  error
}

func (fake *fakeWorkResolver) Resolve(context.Context, *url.URL) (WorkInfo, error) {
	return fake.work, fake.err
}

type fakeMediaOpener struct {
	body []byte
	err  error
}

func (fake *fakeMediaOpener) OpenAsset(_ context.Context, asset MediaAsset) (*http.Response, error) {
	if fake.err != nil {
		return nil, fake.err
	}
	contentType := "video/mp4"
	if asset.Kind == MediaKindImage {
		contentType = "image/jpeg"
	}
	body := fake.body
	if asset.Kind == MediaKindImage && asset.URL != "" {
		body = []byte(asset.URL)
	}
	return &http.Response{
		StatusCode:    http.StatusOK,
		Header:        http.Header{"Content-Type": []string{contentType}},
		Body:          io.NopCloser(bytes.NewReader(body)),
		ContentLength: int64(len(body)),
	}, nil
}

func testWebApplication(work WorkInfo, body []byte) *webApplication {
	return &webApplication{
		resolver:   &fakeWorkResolver{work: work},
		downloader: &fakeMediaOpener{body: body},
		tickets:    newDownloadTicketStore(),
	}
}

// TestWebResolveAndDownload covers the browser's complete application contract:
// same-origin JSON resolution, opaque ticket creation, attachment headers, and
// direct media streaming without exposing the upstream URL.
func TestWebResolveAndDownload(t *testing.T) {
	mediaURL := "https://aweme.snssdk.com/aweme/v1/play/?video_id=test"
	video := WorkInfo{ID: "123", Author: "周期", Title: "测试作品", Kind: WorkKindVideo,
		Assets: []MediaAsset{{URL: mediaURL, Kind: MediaKindVideo, Extension: ".mp4"}}}
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
	if payload.Author != "周期" || payload.DownloadURL == "" || payload.Kind != WorkKindVideo || strings.Contains(resolveRecorder.Body.String(), mediaURL) {
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
	app := testWebApplication(WorkInfo{}, nil)
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
	testWebApplication(WorkInfo{}, nil).routes().ServeHTTP(response, request)
	if response.Code != http.StatusOK || response.Header().Get("Content-Security-Policy") == "" {
		t.Fatalf("static response status = %d, CSP = %q", response.Code, response.Header().Get("Content-Security-Policy"))
	}
}

func TestWebExposesImagePostAsIndividualPreviewsAndDownloads(t *testing.T) {
	work := WorkInfo{ID: "7673098271799701430", Author: "Syoyng", Title: "以你为名的天使主义", Kind: WorkKindImages,
		Assets: []MediaAsset{
			{URL: "first-image", Kind: MediaKindImage, Extension: ".jpeg"},
			{URL: "second-image", Kind: MediaKindImage, Extension: ".jpeg"},
		}}
	app := testWebApplication(work, nil)
	resolve := httptest.NewRequest(http.MethodPost, "/api/resolve", strings.NewReader(`{"shareText":"https://v.douyin.com/example/"}`))
	resolve.Header.Set("Content-Type", "application/json")
	resolveRecorder := httptest.NewRecorder()
	app.routes().ServeHTTP(resolveRecorder, resolve)

	var payload resolveResponse
	if err := json.NewDecoder(resolveRecorder.Body).Decode(&payload); err != nil {
		t.Fatalf("decode resolve response: %v", err)
	}
	if payload.Kind != WorkKindImages || payload.AssetCount != 2 || payload.DownloadURL != "" || len(payload.Assets) != 2 {
		t.Fatalf("resolve response = %#v", payload)
	}
	if payload.Assets[0].PreviewURL == "" || payload.Assets[0].DownloadURL == "" ||
		payload.Assets[0].Filename == payload.Assets[1].Filename {
		t.Fatalf("image actions = %#v", payload.Assets)
	}
	previewRecorder := httptest.NewRecorder()
	app.routes().ServeHTTP(previewRecorder, httptest.NewRequest(http.MethodGet, payload.Assets[0].PreviewURL, nil))
	if previewRecorder.Code != http.StatusOK || previewRecorder.Body.String() != "first-image" ||
		previewRecorder.Header().Get("Content-Disposition") != "" {
		t.Fatalf("preview status = %d headers = %v body = %q", previewRecorder.Code, previewRecorder.Header(), previewRecorder.Body.String())
	}
	downloadRecorder := httptest.NewRecorder()
	app.routes().ServeHTTP(downloadRecorder, httptest.NewRequest(http.MethodGet, payload.Assets[1].DownloadURL, nil))
	if downloadRecorder.Body.String() != "second-image" ||
		!strings.Contains(downloadRecorder.Header().Get("Content-Disposition"), "attachment") {
		t.Fatalf("download headers = %v body = %q", downloadRecorder.Header(), downloadRecorder.Body.String())
	}
}
