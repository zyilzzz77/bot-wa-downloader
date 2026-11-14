package main

import (
	"context"
	"fmt"
	"log"
	"strings"

	"go.mau.fi/whatsmeow/types"
)

// --- Command /play (cari lagu YouTube) ---

// parsePlayCommand memeriksa apakah teks adalah command /play.
// Menerima "play <judul>" atau "/play <judul>", mengembalikan query-nya.
func parsePlayCommand(text string) (string, bool) {
	trimmed := strings.TrimSpace(text)
	lower := strings.ToLower(trimmed)
	var query string
	switch {
	case strings.HasPrefix(lower, "/play "):
		query = strings.TrimSpace(trimmed[len("/play "):])
	case strings.HasPrefix(lower, "play "):
		query = strings.TrimSpace(trimmed[len("play "):])
	default:
		return "", false
	}
	if query == "" {
		return "", false
	}
	return query, true
}

// handlePlayCommand mencari lagu via API play lalu mengirim audio + thumbnail.
func handlePlayCommand(client *ClientWrapper, chatJID types.JID, query string) {
	log.Printf("[play] Mencari lagu %q", query)
	ctx := context.Background()

	processingMsg := client.SendText(ctx, chatJID, fmt.Sprintf("⏳ *Mencari lagu…*\n\n🔍 _%s_", query))

	song, err := SearchPlay(query, apiKey)
	if err != nil {
		log.Printf("[play] ❌ Gagal cari: %v", err)
		errText := fmt.Sprintf("❌ Gagal cari lagu\n\n%s", err.Error())
		if processingMsg != nil {
			client.EditText(ctx, chatJID, processingMsg.ID, errText)
		} else {
			client.SendText(ctx, chatJID, errText)
		}
		return
	}

	log.Printf("[play] ✓ Ketemu: %s", song.Title)

	summary := fmt.Sprintf("🎵 *%s*\n\n👤 *Channel:* %s\n⏱ *Durasi:* %s\n👁 *Views:* %s\n💾 *Size:* %s (%s)\n\n📥 _Mengirim audio…_",
		song.Title, song.Channel, song.Duration, song.Views, song.Data.Size, song.Data.Quality)
	if processingMsg != nil {
		client.EditText(ctx, chatJID, processingMsg.ID, summary)
	} else {
		client.SendText(ctx, chatJID, summary)
	}

	if song.Thumbnail != "" {
		sendImage(client, ctx, chatJID, song.Thumbnail, fmt.Sprintf("🎵 %s", song.Title))
	}

	data, _, err := downloadFileWithProgress(client, ctx, chatJID, song.Data.URL, "audio")
	if err != nil {
		log.Printf("[play] ❌ Gagal download audio: %v", err)
		client.SendText(ctx, chatJID, fmt.Sprintf("❌ Gagal mengunduh audio: %v", err))
		return
	}
	log.Printf("[play] Download audio sukses — %.1fMB, upload ke WA...", float64(len(data))/(1024*1024))
	uploadAndSendAudio(client, ctx, chatJID, data, "audio/mpeg")
	log.Printf("[play] ✓ Audio terkirim!")
}
