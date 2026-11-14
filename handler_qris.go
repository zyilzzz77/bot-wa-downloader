package main

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"go.mau.fi/whatsmeow/types"
)

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
