package main

import (
	"encoding/json"
	"fmt"
	"net/url"
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
