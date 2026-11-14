package main

import (
	"errors"
	"fmt"
	"log"
	"net/url"
	"strings"
)

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
