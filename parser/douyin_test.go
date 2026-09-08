package parser

import (
	"strings"
	"testing"

	"github.com/tidwall/gjson"
)

func TestDouYinParseVideoIDFromPath(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		want    string
		wantErr bool
	}{
		{"抖音视频", "/share/video/7329354490828623130/", "7329354490828623130", false},
		{"抖音图文", "/note/7450123456789012345", "7450123456789012345", false},
		{"精选视频", "https://www.douyin.com/jingxuan?modal_id=7555093909760789812", "7555093909760789812", false},
		{"西瓜视频", "/douyin/share/video/7144194760184594977", "7144194760184594977", false},
		{"异常视频", "", "", true},
		{"非作品路径", "/user/1234567890123456789", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := (douYin{}).parseVideoIdFromPath(tt.path)
			if (err != nil) != tt.wantErr {
				t.Fatalf("parseVideoIdFromPath() error = %v, wantErr %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("parseVideoIdFromPath() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDouYinVideoInfoFromVideoDetail(t *testing.T) {
	detail := gjson.Parse(`{
		"aweme_id":"7450123456789012345",
		"desc":"示例视频",
		"author":{
			"sec_uid":"author-id",
			"nickname":"作者",
			"avatar_thumb":{"url_list":["https://example.com/avatar.webp"]}
		},
		"video":{
			"play_addr":{"uri":"v0200fg10000example"},
			"cover":{"url_list":["https://example.com/cover.webp","https://example.com/cover.jpeg"]}
		}
	}`)

	got, err := (douYin{}).videoInfoFromDetail(detail)
	if err != nil {
		t.Fatalf("videoInfoFromDetail() error = %v", err)
	}
	wantVideoURL := "https://www.iesdouyin.com/aweme/v1/play/?video_id=v0200fg10000example&ratio=1080p&line=0"
	if got.VideoUrl != wantVideoURL {
		t.Errorf("VideoUrl = %q, want %q", got.VideoUrl, wantVideoURL)
	}
	if got.Title != "示例视频" || got.Author.Name != "作者" || got.Author.Uid != "author-id" {
		t.Errorf("metadata = %+v", got)
	}
	if got.CoverUrl != "https://example.com/cover.jpeg" {
		t.Errorf("CoverUrl = %q", got.CoverUrl)
	}
}

func TestDouYinVideoInfoFromImageDetail(t *testing.T) {
	detail := gjson.Parse(`{
		"desc":"示例图集",
		"images":[
			{
				"download_url_list":["https://example.com/obj/blocked","https://example.com/download.jpeg"],
				"url_list":["https://example.com/fallback.jpeg"],
				"video":{"play_addr":{"url_list":["https://example.com/live.mp4"]}}
			},
			{"download_url_list":["https://example.com/obj/only-blocked"]},
			{"url_list":["https://example.com/image-2.jpeg"]}
		],
		"music":{"play_url":{"url_list":["https://example.com/music.mp3"]}}
	}`)

	got, err := (douYin{}).videoInfoFromDetail(detail)
	if err != nil {
		t.Fatalf("videoInfoFromDetail() error = %v", err)
	}
	if got.VideoUrl != "" {
		t.Errorf("VideoUrl = %q, want empty", got.VideoUrl)
	}
	if got.MusicUrl != "https://example.com/music.mp3" {
		t.Errorf("MusicUrl = %q", got.MusicUrl)
	}
	if len(got.Images) != 2 {
		t.Fatalf("len(Images) = %d, want 2", len(got.Images))
	}
	if got.Images[0].Url != "https://example.com/download.jpeg" || got.Images[0].LivePhotoUrl != "https://example.com/live.mp4" {
		t.Errorf("Images[0] = %+v", got.Images[0])
	}
	if got.Images[1].Url != "https://example.com/image-2.jpeg" {
		t.Errorf("Images[1] = %+v", got.Images[1])
	}
}

func TestDouYinVideoInfoRejectsEmptyMedia(t *testing.T) {
	_, err := (douYin{}).videoInfoFromDetail(gjson.Parse(`{"desc":"empty"}`))
	if err == nil || !strings.Contains(err.Error(), "no video or image content") {
		t.Fatalf("videoInfoFromDetail() error = %v", err)
	}
}
