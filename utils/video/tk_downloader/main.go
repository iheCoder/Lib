package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
)

const defaultOutputDirectory = "."

// main only owns process concerns: signal handling, exit status, and standard
// streams. The actual workflow lives in runCLI so argument failures and network
// failures remain testable without terminating a test process.
func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	os.Exit(runCLI(ctx, os.Args[1:], os.Stdout, os.Stderr))
}

// runCLI keeps the user-facing path linear: parse input, resolve one work, then
// download it. Each stage wraps its own errors, so the CLI can stay small while
// still reporting where the workflow stopped.
func runCLI(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("tk_downloader", flag.ContinueOnError)
	flags.SetOutput(stderr)
	outputDirectory := flags.String("o", defaultOutputDirectory, "视频保存目录")
	if err := flags.Parse(args); err != nil {
		return 2
	}

	shareText := strings.TrimSpace(strings.Join(flags.Args(), " "))
	if shareText == "" {
		fmt.Fprintln(stderr, "用法: tk_downloader [-o 保存目录] <抖音分享文本或链接>")
		return 2
	}

	shareURL, err := extractShareURL(shareText)
	if err != nil {
		fmt.Fprintf(stderr, "解析分享文本失败: %v\n", err)
		return 1
	}

	video, err := newResolver().Resolve(ctx, shareURL)
	if err != nil {
		fmt.Fprintf(stderr, "解析抖音作品失败: %v\n", err)
		return 1
	}

	path, err := newDownloader().Download(ctx, video, *outputDirectory)
	if err != nil {
		fmt.Fprintf(stderr, "下载视频失败: %v\n", err)
		return 1
	}

	fmt.Fprintf(stdout, "下载完成\n作者: %s\n作品: %s\n文件: %s\n", video.Author, video.Title, path)
	return 0
}
