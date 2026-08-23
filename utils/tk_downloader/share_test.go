package main

import "testing"

// TestExtractShareURL covers the real copied-text shape, trailing Chinese
// punctuation, and the trust boundary that prevents arbitrary URL downloads.
func TestExtractShareURL(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{
			name:  "complete copied text",
			input: "2.07 复制打开抖音，看看【周期的作品】 https://v.douyin.com/I99hYmaF4jc/ 07/03",
			want:  "https://v.douyin.com/I99hYmaF4jc/",
		},
		{
			name:  "trailing Chinese punctuation",
			input: "打开 https://www.douyin.com/video/7664176772422146725。",
			want:  "https://www.douyin.com/video/7664176772422146725",
		},
		{name: "reject lookalike host", input: "https://douyin.com.evil.example/video/1", wantErr: true},
		{name: "reject unrelated URL", input: "https://example.com/video.mp4", wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := extractShareURL(test.input)
			if test.wantErr {
				if err == nil {
					t.Fatalf("extractShareURL() = %q, want error", got)
				}
				return
			}
			if err != nil || got.String() != test.want {
				t.Fatalf("extractShareURL() = %v, %v; want %q", got, err, test.want)
			}
		})
	}
}
