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
