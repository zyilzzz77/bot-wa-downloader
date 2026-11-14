package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

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
