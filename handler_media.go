package main

import (
	"context"
	"fmt"
	"log"

	"go.mau.fi/whatsmeow/types"
)

// --- Pengiriman media ---

// sendAllMedia mengirim semua media dalam DownloadResult.
func sendAllMedia(client *ClientWrapper, ctx context.Context, chatJID types.JID, r *DownloadResult) {
	// Kirim video
	for i, videoURL := range r.Videos {
		caption := fmt.Sprintf("🎬 Video %d/%d — %s", i+1, len(r.Videos), r.Platform)
		sendVideo(client, ctx, chatJID, videoURL, caption)
	}

	// Kirim gambar
	for i, imgURL := range r.Images {
		caption := fmt.Sprintf("🖼 Gambar %d/%d — %s", i+1, len(r.Images), r.Platform)
		sendImage(client, ctx, chatJID, imgURL, caption)
	}

	// Kirim audio
	for _, audioURL := range r.Audio {
		sendAudio(client, ctx, chatJID, audioURL)
	}

	// Kirim dokumen (file generik terabox: zip, pdf, dsb)
	for i, doc := range r.Documents {
		caption := fmt.Sprintf("📄 Dokumen %d/%d — %s", i+1, len(r.Documents), r.Platform)
		sendDocument(client, ctx, chatJID, doc.URL, doc.FileName, caption)
	}
}

// --- Pengiriman per jenis media ---

// sendVideo mengirim video ke chat.
func sendVideo(client *ClientWrapper, ctx context.Context, chatJID types.JID, videoURL, caption string) {
	log.Printf("[video] Download %s", videoURL[:min(60, len(videoURL))]+"...")
	data, mimeType, err := downloadFileWithProgress(client, ctx, chatJID, videoURL, "video")
	if err != nil {
		log.Printf("[video] ❌ Gagal download: %v", err)
		client.SendText(ctx, chatJID, fmt.Sprintf("❌ Gagal mengunduh video: %v", err))
		return
	}
	log.Printf("[video] Download sukses — %.1fMB, upload ke WA...", float64(len(data))/(1024*1024))
	uploadAndSendVideo(client, ctx, chatJID, data, mimeType, caption)
}

// sendImage mengirim gambar ke chat.
func sendImage(client *ClientWrapper, ctx context.Context, chatJID types.JID, imgURL, caption string) {
	log.Printf("[image] Download...")
	data, mimeType, err := downloadFileWithProgress(client, ctx, chatJID, imgURL, "gambar")
	if err != nil {
		log.Printf("[image] ❌ Gagal download: %v", err)
		client.SendText(ctx, chatJID, fmt.Sprintf("❌ Gagal mengunduh gambar: %v", err))
		return
	}
	log.Printf("[image] Download sukses — %dKB, upload ke WA...", len(data)/1024)
	uploadAndSendImage(client, ctx, chatJID, data, mimeType, caption)
}

// sendAudio mengirim audio ke chat.
func sendAudio(client *ClientWrapper, ctx context.Context, chatJID types.JID, audioURL string) {
	log.Printf("[audio] Download...")
	data, mimeType, err := downloadFileWithProgress(client, ctx, chatJID, audioURL, "audio")
	if err != nil {
		log.Printf("[audio] ❌ Gagal download: %v", err)
		client.SendText(ctx, chatJID, fmt.Sprintf("❌ Gagal mengunduh audio: %v", err))
		return
	}
	log.Printf("[audio] Download sukses — %dKB, upload ke WA...", len(data)/1024)
	uploadAndSendAudio(client, ctx, chatJID, data, mimeType)
}

// sendDocument mengirim file generik (zip, pdf, dsb) sebagai dokumen WA.
func sendDocument(client *ClientWrapper, ctx context.Context, chatJID types.JID, docURL, fileName, caption string) {
	log.Printf("[document] Download %s...", fileName)
	data, mimeType, err := downloadFileWithProgress(client, ctx, chatJID, docURL, fileName)
	if err != nil {
		log.Printf("[document] ❌ Gagal download: %v", err)
		client.SendText(ctx, chatJID, fmt.Sprintf("❌ Gagal mengunduh %s: %v", fileName, err))
		return
	}
	log.Printf("[document] Download sukses — %.1fMB, upload ke WA...", float64(len(data))/(1024*1024))
	uploadAndSendDocument(client, ctx, chatJID, data, mimeType, fileName, caption)
}
