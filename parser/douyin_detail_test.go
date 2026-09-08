package parser

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/go-resty/resty/v2"
)

func TestExtractDouyinAwemeID(t *testing.T) {
	tests := []struct {
		name string
		text string
		want string
	}{
		{"video URL", "https://www.douyin.com/video/7450123456789012345", "7450123456789012345"},
		{"share note URL", "https://www.iesdouyin.com/share/note/7450123456789012346/", "7450123456789012346"},
		{"modal URL", "https://www.douyin.com/jingxuan?modal_id=7450123456789012347", "7450123456789012347"},
		{"JSON item ID", `{"itemId":"7450123456789012348"}`, "7450123456789012348"},
		{"JavaScript item ID", `itemId: '7450123456789012349'`, "7450123456789012349"},
		{"not found", "https://v.douyin.com/abcdef/", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := extractDouyinAwemeID(tt.text); got != tt.want {
				t.Fatalf("extractDouyinAwemeID() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestResolveDouyinAwemeIDFromRedirect(t *testing.T) {
	const videoID = "7450123456789012345"
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/short" {
			if got := request.Header.Get(HttpHeaderUserAgent); got != douyinShareUserAgent {
				t.Errorf("User-Agent = %q, want %q", got, douyinShareUserAgent)
			}
			http.Redirect(writer, request, server.URL+"/video/"+videoID, http.StatusFound)
			return
		}
		writer.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	got, resolvedURL, err := resolveDouyinAwemeID(resty.New(), server.URL+"/short")
	if err != nil {
		t.Fatalf("resolveDouyinAwemeID() error = %v", err)
	}
	if got != videoID {
		t.Errorf("video ID = %q, want %q", got, videoID)
	}
	if resolvedURL == nil || resolvedURL.Path != "/video/"+videoID {
		t.Errorf("resolved URL = %v", resolvedURL)
	}
}

func TestResolveDouyinAwemeIDFromBody(t *testing.T) {
	const videoID = "7450123456789012345"
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = writer.Write([]byte(`<script>window.DATA={"itemId":"` + videoID + `"}</script>`))
	}))
	defer server.Close()

	got, _, err := resolveDouyinAwemeID(resty.New(), server.URL+"/short")
	if err != nil {
		t.Fatalf("resolveDouyinAwemeID() error = %v", err)
	}
	if got != videoID {
		t.Errorf("video ID = %q, want %q", got, videoID)
	}
}

func TestDouyinWebDetailQuery(t *testing.T) {
	const videoID = "7450123456789012345"
	const want = "aweme_id=7450123456789012345&aid=6383&device_platform=webapp"
	if got := douyinWebDetailQuery(videoID); got != want {
		t.Fatalf("douyinWebDetailQuery() = %q, want %q", got, want)
	}
}

func TestDouyinWebDetailFetcherFetchesOfficialDetailWithoutCookieOrSignature(t *testing.T) {
	const videoID = "7450123456789012345"
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/aweme/v1/web/aweme/detail/" {
			t.Errorf("request path = %q", request.URL.Path)
		}
		if request.URL.RawQuery != douyinWebDetailQuery(videoID) {
			t.Errorf("query = %q, want %q", request.URL.RawQuery, douyinWebDetailQuery(videoID))
		}
		if got := request.Header.Get(HttpHeaderUserAgent); got != douyinWebDetailUserAgent {
			t.Errorf("User-Agent = %q, want %q", got, douyinWebDetailUserAgent)
		}
		if got := request.Header.Get(HttpHeaderReferer); got != "https://www.douyin.com/" {
			t.Errorf("Referer = %q", got)
		}
		if got := request.Header.Get(HttpHeaderCookie); got != "" {
			t.Errorf("Cookie = %q, want empty", got)
		}
		if got := request.URL.Query().Get("a_bogus"); got != "" {
			t.Errorf("a_bogus = %q, want empty", got)
		}

		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"aweme_detail":{"aweme_id":"7450123456789012345","desc":"native detail"}}`))
	}))
	defer server.Close()

	fetcher := newDouyinWebDetailFetcher(resty.New())
	fetcher.endpoint = server.URL + "/aweme/v1/web/aweme/detail/"
	data, err := fetcher.fetch(videoID)
	if err != nil {
		t.Fatalf("fetch() error = %v", err)
	}
	if got := data.Get("aweme_id").String(); got != videoID {
		t.Errorf("aweme_id = %q, want %q", got, videoID)
	}
}

func TestDouyinWebDetailFetcherRetriesInvalidResponses(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		switch requests.Add(1) {
		case 1:
			_, _ = writer.Write([]byte("not JSON"))
		case 2:
			_, _ = writer.Write([]byte(`{"status_code":0,"aweme_detail":null}`))
		default:
			_, _ = writer.Write([]byte(`{"aweme_detail":{"aweme_id":"7450123456789012345"}}`))
		}
	}))
	defer server.Close()

	fetcher := newDouyinWebDetailFetcher(resty.New())
	fetcher.endpoint = server.URL
	if _, err := fetcher.fetch("7450123456789012345"); err != nil {
		t.Fatalf("fetch() error = %v", err)
	}
	if got := requests.Load(); got != douyinDetailMaxAttempts {
		t.Errorf("requests = %d, want %d", got, douyinDetailMaxAttempts)
	}
}

func TestDouyinWebDetailFetcherReportsFailureAfterThreeAttempts(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		_, _ = writer.Write([]byte(`{"aweme_detail":null,"filter_detail":{"detail_msg":"作品不可用"}}`))
	}))
	defer server.Close()

	fetcher := newDouyinWebDetailFetcher(resty.New())
	fetcher.endpoint = server.URL
	_, err := fetcher.fetch("7450123456789012345")
	if err == nil {
		t.Fatal("fetch() error = nil, want failure")
	}
	for _, want := range []string{"after 3 attempts", "作品不可用"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("fetch() error = %q, want substring %q", err, want)
		}
	}
	if got := requests.Load(); got != douyinDetailMaxAttempts {
		t.Errorf("requests = %d, want %d", got, douyinDetailMaxAttempts)
	}
}

func TestDouyinWebDetailFetcherRejectsInvalidIDBeforeRequest(t *testing.T) {
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		called = true
	}))
	defer server.Close()

	fetcher := newDouyinWebDetailFetcher(resty.New())
	fetcher.endpoint = server.URL
	_, err := fetcher.fetch("not-an-id")
	if err == nil || !strings.Contains(err.Error(), "invalid douyin video id") {
		t.Fatalf("fetch() error = %v, want invalid ID error", err)
	}
	if called {
		t.Fatal("fetch() made an HTTP request for an invalid ID")
	}
}
