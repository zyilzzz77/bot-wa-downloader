package main

import (
	"context"
	"fmt"
	"log"
	"strings"

	"go.mau.fi/whatsmeow/types"
)

// --- Command /brat & /bratvid (sticker generator) ---

// parseBratCommand memeriksa apakah teks adalah command brat / bratvid.
// Menerima bentuk dengan atau tanpa slash (mis. "/brat halo" atau "brat halo").
// Mengembalikan konten teks, apakah animasi (bratvid), dan apakah ini command.
func parseBratCommand(text string) (content string, animated bool, ok bool) {
	trimmed := strings.TrimSpace(text)
	lower := strings.ToLower(trimmed)

	cases := []struct {
		prefix   string
		animated bool
	}{
		{"/bratvid", true}, {"bratvid", true}, {"!bratvid", true},
		{"/brat", false}, {"brat", false}, {"!brat", false},
	}

	for _, c := range cases {
		if lower == c.prefix {
			return "", c.animated, true
		}
		if strings.HasPrefix(lower, c.prefix+" ") {
			return strings.TrimSpace(trimmed[len(c.prefix)+1:]), c.animated, true
		}
	}
	return "", false, false
}

// handleBrat membuat sticker brat (gambar) atau bratvid (animasi) lalu mengirimnya.
func handleBrat(client *ClientWrapper, chatJID types.JID, content string, animated bool) {
	label := "Brat"
	if animated {
		label = "Brat Video"
	}
	log.Printf("[brat] Membuat %s untuk teks %q", label, content)
	ctx := context.Background()

	processingMsg := client.SendText(ctx, chatJID, fmt.Sprintf("⏳ *Membuat %s…*", label))

	mediaURL, mime, err := GenerateBrat(content, apiKey, animated)
	if err != nil {
		log.Printf("[brat] ❌ Gagal generate: %v", err)
		errText := fmt.Sprintf("❌ Gagal membuat *%s*\n\n%s", label, err.Error())
		if processingMsg != nil {
			client.EditText(ctx, chatJID, processingMsg.ID, errText)
		} else {
			client.SendText(ctx, chatJID, errText)
		}
		return
	}

	data, _, err := downloadFileWithProgress(client, ctx, chatJID, mediaURL, label)
	if err != nil {
		log.Printf("[brat] ❌ Gagal download media: %v", err)
		client.SendText(ctx, chatJID, fmt.Sprintf("❌ Gagal mengunduh media: %v", err))
		return
	}
	log.Printf("[brat] Media %s terunduh (%dKB), konversi ke sticker...", mime, len(data)/1024)

	webp, err := convertToStickerWebP(data, animated)
	if err != nil {
		log.Printf("[brat] ❌ Gagal konversi sticker: %v", err)
		client.SendText(ctx, chatJID, fmt.Sprintf("❌ Gagal membuat sticker: %v", err))
		return
	}
	log.Printf("[brat] Sticker jadi — %dKB, upload ke WA...", len(webp)/1024)

	if processingMsg != nil {
		client.EditText(ctx, chatJID, processingMsg.ID, fmt.Sprintf("✅ *%s* sudah jadi!", label))
	}
	uploadAndSendSticker(client, ctx, chatJID, webp, animated)
	log.Printf("[brat] ✓ Sticker terkirim!")
}
