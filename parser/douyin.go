package parser

import (
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/tidwall/gjson"
)

const douyinVideoURL = "https://www.iesdouyin.com/aweme/v1/play/?video_id=%s&ratio=1080p&line=0"

type douYin struct{}

func (d douYin) parseVideoID(videoID string) (*VideoParseInfo, error) {
	if !isDouyinVideoID(videoID) {
		return nil, fmt.Errorf("invalid douyin video id: %q", videoID)
	}

	data, err := d.fetchNativeVideoDetail(newClient(), videoID)
	if err != nil {
		return nil, fmt.Errorf("get native douyin video detail: %w", err)
	}
	return d.videoInfoFromDetail(data)
}

func (d douYin) videoInfoFromDetail(data gjson.Result) (*VideoParseInfo, error) {
	images := d.parseImageList(data.Get("images").Array())
	playURI := data.Get("video.play_addr.uri").String()
	videoURL := ""
	if playURI != "" {
		videoURL = fmt.Sprintf(douyinVideoURL, playURI)
	}

	if videoURL == "" && len(images) == 0 {
		return nil, errors.New("no video or image content found in the response")
	}

	coverURL := d.getNoWebpURL(data.Get("video.cover.url_list").Array())
	videoInfo := &VideoParseInfo{
		Title:    data.Get("desc").String(),
		VideoUrl: videoURL,
		CoverUrl: coverURL,
		Images:   images,
	}
	if videoURL == "" {
		videoInfo.MusicUrl = data.Get("music.play_url.url_list.0").String()
	}
	videoInfo.Author.Uid = data.Get("author.sec_uid").String()
	videoInfo.Author.Name = data.Get("author.nickname").String()
	videoInfo.Author.Avatar = data.Get("author.avatar_thumb.url_list.0").String()

	return videoInfo, nil
}

func (d douYin) parseImageList(imageItems []gjson.Result) []ImgInfo {
	images := make([]ImgInfo, 0, len(imageItems))
	for _, imageItem := range imageItems {
		candidates := imageItem.Get("download_url_list").Array()
		if len(candidates) == 0 {
			candidates = imageItem.Get("url_list").Array()
		}

		imageURL := ""
		for _, candidate := range candidates {
			if !strings.Contains(candidate.String(), "/obj/") {
				imageURL = candidate.String()
				break
			}
		}
		if imageURL == "" && len(candidates) > 0 {
			imageURL = candidates[0].String()
		}
		if imageURL == "" || strings.Contains(imageURL, "/obj/") {
			continue
		}

		images = append(images, ImgInfo{
			Url:          imageURL,
			LivePhotoUrl: imageItem.Get("video.play_addr.url_list.0").String(),
		})
	}
	return images
}

func (d douYin) parseShareUrl(shareURL string) (*VideoParseInfo, error) {
	parsedURL, err := url.Parse(shareURL)
	if err != nil {
		return nil, err
	}

	switch parsedURL.Hostname() {
	case "www.iesdouyin.com", "www.douyin.com", "v.douyin.com":
		return d.parseResolvedShareURL(shareURL)
	default:
		return nil, fmt.Errorf("douyin not support this host: %s", parsedURL.Hostname())
	}
}

func (d douYin) parseResolvedShareURL(shareURL string) (*VideoParseInfo, error) {
	videoID, resolvedURL, err := resolveDouyinAwemeID(newClient(), shareURL)
	if err != nil {
		return nil, err
	}

	if resolvedURL != nil && strings.Contains(resolvedURL.Hostname(), "ixigua.com") {
		return xiGua{}.parseVideoID(videoID)
	}
	return d.parseVideoID(videoID)
}

func (d douYin) parseAppShareUrl(shareURL string) (*VideoParseInfo, error) {
	return d.parseResolvedShareURL(shareURL)
}

func (d douYin) parsePcShareUrl(shareURL string) (*VideoParseInfo, error) {
	return d.parseResolvedShareURL(shareURL)
}

func (d douYin) parseVideoIdFromPath(urlPath string) (string, error) {
	if strings.TrimSpace(urlPath) == "" {
		return "", errors.New("url path is empty")
	}
	videoID := extractDouyinAwemeID(urlPath)
	if videoID == "" {
		return "", errors.New("parse video id from path fail")
	}
	return videoID, nil
}

func (d douYin) getNoWebpURL(urlList []gjson.Result) string {
	for _, item := range urlList {
		if !strings.Contains(item.String(), ".webp") {
			return item.String()
		}
	}
	if len(urlList) > 0 {
		return urlList[0].String()
	}
	return ""
}
