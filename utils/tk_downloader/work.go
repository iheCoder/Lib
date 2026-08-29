package main

import (
	"errors"
	"fmt"
	"strings"
)

// WorkKind describes the user-visible shape of one Douyin work. Keeping this
// distinction in the domain model prevents image posts from being forced
// through video-only fields later in the download and web layers.
type WorkKind string

const (
	WorkKindVideo  WorkKind = "video"
	WorkKindImages WorkKind = "images"
)

// MediaKind controls request validation and output naming for one remote asset.
// It is intentionally separate from WorkKind because a work owns a collection
// while each network request must validate exactly one response type.
type MediaKind string

const (
	MediaKindVideo MediaKind = "video"
	MediaKindImage MediaKind = "image"
)

type MediaAsset struct {
	URL       string
	Kind      MediaKind
	Extension string
	Width     int
	Height    int
}

// WorkInfo is the stable boundary between Douyin parsing and delivery. It
// contains only metadata needed by the CLI and web application, insulating
// both consumers from Douyin's much larger, frequently changing payload.
type WorkInfo struct {
	ID     string
	Author string
	Title  string
	Kind   WorkKind
	Assets []MediaAsset
}

// Validate makes every downstream assumption explicit at the parsing boundary.
// A video has one video asset; an image post has one or more image assets.
func (work WorkInfo) Validate() error {
	if strings.TrimSpace(work.ID) == "" {
		return errors.New("作品数据缺少 ID")
	}
	if len(work.Assets) == 0 {
		return errors.New("作品数据没有可下载媒体")
	}
	if work.Kind == WorkKindVideo && len(work.Assets) != 1 {
		return errors.New("视频作品必须包含一个媒体文件")
	}
	expectedKind, err := work.expectedMediaKind()
	if err != nil {
		return err
	}
	for index, asset := range work.Assets {
		if asset.Kind != expectedKind || strings.TrimSpace(asset.URL) == "" {
			return fmt.Errorf("作品的第 %d 个媒体文件无效", index+1)
		}
	}
	return nil
}

// expectedMediaKind is the only mapping between aggregate and asset kinds, so
// new work types cannot silently acquire inconsistent download semantics.
func (work WorkInfo) expectedMediaKind() (MediaKind, error) {
	switch work.Kind {
	case WorkKindVideo:
		return MediaKindVideo, nil
	case WorkKindImages:
		return MediaKindImage, nil
	default:
		return "", fmt.Errorf("不支持的作品类型 %q", work.Kind)
	}
}

// IsImagePost keeps type checks readable at delivery branch points.
func (work WorkInfo) IsImagePost() bool {
	return work.Kind == WorkKindImages
}
