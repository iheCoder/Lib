package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	mobileUserAgent        = "Mozilla/5.0 (iPhone; CPU iPhone OS 17_5 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Mobile/15E148"
	maxSharePageSize       = 4 << 20
	routerDataMarker       = "window._ROUTER_DATA"
	sharePageTimeout       = 30 * time.Second
	preferredOriginalRatio = "2160p"
)

var (
	errRouterWorkMissing = errors.New("_ROUTER_DATA 中没有找到作品媒体")
	errRiskVerification  = errors.New("返回了风控校验页，请稍后重试")
)

type resolver struct {
	client       *http.Client
	allowPageURL urlPolicy
	detail       workDetailResolver
}

// videoDetailResolver is the deliberate seam between cheap SSR parsing and
// the browser fallback. Tests can exercise orchestration without launching a
// browser, while production pays the browser cost only after SSR has drifted.
type workDetailResolver interface {
	ResolveByID(context.Context, string) (WorkInfo, error)
}

type routerEnvelope struct {
	LoaderData map[string]json.RawMessage `json:"loaderData"`
}

type videoPagePayload struct {
	ItemID            string `json:"itemId"`
	VideoInfoResponse struct {
		Items []videoItem `json:"item_list"`
	} `json:"videoInfoRes"`
}

type videoItem struct {
	ID     string `json:"aweme_id"`
	Title  string `json:"desc"`
	Author struct {
		Nickname string `json:"nickname"`
	} `json:"author"`
	Video struct {
		PlayAddress struct {
			URLs []string `json:"url_list"`
		} `json:"play_addr"`
	} `json:"video"`
}

// newResolver configures the production page boundary. Mobile SSR remains the
// cheap first strategy; the injected detail resolver handles current pages that
// retain only itemId and defer the actual record to verified browser JavaScript.
func newResolver() *resolver {
	allow := func(candidate *url.URL) bool {
		return isTrustedHTTPSURL(candidate, shareHostSuffixes)
	}
	return &resolver{
		client:       newRestrictedHTTPClient(sharePageTimeout, allow),
		allowPageURL: allow,
		detail:       newBrowserDetailResolver(),
	}
}

// Resolve downloads one bounded HTML page and converts its embedded router
// state into WorkInfo. Network, HTTP, size, and parse failures are reported as
// distinct stages so page-shape changes are diagnosable from CLI output.
func (r *resolver) Resolve(ctx context.Context, shareURL *url.URL) (WorkInfo, error) {
	if !r.allowPageURL(shareURL) {
		return WorkInfo{}, errors.New("分享地址不在受信任的抖音域名下")
	}
	request, err := newSharePageRequest(ctx, shareURL)
	if err != nil {
		return WorkInfo{}, err
	}
	response, err := r.client.Do(request)
	if err != nil {
		return WorkInfo{}, fmt.Errorf("请求分享页: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return WorkInfo{}, fmt.Errorf("分享页返回 HTTP %d", response.StatusCode)
	}

	page, err := readBoundedPage(response.Body)
	if err != nil {
		return WorkInfo{}, err
	}
	work, itemID, err := parseSharePage(page)
	if err == nil {
		return work, nil
	}
	if itemID == "" {
		itemID = itemIDFromWorkPageURL(response.Request.URL)
	}
	canUseBrowser := errors.Is(err, errRouterWorkMissing) || errors.Is(err, errRiskVerification)
	if !canUseBrowser || itemID == "" || r.detail == nil {
		return WorkInfo{}, fmt.Errorf("解析分享页: %w", err)
	}

	// Newer share pages keep only itemId in SSR and fetch the actual work after
	// browser verification. Falling back here preserves the fast legacy path
	// and makes the upstream contract change explicit instead of guessing fields.
	work, err = r.detail.ResolveByID(ctx, itemID)
	if err != nil {
		return WorkInfo{}, fmt.Errorf("解析浏览器作品详情: %w", err)
	}
	return work, nil
}

// newSharePageRequest centralizes the headers that select Douyin's SSR mobile
// response. Accepting HTML explicitly also makes the request's intent visible
// to future maintainers investigating upstream behavior changes.
func newSharePageRequest(ctx context.Context, shareURL *url.URL) (*http.Request, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, shareURL.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("创建分享页请求: %w", err)
	}
	request.Header.Set("User-Agent", mobileUserAgent)
	request.Header.Set("Accept", "text/html,application/xhtml+xml")
	request.Header.Set("Accept-Language", "zh-CN,zh;q=0.9")
	return request, nil
}

// readBoundedPage prevents an upstream error or redirect mistake from placing
// an unbounded response in memory. One extra byte distinguishes an exactly-full
// valid page from a page that exceeded the configured ceiling.
func readBoundedPage(reader io.Reader) ([]byte, error) {
	page, err := io.ReadAll(io.LimitReader(reader, maxSharePageSize+1))
	if err != nil {
		return nil, fmt.Errorf("读取分享页: %w", err)
	}
	if len(page) > maxSharePageSize {
		return nil, fmt.Errorf("分享页超过 %d 字节限制", maxSharePageSize)
	}
	return page, nil
}

// parseVideoInfo scans loader entries instead of depending on the current
// dynamic key `video_(id)/page`. This keeps the parser tolerant of route-key
// renames while retaining a deliberately small typed JSON model.
func parseVideoInfo(page []byte) (WorkInfo, error) {
	work, _, err := parseSharePage(page)
	return work, err
}

// parseSharePage returns both the old embedded video record and the stable
// itemId retained by the new SSR shape. Keeping both outcomes in one parse
// prevents two independent decoders from drifting apart on the same payload.
func parseSharePage(page []byte) (WorkInfo, string, error) {
	routerJSON, err := extractRouterData(page)
	if err != nil {
		if bytes.Contains(page, []byte("byted_acrawler")) {
			return WorkInfo{}, "", errRiskVerification
		}
		return WorkInfo{}, "", err
	}
	var envelope routerEnvelope
	if err := json.Unmarshal(routerJSON, &envelope); err != nil {
		return WorkInfo{}, "", fmt.Errorf("解码 _ROUTER_DATA: %w", err)
	}
	itemID := ""
	for _, raw := range envelope.LoaderData {
		if itemID == "" {
			itemID = itemIDFromLoaderEntry(raw)
		}
		work, found, err := workFromLoaderEntry(raw)
		if err != nil {
			return WorkInfo{}, itemID, err
		}
		if found {
			return work, itemID, nil
		}
	}
	return WorkInfo{}, itemID, errRouterWorkMissing
}

// itemIDFromWorkPageURL recovers evidence already established by a trusted
// redirect. Both video and image posts expose their decimal ID in the final
// canonical path even when the response body itself is a verification page.
func itemIDFromWorkPageURL(pageURL *url.URL) string {
	if pageURL == nil {
		return ""
	}
	parts := strings.Split(strings.Trim(pageURL.Path, "/"), "/")
	if len(parts) != 2 || (parts[0] != "video" && parts[0] != "note") {
		return ""
	}
	if !isDecimalWorkID(parts[1]) {
		return ""
	}
	return parts[1]
}

// itemIDFromLoaderEntry intentionally accepts only decimal work IDs. The ID is
// later interpolated into a fixed Douyin URL, so rejecting unexpected text here
// keeps the browser navigation boundary as narrow as the HTTP boundary.
func itemIDFromLoaderEntry(raw json.RawMessage) string {
	var payload videoPagePayload
	if err := json.Unmarshal(raw, &payload); err != nil || !isDecimalWorkID(payload.ItemID) {
		return ""
	}
	return payload.ItemID
}

// isDecimalWorkID protects every place that turns an external work ID into a
// fixed browser path or query comparison.
func isDecimalWorkID(itemID string) bool {
	if itemID == "" {
		return false
	}
	for _, character := range itemID {
		if character < '0' || character > '9' {
			return false
		}
	}
	return true
}

// extractRouterData locates only the assignment wrapper with byte operations;
// the nested payload itself is always decoded by encoding/json rather than by
// regular expressions, avoiding corruption on escaped text or nested objects.
func extractRouterData(page []byte) ([]byte, error) {
	markerIndex := bytes.Index(page, []byte(routerDataMarker))
	if markerIndex < 0 {
		return nil, errors.New("页面缺少 window._ROUTER_DATA")
	}
	afterMarker := page[markerIndex+len(routerDataMarker):]
	assignmentIndex := bytes.IndexByte(afterMarker, '=')
	if assignmentIndex < 0 {
		return nil, errors.New("_ROUTER_DATA 赋值不完整")
	}
	jsonStart := afterMarker[assignmentIndex+1:]
	scriptEnd := bytes.Index(jsonStart, []byte("</script>"))
	if scriptEnd < 0 {
		return nil, errors.New("_ROUTER_DATA 脚本没有结束标签")
	}
	return bytes.TrimSpace(bytes.TrimSuffix(bytes.TrimSpace(jsonStart[:scriptEnd]), []byte(";"))), nil
}

// videoFromLoaderEntry treats unrelated loader entries as normal, but once a
// video item is found it validates every field required by the download stage.
// This prevents malformed upstream data from becoming a vague file I/O error.
func workFromLoaderEntry(raw json.RawMessage) (WorkInfo, bool, error) {
	var payload videoPagePayload
	if err := json.Unmarshal(raw, &payload); err != nil || len(payload.VideoInfoResponse.Items) == 0 {
		return WorkInfo{}, false, nil
	}
	item := payload.VideoInfoResponse.Items[0]
	if item.ID == "" || len(item.Video.PlayAddress.URLs) == 0 {
		return WorkInfo{}, false, errors.New("作品数据缺少 ID 或播放地址")
	}
	mediaURL, err := buildOriginalPlaybackURL(item.Video.PlayAddress.URLs[0])
	if err != nil {
		return WorkInfo{}, false, err
	}
	work := WorkInfo{
		ID: item.ID, Author: strings.TrimSpace(item.Author.Nickname),
		Title: strings.TrimSpace(item.Title), Kind: WorkKindVideo,
		Assets: []MediaAsset{{URL: mediaURL.String(), Kind: MediaKindVideo, Extension: ".mp4"}},
	}
	return work, true, work.Validate()
}

// buildOriginalPlaybackURL converts the mobile share page's `/playwm/` gateway
// into Douyin's original-content `/play/` gateway. Parsing and rebuilding the
// URL explicitly preserves the video ID while removing watermark-only options;
// a blind string replacement could silently alter an unrelated path.
func buildOriginalPlaybackURL(rawURL string) (*url.URL, error) {
	mediaURL, err := url.Parse(rawURL)
	if err != nil || !isTrustedHTTPSURL(mediaURL, mediaHostSuffixes) {
		return nil, errors.New("作品播放地址不在受信任的媒体域名下")
	}
	if mediaURL.Path != "/aweme/v1/playwm/" && mediaURL.Path != "/aweme/v1/play/" {
		return nil, fmt.Errorf("不支持的抖音播放入口 %q", mediaURL.Path)
	}
	query := mediaURL.Query()
	if query.Get("video_id") == "" {
		return nil, errors.New("作品播放地址缺少 video_id")
	}
	mediaURL.Path = "/aweme/v1/play/"
	query.Del("logo_name")
	query.Set("ratio", preferredOriginalRatio)
	mediaURL.RawQuery = query.Encode()
	return mediaURL, nil
}
