package filesystem

import (
	"os"
	"os/exec"
	"testing"
)

// 生成一个简单的测试视频文件（2秒蓝色背景，1280x720）
func generateTestVideo(filePath string) error {
	cmd := exec.Command("ffmpeg",
		"-f", "lavfi",
		"-i", "color=c=blue:s=1280x720:d=2",
		"-pix_fmt", "yuv420p",
		filePath,
		"-y", // 覆盖输出文件
	)
	return cmd.Run()
}

func TestGetVideoMetadata(t *testing.T) {
	videoPath := "./test_video.mp4"

	// 生成测试视频
	err := generateTestVideo(videoPath)
	if err != nil {
		t.Fatalf("failed to generate test video: %v (is ffmpeg installed?)", err)
	}
	defer os.Remove(videoPath) // 测试结束后删除文件

	// 调用你的函数
	duration, width, height, err := getVideoMetadata(videoPath)
	if err != nil {
		t.Fatalf("getVideoMetadata failed: %v", err)
	}

	t.Logf("Video Duration: %s, Width: %d, Height: %d", duration, width, height)

	// 做简单断言
	if width != 1280 || height != 720 {
		t.Errorf("unexpected video resolution: width=%d, height=%d", width, height)
	}

	if duration == "" {
		t.Errorf("video duration is empty")
	}
}
