package main

import (
	"fmt"
	"net/http"
	"net/url"
	"time"
)

const (
	maximumRedirects      = 10
	responseHeaderTimeout = 20 * time.Second
)

type urlPolicy func(*url.URL) bool

// newRestrictedHTTPClient gives page resolution and media download the same
// redirect safety rule. The transport bounds the time spent waiting for headers;
// body transfer remains governed by the caller's context so large videos can finish.
func newRestrictedHTTPClient(timeout time.Duration, allow urlPolicy) *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.ResponseHeaderTimeout = responseHeaderTimeout
	return &http.Client{
		Transport:     transport,
		Timeout:       timeout,
		CheckRedirect: restrictedRedirectPolicy(allow),
	}
}

// restrictedRedirectPolicy prevents a trusted short link from redirecting the
// downloader into an unrelated or local service. The explicit hop cap also
// turns redirect loops into a deterministic error.
func restrictedRedirectPolicy(allow urlPolicy) func(*http.Request, []*http.Request) error {
	return func(request *http.Request, via []*http.Request) error {
		if len(via) >= maximumRedirects {
			return fmt.Errorf("重定向次数超过 %d 次", maximumRedirects)
		}
		if !allow(request.URL) {
			return fmt.Errorf("拒绝跳转到非受信任地址 %q", request.URL.Host)
		}
		return nil
	}
}
