package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

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

	// YouTube: watch, short link (youtu.be), shorts, live, dan embed.
	youtubeRegex = regexp.MustCompile(
		`https?://(?:www\.|m\.|music\.)?(?:youtube\.com/(?:watch\?v=[\w-]+|shorts/[\w-]+|live/[\w-]+|embed/[\w-]+)|youtu\.be/[\w-]+)`,
	)

	// Google Drive: file (/file/d/ID), open?id=, uc?id=, dan folder (/drive/folders/ID).
	gdriveRegex = regexp.MustCompile(
		`https?://drive\.google\.com/(?:file/d/[\w-]+|open\?id=[\w-]+|uc\?(?:export=download&)?id=[\w-]+|drive/folders/[\w-]+)`,
	)

	// Folder Google Drive — belum didukung, dipakai untuk memberi pesan yang jelas.
	gdriveFolderRegex = regexp.MustCompile(`https?://drive\.google\.com/drive/folders/[\w-]+`)
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
	if m := youtubeRegex.FindString(text); m != "" {
		return "youtube", strings.TrimRight(m, "/")
	}
	if m := gdriveRegex.FindString(text); m != "" {
		return "gdrive", strings.TrimRight(m, "/")
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

// --- Command /qris (generate QRIS via LYDEV Pay) ---

// qrisPollInterval adalah jeda antar pengecekan status pembayaran QRIS.
const qrisPollInterval = 15 * time.Second

// parseQrisCommand memeriksa apakah teks adalah command qris (dengan atau
// tanpa slash). Mengembalikan nominal, apakah ini command, dan apakah
// nominalnya valid.
func parseQrisCommand(text string) (amount int, isCommand bool, validAmount bool) {
	trimmed := strings.TrimSpace(text)
	lower := strings.ToLower(trimmed)

	for _, prefix := range []string{"/qris", "qris", "!qris"} {
		if lower == prefix {
			return 0, true, false
		}
		if strings.HasPrefix(lower, prefix+" ") {
			a, ok := parseQrisAmount(trimmed[len(prefix)+1:])
			return a, true, ok
		}
	}
	return 0, false, false
}

// parseQrisAmount mengurai nominal: bentuk biasa ("10000") maupun bentuk
// ribuan dengan akhiran k ("10k" -> 10000, "10.5k" -> 10500).
func parseQrisAmount(s string) (int, bool) {
	s = strings.TrimSpace(strings.ToLower(s))
	if s == "" {
		return 0, false
	}

	if strings.HasSuffix(s, "k") {
		f, err := strconv.ParseFloat(strings.TrimSpace(strings.TrimSuffix(s, "k")), 64)
		if err != nil || f <= 0 {
			return 0, false
		}
		return int(f * 1000), true
	}

	n, err := strconv.Atoi(strings.NewReplacer(".", "", ",", "").Replace(s))
	if err != nil || n <= 0 {
		return 0, false
	}
	return n, true
}

// handleQRIS membuat QRIS lalu mengirim gambar QR beserta keterangannya, dan
// memulai polling status sampai LUNAS.
func handleQRIS(client *ClientWrapper, chatJID types.JID, amount int) {
	log.Printf("[qris] Membuat QRIS nominal %d", amount)
	ctx := context.Background()

	processingMsg := client.SendText(ctx, chatJID, "⏳ *Sedang membuat QRIS…*")

	payment, err := CreateQRIS(amount, fmt.Sprintf("QRIS %s", formatRupiah(amount)), lydevApiKey)
	if err != nil {
		log.Printf("[qris] ❌ Gagal membuat: %v", err)
		errText := fmt.Sprintf("❌ Gagal membuat QRIS\n\n%s", err.Error())
		if processingMsg != nil {
			client.EditText(ctx, chatJID, processingMsg.ID, errText)
		} else {
			client.SendText(ctx, chatJID, errText)
		}
		return
	}
	log.Printf("[qris] ✓ Order %s — total %d", payment.OrderID, payment.totalAmount())

	// Edit pesan "sedang membuat" menjadi keterangan QRIS. Pesan inilah yang
	// nanti diedit lagi saat pembayaran lunas.
	var infoMsgID string
	if processingMsg != nil {
		infoMsgID = processingMsg.ID
		client.EditText(ctx, chatJID, infoMsgID, qrisInfoText(payment))
	} else {
		if m := client.SendText(ctx, chatJID, qrisInfoText(payment)); m != nil {
			infoMsgID = m.ID
		}
	}

	qrPNG, err := FetchQRISImage(payment.QRURL, lydevApiKey)
	if err != nil {
		log.Printf("[qris] ❌ Gagal ambil QR: %v", err)
		client.SendText(ctx, chatJID, fmt.Sprintf("⚠️ QR belum bisa diambil: %v\n\nBuka link checkout di atas ya.", err))
	} else {
		caption := fmt.Sprintf("🧾 QRIS • %s\n🆔 %s", formatRupiah(payment.totalAmount()), payment.OrderID)
		uploadAndSendImage(client, ctx, chatJID, qrPNG, "image/png", caption)
	}

	if infoMsgID != "" {
		go pollQRISStatus(client, ctx, chatJID, infoMsgID, payment.OrderID, parseLydevTime(payment.ExpiresAt))
	}
}

// pollQRISStatus memantau status pembayaran dan mengedit pesan keterangan
// sampai status final atau kedaluwarsa.
func pollQRISStatus(client *ClientWrapper, ctx context.Context, chatJID types.JID, msgID, orderID string, expiresAt time.Time) {
	log.Printf("[qris] Mulai polling status %s tiap %s", orderID, qrisPollInterval)
	ticker := time.NewTicker(qrisPollInterval)
	defer ticker.Stop()

	for range ticker.C {
		if !expiresAt.IsZero() && time.Now().After(expiresAt) {
			client.EditText(ctx, chatJID, msgID, "⌛ *QRIS kedaluwarsa* — belum dibayar")
			log.Printf("[qris] Order %s kedaluwarsa", orderID)
			return
		}

		payment, err := FetchPaymentStatus(orderID, lydevApiKey)
		if err != nil {
			log.Printf("[qris] gagal cek status %s: %v", orderID, err)
			continue
		}

		switch payment.Status {
		case "PAID":
			client.EditText(ctx, chatJID, msgID, qrisPaidText(payment))
			log.Printf("[qris] ✓ Order %s LUNAS", orderID)
			return
		case "FAILED", "EXPIRED", "CANCELLED", "REFUNDED":
			client.EditText(ctx, chatJID, msgID,
				fmt.Sprintf("❌ *QRIS %s*\n\n🆔 %s", payment.Status, orderID))
			log.Printf("[qris] Order %s berakhir: %s", orderID, payment.Status)
			return
		}
	}
}

// qrisInfoText menyusun keterangan QRIS yang dikirim ke chat.
func qrisInfoText(p *lydevPayment) string {
	var sb strings.Builder
	sb.WriteString("🧾 *QRIS — LYDEV Pay*\n\n")
	fmt.Fprintf(&sb, "🆔 *Order:* %s\n", p.OrderID)
	fmt.Fprintf(&sb, "💰 *Nominal:* %s\n", formatRupiah(p.Amount))
	if fee := p.feeValue(); fee > 0 {
		fmt.Fprintf(&sb, "➕ *Fee:* %s\n", formatRupiah(fee))
	}
	fmt.Fprintf(&sb, "💳 *Total bayar:* %s\n", formatRupiah(p.totalAmount()))
	if t := parseLydevTime(p.ExpiresAt); !t.IsZero() {
		fmt.Fprintf(&sb, "⏳ *Berlaku s/d:* %s\n", formatWaktu(t))
	}
	if p.CheckoutURL != "" {
		fmt.Fprintf(&sb, "🔗 %s\n", p.CheckoutURL)
	}
	sb.WriteString("\n📷 _Scan QR di pesan berikut._")
	return sb.String()
}

// qrisPaidText menyusun keterangan saat pembayaran sudah lunas.
func qrisPaidText(p *lydevPayment) string {
	var sb strings.Builder
	sb.WriteString("✅ *Pembayaran Berhasil*\n\n")
	fmt.Fprintf(&sb, "🆔 *Order:* %s\n", p.OrderID)
	fmt.Fprintf(&sb, "💰 *Nominal:* %s\n", formatRupiah(p.Amount))
	fmt.Fprintf(&sb, "💳 *Total:* %s\n", formatRupiah(p.totalAmount()))
	if t := parseLydevTime(p.PaidAt); !t.IsZero() {
		fmt.Fprintf(&sb, "🕐 *Dibayar:* %s\n", formatWaktu(t))
	}
	sb.WriteString("\nTerima kasih! 🙏")
	return sb.String()
}

var bulanIndonesia = [...]string{"Jan", "Feb", "Mar", "Apr", "Mei", "Jun", "Jul", "Agu", "Sep", "Okt", "Nov", "Des"}

// formatRupiah memformat nominal ke bentuk "Rp10.000".
func formatRupiah(n int) string {
	sign := ""
	if n < 0 {
		sign = "-"
		n = -n
	}
	s := strconv.Itoa(n)
	var sb strings.Builder
	for i, c := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			sb.WriteByte('.')
		}
		sb.WriteRune(c)
	}
	return sign + "Rp" + sb.String()
}

// formatWaktu memformat waktu ke "27 Sep 2026, 17:15" (waktu lokal).
func formatWaktu(t time.Time) string {
	t = t.Local()
	return fmt.Sprintf("%d %s %d, %02d:%02d",
		t.Day(), bulanIndonesia[int(t.Month())-1], t.Year(), t.Hour(), t.Minute())
}

// --- Command /server (status server) ---

// parseServerCommand memeriksa command server (dengan atau tanpa slash).
func parseServerCommand(text string) bool {
	switch strings.ToLower(strings.TrimSpace(text)) {
	case "server", "/server", "!server":
		return true
	}
	return false
}

// handleServer mengirim ringkasan status server ke chat.
func handleServer(client *ClientWrapper, chatJID types.JID) {
	log.Printf("[server] Mengambil status server")
	ctx := context.Background()

	processingMsg := client.SendText(ctx, chatJID, "⏳ *Mengambil status server…*")

	report := buildServerReport()

	if processingMsg != nil {
		client.EditText(ctx, chatJID, processingMsg.ID, report)
	} else {
		client.SendText(ctx, chatJID, report)
	}
	log.Printf("[server] ✓ Status terkirim")
}

// --- Command /bench (benchmark server ala bench.sh) ---

// parseBenchCommand memeriksa command bench (dengan atau tanpa slash).
func parseBenchCommand(text string) bool {
	switch strings.ToLower(strings.TrimSpace(text)) {
	case "bench", "/bench", "!bench":
		return true
	}
	return false
}

// handleBench menjalankan benchmark lalu mengirim hasilnya ke chat.
func handleBench(client *ClientWrapper, chatJID types.JID) {
	log.Printf("[bench] Menjalankan benchmark")
	ctx := context.Background()

	processingMsg := client.SendText(ctx, chatJID,
		"⏳ *Menjalankan benchmark…*\n\n_Info sistem + I/O + speedtest. Butuh 1–2 menit._")

	report, err := runBenchmark()
	if err != nil {
		log.Printf("[bench] ⚠️ %v", err)
		if report == "" {
			report = err.Error()
		} else {
			report = fmt.Sprintf("%s\n\n⚠️ %v", report, err)
		}
	}

	text := "📊 *Benchmark Server*\n\n```\n" + report + "\n```"
	if processingMsg != nil {
		client.EditText(ctx, chatJID, processingMsg.ID, text)
	} else {
		client.SendText(ctx, chatJID, text)
	}
	log.Printf("[bench] ✓ Selesai")
}

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

// --- Google Drive ---

// handleGDrive mengunduh file Google Drive lalu mengirimnya sebagai dokumen.
func handleGDrive(client *ClientWrapper, chatJID types.JID, gdriveURL string) {
	log.Printf("[gdrive] Menerima link %s", gdriveURL)
	ctx := context.Background()

	if gdriveFolderRegex.MatchString(gdriveURL) {
		client.SendText(ctx, chatJID, "❌ Link *folder* Google Drive belum didukung — kirim link file-nya ya.")
		return
	}

	processingMsg := client.SendText(ctx, chatJID, "⏳ *Memproses Google Drive…*")

	data, fileName, mimeType, err := DownloadGDrive(gdriveURL, apiKey)
	if err != nil {
		log.Printf("[gdrive] ❌ Gagal: %v", err)
		errText := fmt.Sprintf("❌ Gagal download *Google Drive*\n\n%s", err.Error())
		if processingMsg != nil {
			client.EditText(ctx, chatJID, processingMsg.ID, errText)
		} else {
			client.SendText(ctx, chatJID, errText)
		}
		return
	}

	sizeMB := float64(len(data)) / (1024 * 1024)
	log.Printf("[gdrive] ✓ %s — %.1fMB (%s)", fileName, sizeMB, mimeType)

	summary := fmt.Sprintf("✅ *File Google Drive*\n\n📄 *Nama:* %s\n💾 *Ukuran:* %.1f MB\n🧩 *Tipe:* %s\n\n📥 _Mengirim file…_",
		fileName, sizeMB, mimeType)
	if processingMsg != nil {
		client.EditText(ctx, chatJID, processingMsg.ID, summary)
	} else {
		client.SendText(ctx, chatJID, summary)
	}

	uploadAndSendDocument(client, ctx, chatJID, data, mimeType, fileName, fmt.Sprintf("📄 %s", fileName))
	log.Printf("[gdrive] ✓ Terkirim!")
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

// --- Helper download file dengan progres ---

const (
	// downloadProgressDelay adalah jeda sebelum pesan progres pertama dikirim,
	// agar unduhan yang cepat tidak menambah pesan baru.
	downloadProgressDelay = 2 * time.Second
	// downloadProgressInterval adalah jeda minimum antar edit pesan progres.
	downloadProgressInterval = 3 * time.Second
)

// downloadFileWithProgress mengunduh file sambil melaporkan progres (persentase,
// kecepatan, dan estimasi waktu tersisa) lewat pesan WhatsApp yang diedit
// berkala. Pesan hanya muncul bila unduhan berjalan lambat, sehingga unduhan
// kecil/cepat tidak menambah pesan. Progres tetap di-update saat koneksi
// tersendat, sehingga estimasi ikut menyesuaikan.
func downloadFileWithProgress(client *ClientWrapper, ctx context.Context, chatJID types.JID, rawURL, label string) ([]byte, string, error) {
	resp, err := httpClient.Get(rawURL)
	if err != nil {
		return nil, "", fmt.Errorf("gagal mengunduh: %w", err)
	}
	defer resp.Body.Close()

	mimeType := resp.Header.Get("Content-Type")
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}

	total := resp.ContentLength
	start := time.Now()

	var downloaded int64
	var msgID string

	stop := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()

		timer := time.NewTimer(downloadProgressDelay)
		defer timer.Stop()
		select {
		case <-stop:
			return
		case <-timer.C:
		}

		first := client.SendText(ctx, chatJID,
			progressText(label, atomic.LoadInt64(&downloaded), total, time.Since(start)))
		if first == nil {
			return
		}
		msgID = first.ID

		ticker := time.NewTicker(downloadProgressInterval)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
				client.EditText(ctx, chatJID, msgID,
					progressText(label, atomic.LoadInt64(&downloaded), total, time.Since(start)))
			}
		}
	}()

	var buf bytes.Buffer
	chunk := make([]byte, 32*1024)
	for {
		n, readErr := resp.Body.Read(chunk)
		if n > 0 {
			buf.Write(chunk[:n])
			atomic.AddInt64(&downloaded, int64(n))
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			close(stop)
			wg.Wait()
			if msgID != "" {
				client.EditText(ctx, chatJID, msgID,
					fmt.Sprintf("❌ *Gagal mendownload %s* — koneksi terputus", label))
			}
			return nil, "", fmt.Errorf("gagal membaca: %w", readErr)
		}
	}

	close(stop)
	wg.Wait()

	if msgID != "" {
		client.EditText(ctx, chatJID, msgID,
			fmt.Sprintf("✅ *%s* terunduh — %.1f MB dalam %s",
				label, float64(buf.Len())/(1024*1024), humanDuration(time.Since(start).Seconds())))
	}

	return buf.Bytes(), mimeType, nil
}

// progressText menyusun teks progres unduhan: bar, persentase, ukuran,
// kecepatan, dan estimasi waktu tersisa.
func progressText(label string, downloaded, total int64, elapsed time.Duration) string {
	secs := elapsed.Seconds()
	if secs <= 0 {
		secs = 0.001
	}
	speed := float64(downloaded) / secs

	var sb strings.Builder
	fmt.Fprintf(&sb, "⬇️ *Mendownload %s…*\n\n", label)

	if total > 0 {
		pct := float64(downloaded) / float64(total) * 100
		fmt.Fprintf(&sb, "%s  %.0f%%\n", progressBar(pct), pct)
		fmt.Fprintf(&sb, "📦 %s / %s\n", humanBytes(downloaded), humanBytes(total))
	} else {
		fmt.Fprintf(&sb, "📦 %s terunduh\n", humanBytes(downloaded))
	}

	fmt.Fprintf(&sb, "🚀 %s/detik", humanBytes(int64(speed)))

	if total > 0 && speed > 0 && total > downloaded {
		eta := float64(total-downloaded) / speed
		fmt.Fprintf(&sb, "\n⏳ Estimasi: %s", humanDuration(eta))
	}
	return sb.String()
}

// progressBar membuat bar progres 10 segmen dari persentase.
func progressBar(pct float64) string {
	const segments = 10
	filled := int(pct / 100 * segments)
	if filled < 0 {
		filled = 0
	}
	if filled > segments {
		filled = segments
	}
	return strings.Repeat("▰", filled) + strings.Repeat("▱", segments-filled)
}

// humanBytes memformat ukuran byte ke satuan yang mudah dibaca.
func humanBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	units := []string{"KB", "MB", "GB", "TB"}
	value := float64(n)
	i := -1
	for value >= unit && i < len(units)-1 {
		value /= unit
		i++
	}
	return fmt.Sprintf("%.1f %s", value, units[i])
}

// humanDuration memformat durasi (detik) ke teks ringkas.
func humanDuration(seconds float64) string {
	if seconds < 0 {
		return "?"
	}
	total := int(seconds + 0.5)
	if total < 60 {
		return fmt.Sprintf("%ds", total)
	}
	return fmt.Sprintf("%dm %ds", total/60, total%60)
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
