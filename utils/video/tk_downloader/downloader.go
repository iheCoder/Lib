package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"unicode"
)

const (
	maximumFilenameRunes = 100
	downloadBufferSize   = 256 << 10
	maximumNameAttempts  = 1000
	mediaReferer         = "https://www.iesdouyin.com/"
)

type downloader struct {
	client        *http.Client
	allowMediaURL urlPolicy
}

// newDownloader has no whole-request timeout because video size and network
// speed vary widely. Header waiting is still bounded by the shared transport,
// and Ctrl+C cancels body transfer through the request context.
func newDownloader() *downloader {
	allow := func(candidate *url.URL) bool {
		return isTrustedHTTPSURL(candidate, mediaHostSuffixes)
	}
	return &downloader{
		client:        newRestrictedHTTPClient(0, allow),
		allowMediaURL: allow,
	}
}

// Download streams one media response into a temporary file and publishes it
// only after a complete copy. The final path is never overwritten; repeated
// downloads receive a numeric suffix instead of destroying an existing file.
func (d *downloader) Download(ctx context.Context, video VideoInfo, outputDirectory string) (string, error) {
	mediaURL, err := url.Parse(video.MediaURL)
	if err != nil || !d.allowMediaURL(mediaURL) {
		return "", errors.New("播放地址无效或不受信任")
	}
	if err := os.MkdirAll(outputDirectory, 0o755); err != nil {
		return "", fmt.Errorf("创建保存目录: %w", err)
	}

	response, err := d.requestMedia(ctx, mediaURL)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return "", fmt.Errorf("媒体服务器返回 HTTP %d", response.StatusCode)
	}
	if !isPlausibleMediaType(response.Header.Get("Content-Type")) {
		return "", fmt.Errorf("媒体服务器返回了非视频内容 %q", response.Header.Get("Content-Type"))
	}

	temporaryPath, err := writeTemporaryVideo(response.Body, outputDirectory, buildFilename(video))
	if err != nil {
		return "", err
	}
	finalPath, err := publishWithoutOverwrite(temporaryPath, outputDirectory, buildFilename(video))
	if err != nil {
		_ = os.Remove(temporaryPath)
		return "", err
	}
	return finalPath, nil
}

// requestMedia adds the same browser identity used for page resolution. Some
// media gateways inspect both User-Agent and Referer before redirecting to the
// actual CDN object, so these headers are part of the download contract.
func (d *downloader) requestMedia(ctx context.Context, mediaURL *url.URL) (*http.Response, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, mediaURL.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("创建媒体请求: %w", err)
	}
	request.Header.Set("User-Agent", mobileUserAgent)
	request.Header.Set("Referer", mediaReferer)
	request.Header.Set("Accept", "video/mp4,video/*;q=0.9,*/*;q=0.1")
	response, err := d.client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("请求媒体文件: %w", err)
	}
	return response, nil
}

// isPlausibleMediaType rejects the most common anti-bot and API error bodies.
// An absent type remains acceptable because several CDNs omit it even when the
// response body is a valid MP4 stream.
func isPlausibleMediaType(contentType string) bool {
	contentType = strings.ToLower(strings.TrimSpace(strings.Split(contentType, ";")[0]))
	return contentType == "" || strings.HasPrefix(contentType, "video/") || contentType == "application/octet-stream"
}

// writeTemporaryVideo performs the only large-data operation in the program.
// A fixed copy buffer keeps memory usage stable, and every failure path closes
// and removes the partial file before returning to the caller.
func writeTemporaryVideo(reader io.Reader, outputDirectory, filename string) (temporaryPath string, resultErr error) {
	file, err := os.CreateTemp(outputDirectory, "."+filename+"-*.part")
	if err != nil {
		return "", fmt.Errorf("创建临时文件: %w", err)
	}
	temporaryPath = file.Name()
	defer func() {
		if closeErr := file.Close(); resultErr == nil && closeErr != nil {
			resultErr = fmt.Errorf("关闭临时文件: %w", closeErr)
		}
		if resultErr != nil {
			_ = os.Remove(temporaryPath)
		}
	}()
	if _, err := io.CopyBuffer(file, reader, make([]byte, downloadBufferSize)); err != nil {
		return "", fmt.Errorf("写入视频: %w", err)
	}
	if err := file.Chmod(0o644); err != nil {
		return "", fmt.Errorf("设置视频文件权限: %w", err)
	}
	return temporaryPath, nil
}

// publishWithoutOverwrite uses a same-directory hard link as an atomic publish:
// it fails rather than replacing an existing target. Numeric suffixes resolve
// ordinary name collisions without a check-then-rename race.
func publishWithoutOverwrite(temporaryPath, outputDirectory, filename string) (string, error) {
	extension := filepath.Ext(filename)
	stem := strings.TrimSuffix(filename, extension)
	for index := 0; index < maximumNameAttempts; index++ {
		candidateName := filename
		if index > 0 {
			candidateName = fmt.Sprintf("%s (%d)%s", stem, index, extension)
		}
		candidatePath := filepath.Join(outputDirectory, candidateName)
		if err := os.Link(temporaryPath, candidatePath); err == nil {
			if removeErr := os.Remove(temporaryPath); removeErr != nil {
				return "", fmt.Errorf("清理临时文件: %w", removeErr)
			}
			return candidatePath, nil
		} else if !errors.Is(err, os.ErrExist) {
			return "", fmt.Errorf("发布视频文件: %w", err)
		}
	}
	return "", fmt.Errorf("同名文件超过 %d 个", maximumNameAttempts)
}

// buildFilename preserves useful Chinese metadata while removing path syntax,
// control characters, and excessive length. The immutable work ID remains at
// the end so similarly titled videos are still distinguishable.
func buildFilename(video VideoInfo) string {
	metadataParts := make([]string, 0, 2)
	for _, value := range []string{video.Author, video.Title} {
		if cleaned := sanitizeFilenamePart(value); cleaned != "" {
			metadataParts = append(metadataParts, cleaned)
		}
	}
	identifier := sanitizeFilenamePart(video.ID)
	identifierSuffix := ""
	if identifier != "" {
		identifierSuffix = "_" + identifier
	}
	metadata := strings.Join(metadataParts, "_")
	metadataLimit := maximumFilenameRunes - len([]rune(identifierSuffix))
	if metadataLimit > 0 {
		metadata = truncateRunes(metadata, metadataLimit)
	} else {
		metadata = ""
	}
	base := metadata + identifierSuffix
	base = strings.TrimPrefix(base, "_")
	if base == "" {
		base = "douyin-video"
	}
	return base + ".mp4"
}

// sanitizeFilenamePart maps filesystem separators and control characters to
// spaces, then collapses whitespace. It deliberately retains Unicode titles so
// the downloaded file remains recognizable to Chinese users.
func sanitizeFilenamePart(value string) string {
	cleaned := strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || strings.ContainsRune(`/\:*?"<>|`, r) {
			return ' '
		}
		return r
	}, value)
	return strings.Join(strings.Fields(cleaned), " ")
}

// truncateRunes limits by Unicode code points rather than bytes, avoiding a
// filename that ends in the middle of a UTF-8 encoded Chinese character.
func truncateRunes(value string, maximum int) string {
	runes := []rune(value)
	if len(runes) <= maximum {
		return value
	}
	return string(runes[:maximum])
}
