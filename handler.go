package main

import (
	"context"
	"fmt"
	"log"

	"go.mau.fi/whatsmeow/types/events"
)

// --- Penanganan pesan masuk ---

// handleMessage dipanggil setiap kali ada pesan masuk.
func handleMessage(client *ClientWrapper, evt *events.Message) {
	// Abaikan pesan dari diri sendiri
	if evt.Info.IsFromMe {
		return
	}

	// Ambil teks dari percakapan atau extended text
	text := evt.Message.GetConversation()
	if text == "" {
		if ext := evt.Message.GetExtendedTextMessage(); ext != nil {
			text = ext.GetText()
		}
	}
	if text == "" {
		return
	}

	if query, ok := parsePlayCommand(text); ok {
		handlePlayCommand(client, evt.Info.Chat, query)
		return
	}

	if cmd, ok := parseYouTubeCommand(text); ok {
		handleYouTube(client, evt.Info.Chat, cmd.url, cmd.mediaType, cmd.quality)
		return
	}

	if content, animated, ok := parseBratCommand(text); ok {
		if content == "" {
			client.SendText(context.Background(), evt.Info.Chat,
				"Kirim teksnya ya. Contoh:\n*brat* halo dunia\n*bratvid* halo dunia")
			return
		}
		handleBrat(client, evt.Info.Chat, content, animated)
		return
	}

	if amount, isCommand, valid := parseQrisCommand(text); isCommand {
		if !valid {
			client.SendText(context.Background(), evt.Info.Chat,
				"Kirim nominalnya ya. Contoh:\n*qris 10000*\n*qris 10k* (Rp10.000)")
			return
		}
		handleQRIS(client, evt.Info.Chat, amount)
		return
	}

	if parseServerCommand(text) {
		handleServer(client, evt.Info.Chat)
		return
	}

	if parseBenchCommand(text) {
		handleBench(client, evt.Info.Chat)
		return
	}

	if cmd, ok := parseSholatCommand(text); ok {
		handleSholatCommand(client, evt.Info.Chat, cmd)
		return
	}

	platform, detectedURL := detectURL(text)
	if detectedURL == "" {
		return
	}

	// YouTube & Google Drive punya alur sendiri, bukan media per-URL.
	switch platform {
	case "youtube":
		// Link YouTube tanpa command → unduh video kualitas default.
		handleYouTube(client, evt.Info.Chat, detectedURL, youTubeTypeVideo, youTubeDefaultVideo)
		return
	case "gdrive":
		handleGDrive(client, evt.Info.Chat, detectedURL)
		return
	case "spotify":
		handleSpotify(client, evt.Info.Chat, detectedURL)
		return
	}

	chatJID := evt.Info.Chat
	log.Printf("[%s] Menerima link %s dari %s", platform, detectedURL, chatJID)

	ctx := context.Background()

	// Kirim pesan "sedang diproses"
	log.Printf("[%s] Mengirim pesan processing...", platform)
	processingMsg := client.SendText(ctx, chatJID, "⏳ *Sedang memproses…*")

	// Download konten
	log.Printf("[%s] Mendownload dari API...", platform)
	result, err := download(platform, detectedURL)
	if err != nil {
		log.Printf("[%s] ❌ Gagal download: %v", platform, err)
		errText := fmt.Sprintf("❌ Gagal download *%s*\n\n%s", platform, err.Error())
		if processingMsg != nil {
			client.EditText(ctx, chatJID, processingMsg.ID, errText)
		} else {
			client.SendText(ctx, chatJID, errText)
		}
		return
	}

	log.Printf("[%s] ✓ Download sukses — %d video, %d gambar, %d audio, %d dokumen",
		platform, len(result.Videos), len(result.Images), len(result.Audio), len(result.Documents))

	// Edit pesan pemrosesan dengan ringkasan hasil
	summary := buildSummary(result)
	if processingMsg != nil {
		client.EditText(ctx, chatJID, processingMsg.ID, summary)
	} else {
		client.SendText(ctx, chatJID, summary)
	}

	// Kirim semua media
	log.Printf("[%s] Mengirim media...", platform)
	sendAllMedia(client, ctx, chatJID, result)
	log.Printf("[%s] ✓ Semua media terkirim!", platform)
}
