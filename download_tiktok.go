package main

import (
	"fmt"
	"log"
)

// DownloadTikTok mengambil data media TikTok melalui API neoxr.
// Mencoba beberapa endpoint secara berurutan
// (snaptik-v2 → snaptik → aio → tiktok) dan memakai hasil pertama yang berhasil.
func DownloadTikTok(tiktokURL, apiKey string) (*DownloadResult, error) {
	var lastErr error
	for _, ep := range tiktokEndpoints(tiktokURL, apiKey) {
		result, err := fetchTikTok(ep)
		if err != nil {
			log.Printf("[tiktok] %s gagal: %v — mencoba fallback...", ep.name, err)
			// API key salah / kuota habis tidak akan berubah di endpoint lain.
			if isFatalAPIError(err) {
				return nil, err
			}
			lastErr = err
			continue
		}
		log.Printf("[tiktok] ✓ berhasil via %s", ep.name)
		return result, nil
	}

	return nil, fmt.Errorf("semua endpoint TikTok gagal: %w", lastErr)
}

// fetchTikTok memanggil satu endpoint TikTok dan mem-parse hasilnya.
func fetchTikTok(ep tiktokEndpoint) (*DownloadResult, error) {
	body, err := callAPIWithRetry(ep.url, "tiktok")
	if err != nil {
		return nil, err
	}

	var result *DownloadResult
	if ep.useAio {
		result, err = parseAioResponse(body)
	} else {
		result, err = parseSnaptikResponse(body)
	}
	if err != nil {
		return nil, err
	}

	if len(result.Videos) == 0 && len(result.Images) == 0 && len(result.Audio) == 0 {
		return nil, fmt.Errorf("endpoint %s tidak mengembalikan media", ep.name)
	}
	return result, nil
}

// parseSnaptikResponse mem-parse respons endpoint snaptik-v2 / snaptik.
func parseSnaptikResponse(body []byte) (*DownloadResult, error) {
	var apiResp TikTokResponse
	if err := decodeAPIResponse(body, "TikTok", &apiResp); err != nil {
		return nil, err
	}

	if !apiResp.Status {
		return nil, apiStatusError(apiResp.Message,
			"API TikTok menolak permintaan — pastikan link valid dan video publik.")
	}

	result := &DownloadResult{
		Platform: "tiktok",
		Creator:  apiResp.Creator,
	}
	if apiResp.Data.Video != "" {
		result.Videos = append(result.Videos, apiResp.Data.Video)
	}
	if apiResp.Data.Audio != "" {
		result.Audio = append(result.Audio, apiResp.Data.Audio)
	}
	for _, photo := range apiResp.Data.Photo {
		if photo != "" {
			result.Images = append(result.Images, photo)
		}
	}
	return result, nil
}

// parseAioResponse mem-parse respons endpoint aio / tiktok.
func parseAioResponse(body []byte) (*DownloadResult, error) {
	var apiResp AioResponse
	if err := decodeAPIResponse(body, "TikTok", &apiResp); err != nil {
		return nil, err
	}

	if !apiResp.Status {
		return nil, apiStatusError(apiResp.Message,
			"API TikTok menolak permintaan — pastikan link valid dan video publik.")
	}

	result := &DownloadResult{
		Platform: "tiktok",
		Creator:  apiResp.Creator,
	}
	if video := parseStringField(apiResp.Data.Video); video != "" {
		result.Videos = append(result.Videos, video)
	}
	if audio := parseStringField(apiResp.Data.Audio); audio != "" {
		result.Audio = append(result.Audio, audio)
	}
	for _, photo := range parsePhotoURLs(apiResp.Data.Photo) {
		if photo != "" {
			result.Images = append(result.Images, photo)
		}
	}
	return result, nil
}
