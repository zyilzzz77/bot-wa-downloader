package main

import (
	"fmt"
	"strings"
)

// download memanggil API yang sesuai berdasarkan platform.
func download(platform, rawURL string) (*DownloadResult, error) {
	switch platform {
	case "tiktok":
		return DownloadTikTok(rawURL, apiKey)
	case "instagram":
		return DownloadInstagram(rawURL, apiKey)
	case "terabox":
		return DownloadTerabox(rawURL, apiKey)
	case "threads":
		return DownloadThreads(rawURL, apiKey)
	default:
		return nil, fmt.Errorf("platform tidak dikenal: %s", platform)
	}
}

// buildSummary membuat teks ringkasan hasil download.
func buildSummary(r *DownloadResult) string {
	var sb strings.Builder
	sb.WriteString("✅ *Download Berhasil!*\n\n")
	fmt.Fprintf(&sb, "📱 *Platform:* %s\n", titleCase(r.Platform))
	if len(r.Videos) > 0 {
		fmt.Fprintf(&sb, "🎬 *Video:* %d file\n", len(r.Videos))
	}
	if len(r.Images) > 0 {
		fmt.Fprintf(&sb, "🖼  *Gambar:* %d file\n", len(r.Images))
	}
	if len(r.Audio) > 0 {
		fmt.Fprintf(&sb, "🎵 *Audio:* %d file\n", len(r.Audio))
	}
	if len(r.Documents) > 0 {
		fmt.Fprintf(&sb, "📄 *Dokumen:* %d file\n", len(r.Documents))
		for _, doc := range r.Documents {
			fmt.Fprintf(&sb, "• %s\n", doc.FileName)
		}
	}
	sb.WriteString("\n📥 _Mengirim media…_")
	return sb.String()
}

// titleCase mengubah huruf pertama string menjadi kapital.
func titleCase(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}
