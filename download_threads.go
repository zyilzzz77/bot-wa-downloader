package main

import (
	"fmt"
	"net/url"
	"strings"
)

// classifyMedias memetakan item {type, url} ala Instagram/Threads
// ke Videos/Images/Audio berdasarkan type-nya.
func classifyMedias(result *DownloadResult, medias []InstagramMedia) {
	for _, media := range medias {
		if media.URL == "" {
			continue
		}
		switch strings.ToLower(media.Type) {
		case "mp4", "video", "mp3", "audio":
			if strings.ToLower(media.Type) == "mp3" || strings.ToLower(media.Type) == "audio" {
				result.Audio = append(result.Audio, media.URL)
			} else {
				result.Videos = append(result.Videos, media.URL)
			}
		case "jpg", "jpeg", "png", "webp", "gif", "image", "photo":
			result.Images = append(result.Images, media.URL)
		default:
			result.Videos = append(result.Videos, media.URL)
		}
	}
}

// DownloadThreads mengambil data media Threads melalui API neoxr.
func DownloadThreads(threadsURL, apiKey string) (*DownloadResult, error) {
	apiURL := fmt.Sprintf(
		"https://api.neoxr.eu/api/threads?url=%s&apikey=%s",
		url.QueryEscape(threadsURL),
		apiKey,
	)

	body, err := callAPIWithRetry(apiURL, "threads")
	if err != nil {
		return nil, err
	}

	var apiResp ThreadsResponse
	if err := decodeAPIResponse(body, "Threads", &apiResp); err != nil {
		return nil, err
	}

	if !apiResp.Status {
		return nil, apiStatusError(apiResp.Message,
			"API Threads menolak permintaan — pastikan link valid dan akun publik.")
	}

	result := &DownloadResult{
		Platform: "threads",
		Creator:  apiResp.Creator,
	}

	classifyMedias(result, apiResp.Data)

	if len(result.Videos) == 0 && len(result.Images) == 0 && len(result.Audio) == 0 {
		return nil, fmt.Errorf("API Threads tidak mengembalikan media — pastikan link valid")
	}

	return result, nil
}
