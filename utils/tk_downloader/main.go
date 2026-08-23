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
	outputDirectory := flags.String("o", defaultOutputDirectory, "作品保存目录")
	webMode := flags.Bool("web", false, "启动网页端")
	webAddress := flags.String("addr", defaultWebAddress, "网页端监听地址")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if *webMode {
		if err := runWebServer(ctx, *webAddress, stdout); err != nil {
			fmt.Fprintf(stderr, "网页服务运行失败: %v\n", err)
			return 1
		}
		return 0
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

	work, err := newResolver().Resolve(ctx, shareURL)
	if err != nil {
		fmt.Fprintf(stderr, "解析抖音作品失败: %v\n", err)
		return 1
	}

	result, err := newDownloader().Download(ctx, work, *outputDirectory)
	if err != nil {
		fmt.Fprintf(stderr, "下载作品失败: %v\n", err)
		return 1
	}

	writeDownloadSummary(stdout, work, result)
	return 0
}

// writeDownloadSummary makes the aggregate visible without leaking download
// branching into the main workflow.
func writeDownloadSummary(output io.Writer, work WorkInfo, result DownloadResult) {
	if work.IsImagePost() {
		fmt.Fprintf(output, "下载完成\n作者: %s\n作品: %s\n类型: 图集（%d 张）\n目录: %s\n", work.Author, work.Title, len(work.Assets), result.Location)
		return
	}
	fmt.Fprintf(output, "下载完成\n作者: %s\n作品: %s\n类型: 视频\n文件: %s\n", work.Author, work.Title, result.Location)
}
