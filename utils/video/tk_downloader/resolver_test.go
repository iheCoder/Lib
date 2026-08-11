package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

const sampleRouterData = `{"loaderData":{"video_layout":null,"video_(id)/page":{"videoInfoRes":{"item_list":[{"aweme_id":"7664176772422146725","desc":"丘比特的力气太小","author":{"nickname":"周期"},"video":{"play_addr":{"url_list":["https://aweme.snssdk.com/aweme/v1/playwm/?video_id=test"]}}}]}}}}`

// TestParseVideoInfo locks down the narrow mapping from Douyin's large router
// payload to VideoInfo, including the dynamic loader key and escaped JSON text.
func TestParseVideoInfo(t *testing.T) {
	page := []byte(`<html><script>window._ROUTER_DATA = ` + sampleRouterData + `</script></html>`)
	video, err := parseVideoInfo(page)
	if err != nil {
		t.Fatalf("parseVideoInfo() error = %v", err)
	}
	if video.ID != "7664176772422146725" || video.Author != "周期" || video.Title != "丘比特的力气太小" {
		t.Fatalf("parseVideoInfo() = %#v", video)
	}
	if !strings.Contains(video.MediaURL, "video_id=test") {
		t.Fatalf("MediaURL = %q", video.MediaURL)
	}
}

// TestResolverFollowsRedirect verifies the complete page half of the workflow:
// short URL, redirect, mobile headers, bounded HTML read, and router extraction.
func TestResolverFollowsRedirect(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("User-Agent") != mobileUserAgent {
			t.Errorf("User-Agent = %q", request.Header.Get("User-Agent"))
		}
		if request.URL.Path == "/short" {
			http.Redirect(writer, request, "/share/video/7664176772422146725", http.StatusFound)
			return
		}
		fmt.Fprintf(writer, `<script>window._ROUTER_DATA = %s</script>`, sampleRouterData)
	}))
	defer server.Close()

	allowTestServer := func(candidate *url.URL) bool { return candidate.Host == strings.TrimPrefix(server.URL, "http://") }
	testResolver := &resolver{client: server.Client(), allowPageURL: allowTestServer}
	shareURL, _ := url.Parse(server.URL + "/short")
	video, err := testResolver.Resolve(context.Background(), shareURL)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if video.ID != "7664176772422146725" {
		t.Fatalf("Resolve() = %#v", video)
	}
}

func TestParseVideoInfoExplainsRiskPage(t *testing.T) {
	_, err := parseVideoInfo([]byte(`<script>window.byted_acrawler.init()</script>`))
	if err == nil || !strings.Contains(err.Error(), "风控校验页") {
		t.Fatalf("parseVideoInfo() error = %v", err)
	}
}
