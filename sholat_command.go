package main

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"go.mau.fi/whatsmeow/types"
)

// --- Command /sholat ---

// sholatCommand adalah hasil parsing command sholat.
type sholatCommand struct {
	city string
	off  bool
}

// parseSholatCommand memeriksa apakah teks adalah command sholat.
// Menerima "/sholat <kota>", "sholat <kota>", dan "/sholat off".
func parseSholatCommand(text string) (sholatCommand, bool) {
	trimmed := strings.TrimSpace(text)
	lower := strings.ToLower(trimmed)

	for _, prefix := range []string{"/sholat", "sholat", "!sholat"} {
		if lower == prefix {
			return sholatCommand{}, true
		}
		if strings.HasPrefix(lower, prefix+" ") {
			arg := strings.TrimSpace(trimmed[len(prefix)+1:])
			switch strings.ToLower(arg) {
			case "off", "mati", "stop", "berhenti":
				return sholatCommand{off: true}, true
			}
			return sholatCommand{city: arg}, true
		}
	}
	return sholatCommand{}, false
}

// handleSholatCommand mengatur kota langganan dan mengirim jadwal sholat.
func handleSholatCommand(client *ClientWrapper, chatJID types.JID, cmd sholatCommand) {
	ctx := context.Background()

	if cmd.off {
		if !sholatMgr.unsubscribe(chatJID) {
			client.SendText(ctx, chatJID, "ℹ️ Pengingat sholat memang belum aktif di chat ini.")
			return
		}
		client.SendText(ctx, chatJID, "🔕 *Pengingat sholat dimatikan* untuk chat ini.")
		log.Printf("[sholat] Langganan %s dihentikan", chatJID)
		return
	}

	city := cmd.city
	if city == "" {
		c, ok := sholatMgr.cityFor(chatJID)
		if !ok {
			client.SendText(ctx, chatJID,
				"🕌 *Pengingat Sholat*\n\nAtur kota dulu ya. Contoh:\n\n• *sholat jakarta timur*\n• *sholat bandung*\n\nSetelah itu jadwal hari ini langsung dikirim dan pengingat otomatis aktif.")
			return
		}
		city = c
	}

	log.Printf("[sholat] Mengambil jadwal untuk %q (chat %s)", city, chatJID)
	processingMsg := client.SendText(ctx, chatJID,
		fmt.Sprintf("⏳ *Mengambil jadwal sholat %s…*", titleCase(city)))

	schedule, err := sholatMgr.schedule(city)
	if err != nil {
		log.Printf("[sholat] ❌ Gagal mengambil jadwal: %v", err)
		errText := fmt.Sprintf("❌ Gagal mengambil jadwal sholat\n\n%s", err.Error())
		if processingMsg != nil {
			client.EditText(ctx, chatJID, processingMsg.ID, errText)
		} else {
			client.SendText(ctx, chatJID, errText)
		}
		return
	}

	// Simpan kota kanonik dari API dan aktifkan pengingat untuk chat ini.
	sholatMgr.subscribe(chatJID, schedule.City)

	text := sholatScheduleText(schedule) +
		"\n\n🔔 _Pengingat otomatis aktif — pesan dikirim saat masuk waktu sholat._"
	if processingMsg != nil {
		client.EditText(ctx, chatJID, processingMsg.ID, text)
	} else {
		client.SendText(ctx, chatJID, text)
	}
	log.Printf("[sholat] ✓ Jadwal %s terkirim ke %s", schedule.City, chatJID)
}

// --- Teks pesan ---

// sholatReminderText menyusun pesan pengingat saat masuk waktu sholat.
func sholatReminderText(schedule *sholatSchedule, p prayerTime) string {
	return fmt.Sprintf(
		"🕌 *Waktu Sholat %s*\n\n📍 *%s*\n🕐 %s WIB\n\nSudah masuk waktu sholat %s. Yuk tunaikan sholat dulu.",
		p.Name, schedule.City, p.At.Format("15:04"), p.Name,
	)
}

// sholatScheduleText menyusun pesan jadwal sholat harian.
func sholatScheduleText(schedule *sholatSchedule) string {
	var sb strings.Builder
	sb.WriteString("🕌 *Jadwal Sholat*\n\n")
	fmt.Fprintf(&sb, "📍 *%s*\n", schedule.City)
	if schedule.DateText != "" {
		fmt.Fprintf(&sb, "📅 %s\n", schedule.DateText)
	}
	sb.WriteString("\n")

	icons := make(map[string]string, len(sholatPrayers))
	for _, p := range sholatPrayers {
		icons[p.label] = p.icon
	}
	for _, p := range schedule.Prayers {
		fmt.Fprintf(&sb, "%s %s — %s\n", icons[p.Name], p.Name, p.At.Format("15:04"))
	}

	if next, ok := nextPrayer(schedule, time.Now().In(wib)); ok {
		fmt.Fprintf(&sb, "\n⏳ Berikutnya: *%s* pukul %s WIB", next.Name, next.At.Format("15:04"))
	} else {
		sb.WriteString("\n⏳ Semua waktu sholat hari ini sudah lewat.")
	}
	return sb.String()
}

// nextPrayer mencari waktu sholat berikutnya setelah sekarang.
func nextPrayer(schedule *sholatSchedule, now time.Time) (prayerTime, bool) {
	for _, p := range schedule.Prayers {
		if p.At.After(now) {
			return p, true
		}
	}
	return prayerTime{}, false
}
