package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
)

const (
	browserDetailTimeout = 35 * time.Second
	detailAPIPath        = "/aweme/v1/web/aweme/detail/"
)

type browserDetailResolver struct {
	executable string
	timeout    time.Duration
	slot       chan struct{}
}

type browserResponse struct {
	body []byte
	err  error
}

type rawDetailResponse struct {
	StatusCode  int           `json:"status_code"`
	StatusMsg   string        `json:"status_msg"`
	AwemeDetail rawDetailItem `json:"aweme_detail"`
}

type rawDetailItem struct {
	ID     string `json:"aweme_id"`
	Title  string `json:"desc"`
	Author struct {
		Nickname string `json:"nickname"`
	} `json:"author"`
	Video  rawDetailVideo   `json:"video"`
	Images []rawDetailImage `json:"images"`
}

type rawDetailImage struct {
	Width  int      `json:"width"`
	Height int      `json:"height"`
	URLs   []string `json:"url_list"`
}

type rawDetailVideo struct {
	Width           int               `json:"width"`
	Height          int               `json:"height"`
	PlayAddress     rawMediaAddress   `json:"play_addr"`
	H264PlayAddress rawMediaAddress   `json:"play_addr_h264"`
	BitRates        []rawVideoBitRate `json:"bit_rate"`
}

type rawMediaAddress struct {
	Width    int      `json:"width"`
	Height   int      `json:"height"`
	DataSize int64    `json:"data_size"`
	URLs     []string `json:"url_list"`
}

type rawVideoBitRate struct {
	BitRate     int             `json:"bit_rate"`
	GearName    string          `json:"gear_name"`
	IsH265      int             `json:"is_h265"`
	PlayAddress rawMediaAddress `json:"play_addr"`
}

type mediaCandidate struct {
	address rawMediaAddress
	bitRate int
}

// newBrowserDetailResolver finds a locally installed Chromium-family browser.
// Missing browsers are represented by an empty path so startup still succeeds;
// users with legacy SSR links should not be blocked by an unused fallback.
func newBrowserDetailResolver() workDetailResolver {
	return &browserDetailResolver{
		executable: findBrowserExecutable(),
		timeout:    browserDetailTimeout,
		slot:       make(chan struct{}, 1),
	}
}

// ResolveByID runs one isolated browser only when Douyin withheld work data
// from SSR. A single slot prevents concurrent web requests from spawning an
// unbounded number of Chrome processes on the user's machine.
func (r *browserDetailResolver) ResolveByID(ctx context.Context, itemID string) (WorkInfo, error) {
	if !isDecimalWorkID(itemID) {
		return WorkInfo{}, errors.New("浏览器解析收到无效作品 ID")
	}
	if r.executable == "" {
		return WorkInfo{}, errors.New("当前分享页需要浏览器解析，但未找到 Chrome、Chromium 或 Edge")
	}
	if err := acquireBrowserSlot(ctx, r.slot); err != nil {
		return WorkInfo{}, err
	}
	defer func() { <-r.slot }()

	timedCtx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()
	body, err := captureDetailResponse(timedCtx, r.executable, itemID)
	if err != nil {
		return WorkInfo{}, err
	}
	return parseBrowserDetail(body, itemID)
}

// acquireBrowserSlot makes queue cancellation visible instead of leaving a web
// request blocked after its client has gone away.
func acquireBrowserSlot(ctx context.Context, slot chan struct{}) error {
	select {
	case slot <- struct{}{}:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("等待浏览器解析: %w", ctx.Err())
	}
}

// captureDetailResponse observes the same public JSON response used by the
// official page. Chrome owns the evolving JS challenge and cookies; this code
// only accepts the exact detail endpoint for the expected numeric work ID.
func captureDetailResponse(ctx context.Context, executable, itemID string) ([]byte, error) {
	allocatorOptions := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.ExecPath(executable),
		chromedp.Flag("autoplay-policy", "user-gesture-required"),
		chromedp.Flag("mute-audio", true),
	)
	allocatorCtx, cancelAllocator := chromedp.NewExecAllocator(ctx, allocatorOptions...)
	defer cancelAllocator()
	browserCtx, cancelBrowser := chromedp.NewContext(allocatorCtx)
	defer cancelBrowser()

	result := make(chan browserResponse, 1)
	watchDetailResponse(browserCtx, itemID, result)
	pageURL := "https://www.douyin.com/video/" + itemID
	if err := chromedp.Run(browserCtx, network.Enable(), chromedp.Navigate(pageURL)); err != nil {
		return nil, fmt.Errorf("启动浏览器作品页: %w", err)
	}
	select {
	case response := <-result:
		return response.body, response.err
	case <-ctx.Done():
		return nil, fmt.Errorf("等待浏览器作品详情: %w", ctx.Err())
	}
}

// watchDetailResponse waits for loadingFinished before reading the body. CDP
// reports response headers earlier, but reading at that point intermittently
// fails with "No data found for resource" on otherwise successful requests.
func watchDetailResponse(ctx context.Context, itemID string, result chan<- browserResponse) {
	requests := make(map[network.RequestID]int64)
	chromedp.ListenTarget(ctx, func(event any) {
		switch event := event.(type) {
		case *network.EventResponseReceived:
			if isExpectedDetailResponse(event.Response.URL, itemID) {
				requests[event.RequestID] = int64(event.Response.Status)
			}
		case *network.EventLoadingFinished:
			if status, found := requests[event.RequestID]; found {
				if status == 200 {
					go readBrowserResponse(ctx, event.RequestID, result)
				} else {
					publishBrowserResult(result, nil, fmt.Errorf("浏览器作品详情返回 HTTP %d", status))
				}
				delete(requests, event.RequestID)
			}
		case *network.EventLoadingFailed:
			if _, found := requests[event.RequestID]; found {
				publishBrowserResult(result, nil, fmt.Errorf("浏览器作品详情加载失败: %s", event.ErrorText))
				delete(requests, event.RequestID)
			}
		}
	})
}

// readBrowserResponse runs outside the event callback because executing a CDP
// command synchronously from that callback would deadlock the target listener.
func readBrowserResponse(ctx context.Context, requestID network.RequestID, result chan<- browserResponse) {
	target := chromedp.FromContext(ctx).Target
	body, err := network.GetResponseBody(requestID).Do(cdp.WithExecutor(ctx, target))
	if err != nil {
		err = fmt.Errorf("读取浏览器作品详情: %w", err)
	}
	publishBrowserResult(result, body, err)
}

// publishBrowserResult makes duplicate detail requests harmless: the first
// terminal result wins and later page retries cannot block the CDP listener.
func publishBrowserResult(result chan<- browserResponse, body []byte, err error) {
	select {
	case result <- browserResponse{body: body, err: err}:
	default:
	}
}

// isExpectedDetailResponse prevents an unrelated page request from crossing
// the parser boundary, even if Douyin adds similarly named API calls later.
func isExpectedDetailResponse(rawURL, itemID string) bool {
	parsed, err := url.Parse(rawURL)
	return err == nil && parsed.Scheme == "https" && parsed.Hostname() == "www.douyin.com" &&
		parsed.Path == detailAPIPath && parsed.Query().Get("aweme_id") == itemID
}

// parseBrowserDetail maps the large response into one stable work model. Image
// posts are detected before video selection because their video field can hold
// music metadata that must never be mistaken for the primary media.
func parseBrowserDetail(body []byte, expectedID string) (WorkInfo, error) {
	var response rawDetailResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return WorkInfo{}, fmt.Errorf("解码浏览器作品详情: %w", err)
	}
	item := response.AwemeDetail
	if response.StatusCode != 0 || item.ID == "" || item.ID != expectedID {
		return WorkInfo{}, fmt.Errorf("浏览器作品详情无效: status=%d message=%q", response.StatusCode, response.StatusMsg)
	}
	if len(item.Images) > 0 {
		return imageWorkFromDetail(item)
	}
	mediaURL, err := selectBestH264Media(item.Video)
	if err != nil {
		return WorkInfo{}, err
	}
	work := detailWork(item, WorkKindVideo, []MediaAsset{{
		URL: mediaURL, Kind: MediaKindVideo, Extension: ".mp4",
	}})
	return work, work.Validate()
}

// imageWorkFromDetail preserves Douyin's image order and uses url_list rather
// than download_url_list. The latter currently points at a watermarked image
// transform, while url_list exposes the public unwatermarked display image.
func imageWorkFromDetail(item rawDetailItem) (WorkInfo, error) {
	assets := make([]MediaAsset, 0, len(item.Images))
	for index, image := range item.Images {
		imageURL, extension := selectPreferredImageURL(image.URLs)
		if imageURL == "" {
			return WorkInfo{}, fmt.Errorf("图集第 %d 张图片没有受信任的下载地址", index+1)
		}
		assets = append(assets, MediaAsset{
			URL: imageURL, Kind: MediaKindImage, Extension: extension,
			Width: image.Width, Height: image.Height,
		})
	}
	work := detailWork(item, WorkKindImages, assets)
	return work, work.Validate()
}

// selectPreferredImageURL prefers JPEG for broad local viewer compatibility.
// Douyin often lists WEBP first and JPEG later at the same public dimensions;
// all candidates still cross the shared trusted-CDN boundary.
func selectPreferredImageURL(candidates []string) (string, string) {
	var fallbackURL, fallbackExtension string
	for _, candidate := range candidates {
		parsed, err := url.Parse(candidate)
		if err != nil || !isTrustedHTTPSURL(parsed, mediaHostSuffixes) {
			continue
		}
		extension := normalizedImageExtension(filepath.Ext(parsed.Path))
		if extension == ".jpg" {
			return parsed.String(), extension
		}
		if fallbackURL == "" && extension != "" {
			fallbackURL, fallbackExtension = parsed.String(), extension
		}
	}
	return fallbackURL, fallbackExtension
}

// normalizedImageExtension restricts output names to formats accepted by the
// parser and folds JPEG's two common suffixes into one stable extension.
func normalizedImageExtension(extension string) string {
	switch strings.ToLower(extension) {
	case ".jpg", ".jpeg":
		return ".jpg"
	case ".webp", ".png":
		return strings.ToLower(extension)
	default:
		return ""
	}
}

// detailWork centralizes metadata trimming for both detail-response branches.
func detailWork(item rawDetailItem, kind WorkKind, assets []MediaAsset) WorkInfo {
	return WorkInfo{
		ID: item.ID, Author: strings.TrimSpace(item.Author.Nickname),
		Title: strings.TrimSpace(item.Title), Kind: kind, Assets: assets,
	}
}

// selectBestH264Media ranks direct H.264 transcodes by pixel count, bitrate,
// then byte size. H.265 is skipped for broad browser/player compatibility, and
// the generic play address remains a fallback when bit_rate is absent.
func selectBestH264Media(video rawDetailVideo) (string, error) {
	candidates := make([]mediaCandidate, 0, len(video.BitRates)+2)
	for _, bitRate := range video.BitRates {
		if isH265BitRate(bitRate) {
			continue
		}
		candidates = append(candidates, mediaCandidate{address: bitRate.PlayAddress, bitRate: bitRate.BitRate})
	}
	candidates = append(candidates,
		mediaCandidate{address: video.H264PlayAddress},
		mediaCandidate{address: video.PlayAddress},
	)

	bestURL := ""
	bestScore := [3]int64{-1, -1, -1}
	for _, candidate := range candidates {
		candidateURL := firstTrustedMediaURL(candidate.address.URLs)
		score := candidateScore(candidate)
		if candidateURL != "" && greaterScore(score, bestScore) {
			bestURL, bestScore = candidateURL, score
		}
	}
	if bestURL == "" {
		return "", errors.New("浏览器作品详情没有受信任的 H.264 播放地址")
	}
	return bestURL, nil
}

// isH265BitRate recognizes both the explicit flag and common upstream naming.
// Those streams may be smaller, but H.264 is more portable for saved MP4 files.
func isH265BitRate(bitRate rawVideoBitRate) bool {
	gearName := strings.ToLower(bitRate.GearName)
	return bitRate.IsH265 != 0 || strings.Contains(gearName, "h265") ||
		strings.Contains(gearName, "hevc") || strings.Contains(gearName, "bytevc1")
}

// candidateScore keeps unknown dimensions at zero. Falling back to the video's
// canvas size would let an unlabelled low-resolution play_addr outrank a known
// high-resolution bit_rate entry.
func candidateScore(candidate mediaCandidate) [3]int64 {
	return [3]int64{
		int64(candidate.address.Width) * int64(candidate.address.Height),
		int64(candidate.bitRate), candidate.address.DataSize,
	}
}

// greaterScore compares the ordered resolution, bitrate, and byte-size tuple.
func greaterScore(left, right [3]int64) bool {
	for index := range left {
		if left[index] != right[index] {
			return left[index] > right[index]
		}
	}
	return false
}

// firstTrustedMediaURL chooses the first CDN alternative that satisfies the
// same outbound host policy enforced again by the downloader.
func firstTrustedMediaURL(candidates []string) string {
	for _, candidate := range candidates {
		parsed, err := url.Parse(candidate)
		if err == nil && isTrustedHTTPSURL(parsed, mediaHostSuffixes) {
			return parsed.String()
		}
	}
	return ""
}

// findBrowserExecutable keeps platform details out of the resolver flow and
// returns only executable files. PATH candidates cover Linux installations;
// explicit app paths cover the common macOS layout.
func findBrowserExecutable() string {
	for _, candidate := range browserCandidates() {
		if strings.ContainsRune(candidate, os.PathSeparator) {
			if info, err := os.Stat(candidate); err == nil && !info.IsDir() && info.Mode()&0111 != 0 {
				return candidate
			}
			continue
		}
		if path, err := exec.LookPath(candidate); err == nil {
			return path
		}
	}
	return ""
}

// browserCandidates lists platform-specific browser locations in preference
// order; findBrowserExecutable performs the actual executable validation.
func browserCandidates() []string {
	switch runtime.GOOS {
	case "darwin":
		candidates := []string{
			"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
			"/Applications/Microsoft Edge.app/Contents/MacOS/Microsoft Edge",
			"/Applications/Chromium.app/Contents/MacOS/Chromium",
		}
		if home, err := os.UserHomeDir(); err == nil {
			candidates = append(candidates,
				filepath.Join(home, "Applications/Google Chrome.app/Contents/MacOS/Google Chrome"),
				filepath.Join(home, "Applications/Microsoft Edge.app/Contents/MacOS/Microsoft Edge"),
			)
		}
		return candidates
	case "windows":
		return windowsBrowserCandidates()
	default:
		return []string{"google-chrome", "chromium", "chromium-browser", "microsoft-edge"}
	}
}

// windowsBrowserCandidates covers standard per-user and system installations
// before falling back to PATH. Empty environment entries are harmless and are
// rejected by the executable checks in findBrowserExecutable.
func windowsBrowserCandidates() []string {
	localAppData := os.Getenv("LOCALAPPDATA")
	programFiles := os.Getenv("PROGRAMFILES")
	programFilesX86 := os.Getenv("PROGRAMFILES(X86)")
	return []string{
		filepath.Join(localAppData, "Google/Chrome/Application/chrome.exe"),
		filepath.Join(localAppData, "Microsoft/Edge/Application/msedge.exe"),
		filepath.Join(programFiles, "Google/Chrome/Application/chrome.exe"),
		filepath.Join(programFiles, "Microsoft/Edge/Application/msedge.exe"),
		filepath.Join(programFilesX86, "Google/Chrome/Application/chrome.exe"),
		filepath.Join(programFilesX86, "Microsoft/Edge/Application/msedge.exe"),
		"chrome.exe", "msedge.exe", "chromium.exe",
	}
}
