package main

import (
	"regexp"
	"strings"
)

// --- Pola regex untuk deteksi URL ---

var (
	// TikTok: tautan standar, pendek (vt/vm), dan slideshow (photo).
	tiktokRegex = regexp.MustCompile(
		`https?://(?:www\.)?(?:tiktok\.com/@[\w.-]+/(?:video|photo)/\d+|vt\.tiktok\.com/[\w]+/?|vm\.tiktok\.com/[\w]+/?)`,
	)

	// Instagram: post, reel, tv, stories.
	instagramRegex = regexp.MustCompile(
		`https?://(?:www\.)?instagram\.com/(?:p|reel|tv|stories)/[\w-]+/?`,
	)

	// Terabox: share link (/s/xxx) dan domain mirror (teraboxapp, mirrobox, dsb).
	teraboxRegex = regexp.MustCompile(
		`https?://(?:www\.)?(?:terabox\.(?:com|app)|mirrobox\.com|nephobox\.com|freemidata\.com|1024tera\.com|4funbox\.(?:com|s\.top)|teraboxapp\.com|momerybox\.com|tibibox\.com|uymyo\.com)/s/[\w-]+/?`,
	)

	// Threads: post (@user/ID), share link, dan short link.
	threadsRegex = regexp.MustCompile(
		`https?://(?:www\.)?threads\.(?:com|net)/(?:@[\w.-]+/post/[\w-]+|share/[\w-]+/?|@[\w.-]+/[\w-]+/?)`,
	)

	// YouTube: watch, short link (youtu.be), shorts, live, dan embed.
	youtubeRegex = regexp.MustCompile(
		`https?://(?:www\.|m\.|music\.)?(?:youtube\.com/(?:watch\?v=[\w-]+|shorts/[\w-]+|live/[\w-]+|embed/[\w-]+)|youtu\.be/[\w-]+)`,
	)

	// Spotify: track dan playlist (termasuk path locale intl-xx).
	spotifyRegex = regexp.MustCompile(
		`https?://open\.spotify\.com/(?:intl-[\w-]+/)?(?:track|playlist)/[\w]+`,
	)

	// Google Drive: file (/file/d/ID), open?id=, uc?id=, dan folder (/drive/folders/ID).
	gdriveRegex = regexp.MustCompile(
		`https?://drive\.google\.com/(?:file/d/[\w-]+|open\?id=[\w-]+|uc\?(?:export=download&)?id=[\w-]+|drive/folders/[\w-]+)`,
	)
)

// detectURL memeriksa teks dan mengembalikan platform serta URL pertama yang ditemukan.
func detectURL(text string) (platform, rawURL string) {
	if m := tiktokRegex.FindString(text); m != "" {
		return "tiktok", strings.TrimRight(m, "/")
	}
	if m := instagramRegex.FindString(text); m != "" {
		return "instagram", strings.TrimRight(m, "/")
	}
	if m := teraboxRegex.FindString(text); m != "" {
		return "terabox", strings.TrimRight(m, "/")
	}
	if m := threadsRegex.FindString(text); m != "" {
		return "threads", strings.TrimRight(m, "/")
	}
	if m := spotifyRegex.FindString(text); m != "" {
		return "spotify", strings.TrimRight(m, "/")
	}
	if m := youtubeRegex.FindString(text); m != "" {
		return "youtube", strings.TrimRight(m, "/")
	}
	if m := gdriveRegex.FindString(text); m != "" {
		return "gdrive", strings.TrimRight(m, "/")
	}
	return "", ""
}
