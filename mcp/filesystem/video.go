package main

import (
	"encoding/json"
	"fmt"
	"os/exec"
)

type ffprobeOutput struct {
	Streams []struct {
		Width    int    `json:"width"`
		Height   int    `json:"height"`
		Duration string `json:"duration"`
	} `json:"streams"`
}

func getVideoMetadata(filePath string) (duration string, width int, height int, err error) {
	cmd := exec.Command("ffprobe",
		"-v", "error",
		"-select_streams", "v:0",
		"-show_entries", "stream=width,height,duration",
		"-of", "json",
		filePath,
	)
	output, err := cmd.Output()
	if err != nil {
		return "", 0, 0, err
	}

	var result ffprobeOutput
	if err := json.Unmarshal(output, &result); err != nil {
		return "", 0, 0, err
	}

	if len(result.Streams) == 0 {
		return "", 0, 0, fmt.Errorf("no video stream found")
	}

	stream := result.Streams[0]
	return stream.Duration, stream.Width, stream.Height, nil
}
