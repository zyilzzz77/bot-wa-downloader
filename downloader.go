package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"strings"
	"time"
)

// --- TikTok API response types ---

// TikTokResponse merepresentasikan respons dari API snaptik-v2 / snaptik.
type TikTokResponse struct {
	Creator string     `json:"creator"`
	Status  bool       `json:"status"`
	Message string     `json:"msg"`
	Data    TikTokData `json:"data"`
}

// TikTokData berisi media TikTok hasil ekstraksi.
type TikTokData struct {
	Video   string   `json:"video"`
	VideoHD string   `json:"videoHD"`
	Audio   string   `json:"audio"`
	Photo   []string `json:"photo"`
}

// AioResponse merepresentasikan respons dari API aio (fallback TikTok).
type AioResponse struct {
	Creator string  `json:"creator"`
	Status  bool    `json:"status"`
	Message string  `json:"msg"`
	Data    AioData `json:"data"`
}

// AioData berisi media dari API aio. Field "photo" bisa berupa array URL
// atau boolean false, sedangkan "video"/"audio" bisa berupa string URL atau
// boolean false, sehingga disimpan mentah dan di-parse terpisah.
type AioData struct {
	Video   json.RawMessage `json:"video"`
	VideoWM json.RawMessage `json:"videoWM"`
	Audio   json.RawMessage `json:"audio"`
	Photo   json.RawMessage `json:"photo"`
}

// parseStringField mengekstrak nilai string dari field API yang bisa berupa
// string URL atau boolean false (mis. "audio": false). Mengembalikan "" jika
// field kosong atau bukan string.
func parseStringField(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return ""
	}
	return s
}

// parsePhotoURLs mengekstrak URL foto dari field "photo" API aio.
// Mengembalikan nil jika field bukan array string (mis. boolean false).
func parsePhotoURLs(raw json.RawMessage) []string {
	if len(raw) == 0 {
		return nil
	}
	var photos []string
	if err := json.Unmarshal(raw, &photos); err != nil {
		return nil
	}
	return photos
}

// tiktokEndpoint adalah satu kandidat endpoint API TikTok.
// useAio menandai endpoint yang memakai bentuk respons data{photo,audio,video}
// (aio & tiktok), bukan bentuk snaptik.
type tiktokEndpoint struct {
	name   string
	url    string
	useAio bool
}

// tiktokEndpoints membuat daftar endpoint TikTok berurutan sesuai prioritas.
func tiktokEndpoints(tiktokURL, apiKey string) []tiktokEndpoint {
	escaped := url.QueryEscape(tiktokURL)
	return []tiktokEndpoint{
		{"snaptik-v2", fmt.Sprintf("https://api.neoxr.eu/api/snaptik-v2?url=%s&apikey=%s", escaped, apiKey), false},
		{"snaptik", fmt.Sprintf("https://api.neoxr.eu/api/snaptik?url=%s&apikey=%s", escaped, apiKey), false},
		{"aio", fmt.Sprintf("https://api.neoxr.eu/api/aio?url=%s&apikey=%s", escaped, apiKey), true},
		{"tiktok", fmt.Sprintf("https://api.neoxr.eu/api/tiktok?url=%s&apikey=%s", escaped, apiKey), true},
	}
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
	Creator string           `json:"creator"`
	Status  bool             `json:"status"`
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

// --- YouTube download API response types ---

// YouTubeResponse merepresentasikan respons dari API youtube.
// Bentuknya sama dengan PlayResponse, plus field publish & fduration untuk audio.
type YouTubeResponse struct {
	Creator   string   `json:"creator"`
	Status    bool     `json:"status"`
	Message   string   `json:"msg"`
	ID        string   `json:"id"`
	Title     string   `json:"title"`
	Thumbnail string   `json:"thumbnail"`
	Duration  string   `json:"duration"`
	FDuration string   `json:"fduration"`
	Channel   string   `json:"channel"`
	Views     string   `json:"views"`
	Data      PlayData `json:"data"`
}

// DurationText mengembalikan durasi siap tampil.
// Video memakai field "duration", audio memakai "fduration".
func (y *YouTubeResponse) DurationText() string {
	if y.FDuration != "" {
		return y.FDuration
	}
	return y.Duration
}

// --- Google Drive API response types ---

// GDriveResponse merepresentasikan respons dari API gdrive.
type GDriveResponse struct {
	Creator string     `json:"creator"`
	Status  bool       `json:"status"`
	Message string     `json:"msg"`
	Data    GDriveData `json:"data"`
}

// GDriveData berisi link unduhan file Google Drive.
type GDriveData struct {
	URL string `json:"url"`
}

// --- Brat (sticker) API response types ---

// BratResponse merepresentasikan respons dari API brat / bratvid.
type BratResponse struct {
	Creator string   `json:"creator"`
	Status  bool     `json:"status"`
	Message string   `json:"msg"`
	Data    BratData `json:"data"`
}

// BratData berisi file gambar (brat) atau video (bratvid) hasil generator.
type BratData struct {
	URL  string `json:"url"`
	Mime string `json:"mime"`
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
// Mencoba beberapa endpoint secara berurutan
// (snaptik-v2 → snaptik → aio → tiktok) dan memakai hasil pertama yang berhasil.
func DownloadTikTok(tiktokURL, apiKey string) (*DownloadResult, error) {
	var lastErr error
	for _, ep := range tiktokEndpoints(tiktokURL, apiKey) {
		result, err := fetchTikTok(ep)
		if err != nil {
			log.Printf("[tiktok] %s gagal: %v — mencoba fallback...", ep.name, err)
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
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return nil, fmt.Errorf("gagal membaca respons API TikTok: %w", err)
	}

	if !apiResp.Status {
		return nil, fmt.Errorf("API TikTok mengembalikan status gagal — %s", apiResp.Message)
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

// parseAioResponse mem-parse respons endpoint aio.
func parseAioResponse(body []byte) (*DownloadResult, error) {
	var apiResp AioResponse
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return nil, fmt.Errorf("gagal membaca respons API TikTok: %w", err)
	}

	if !apiResp.Status {
		return nil, fmt.Errorf("API TikTok mengembalikan status gagal — %s", apiResp.Message)
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

// --- YouTube downloader ---

// Pilihan unduhan YouTube: jenis media dan kualitas default-nya.
const (
	youTubeTypeVideo    = "video"
	youTubeTypeAudio    = "audio"
	youTubeDefaultVideo = "720p"
	youTubeDefaultAudio = "128kbps"
)

// Ladder kualitas YouTube dari tertinggi ke terendah. Dipakai untuk fallback
// saat kualitas yang diminta tidak tersedia di video tersebut.
var (
	youTubeVideoQualities = []string{"2160p", "1440p", "1080p", "720p", "480p", "360p", "240p", "144p"}
	youTubeAudioQualities = []string{"320kbps", "256kbps", "192kbps", "160kbps", "128kbps", "96kbps", "64kbps"}
)

// youTubeFallbackQualities mengembalikan kualitas yang dicoba setelah kualitas
// yang diminta, dari yang terdekat sampai terendah. Kosong kalau tidak dikenali.
func youTubeFallbackQualities(mediaType, quality string) []string {
	ladder := youTubeVideoQualities
	if mediaType == youTubeTypeAudio {
		ladder = youTubeAudioQualities
	}
	for i, q := range ladder {
		if q == quality {
			return ladder[i+1:]
		}
	}
	return nil
}

// isQualityUnavailable memeriksa apakah error berasal dari pesan API
// "Quality <x> not available" — bukan error lain seperti link tidak valid.
func isQualityUnavailable(err error) bool {
	msg := strings.ToLower(err.Error())
	return strings.HasPrefix(msg, "quality ") && strings.Contains(msg, "not available")
}

// fetchYouTube memanggil API youtube satu kali untuk satu kualitas.
func fetchYouTube(youtubeURL, mediaType, quality, apiKey string) (*YouTubeResponse, error) {
	apiURL := fmt.Sprintf(
		"https://api.neoxr.eu/api/youtube?url=%s&type=%s&quality=%s&apikey=%s",
		url.QueryEscape(youtubeURL),
		mediaType,
		url.QueryEscape(quality),
		apiKey,
	)

	body, err := callAPIWithRetry(apiURL, "youtube")
	if err != nil {
		return nil, err
	}

	var apiResp YouTubeResponse
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return nil, fmt.Errorf("gagal membaca respons API YouTube: %w", err)
	}

	if !apiResp.Status {
		if apiResp.Message != "" {
			return nil, errors.New(apiResp.Message)
		}
		return nil, fmt.Errorf("API YouTube mengembalikan status gagal — pastikan link valid")
	}
	if apiResp.Data.URL == "" {
		return nil, fmt.Errorf("API YouTube tidak mengembalikan link download — coba lagi")
	}

	return &apiResp, nil
}

// DownloadYouTube mengambil link unduhan YouTube melalui API neoxr.
// mediaType "video" mengembalikan mp4, "audio" mengembalikan mp3.
// Kalau kualitas yang diminta tidak tersedia, otomatis turun ke kualitas
// terdekat di bawahnya; kualitas yang akhirnya dipakai ada di Data.Quality.
func DownloadYouTube(youtubeURL, mediaType, quality, apiKey string) (*YouTubeResponse, error) {
	resp, err := fetchYouTube(youtubeURL, mediaType, quality, apiKey)
	if err == nil {
		return resp, nil
	}
	if !isQualityUnavailable(err) {
		return nil, err
	}

	requestedErr := err
	for _, fallback := range youTubeFallbackQualities(mediaType, quality) {
		log.Printf("[youtube] Kualitas %s tidak tersedia — coba %s...", quality, fallback)

		resp, err = fetchYouTube(youtubeURL, mediaType, fallback, apiKey)
		if err == nil {
			log.Printf("[youtube] ✓ Pakai kualitas %s", fallback)
			return resp, nil
		}
		if !isQualityUnavailable(err) {
			return nil, err
		}
	}

	return nil, requestedErr
}

// --- Google Drive downloader ---

// gdriveIDRegex mengambil file ID dari link Google Drive.
var gdriveIDRegex = regexp.MustCompile(`(?:/file/d/|/folders/|[?&]id=)([\w-]+)`)

// gdriveFileNameRegex mengambil nama file dari header Content-Disposition.
var gdriveFileNameRegex = regexp.MustCompile(`filename\*?=(?:UTF-8'')?"?([^";]+)"?`)

// gdriveFileID mengembalikan file ID dari sebuah link Google Drive.
func gdriveFileID(link string) string {
	if m := gdriveIDRegex.FindStringSubmatch(link); m != nil {
		return m[1]
	}
	return ""
}

// gdriveDirectURL membangun link unduhan resmi Google Drive untuk sebuah file ID.
// Dipakai saat link dari API ternyata halaman HTML, bukan file.
func gdriveDirectURL(fileID string) string {
	return fmt.Sprintf(
		"https://drive.usercontent.google.com/download?id=%s&export=download&confirm=t",
		fileID,
	)
}

// fetchGDriveFile mengunduh isi sebuah link dan mengembalikan byte, nama file, serta MIME-nya.
func fetchGDriveFile(downloadURL string) ([]byte, string, string, error) {
	resp, err := httpClient.Get(downloadURL)
	if err != nil {
		return nil, "", "", fmt.Errorf("gagal menghubungi Google Drive: %w", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", "", fmt.Errorf("gagal membaca file: %w", err)
	}

	disposition := resp.Header.Get("Content-Disposition")
	fileName := ""
	if m := gdriveFileNameRegex.FindStringSubmatch(disposition); m != nil {
		fileName = strings.TrimSpace(m[1])
	}

	return data, fileName, resp.Header.Get("Content-Type"), nil
}

// DownloadGDrive mengambil file Google Drive lewat API neoxr.
// Mengembalikan isi file beserta nama file dan MIME-nya.
// Link yang tidak dikenali API (misal open?id=) dilewatkan apa adanya, sehingga
// yang terunduh halaman HTML Google — kasus itu diulang lewat endpoint resmi.
func DownloadGDrive(gdriveURL, apiKey string) ([]byte, string, string, error) {
	apiURL := fmt.Sprintf(
		"https://api.neoxr.eu/api/gdrive?url=%s&apikey=%s",
		url.QueryEscape(gdriveURL),
		apiKey,
	)

	body, err := callAPIWithRetry(apiURL, "gdrive")
	if err != nil {
		return nil, "", "", err
	}

	var apiResp GDriveResponse
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return nil, "", "", fmt.Errorf("gagal membaca respons API Google Drive: %w", err)
	}

	if !apiResp.Status {
		if apiResp.Message != "" {
			return nil, "", "", errors.New(apiResp.Message)
		}
		return nil, "", "", fmt.Errorf("API Google Drive mengembalikan status gagal — pastikan link valid")
	}
	if apiResp.Data.URL == "" {
		return nil, "", "", fmt.Errorf("API Google Drive tidak mengembalikan link download — coba lagi")
	}

	data, fileName, mimeType, err := fetchGDriveFile(apiResp.Data.URL)
	if err != nil {
		return nil, "", "", err
	}

	if strings.HasPrefix(mimeType, "text/html") {
		fileID := gdriveFileID(apiResp.Data.URL)
		if fileID == "" {
			return nil, "", "", fmt.Errorf("Google Drive mengembalikan halaman web, bukan file — pastikan link bisa diakses publik")
		}

		log.Printf("[gdrive] Dapat halaman HTML — ulangi lewat endpoint download resmi...")
		data, fileName, mimeType, err = fetchGDriveFile(gdriveDirectURL(fileID))
		if err != nil {
			return nil, "", "", err
		}
	}

	if strings.HasPrefix(mimeType, "text/html") {
		return nil, "", "", fmt.Errorf("Google Drive tidak mengembalikan file — pastikan link bisa diakses publik")
	}
	if fileName == "" {
		fileName = "gdrive-file"
	}

	return data, fileName, mimeType, nil
}

// --- Brat (sticker) ---

// GenerateBrat meminta generator brat (gambar) atau bratvid (video animasi)
// dari API neoxr. Mengembalikan URL media beserta MIME-nya.
func GenerateBrat(text, apiKey string, animated bool) (string, string, error) {
	endpoint := "brat"
	if animated {
		endpoint = "bratvid"
	}

	apiURL := fmt.Sprintf(
		"https://api.neoxr.eu/api/%s?text=%s&apikey=%s",
		endpoint,
		url.QueryEscape(text),
		apiKey,
	)

	body, err := callAPIWithRetry(apiURL, endpoint)
	if err != nil {
		return "", "", err
	}

	var apiResp BratResponse
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return "", "", fmt.Errorf("gagal membaca respons API %s: %w", endpoint, err)
	}

	if !apiResp.Status {
		if apiResp.Message != "" {
			return "", "", errors.New(apiResp.Message)
		}
		return "", "", fmt.Errorf("API %s mengembalikan status gagal", endpoint)
	}
	if apiResp.Data.URL == "" {
		return "", "", fmt.Errorf("API %s tidak mengembalikan media — coba lagi", endpoint)
	}

	return apiResp.Data.URL, apiResp.Data.Mime, nil
}
