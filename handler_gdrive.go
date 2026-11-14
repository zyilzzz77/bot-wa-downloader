package main

import (
	"context"
	"fmt"
	"log"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/types"
)

// --- Google Drive ---

// handleGDrive mengunduh file atau isi folder Google Drive lalu mengirimnya
// sebagai dokumen (nama file asli dipertahankan).
func handleGDrive(client *ClientWrapper, chatJID types.JID, gdriveURL string) {
	log.Printf("[gdrive] Menerima link %s", gdriveURL)
	ctx := context.Background()

	processingMsg := client.SendText(ctx, chatJID, "⏳ *Memproses Google Drive…*")

	listing, err := FetchGDrive(gdriveURL, apiKey)
	if err != nil {
		log.Printf("[gdrive] ❌ Gagal: %v", err)
		editOrSend(client, ctx, chatJID, processingMsg,
			fmt.Sprintf("❌ Gagal download *Google Drive*\n\n%s", err.Error()))
		return
	}

	if listing.IsFolder {
		handleGDriveFolder(client, ctx, chatJID, processingMsg, listing)
		return
	}

	handleGDriveItem(client, ctx, chatJID, processingMsg, listing.Items[0])
}

// handleGDriveFolder mengunduh dan mengirim semua file dalam satu folder
// secara berurutan, sambil memperbarui pesan status.
func handleGDriveFolder(client *ClientWrapper, ctx context.Context, chatJID types.JID, processingMsg *whatsmeow.SendResponse, listing *GDriveListing) {
	total := len(listing.Items)
	name := listing.Name
	if name == "" {
		name = "folder"
	}

	header := fmt.Sprintf("📁 *Folder Google Drive*\n\n📂 *Nama:* %s\n📄 *Jumlah file:* %d", name, total)
	if listing.FolderCount > 0 {
		header += fmt.Sprintf("\n📁 *Sub-folder dilewati:* %d", listing.FolderCount)
	}

	editOrSend(client, ctx, chatJID, processingMsg, header+"\n\n📥 _Mengirim file…_")

	sent := 0
	for i, item := range listing.Items {
		fileName := item.Name
		if fileName == "" {
			fileName = fmt.Sprintf("file-%d", i+1)
		}

		editOrSend(client, ctx, chatJID, processingMsg,
			fmt.Sprintf("%s\n\n📥 _Mengirim %d/%d — %s…_", header, i+1, total, fileName))

		data, realName, mimeType, err := DownloadGDriveFile(item)
		if err != nil {
			log.Printf("[gdrive] ❌ Gagal %s: %v", fileName, err)
			editOrSend(client, ctx, chatJID, processingMsg,
				fmt.Sprintf("%s\n\n❌ Gagal mengunduh %s\n%s", header, fileName, err.Error()))
			continue
		}

		sizeMB := float64(len(data)) / (1024 * 1024)
		log.Printf("[gdrive] ✓ %s — %.1fMB (%s)", realName, sizeMB, mimeType)

		uploadAndSendDocument(client, ctx, chatJID, data, mimeType, realName, fmt.Sprintf("📄 %s", realName))
		sent++
	}

	editOrSend(client, ctx, chatJID, processingMsg,
		fmt.Sprintf("%s\n\n✅ _Selesai — %d/%d file terkirim._", header, sent, total))
	log.Printf("[gdrive] ✓ Folder selesai: %d/%d file terkirim", sent, total)
}

// handleGDriveItem mengunduh dan mengirim satu file Google Drive.
func handleGDriveItem(client *ClientWrapper, ctx context.Context, chatJID types.JID, processingMsg *whatsmeow.SendResponse, item GDriveItem) {
	data, fileName, mimeType, err := DownloadGDriveFile(item)
	if err != nil {
		log.Printf("[gdrive] ❌ Gagal: %v", err)
		editOrSend(client, ctx, chatJID, processingMsg,
			fmt.Sprintf("❌ Gagal download *Google Drive*\n\n%s", err.Error()))
		return
	}

	sizeMB := float64(len(data)) / (1024 * 1024)
	log.Printf("[gdrive] ✓ %s — %.1fMB (%s)", fileName, sizeMB, mimeType)

	summary := fmt.Sprintf("✅ *File Google Drive*\n\n📄 *Nama:* %s\n💾 *Ukuran:* %.1f MB\n🧩 *Tipe:* %s\n\n📥 _Mengirim file…_",
		fileName, sizeMB, mimeType)
	editOrSend(client, ctx, chatJID, processingMsg, summary)

	uploadAndSendDocument(client, ctx, chatJID, data, mimeType, fileName, fmt.Sprintf("📄 %s", fileName))
	log.Printf("[gdrive] ✓ Terkirim!")
}
