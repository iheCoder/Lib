package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestDownloaderStreamsAndPreservesExistingFiles covers media redirection,
// content persistence, filename sanitization, and collision-safe publishing.
func TestDownloaderStreamsAndPreservesExistingFiles(t *testing.T) {
	videoBytes := []byte("fake-mp4-content")
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/play" {
			http.Redirect(writer, request, "/cdn/video.mp4", http.StatusFound)
			return
		}
		writer.Header().Set("Content-Type", "video/mp4")
		_, _ = writer.Write(videoBytes)
	}))
	defer server.Close()

	allowTestServer := func(candidate *url.URL) bool { return candidate.Host == strings.TrimPrefix(server.URL, "http://") }
	testDownloader := &downloader{client: server.Client(), allowMediaURL: allowTestServer}
	video := WorkInfo{ID: "123", Author: "作/者", Title: "标题", Kind: WorkKindVideo,
		Assets: []MediaAsset{{URL: server.URL + "/play", Kind: MediaKindVideo, Extension: ".mp4"}}}
	outputDirectory := t.TempDir()

	first, err := testDownloader.Download(context.Background(), video, outputDirectory)
	if err != nil {
		t.Fatalf("first Download() error = %v", err)
	}
	second, err := testDownloader.Download(context.Background(), video, outputDirectory)
	if err != nil {
		t.Fatalf("second Download() error = %v", err)
	}
	if first.Location == second.Location || !strings.Contains(second.Location, " (1).mp4") {
		t.Fatalf("collision paths = %q and %q", first.Location, second.Location)
	}
	assertFileContent(t, first.Location, videoBytes)
	assertFileContent(t, second.Location, videoBytes)
	if matches, _ := filepath.Glob(filepath.Join(outputDirectory, "*.part")); len(matches) != 0 {
		t.Fatalf("partial files remain: %v", matches)
	}
}

func TestDownloaderRejectsHTMLResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "text/html")
		_, _ = writer.Write([]byte("captcha"))
	}))
	defer server.Close()

	allowTestServer := func(*url.URL) bool { return true }
	testDownloader := &downloader{client: server.Client(), allowMediaURL: allowTestServer}
	work := WorkInfo{ID: "123", Kind: WorkKindVideo,
		Assets: []MediaAsset{{URL: server.URL, Kind: MediaKindVideo, Extension: ".mp4"}}}
	_, err := testDownloader.Download(context.Background(), work, t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "不匹配") {
		t.Fatalf("Download() error = %v", err)
	}
}

func TestBuildFilenameRetainsIDWhenTitleIsLong(t *testing.T) {
	video := WorkInfo{ID: "7664176772422146725", Author: "作者", Title: strings.Repeat("长", 200), Kind: WorkKindVideo}
	filename := buildDownloadFilename(video)
	if !strings.HasSuffix(filename, "_7664176772422146725.mp4") {
		t.Fatalf("buildFilename() = %q, work ID was truncated", filename)
	}
	if len([]rune(strings.TrimSuffix(filename, ".mp4"))) > maximumFilenameRunes {
		t.Fatalf("buildFilename() = %q, exceeds rune limit", filename)
	}
}

// TestDownloaderSavesImagePostAsOrderedDirectory covers the CLI image contract:
// one exclusive directory, stable numeric ordering, and exact response bytes.
func TestDownloaderSavesImagePostAsOrderedDirectory(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "image/jpeg")
		_, _ = writer.Write([]byte(strings.TrimPrefix(request.URL.Path, "/")))
	}))
	defer server.Close()

	allowTestServer := func(*url.URL) bool { return true }
	testDownloader := &downloader{client: server.Client(), allowMediaURL: allowTestServer}
	work := WorkInfo{ID: "7673098271799701430", Author: "Syoyng", Title: "以你为名的天使主义", Kind: WorkKindImages,
		Assets: []MediaAsset{
			{URL: server.URL + "/first", Kind: MediaKindImage, Extension: ".jpeg"},
			{URL: server.URL + "/second", Kind: MediaKindImage, Extension: ".jpeg"},
		}}
	result, err := testDownloader.Download(context.Background(), work, t.TempDir())
	if err != nil {
		t.Fatalf("Download() error = %v", err)
	}
	if len(result.Files) != 2 || filepath.Base(result.Files[0]) != "01.jpg" || filepath.Base(result.Files[1]) != "02.jpg" {
		t.Fatalf("Download() result = %#v", result)
	}
	assertFileContent(t, result.Files[0], []byte("first"))
	assertFileContent(t, result.Files[1], []byte("second"))
}

func TestDownloaderRollsBackPartialImagePost(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/second" {
			writer.Header().Set("Content-Type", "text/html")
			return
		}
		writer.Header().Set("Content-Type", "image/jpeg")
		_, _ = writer.Write([]byte("first"))
	}))
	defer server.Close()

	testDownloader := &downloader{client: server.Client(), allowMediaURL: func(*url.URL) bool { return true }}
	work := WorkInfo{ID: "123", Kind: WorkKindImages, Assets: []MediaAsset{
		{URL: server.URL + "/first", Kind: MediaKindImage, Extension: ".jpg"},
		{URL: server.URL + "/second", Kind: MediaKindImage, Extension: ".jpg"},
	}}
	outputDirectory := t.TempDir()
	if _, err := testDownloader.Download(context.Background(), work, outputDirectory); err == nil {
		t.Fatal("Download() unexpectedly succeeded")
	}
	entries, err := os.ReadDir(outputDirectory)
	if err != nil || len(entries) != 0 {
		t.Fatalf("partial image directory remains: entries=%v error=%v", entries, err)
	}
}

// assertFileContent keeps persistence assertions consistent without hiding the
// path being inspected when a download test fails.
func assertFileContent(t *testing.T, path string, want []byte) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%q) error = %v", path, err)
	}
	if string(got) != string(want) {
		t.Fatalf("ReadFile(%q) = %q, want %q", path, got, want)
	}
}
