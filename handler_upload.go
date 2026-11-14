package main

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	waE2E "go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"google.golang.org/protobuf/proto"
)

// --- Upload helpers (byte-based) ---

// uploadAndSendVideo uploads raw bytes as video and sends to chat.
func uploadAndSendVideo(client *ClientWrapper, ctx context.Context, chatJID types.JID, data []byte, mimeType, caption string) {
	if !strings.HasPrefix(mimeType, "video/") {
		mimeType = "video/mp4"
	}
	uploaded, err := client.Upload(ctx, data, whatsmeowMediaVideo)
	if err != nil {
		log.Printf("[video] ❌ Gagal upload: %v", err)
		client.SendText(ctx, chatJID, fmt.Sprintf("❌ Gagal upload video: %v", err))
		return
	}
	log.Printf("[video] ✓ Terkirim!")
	client.SendMessage(ctx, chatJID, &waE2E.Message{
		VideoMessage: &waE2E.VideoMessage{
			Caption:       proto.String(caption),
			URL:           proto.String(uploaded.URL),
			DirectPath:    proto.String(uploaded.DirectPath),
			MediaKey:      uploaded.MediaKey,
			FileEncSHA256: uploaded.FileEncSHA256,
			FileSHA256:    uploaded.FileSHA256,
			FileLength:    proto.Uint64(uploaded.FileLength),
			Mimetype:      proto.String(mimeType),
		},
	})
}

// uploadAndSendImage uploads raw bytes as image and sends to chat.
func uploadAndSendImage(client *ClientWrapper, ctx context.Context, chatJID types.JID, data []byte, mimeType, caption string) {
	if !strings.HasPrefix(mimeType, "image/") {
		mimeType = "image/jpeg"
	}
	uploaded, err := client.Upload(ctx, data, whatsmeowMediaImage)
	if err != nil {
		log.Printf("[image] ❌ Gagal upload: %v", err)
		client.SendText(ctx, chatJID, fmt.Sprintf("❌ Gagal upload gambar: %v", err))
		return
	}
	log.Printf("[image] ✓ Terkirim!")
	client.SendMessage(ctx, chatJID, &waE2E.Message{
		ImageMessage: &waE2E.ImageMessage{
			Caption:       proto.String(caption),
			URL:           proto.String(uploaded.URL),
			DirectPath:    proto.String(uploaded.DirectPath),
			MediaKey:      uploaded.MediaKey,
			FileEncSHA256: uploaded.FileEncSHA256,
			FileSHA256:    uploaded.FileSHA256,
			FileLength:    proto.Uint64(uploaded.FileLength),
			Mimetype:      proto.String(mimeType),
		},
	})
}

// uploadAndSendSticker uploads raw WebP bytes and sends them as a sticker.
// Sticker WhatsApp memakai media type image pada skema upload.
func uploadAndSendSticker(client *ClientWrapper, ctx context.Context, chatJID types.JID, data []byte, animated bool) {
	uploaded, err := client.Upload(ctx, data, whatsmeowMediaImage)
	if err != nil {
		log.Printf("[sticker] ❌ Gagal upload: %v", err)
		client.SendText(ctx, chatJID, fmt.Sprintf("❌ Gagal upload sticker: %v", err))
		return
	}
	client.SendMessage(ctx, chatJID, &waE2E.Message{
		StickerMessage: &waE2E.StickerMessage{
			URL:           proto.String(uploaded.URL),
			DirectPath:    proto.String(uploaded.DirectPath),
			MediaKey:      uploaded.MediaKey,
			FileEncSHA256: uploaded.FileEncSHA256,
			FileSHA256:    uploaded.FileSHA256,
			FileLength:    proto.Uint64(uploaded.FileLength),
			Mimetype:      proto.String("image/webp"),
			Width:         proto.Uint32(512),
			Height:        proto.Uint32(512),
			IsAnimated:    proto.Bool(animated),
			StickerSentTS: proto.Int64(time.Now().UnixMilli()),
		},
	})
}

// uploadAndSendAudio uploads raw bytes as audio and sends to chat.
func uploadAndSendAudio(client *ClientWrapper, ctx context.Context, chatJID types.JID, data []byte, mimeType string) {
	if !strings.HasPrefix(mimeType, "audio/") {
		mimeType = "audio/mp3"
	}
	uploaded, err := client.Upload(ctx, data, whatsmeowMediaAudio)
	if err != nil {
		log.Printf("[audio] ❌ Gagal upload: %v", err)
		client.SendText(ctx, chatJID, fmt.Sprintf("❌ Gagal upload audio: %v", err))
		return
	}
	log.Printf("[audio] ✓ Terkirim!")
	client.SendMessage(ctx, chatJID, &waE2E.Message{
		AudioMessage: &waE2E.AudioMessage{
			URL:           proto.String(uploaded.URL),
			DirectPath:    proto.String(uploaded.DirectPath),
			MediaKey:      uploaded.MediaKey,
			FileEncSHA256: uploaded.FileEncSHA256,
			FileSHA256:    uploaded.FileSHA256,
			FileLength:    proto.Uint64(uploaded.FileLength),
			Mimetype:      proto.String(mimeType),
		},
	})
}

// uploadAndSendDocument uploads raw bytes as document and sends to chat.
func uploadAndSendDocument(client *ClientWrapper, ctx context.Context, chatJID types.JID, data []byte, mimeType, fileName, caption string) {
	if mimeType == "" || mimeType == "application/octet-stream" {
		mimeType = mimeTypeByExtension(fileName)
	}
	uploaded, err := client.Upload(ctx, data, whatsmeowMediaDocument)
	if err != nil {
		log.Printf("[document] ❌ Gagal upload: %v", err)
		client.SendText(ctx, chatJID, fmt.Sprintf("❌ Gagal upload %s: %v", fileName, err))
		return
	}
	log.Printf("[document] ✓ Terkirim!")
	client.SendMessage(ctx, chatJID, &waE2E.Message{
		DocumentMessage: &waE2E.DocumentMessage{
			Caption:       proto.String(caption),
			URL:           proto.String(uploaded.URL),
			DirectPath:    proto.String(uploaded.DirectPath),
			MediaKey:      uploaded.MediaKey,
			FileEncSHA256: uploaded.FileEncSHA256,
			FileSHA256:    uploaded.FileSHA256,
			FileLength:    proto.Uint64(uploaded.FileLength),
			Mimetype:      proto.String(mimeType),
			FileName:      proto.String(fileName),
		},
	})
}

// mimeTypeByExtension menebak MIME dari ekstensi nama file.
func mimeTypeByExtension(fileName string) string {
	lower := strings.ToLower(fileName)
	switch {
	case strings.HasSuffix(lower, ".zip"):
		return "application/zip"
	case strings.HasSuffix(lower, ".rar"):
		return "application/x-rar-compressed"
	case strings.HasSuffix(lower, ".7z"):
		return "application/x-7z-compressed"
	case strings.HasSuffix(lower, ".pdf"):
		return "application/pdf"
	case strings.HasSuffix(lower, ".mp4"):
		return "video/mp4"
	case strings.HasSuffix(lower, ".mp3"):
		return "audio/mpeg"
	default:
		return "application/octet-stream"
	}
}
