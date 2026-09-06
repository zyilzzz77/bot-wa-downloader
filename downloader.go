package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"path"
	"strings"
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

// --- Threads API response types ---

// ThreadsResponse merepresentasikan respons dari API threads.
// Bentuknya sama dengan Instagram: array {type, url}.
type ThreadsResponse struct {
	Creator string          `json:"creator"`
	Status  bool            `json:"status"`
	Data    []InstagramMedia `json:"data"`
}

// --- Play (YouTube search) API response types ---

// PlayResponse merepresentasikan respons dari API play.
type PlayResponse struct {
	Creator         string   `json:"creator"`
	Status          bool     `json:"status"`
	ID              string   `json:"id"`
	Title           string   `json:"title"`
	Thumbnail       string   `json:"thumbnail"`
	Duration        string   `json:"duration"`
	DurationSeconds int      `json:"duration_seconds"`
	Channel         string   `json:"channel"`
	Views           string   `json:"views"`
	Data            PlayData `json:"data"`
}

// PlayData berisi file audio hasil pencarian lagu.
type PlayData struct {
	Filename  string `json:"filename"`
	Quality   string `json:"quality"`
	Size      string `json:"size"`
	Extension string `json:"extension"`
	URL       string `json:"url"`
}

// --- Terabox API response types ---

// TeraboxResponse merepresentasikan respons dari API terabox.
type TeraboxResponse struct {
	Creator string        `json:"creator"`
	Status  bool          `json:"status"`
	Data    []TeraboxFile `json:"data"`
}

// TeraboxFile berisi satu file dari share link Terabox.
type TeraboxFile struct {
	ServerFilename string `json:"server_filename"`
	Size           string `json:"size"`
	Bytes          string `json:"bytes"`
	Dlink          string `json:"dlink"`
	URL            string `json:"url"`
	Duration       int    `json:"duration"`
}

// DocumentFile adalah file generik (selain video/gambar/audio) beserta nama filenya.
type DocumentFile struct {
	URL      string
	FileName string
}

// DownloadResult adalah hasil download yang sudah diolah dan siap dikirim.
type DownloadResult struct {
	Platform  string         // "tiktok", "instagram", "threads", atau "terabox"
	Creator   string         // info kreator dari API
	Videos    []string       // URL video
	Images    []string       // URL gambar
	Audio     []string       // URL audio
	Documents []DocumentFile // file generik (terabox non-video)
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
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return nil, fmt.Errorf("gagal membaca respons API Threads: %w", err)
	}

	if !apiResp.Status {
		return nil, fmt.Errorf("API Threads mengembalikan status gagal — pastikan link valid")
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

// teraboxExtVideo adalah ekstensi yang dikirim sebagai video WA.
var teraboxExtVideo = map[string]bool{
	".mp4": true, ".mov": true, ".mkv": true, ".webm": true,
	".avi": true, ".3gp": true, ".m4v": true,
}

// teraboxExtImage adalah ekstensi yang dikirim sebagai gambar WA.
var teraboxExtImage = map[string]bool{
	".jpg": true, ".jpeg": true, ".png": true, ".webp": true, ".gif": true,
}

// teraboxExtAudio adalah ekstensi yang dikirim sebagai audio WA.
var teraboxExtAudio = map[string]bool{
	".mp3": true, ".ogg": true, ".wav": true, ".m4a": true, ".opus": true,
}

// DownloadTerabox mengambil data file Terabox melalui API neoxr.
// Video/gambar/audio dipetakan ke Videos/Images/Audio berdasarkan ekstensi,
// sisanya (zip, pdf, dsb) masuk ke Documents beserta nama filenya.
func DownloadTerabox(teraboxURL, apiKey string) (*DownloadResult, error) {
	apiURL := fmt.Sprintf(
		"https://api.neoxr.eu/api/terabox?url=%s&apikey=%s",
		url.QueryEscape(teraboxURL),
		apiKey,
	)

	body, err := callAPIWithRetry(apiURL, "terabox")
	if err != nil {
		return nil, err
	}

	var apiResp TeraboxResponse
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return nil, fmt.Errorf("gagal membaca respons API Terabox: %w", err)
	}

	if !apiResp.Status {
		return nil, fmt.Errorf("API Terabox mengembalikan status gagal — pastikan link valid")
	}
	if len(apiResp.Data) == 0 {
		return nil, fmt.Errorf("API Terabox tidak mengembalikan file — pastikan link valid")
	}

	result := &DownloadResult{
		Platform: "terabox",
		Creator:  apiResp.Creator,
	}

	for _, f := range apiResp.Data {
		dlURL := f.Dlink
		if dlURL == "" {
			dlURL = f.URL
		}
		if dlURL == "" {
			continue
		}
		name := f.ServerFilename
		if name == "" {
			name = "terabox-file"
		}
		ext := strings.ToLower(path.Ext(name))

		switch {
		case teraboxExtVideo[ext] || (ext == "" && f.Duration > 0):
			result.Videos = append(result.Videos, dlURL)
		case teraboxExtImage[ext]:
			result.Images = append(result.Images, dlURL)
		case teraboxExtAudio[ext]:
			result.Audio = append(result.Audio, dlURL)
		default:
			result.Documents = append(result.Documents, DocumentFile{URL: dlURL, FileName: name})
		}
	}

	if len(result.Videos) == 0 && len(result.Images) == 0 &&
		len(result.Audio) == 0 && len(result.Documents) == 0 {
		return nil, fmt.Errorf("API Terabox tidak mengembalikan link download — pastikan link valid")
	}

	return result, nil
}

// SearchPlay mencari lagu di YouTube melalui API play neoxr.
// Mengembalikan info lagu beserta URL audio mp3-nya.
func SearchPlay(query, apiKey string) (*PlayResponse, error) {
	apiURL := fmt.Sprintf(
		"https://api.neoxr.eu/api/play?q=%s&apikey=%s",
		url.QueryEscape(query),
		apiKey,
	)

	body, err := callAPIWithRetry(apiURL, "play")
	if err != nil {
		return nil, err
	}

	var apiResp PlayResponse
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return nil, fmt.Errorf("gagal membaca respons API Play: %w", err)
	}

	if !apiResp.Status {
		return nil, fmt.Errorf("lagu tidak ditemukan — coba kata kunci lain")
	}
	if apiResp.Data.URL == "" {
		return nil, fmt.Errorf("API Play tidak mengembalikan link audio — coba lagi")
	}

	return &apiResp, nil
}
