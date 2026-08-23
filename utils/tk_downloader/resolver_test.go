package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

const sampleRouterData = `{"loaderData":{"video_layout":null,"video_(id)/page":{"videoInfoRes":{"item_list":[{"aweme_id":"7664176772422146725","desc":"丘比特的力气太小","author":{"nickname":"周期"},"video":{"play_addr":{"url_list":["https://aweme.snssdk.com/aweme/v1/playwm/?video_id=test"]}}}]}}}}`

type fakeDetailResolver struct {
	wantID string
	work   WorkInfo
	err    error
	calls  int
}

// ResolveByID records the fallback boundary so resolver tests can prove that a
// new SSR payload delegates by itemId without launching a real browser.
func (fake *fakeDetailResolver) ResolveByID(_ context.Context, itemID string) (WorkInfo, error) {
	fake.calls++
	if itemID != fake.wantID {
		return WorkInfo{}, fmt.Errorf("itemID = %q, want %q", itemID, fake.wantID)
	}
	return fake.work, fake.err
}

// TestParseVideoInfo locks down the narrow mapping from Douyin's large router
// payload to VideoInfo, including the dynamic loader key and escaped JSON text.
func TestParseVideoInfo(t *testing.T) {
	page := []byte(`<html><script>window._ROUTER_DATA = ` + sampleRouterData + `</script></html>`)
	work, err := parseVideoInfo(page)
	if err != nil {
		t.Fatalf("parseVideoInfo() error = %v", err)
	}
	if work.ID != "7664176772422146725" || work.Author != "周期" || work.Title != "丘比特的力气太小" {
		t.Fatalf("parseVideoInfo() = %#v", work)
	}
	mediaURL, err := url.Parse(work.Assets[0].URL)
	if err != nil {
		t.Fatalf("url.Parse(MediaURL) error = %v", err)
	}
	if mediaURL.Path != "/aweme/v1/play/" || mediaURL.Query().Get("ratio") != preferredOriginalRatio {
		t.Fatalf("MediaURL = %q", work.Assets[0].URL)
	}
}

func TestBuildOriginalPlaybackURLRemovesWatermarkOptions(t *testing.T) {
	rawURL := "https://aweme.snssdk.com/aweme/v1/playwm/?line=0&logo_name=aweme_diversion_search&ratio=720p&video_id=source-id"
	mediaURL, err := buildOriginalPlaybackURL(rawURL)
	if err != nil {
		t.Fatalf("buildOriginalPlaybackURL() error = %v", err)
	}
	if mediaURL.Path != "/aweme/v1/play/" {
		t.Fatalf("Path = %q", mediaURL.Path)
	}
	if mediaURL.Query().Get("video_id") != "source-id" || mediaURL.Query().Get("ratio") != preferredOriginalRatio {
		t.Fatalf("Query = %q", mediaURL.RawQuery)
	}
	if mediaURL.Query().Has("logo_name") {
		t.Fatalf("watermark option remains in %q", mediaURL.RawQuery)
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
	work, err := testResolver.Resolve(context.Background(), shareURL)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if work.ID != "7664176772422146725" {
		t.Fatalf("Resolve() = %#v", work)
	}
}

// TestResolverFallsBackFromItemID covers the exact upstream drift that caused
// the regression: videoInfoRes disappeared, but itemId remained in loaderData.
func TestResolverFallsBackFromItemID(t *testing.T) {
	const itemID = "7654487788875770865"
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		fmt.Fprintf(writer, `<script>window._ROUTER_DATA = {"loaderData":{"video_(id)/page":{"itemId":%q}}}</script>`, itemID)
	}))
	defer server.Close()

	fallback := &fakeDetailResolver{
		wantID: itemID,
		work: WorkInfo{ID: itemID, Author: "陳博榕", Title: "头发乱乱的", Kind: WorkKindVideo,
			Assets: []MediaAsset{{URL: "https://v3.douyinvod.com/video.mp4", Kind: MediaKindVideo, Extension: ".mp4"}}},
	}
	allowTestServer := func(candidate *url.URL) bool { return candidate.Host == strings.TrimPrefix(server.URL, "http://") }
	testResolver := &resolver{client: server.Client(), allowPageURL: allowTestServer, detail: fallback}
	shareURL, _ := url.Parse(server.URL)

	work, err := testResolver.Resolve(context.Background(), shareURL)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if work.ID != itemID || fallback.calls != 1 {
		t.Fatalf("Resolve() = %#v, fallback calls = %d", work, fallback.calls)
	}
}

// TestResolverFallsBackFromRiskPageFinalURL freezes the second recovery path:
// a trusted redirect can reveal the work ID even when SSR is replaced by a
// verification page.
func TestResolverFallsBackFromRiskPageFinalURL(t *testing.T) {
	const itemID = "7673098271799701430"
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/short" {
			http.Redirect(writer, request, "/note/"+itemID, http.StatusFound)
			return
		}
		_, _ = writer.Write([]byte(`<script>window.byted_acrawler.init()</script>`))
	}))
	defer server.Close()

	fallbackWork := WorkInfo{ID: itemID, Kind: WorkKindImages,
		Assets: []MediaAsset{{URL: "https://p3-pc-sign.douyinpic.com/image.jpeg", Kind: MediaKindImage, Extension: ".jpg"}}}
	fallback := &fakeDetailResolver{wantID: itemID, work: fallbackWork}
	allowTestServer := func(candidate *url.URL) bool { return candidate.Host == strings.TrimPrefix(server.URL, "http://") }
	resolver := &resolver{client: server.Client(), allowPageURL: allowTestServer, detail: fallback}
	shareURL, _ := url.Parse(server.URL + "/short")

	work, err := resolver.Resolve(context.Background(), shareURL)
	if err != nil || work.ID != itemID || fallback.calls != 1 {
		t.Fatalf("Resolve() = %#v, %v; fallback calls = %d", work, err, fallback.calls)
	}
}

func TestParseSharePageRejectsNonNumericFallbackID(t *testing.T) {
	page := []byte(`<script>window._ROUTER_DATA = {"loaderData":{"video_(id)/page":{"itemId":"https://attacker.invalid"}}}</script>`)
	_, itemID, err := parseSharePage(page)
	if !errors.Is(err, errRouterWorkMissing) || itemID != "" {
		t.Fatalf("parseSharePage() itemID = %q, error = %v", itemID, err)
	}
}

func TestParseVideoInfoExplainsRiskPage(t *testing.T) {
	_, err := parseVideoInfo([]byte(`<script>window.byted_acrawler.init()</script>`))
	if err == nil || !strings.Contains(err.Error(), "风控校验页") {
		t.Fatalf("parseVideoInfo() error = %v", err)
	}
}
