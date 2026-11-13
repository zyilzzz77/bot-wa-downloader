package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
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
// Endpoint "tiktok" ditaruh pertama karena paling stabil saat ini; endpoint
// snaptik sering membalas "Content unavailable" untuk video yang sebenarnya ada.
func tiktokEndpoints(tiktokURL, apiKey string) []tiktokEndpoint {
	escaped := url.QueryEscape(tiktokURL)
	return []tiktokEndpoint{
		{"tiktok", fmt.Sprintf("https://api.neoxr.eu/api/tiktok?url=%s&apikey=%s", escaped, apiKey), true},
		{"aio", fmt.Sprintf("https://api.neoxr.eu/api/aio?url=%s&apikey=%s", escaped, apiKey), true},
		{"snaptik-v2", fmt.Sprintf("https://api.neoxr.eu/api/snaptik-v2?url=%s&apikey=%s", escaped, apiKey), false},
		{"snaptik", fmt.Sprintf("https://api.neoxr.eu/api/snaptik?url=%s&apikey=%s", escaped, apiKey), false},
	}
}

// --- Instagram API response types ---

// InstagramResponse merepresentasikan respons dari API ig.
type InstagramResponse struct {
	Creator string           `json:"creator"`
	Status  bool             `json:"status"`
	Message string           `json:"msg"`
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
	Message string           `json:"msg"`
	Data    []InstagramMedia `json:"data"`
}

// --- Play (YouTube search) API response types ---

// PlayResponse merepresentasikan respons dari API play.
type PlayResponse struct {
	Creator         string   `json:"creator"`
	Status          bool     `json:"status"`
	Message         string   `json:"msg"`
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
	Message string        `json:"msg"`
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

const (
	maxRetries = 3
	// apiUserAgent dipakai saat memanggil API. User-Agent default Go
	// ("Go-http-client/1.1") sering ditolak Cloudflare dan dibalas halaman HTML.
	apiUserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36"
	// maxSnippetLength membatasi panjang body yang ditulis ke log.
	maxSnippetLength = 200
)

// callAPIWithRetry memanggil API dengan auto-retry (backoff 2s, 4s, 8s).
// Retry dilakukan untuk gangguan yang bersifat sementara: status 5xx,
// 429 (rate limit), dan respons yang bukan JSON (mis. halaman Cloudflare).
func callAPIWithRetry(apiURL, platform string) ([]byte, error) {
	var lastErr error
	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			delay := time.Duration(1<<attempt) * time.Second // 2s, 4s, 8s
			log.Printf("[%s] Retry %d/%d — menunggu %v...", platform, attempt, maxRetries-1, delay)
			time.Sleep(delay)
		}

		body, retryable, err := callAPIOnce(apiURL, platform)
		if err == nil {
			return body, nil
		}

		log.Printf("[%s] %v (retryable=%t)", platform, err, retryable)
		lastErr = err
		if !retryable {
			return nil, err
		}
	}
	return nil, lastErr
}

// callAPIOnce menjalankan satu request ke API dan memastikan responsnya JSON.
// retryable menandai error yang masih mungkin berhasil bila diulang.
func callAPIOnce(apiURL, platform string) (body []byte, retryable bool, err error) {
	req, err := http.NewRequest(http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, false, fmt.Errorf("gagal menyiapkan request ke API %s", platform)
	}
	req.Header.Set("User-Agent", apiUserAgent)
	req.Header.Set("Accept", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, true, fmt.Errorf("gagal menghubungi server API — %s", networkErrorText(err))
	}
	defer resp.Body.Close()

	body, err = io.ReadAll(resp.Body)
	if err != nil {
		return nil, true, fmt.Errorf("gagal membaca respons API — koneksi terputus")
	}

	switch {
	case resp.StatusCode >= 500:
		return nil, true, fmt.Errorf("server API sedang bermasalah (HTTP %d) — coba lagi nanti", resp.StatusCode)
	case resp.StatusCode == http.StatusTooManyRequests:
		return nil, true, fmt.Errorf("server API sedang membatasi permintaan — coba lagi nanti")
	case resp.StatusCode == http.StatusUnauthorized, resp.StatusCode == http.StatusForbidden:
		return nil, false, &fatalAPIError{
			fmt.Sprintf("server API menolak permintaan (HTTP %d) — periksa API key", resp.StatusCode),
		}
	}

	// Cloudflare membalas halaman HTML (status 200) saat situs diblokir atau
	// sedang gangguan. Body seperti ini tidak mungkin diparse sebagai JSON.
	if !isJSONBody(body) {
		log.Printf("[%s] respons bukan JSON — body: %s", platform, snippet(body))
		return nil, true, fmt.Errorf("server API mengirim halaman web, bukan data — coba lagi nanti")
	}

	return body, false, nil
}

// isJSONBody memeriksa apakah body respons berupa objek/array JSON, bukan HTML.
func isJSONBody(body []byte) bool {
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 {
		return false
	}
	return trimmed[0] == '{' || trimmed[0] == '['
}

// decodeAPIResponse mem-parse body JSON ke dst. Detail teknis hanya masuk log;
// pengguna menerima pesan yang bisa dimengerti.
func decodeAPIResponse(body []byte, platform string, dst any) error {
	if err := json.Unmarshal(body, dst); err != nil {
		log.Printf("[%s] respons API tidak bisa diparse: %v — body: %s", platform, err, snippet(body))
		if !isJSONBody(body) {
			return fmt.Errorf("server API %s mengirim halaman web, bukan data — coba lagi nanti", platform)
		}
		return fmt.Errorf("respons API %s tidak bisa dibaca — coba lagi nanti", platform)
	}
	return nil
}

// snippet memotong body agar log tetap ringkas.
func snippet(body []byte) string {
	s := strings.TrimSpace(string(body))
	if len(s) > maxSnippetLength {
		return s[:maxSnippetLength] + "…"
	}
	return s
}

// networkErrorText meringkas error jaringan menjadi teks yang enak dibaca.
func networkErrorText(err error) string {
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return "koneksi ke server timeout"
	}

	msg := err.Error()
	switch {
	case strings.Contains(msg, "no such host"):
		return "server API tidak bisa dihubungi (DNS gagal)"
	case strings.Contains(msg, "connection refused"):
		return "server API menolak koneksi"
	case strings.Contains(msg, "connection reset"), strings.Contains(msg, "EOF"):
		return "koneksi ke server terputus"
	}
	return "tidak bisa terhubung ke server API"
}

// apiAuthMessage menerjemahkan pesan masalah API key/kuota. ok bernilai false
// kalau pesan bukan soal autentikasi.
func apiAuthMessage(msg string) (text string, ok bool) {
	lower := strings.ToLower(strings.TrimSpace(msg))
	switch {
	case strings.Contains(lower, "apikey"), strings.Contains(lower, "api key"):
		return "API key tidak valid atau sudah kedaluwarsa — hubungi admin bot.", true
	case strings.Contains(lower, "limit"):
		return "Kuota API sudah habis — coba lagi nanti.", true
	}
	return "", false
}

// apiMessageToUser menerjemahkan pesan error API neoxr (Inggris) menjadi pesan
// yang jelas untuk pengguna. Pesan yang tidak dikenali dikembalikan apa adanya.
func apiMessageToUser(msg string) string {
	text := strings.TrimSpace(msg)
	if text == "" {
		return ""
	}
	if auth, ok := apiAuthMessage(text); ok {
		return auth
	}

	lower := strings.ToLower(text)
	switch {
	case strings.Contains(lower, "not available"),
		strings.Contains(lower, "unavailable"),
		strings.Contains(lower, "private"),
		strings.Contains(lower, "deleted"),
		strings.Contains(lower, "not found"):
		return "Konten tidak tersedia — pastikan link valid, akun publik, dan postingan belum dihapus."
	}

	return text
}

// fatalAPIError menandai error yang tidak akan berubah walau endpoint lain
// atau percobaan ulang dilakukan (mis. API key salah, kuota habis).
type fatalAPIError struct{ msg string }

func (e *fatalAPIError) Error() string { return e.msg }

// isFatalAPIError melaporkan apakah error termasuk fatal.
func isFatalAPIError(err error) bool {
	var fatal *fatalAPIError
	return errors.As(err, &fatal)
}

// apiStatusError membentuk error dari respons API berstatus gagal.
func apiStatusError(msg, fallback string) error {
	if auth, ok := apiAuthMessage(msg); ok {
		return &fatalAPIError{auth}
	}
	if friendly := apiMessageToUser(msg); friendly != "" {
		return errors.New(friendly)
	}
	return errors.New(fallback)
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
	if err := decodeAPIResponse(body, "Instagram", &apiResp); err != nil {
		return nil, err
	}

	if !apiResp.Status {
		return nil, apiStatusError(apiResp.Message,
			"API Instagram menolak permintaan — pastikan link valid dan akun publik.")
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
	if err := decodeAPIResponse(body, "Terabox", &apiResp); err != nil {
		return nil, err
	}

	if !apiResp.Status {
		return nil, apiStatusError(apiResp.Message,
			"API Terabox menolak permintaan — pastikan link valid dan bisa diakses publik.")
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
	if err := decodeAPIResponse(body, "Play", &apiResp); err != nil {
		return nil, err
	}

	if !apiResp.Status {
		return nil, apiStatusError(apiResp.Message, "lagu tidak ditemukan — coba kata kunci lain")
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

// isQualityMessage mendeteksi pesan API berbentuk "Quality <x> not available".
func isQualityMessage(msg string) bool {
	m := strings.ToLower(strings.TrimSpace(msg))
	return strings.HasPrefix(m, "quality ") && strings.Contains(m, "not available")
}

// isQualityUnavailable memeriksa apakah error berasal dari pesan API
// "Quality <x> not available" — bukan error lain seperti link tidak valid.
func isQualityUnavailable(err error) bool {
	return err != nil && isQualityMessage(err.Error())
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
	if err := decodeAPIResponse(body, "YouTube", &apiResp); err != nil {
		return nil, err
	}

	if !apiResp.Status {
		// Pesan kualitas dibiarkan apa adanya karena dipakai untuk menentukan
		// fallback kualitas di DownloadYouTube.
		if isQualityMessage(apiResp.Message) {
			return nil, errors.New(apiResp.Message)
		}
		return nil, apiStatusError(apiResp.Message,
			"API YouTube menolak permintaan — pastikan link valid dan video publik.")
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

	return nil, fmt.Errorf("kualitas %s tidak tersedia untuk konten ini — coba kualitas lain", quality)
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
	if err := decodeAPIResponse(body, "Google Drive", &apiResp); err != nil {
		return nil, "", "", err
	}

	if !apiResp.Status {
		return nil, "", "", apiStatusError(apiResp.Message,
			"API Google Drive menolak permintaan — pastikan link valid dan file bisa diakses publik.")
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
	if err := decodeAPIResponse(body, endpoint, &apiResp); err != nil {
		return "", "", err
	}

	if !apiResp.Status {
		return "", "", apiStatusError(apiResp.Message,
			fmt.Sprintf("API %s menolak permintaan — coba lagi nanti.", endpoint))
	}
	if apiResp.Data.URL == "" {
		return "", "", fmt.Errorf("API %s tidak mengembalikan media — coba lagi", endpoint)
	}

	return apiResp.Data.URL, apiResp.Data.Mime, nil
}
