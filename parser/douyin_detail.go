package parser

import (
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/go-resty/resty/v2"
	"github.com/tidwall/gjson"
)

const (
	douyinWebDetailEndpoint  = "https://www.douyin.com/aweme/v1/web/aweme/detail/"
	douyinWebDetailUserAgent = "Mozilla/5.0 (compatible; Bingbot/2.0; +http://www.bing.com/bingbot.htm)"
	douyinShareUserAgent     = "Mozilla/5.0 (Linux; Android 11; SAMSUNG SM-G973U) AppleWebKit/537.36 (KHTML, like Gecko) SamsungBrowser/14.2 Chrome/87.0.4280.141 Mobile Safari/537.36"
	douyinDetailMaxAttempts  = 3
)

var douyinAwemeIDPatterns = []*regexp.Regexp{
	regexp.MustCompile(`/(?:share/)?(?:video|note)/(\d{8,})`),
	regexp.MustCompile(`(?:[/?&]|^)modal_id=(\d{8,})`),
	regexp.MustCompile(`"itemId"\s*:\s*"(\d+)"`),
	regexp.MustCompile(`itemId["']?\s*:\s*["']?(\d{8,})`),
}

type douyinWebDetailFetcher struct {
	client      *resty.Client
	endpoint    string
	maxAttempts int
}

func (d douYin) fetchNativeVideoDetail(client *resty.Client, videoID string) (gjson.Result, error) {
	return newDouyinWebDetailFetcher(client).fetch(videoID)
}

func newDouyinWebDetailFetcher(client *resty.Client) douyinWebDetailFetcher {
	if client == nil {
		client = newClient()
	}

	return douyinWebDetailFetcher{
		client:      client,
		endpoint:    douyinWebDetailEndpoint,
		maxAttempts: douyinDetailMaxAttempts,
	}
}

func (f douyinWebDetailFetcher) fetch(videoID string) (gjson.Result, error) {
	if !isDouyinVideoID(videoID) {
		return gjson.Result{}, fmt.Errorf("invalid douyin video id: %q", videoID)
	}
	if f.client == nil {
		return gjson.Result{}, errors.New("douyin detail client is nil")
	}
	if f.endpoint == "" {
		return gjson.Result{}, errors.New("douyin detail endpoint is empty")
	}

	maxAttempts := f.maxAttempts
	if maxAttempts <= 0 {
		maxAttempts = douyinDetailMaxAttempts
	}
	requestURL := f.endpoint + "?" + douyinWebDetailQuery(videoID)
	var lastError error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		response, err := f.client.R().
			SetHeaders(map[string]string{
				HttpHeaderUserAgent: douyinWebDetailUserAgent,
				HttpHeaderReferer:   "https://www.douyin.com/",
			}).
			Get(requestURL)
		if err != nil {
			lastError = fmt.Errorf("request douyin detail: %w", err)
			continue
		}
		if response == nil {
			lastError = errors.New("douyin detail returned empty response")
			continue
		}

		body := response.Body()
		if !gjson.ValidBytes(body) {
			lastError = fmt.Errorf("douyin detail returned invalid JSON (status %d)", response.StatusCode())
			continue
		}

		data := gjson.GetBytes(body, "aweme_detail")
		if data.IsObject() {
			return data, nil
		}

		message := strings.TrimSpace(gjson.GetBytes(body, "filter_detail.detail_msg").String())
		if message == "" {
			message = strings.TrimSpace(gjson.GetBytes(body, "status_msg").String())
		}
		if message == "" {
			message = "aweme_detail is missing"
		}
		lastError = fmt.Errorf("douyin detail unavailable (status %d): %s", response.StatusCode(), message)
	}

	return gjson.Result{}, fmt.Errorf("no video or image content found after %d attempts: %w", maxAttempts, lastError)
}

func douyinWebDetailQuery(videoID string) string {
	return "aweme_id=" + url.QueryEscape(videoID) + "&aid=6383&device_platform=webapp"
}

func extractDouyinAwemeID(text string) string {
	for _, pattern := range douyinAwemeIDPatterns {
		match := pattern.FindStringSubmatch(text)
		if len(match) > 1 {
			return match[1]
		}
	}
	return ""
}

func resolveDouyinAwemeID(client *resty.Client, shareURL string) (string, *url.URL, error) {
	if videoID := extractDouyinAwemeID(shareURL); videoID != "" {
		parsedURL, _ := url.Parse(shareURL)
		return videoID, parsedURL, nil
	}
	if client == nil {
		return "", nil, errors.New("douyin share client is nil")
	}

	response, err := client.R().
		SetHeader(HttpHeaderUserAgent, douyinShareUserAgent).
		Get(shareURL)
	if err != nil {
		return "", nil, fmt.Errorf("resolve douyin share URL: %w", err)
	}
	if response == nil {
		return "", nil, errors.New("douyin share URL returned empty response")
	}

	var resolvedURL *url.URL
	if response.RawResponse != nil && response.RawResponse.Request != nil {
		resolvedURL = response.RawResponse.Request.URL
	}
	if resolvedURL != nil {
		if videoID := extractDouyinAwemeID(resolvedURL.String()); videoID != "" {
			return videoID, resolvedURL, nil
		}
	}
	if videoID := extractDouyinAwemeID(string(response.Body())); videoID != "" {
		return videoID, resolvedURL, nil
	}

	return "", resolvedURL, errors.New("video ID not found in douyin URL")
}

func isDouyinVideoID(value string) bool {
	if len(value) < 8 || len(value) > 32 {
		return false
	}
	for _, char := range value {
		if char < '0' || char > '9' {
			return false
		}
	}
	return true
}
