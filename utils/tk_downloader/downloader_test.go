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
	video := VideoInfo{ID: "123", Author: "作/者", Title: "标题", MediaURL: server.URL + "/play"}
	outputDirectory := t.TempDir()

	first, err := testDownloader.Download(context.Background(), video, outputDirectory)
	if err != nil {
		t.Fatalf("first Download() error = %v", err)
	}
	second, err := testDownloader.Download(context.Background(), video, outputDirectory)
	if err != nil {
		t.Fatalf("second Download() error = %v", err)
	}
	if first == second || !strings.Contains(second, " (1).mp4") {
		t.Fatalf("collision paths = %q and %q", first, second)
	}
	assertFileContent(t, first, videoBytes)
	assertFileContent(t, second, videoBytes)
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
	_, err := testDownloader.Download(context.Background(), VideoInfo{ID: "123", MediaURL: server.URL}, t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "非视频内容") {
		t.Fatalf("Download() error = %v", err)
	}
}

func TestBuildFilenameRetainsIDWhenTitleIsLong(t *testing.T) {
	video := VideoInfo{ID: "7664176772422146725", Author: "作者", Title: strings.Repeat("长", 200)}
	filename := buildFilename(video)
	if !strings.HasSuffix(filename, "_7664176772422146725.mp4") {
		t.Fatalf("buildFilename() = %q, work ID was truncated", filename)
	}
	if len([]rune(strings.TrimSuffix(filename, ".mp4"))) > maximumFilenameRunes {
		t.Fatalf("buildFilename() = %q, exceeds rune limit", filename)
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
