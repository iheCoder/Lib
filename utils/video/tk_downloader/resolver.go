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
	mobileUserAgent  = "Mozilla/5.0 (iPhone; CPU iPhone OS 17_5 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Mobile/15E148"
	maxSharePageSize = 4 << 20
	routerDataMarker = "window._ROUTER_DATA"
	sharePageTimeout = 30 * time.Second
)

// VideoInfo is the stable boundary between Douyin page parsing and file
// downloading. Keeping only required fields isolates the downloader from the
// much larger and frequently changing SSR payload.
type VideoInfo struct {
	ID       string
	Author   string
	Title    string
	MediaURL string
}

type resolver struct {
	client       *http.Client
	allowPageURL urlPolicy
}

type routerEnvelope struct {
	LoaderData map[string]json.RawMessage `json:"loaderData"`
}

type videoPagePayload struct {
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

// newResolver configures the production page boundary. A mobile user agent is
// supplied per request because Douyin's mobile share page currently renders the
// work data server-side, while the desktop page may return an anti-bot bootstrap.
func newResolver() *resolver {
	allow := func(candidate *url.URL) bool {
		return isTrustedHTTPSURL(candidate, shareHostSuffixes)
	}
	return &resolver{
		client:       newRestrictedHTTPClient(sharePageTimeout, allow),
		allowPageURL: allow,
	}
}

// Resolve downloads one bounded HTML page and converts its embedded router
// state into VideoInfo. Network, HTTP, size, and parse failures are reported as
// distinct stages so page-shape changes are diagnosable from CLI output.
func (r *resolver) Resolve(ctx context.Context, shareURL *url.URL) (VideoInfo, error) {
	if !r.allowPageURL(shareURL) {
		return VideoInfo{}, errors.New("分享地址不在受信任的抖音域名下")
	}
	request, err := newSharePageRequest(ctx, shareURL)
	if err != nil {
		return VideoInfo{}, err
	}
	response, err := r.client.Do(request)
	if err != nil {
		return VideoInfo{}, fmt.Errorf("请求分享页: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return VideoInfo{}, fmt.Errorf("分享页返回 HTTP %d", response.StatusCode)
	}

	page, err := readBoundedPage(response.Body)
	if err != nil {
		return VideoInfo{}, err
	}
	video, err := parseVideoInfo(page)
	if err != nil {
		return VideoInfo{}, fmt.Errorf("解析分享页: %w", err)
	}
	return video, nil
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
func parseVideoInfo(page []byte) (VideoInfo, error) {
	routerJSON, err := extractRouterData(page)
	if err != nil {
		if bytes.Contains(page, []byte("byted_acrawler")) {
			return VideoInfo{}, errors.New("抖音返回了风控校验页，请稍后重试")
		}
		return VideoInfo{}, err
	}
	var envelope routerEnvelope
	if err := json.Unmarshal(routerJSON, &envelope); err != nil {
		return VideoInfo{}, fmt.Errorf("解码 _ROUTER_DATA: %w", err)
	}
	for _, raw := range envelope.LoaderData {
		video, found, err := videoFromLoaderEntry(raw)
		if err != nil {
			return VideoInfo{}, err
		}
		if found {
			return video, nil
		}
	}
	return VideoInfo{}, errors.New("_ROUTER_DATA 中没有找到视频作品")
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
func videoFromLoaderEntry(raw json.RawMessage) (VideoInfo, bool, error) {
	var payload videoPagePayload
	if err := json.Unmarshal(raw, &payload); err != nil || len(payload.VideoInfoResponse.Items) == 0 {
		return VideoInfo{}, false, nil
	}
	item := payload.VideoInfoResponse.Items[0]
	if item.ID == "" || len(item.Video.PlayAddress.URLs) == 0 {
		return VideoInfo{}, false, errors.New("作品数据缺少 ID 或播放地址")
	}
	mediaURL, err := url.Parse(item.Video.PlayAddress.URLs[0])
	if err != nil || !isTrustedHTTPSURL(mediaURL, mediaHostSuffixes) {
		return VideoInfo{}, false, errors.New("作品播放地址不在受信任的媒体域名下")
	}
	return VideoInfo{
		ID:       item.ID,
		Author:   strings.TrimSpace(item.Author.Nickname),
		Title:    strings.TrimSpace(item.Title),
		MediaURL: mediaURL.String(),
	}, true, nil
}
