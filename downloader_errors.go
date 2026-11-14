package main

import (
	"errors"
	"net"
	"strings"
)

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
