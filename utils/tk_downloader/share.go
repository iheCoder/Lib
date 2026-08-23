package main

import (
	"errors"
	"net/url"
	"regexp"
	"strings"
)

var shareURLPattern = regexp.MustCompile(`https?://[^\s"'<>]+`)

var shareHostSuffixes = []string{
	"douyin.com",
	"iesdouyin.com",
}

var mediaHostSuffixes = []string{
	"snssdk.com",
	"douyinvod.com",
	"idouyinvod.com",
	"douyincdn.com",
	"bytefcdn.com",
	"volccdn.com",
	"smtcdns.com",
	"ourdvsss.com",
	"jomodns.com",
	"pstatp.com",
	"ixigua.com",
	"amemv.com",
}

const trailingSharePunctuation = `.,;:!?)]}，。；：！？）】》`

// extractShareURL accepts the text users actually copy from Douyin rather than
// requiring them to isolate the short link manually. Only an HTTPS URL under a
// Douyin-owned share domain is returned; unrelated URLs in the prose are ignored.
func extractShareURL(input string) (*url.URL, error) {
	for _, candidate := range shareURLPattern.FindAllString(input, -1) {
		candidate = strings.TrimRight(candidate, trailingSharePunctuation)
		parsed, err := url.Parse(candidate)
		if err == nil && isTrustedHTTPSURL(parsed, shareHostSuffixes) {
			return parsed, nil
		}
	}
	return nil, errors.New("没有找到受支持的抖音 HTTPS 分享链接")
}

// isTrustedHTTPSURL is the single outbound-network boundary. Exact hosts and
// their subdomains are accepted, while suffix lookalikes such as douyin.com.evil
// are rejected by requiring a dot before every suffix match.
func isTrustedHTTPSURL(candidate *url.URL, suffixes []string) bool {
	if candidate == nil || candidate.Scheme != "https" || candidate.User != nil {
		return false
	}
	host := strings.ToLower(strings.TrimSuffix(candidate.Hostname(), "."))
	for _, suffix := range suffixes {
		if host == suffix || strings.HasSuffix(host, "."+suffix) {
			return true
		}
	}
	return false
}
