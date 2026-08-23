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

// DownloadResult gives callers one user-facing location while retaining every
// concrete file for verification and future progress reporting.
type DownloadResult struct {
	Location string
	Files    []string
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

// Download routes the resolved aggregate without leaking work-type branches
// into the CLI. Videos publish one file; image posts publish an isolated folder
// and remove that folder if any image fails.
func (d *downloader) Download(ctx context.Context, work WorkInfo, outputDirectory string) (DownloadResult, error) {
	if err := work.Validate(); err != nil {
		return DownloadResult{}, err
	}
	if err := os.MkdirAll(outputDirectory, 0o755); err != nil {
		return DownloadResult{}, fmt.Errorf("创建保存目录: %w", err)
	}
	if work.IsImagePost() {
		return d.downloadImages(ctx, work, outputDirectory)
	}
	return d.downloadVideo(ctx, work, outputDirectory)
}

// downloadVideo preserves the original collision-safe single-file behavior.
func (d *downloader) downloadVideo(ctx context.Context, work WorkInfo, outputDirectory string) (DownloadResult, error) {
	response, err := d.OpenAsset(ctx, work.Assets[0])
	if err != nil {
		return DownloadResult{}, err
	}
	defer response.Body.Close()

	filename := buildDownloadFilename(work)
	temporaryPath, err := writeTemporaryAsset(response.Body, outputDirectory, filename)
	if err != nil {
		return DownloadResult{}, err
	}
	finalPath, err := publishWithoutOverwrite(temporaryPath, outputDirectory, filename)
	if err != nil {
		_ = os.Remove(temporaryPath)
		return DownloadResult{}, err
	}
	return DownloadResult{Location: finalPath, Files: []string{finalPath}}, nil
}

// downloadImages creates a unique folder before writing numbered files. The
// folder is owned exclusively by this call, making precise rollback safe.
func (d *downloader) downloadImages(ctx context.Context, work WorkInfo, outputDirectory string) (result DownloadResult, resultErr error) {
	directory, err := createUniqueDirectory(outputDirectory, buildWorkBaseName(work)+"_images")
	if err != nil {
		return DownloadResult{}, err
	}
	defer func() {
		if resultErr != nil {
			_ = os.RemoveAll(directory)
		}
	}()

	files := make([]string, 0, len(work.Assets))
	for index, asset := range work.Assets {
		path, err := d.downloadImage(ctx, asset, directory, buildAssetFilename(index, asset))
		if err != nil {
			return DownloadResult{}, fmt.Errorf("下载第 %d 张图片: %w", index+1, err)
		}
		files = append(files, path)
	}
	return DownloadResult{Location: directory, Files: files}, nil
}

// downloadImage reuses the same temporary-write and atomic-publish path as
// videos, so interrupted image responses never appear as completed files.
func (d *downloader) downloadImage(ctx context.Context, asset MediaAsset, directory, filename string) (string, error) {
	response, err := d.OpenAsset(ctx, asset)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	temporaryPath, err := writeTemporaryAsset(response.Body, directory, filename)
	if err != nil {
		return "", err
	}
	path, err := publishWithoutOverwrite(temporaryPath, directory, filename)
	if err != nil {
		_ = os.Remove(temporaryPath)
	}
	return path, err
}

// OpenAsset validates and opens one response without buffering its body. Both
// the CLI file writer and web delivery use this boundary, ensuring
// they apply identical trust, redirect, status, and content-type checks.
func (d *downloader) OpenAsset(ctx context.Context, asset MediaAsset) (*http.Response, error) {
	mediaURL, err := url.Parse(asset.URL)
	if err != nil || !d.allowMediaURL(mediaURL) {
		return nil, errors.New("媒体地址无效或不受信任")
	}
	response, err := d.requestMedia(ctx, mediaURL, asset.Kind)
	if err != nil {
		return nil, err
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		response.Body.Close()
		return nil, fmt.Errorf("媒体服务器返回 HTTP %d", response.StatusCode)
	}
	if !isPlausibleMediaType(response.Header.Get("Content-Type"), asset.Kind) {
		response.Body.Close()
		return nil, fmt.Errorf("媒体服务器返回了不匹配的内容 %q", response.Header.Get("Content-Type"))
	}
	return response, nil
}

// requestMedia adds the same browser identity used for page resolution. Some
// media gateways inspect both User-Agent and Referer before redirecting to the
// actual CDN object, so these headers are part of the download contract.
func (d *downloader) requestMedia(ctx context.Context, mediaURL *url.URL, kind MediaKind) (*http.Response, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, mediaURL.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("创建媒体请求: %w", err)
	}
	request.Header.Set("User-Agent", mobileUserAgent)
	request.Header.Set("Referer", mediaReferer)
	request.Header.Set("Accept", mediaAcceptHeader(kind))
	response, err := d.client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("请求媒体文件: %w", err)
	}
	return response, nil
}

// isPlausibleMediaType rejects the most common anti-bot and API error bodies.
// An absent type remains acceptable because several CDNs omit it even when the
// response body is a valid MP4 stream.
func isPlausibleMediaType(contentType string, kind MediaKind) bool {
	contentType = strings.ToLower(strings.TrimSpace(strings.Split(contentType, ";")[0]))
	if contentType == "" || contentType == "application/octet-stream" {
		return true
	}
	return kind == MediaKindVideo && strings.HasPrefix(contentType, "video/") ||
		kind == MediaKindImage && strings.HasPrefix(contentType, "image/")
}

// mediaAcceptHeader advertises formats appropriate to the validated asset kind.
func mediaAcceptHeader(kind MediaKind) string {
	if kind == MediaKindImage {
		return "image/avif,image/webp,image/jpeg,image/png,image/*;q=0.9,*/*;q=0.1"
	}
	return "video/mp4,video/*;q=0.9,*/*;q=0.1"
}

// writeTemporaryAsset performs the only large-data operation in the program.
// A fixed copy buffer keeps memory usage stable, and every failure path closes
// and removes the partial file before returning to the caller.
func writeTemporaryAsset(reader io.Reader, outputDirectory, filename string) (temporaryPath string, resultErr error) {
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
		return "", fmt.Errorf("写入媒体文件: %w", err)
	}
	if err := file.Chmod(0o644); err != nil {
		return "", fmt.Errorf("设置媒体文件权限: %w", err)
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
			return "", fmt.Errorf("发布媒体文件: %w", err)
		}
	}
	return "", fmt.Errorf("同名文件超过 %d 个", maximumNameAttempts)
}

// buildDownloadFilename maps a work to its browser or CLI artifact name.
func buildDownloadFilename(work WorkInfo) string {
	extension := ".mp4"
	if work.IsImagePost() {
		extension = ".zip"
	}
	return buildWorkBaseName(work) + extension
}

// buildWorkBaseName preserves useful Chinese metadata while removing path
// syntax, control characters, and excessive length. The immutable work ID
// remains at the end so similarly titled works stay distinguishable.
func buildWorkBaseName(work WorkInfo) string {
	metadataParts := make([]string, 0, 2)
	for _, value := range []string{work.Author, work.Title} {
		if cleaned := sanitizeFilenamePart(value); cleaned != "" {
			metadataParts = append(metadataParts, cleaned)
		}
	}
	identifier := sanitizeFilenamePart(work.ID)
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
		base = "douyin-work"
	}
	return base
}

// buildAssetFilename uses fixed-width numbering for image order and trusts only
// parser-normalized extensions, keeping ZIP and filesystem entries portable.
func buildAssetFilename(index int, asset MediaAsset) string {
	extension := normalizedImageExtension(asset.Extension)
	if extension == "" {
		extension = ".jpg"
	}
	return fmt.Sprintf("%02d%s", index+1, extension)
}

// buildIndividualImageFilename gives a standalone browser download enough work
// context to remain recognizable outside its original numbered directory.
func buildIndividualImageFilename(work WorkInfo, index int, asset MediaAsset) string {
	return buildWorkBaseName(work) + "_" + buildAssetFilename(index, asset)
}

// createUniqueDirectory uses mkdir itself as the collision check, avoiding a
// check-then-create race when two CLI downloads start at the same time.
func createUniqueDirectory(parent, name string) (string, error) {
	for index := 0; index < maximumNameAttempts; index++ {
		candidateName := name
		if index > 0 {
			candidateName = fmt.Sprintf("%s (%d)", name, index)
		}
		candidate := filepath.Join(parent, candidateName)
		if err := os.Mkdir(candidate, 0o755); err == nil {
			return candidate, nil
		} else if !errors.Is(err, os.ErrExist) {
			return "", fmt.Errorf("创建图集目录: %w", err)
		}
	}
	return "", fmt.Errorf("同名目录超过 %d 个", maximumNameAttempts)
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
