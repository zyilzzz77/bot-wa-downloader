package main

import (
	"context"
	"fmt"
	"log"
	"strings"

	"go.mau.fi/whatsmeow/types"
)

// --- Command /yt & /ytmp3 (unduh video / audio YouTube) ---

// youTubeCommand adalah hasil parsing command unduh YouTube.
type youTubeCommand struct {
	url       string
	mediaType string
	quality   string
}

// parseYouTubeCommand memeriksa apakah teks adalah command unduh YouTube.
// Format: "/yt <url> [kualitas]" untuk video, "/ytmp3 <url> [bitrate]" untuk audio.
func parseYouTubeCommand(text string) (youTubeCommand, bool) {
	fields := strings.Fields(strings.TrimSpace(text))
	if len(fields) < 2 {
		return youTubeCommand{}, false
	}

	var mediaType, defaultQuality string
	switch strings.ToLower(fields[0]) {
	case "/yt", "yt", "!yt":
		mediaType, defaultQuality = youTubeTypeVideo, youTubeDefaultVideo
	case "/ytmp3", "ytmp3", "!ytmp3":
		mediaType, defaultQuality = youTubeTypeAudio, youTubeDefaultAudio
	default:
		return youTubeCommand{}, false
	}

	rawURL := fields[1]
	if !youtubeRegex.MatchString(rawURL) {
		return youTubeCommand{}, false
	}

	quality := defaultQuality
	if len(fields) > 2 {
		quality = fields[2]
	}

	return youTubeCommand{url: rawURL, mediaType: mediaType, quality: quality}, true
}

// handleYouTube mengunduh video atau audio YouTube lalu mengirimnya ke chat.
func handleYouTube(client *ClientWrapper, chatJID types.JID, youtubeURL, mediaType, quality string) {
	log.Printf("[youtube] Menerima permintaan %s %s (kualitas %s)", mediaType, youtubeURL, quality)
	ctx := context.Background()

	processingMsg := client.SendText(ctx, chatJID, fmt.Sprintf("⏳ *Memproses YouTube…*\n\n🎚 _%s • %s_", mediaType, quality))

	video, err := DownloadYouTube(youtubeURL, mediaType, quality, apiKey)
	if err != nil {
		log.Printf("[youtube] ❌ Gagal: %v", err)
		errText := fmt.Sprintf("❌ Gagal download *YouTube*\n\n%s", err.Error())
		if processingMsg != nil {
			client.EditText(ctx, chatJID, processingMsg.ID, errText)
		} else {
			client.SendText(ctx, chatJID, errText)
		}
		return
	}

	log.Printf("[youtube] ✓ Ketemu: %s (%s, %s)", video.Title, video.Data.Quality, video.Data.Size)

	summary := fmt.Sprintf("🎬 *%s*\n\n👤 *Channel:* %s\n⏱ *Durasi:* %s\n👁 *Views:* %s\n🎚 *Kualitas:* %s (%s)",
		video.Title, video.Channel, video.DurationText(), video.Views, video.Data.Quality, video.Data.Size)
	if video.Data.Quality != quality {
		summary += fmt.Sprintf("\n⚠️ _%s tidak tersedia, pakai %s_", quality, video.Data.Quality)
	}
	summary += fmt.Sprintf("\n\n📥 _Mengirim %s…_", mediaType)

	if processingMsg != nil {
		client.EditText(ctx, chatJID, processingMsg.ID, summary)
	} else {
		client.SendText(ctx, chatJID, summary)
	}

	if video.Thumbnail != "" {
		sendImage(client, ctx, chatJID, video.Thumbnail, fmt.Sprintf("🎬 %s", video.Title))
	}

	data, mimeType, err := downloadFileWithProgress(client, ctx, chatJID, video.Data.URL, mediaType)
	if err != nil {
		log.Printf("[youtube] ❌ Gagal download file: %v", err)
		client.SendText(ctx, chatJID, fmt.Sprintf("❌ Gagal mengunduh file: %v", err))
		return
	}
	log.Printf("[youtube] Download sukses — %.1fMB, upload ke WA...", float64(len(data))/(1024*1024))

	if mediaType == youTubeTypeAudio {
		uploadAndSendAudio(client, ctx, chatJID, data, "audio/mpeg")
	} else {
		uploadAndSendVideo(client, ctx, chatJID, data, mimeType, fmt.Sprintf("🎬 %s", video.Title))
	}
	log.Printf("[youtube] ✓ Terkirim!")
}
