package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"regexp"
	"strings"

	waE2E "go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	"google.golang.org/protobuf/proto"
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
	return "", ""
}

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

	platform, detectedURL := detectURL(text)
	if detectedURL == "" {
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

	data, _, err := downloadFile(song.Data.URL)
	if err != nil {
		log.Printf("[play] ❌ Gagal download audio: %v", err)
		client.SendText(ctx, chatJID, fmt.Sprintf("❌ Gagal mengunduh audio: %v", err))
		return
	}
	log.Printf("[play] Download audio sukses — %.1fMB, upload ke WA...", float64(len(data))/(1024*1024))
	uploadAndSendAudio(client, ctx, chatJID, data, "audio/mpeg")
	log.Printf("[play] ✓ Audio terkirim!")
}

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

// --- Helper download file ---

// downloadFile mengunduh file dari URL dan mengembalikan byte-nya.
func downloadFile(rawURL string) ([]byte, string, error) {
	resp, err := httpClient.Get(rawURL)
	if err != nil {
		return nil, "", fmt.Errorf("gagal mengunduh: %w", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", fmt.Errorf("gagal membaca: %w", err)
	}

	mimeType := resp.Header.Get("Content-Type")
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}

	return data, mimeType, nil
}

// --- Pengiriman per jenis media ---

// sendVideo mengirim video ke chat.
func sendVideo(client *ClientWrapper, ctx context.Context, chatJID types.JID, videoURL, caption string) {
	log.Printf("[video] Download %s", videoURL[:min(60, len(videoURL))]+"...")
	data, mimeType, err := downloadFile(videoURL)
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
	data, mimeType, err := downloadFile(imgURL)
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
	data, mimeType, err := downloadFile(audioURL)
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
	data, mimeType, err := downloadFile(docURL)
	if err != nil {
		log.Printf("[document] ❌ Gagal download: %v", err)
		client.SendText(ctx, chatJID, fmt.Sprintf("❌ Gagal mengunduh %s: %v", fileName, err))
		return
	}
	log.Printf("[document] Download sukses — %.1fMB, upload ke WA...", float64(len(data))/(1024*1024))
	uploadAndSendDocument(client, ctx, chatJID, data, mimeType, fileName, caption)
}

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
