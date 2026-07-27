package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"time"
)

// --- TikTok API response types ---

// TikTokResponse merepresentasikan respons dari API snaptik-v2.
type TikTokResponse struct {
	Creator string     `json:"creator"`
	Status  bool       `json:"status"`
	Data    TikTokData `json:"data"`
}

// TikTokData berisi media TikTok hasil ekstraksi.
type TikTokData struct {
	Video   string   `json:"video"`
	VideoHD string   `json:"videoHD"`
	Audio   string   `json:"audio"`
	Photo   []string `json:"photo"`
}

// --- Instagram API response types ---

// InstagramResponse merepresentasikan respons dari API ig.
type InstagramResponse struct {
	Creator string           `json:"creator"`
	Status  bool             `json:"status"`
	Data    []InstagramMedia `json:"data"`
}

// InstagramMedia berisi satu item media Instagram.
type InstagramMedia struct {
	Type string `json:"type"` // "mp4", "jpg", "jpeg", "png", dsb.
	URL  string `json:"url"`
}

// --- Hasil download gabungan ---

// DownloadResult adalah hasil download yang sudah diolah dan siap dikirim.
type DownloadResult struct {
	Platform string   // "tiktok" atau "instagram"
	Creator  string   // info kreator dari API
	Videos   []string // URL video
	Images   []string // URL gambar
	Audio    []string // URL audio
}

// httpClient digunakan bersama dengan timeout yang cukup untuk download.
var httpClient = &http.Client{Timeout: 120 * time.Second}

// --- Retry wrapper ---

const maxRetries = 3

// callAPIWithRetry memanggil API dengan auto-retry (backoff 2s, 4s, 8s).
// Retry hanya untuk status 5xx (server error).
func callAPIWithRetry(apiURL, platform string) ([]byte, error) {
	var lastErr error
	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			delay := time.Duration(1<<attempt) * time.Second // 2s, 4s, 8s
			log.Printf("[%s] Retry %d/%d — menunggu %v...", platform, attempt, maxRetries-1, delay)
			time.Sleep(delay)
		}

		resp, err := httpClient.Get(apiURL)
		if err != nil {
			lastErr = fmt.Errorf("gagal menghubungi API: %w", err)
			continue
		}

		body, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()

		if readErr != nil {
			lastErr = fmt.Errorf("gagal membaca respons API: %w", readErr)
			continue
		}

		// Status 5xx → server error, retry. Status lain → langsung return.
		if resp.StatusCode >= 500 {
			lastErr = fmt.Errorf("API %s error %d: %s", platform, resp.StatusCode, string(body))
			continue
		}

		return body, nil
	}
	return nil, lastErr
}

// DownloadTikTok mengambil data media TikTok melalui API neoxr.
func DownloadTikTok(tiktokURL, apiKey string) (*DownloadResult, error) {
	apiURL := fmt.Sprintf(
		"https://api.neoxr.eu/api/snaptik-v2?url=%s&apikey=%s",
		url.QueryEscape(tiktokURL),
		apiKey,
	)

	body, err := callAPIWithRetry(apiURL, "tiktok")
	if err != nil {
		return nil, err
	}

	var apiResp TikTokResponse
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return nil, fmt.Errorf("gagal membaca respons API TikTok: %w", err)
	}

	if !apiResp.Status {
		return nil, fmt.Errorf("API TikTok mengembalikan status gagal — pastikan link valid")
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

// DownloadInstagram mengambil data media Instagram melalui API neoxr.
func DownloadInstagram(igURL, apiKey string) (*DownloadResult, error) {
	apiURL := fmt.Sprintf(
		"https://api.neoxr.eu/api/ig?url=%s&apikey=%s",
		url.QueryEscape(igURL),
		apiKey,
	)

	body, err := callAPIWithRetry(apiURL, "instagram")
	if err != nil {
		return nil, err
	}

	var apiResp InstagramResponse
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return nil, fmt.Errorf("gagal membaca respons API Instagram: %w", err)
	}

	if !apiResp.Status {
		return nil, fmt.Errorf("API Instagram mengembalikan status gagal — pastikan link valid")
	}

	result := &DownloadResult{
		Platform: "instagram",
		Creator:  apiResp.Creator,
	}

	for _, media := range apiResp.Data {
		if media.URL == "" {
			continue
		}
		switch media.Type {
		case "mp4", "video":
			result.Videos = append(result.Videos, media.URL)
		case "jpg", "jpeg", "png", "webp", "image":
			result.Images = append(result.Images, media.URL)
		default:
			result.Videos = append(result.Videos, media.URL)
		}
	}

	return result, nil
}
