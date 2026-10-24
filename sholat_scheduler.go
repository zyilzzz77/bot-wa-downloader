package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"go.mau.fi/whatsmeow/types"
)

const (
	// sholatCheckInterval adalah jeda pengecekan waktu sholat oleh penjadwal.
	sholatCheckInterval = 30 * time.Second
	// sholatGracePeriod adalah batas keterlambatan pengingat masih dikirim
	// (mis. bot baru hidup kembali setelah koneksi putus).
	sholatGracePeriod = 10 * time.Minute
)

// run menjalankan loop pengecekan waktu sholat. Pengecekan pertama langsung
// dilakukan agar jadwal hari ini segera tersedia setelah bot tersambung.
func (m *sholatManager) run(client *ClientWrapper) {
	m.mu.Lock()
	m.client = client
	m.mu.Unlock()

	m.tick()

	ticker := time.NewTicker(sholatCheckInterval)
	defer ticker.Stop()
	for range ticker.C {
		m.tick()
	}
}

// tick mengambil jadwal terbaru tiap kota dan mengirim pengingat yang sudah
// jatuh tempo. Perbandingan tanggal di sini juga yang memicu pengambilan
// jadwal baru otomatis saat pergantian hari (00:00 WIB).
func (m *sholatManager) tick() {
	m.mu.Lock()
	client := m.client
	subs := make(map[string]string, len(m.chats))
	for jid, city := range m.chats {
		subs[jid] = city
	}
	m.mu.Unlock()

	if client == nil || len(subs) == 0 {
		return
	}

	now := time.Now().In(wib)

	// Reset penanda saat berganti hari supaya pengingat hari baru tidak
	// dianggap sudah terkirim.
	m.mu.Lock()
	if m.lastDay.IsZero() || !sameDay(m.lastDay, now) {
		m.sent = map[string]bool{}
		m.lastDay = now
	}
	m.mu.Unlock()

	// Kelompokkan chat per kota agar satu kota hanya memanggil API sekali.
	byCity := map[string][]string{}
	for jid, city := range subs {
		key := normalizeCity(city)
		byCity[key] = append(byCity[key], jid)
	}

	for _, jids := range byCity {
		city := subs[jids[0]]
		schedule, err := m.schedule(city)
		if err != nil {
			log.Printf("[sholat] gagal mengambil jadwal %q: %v", city, err)
			continue
		}
		for _, jid := range jids {
			m.sendDue(client, jid, schedule, now)
		}
	}
}

// sendDue mengirim pengingat untuk waktu sholat yang baru saja masuk.
func (m *sholatManager) sendDue(client *ClientWrapper, jidStr string, schedule *sholatSchedule, now time.Time) {
	jid, err := types.ParseJID(jidStr)
	if err != nil {
		return
	}

	for _, p := range schedule.Prayers {
		if now.Before(p.At) || now.Sub(p.At) > sholatGracePeriod {
			continue
		}
		if m.isSent(jidStr, schedule.Day, p.Name) {
			continue
		}

		// Jangan tandai terkirim kalau pengiriman gagal, agar dicoba lagi
		// pada pengecekan berikutnya selama masih dalam grace period.
		if client.SendText(context.Background(), jid, sholatReminderText(schedule, p)) == nil {
			log.Printf("[sholat] ❌ gagal kirim pengingat %s ke %s", p.Name, jidStr)
			continue
		}

		m.markSent(jidStr, schedule.Day, p.Name)
		log.Printf("[sholat] ✓ pengingat %s terkirim ke %s (%s)", p.Name, jidStr, schedule.City)
	}
}

// isSent memeriksa apakah pengingat sudah dikirim.
func (m *sholatManager) isSent(jid string, day time.Time, prayer string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.sent[sholatSentKey(jid, day, prayer)]
}

// markSent menandai pengingat sudah dikirim.
func (m *sholatManager) markSent(jid string, day time.Time, prayer string) {
	m.mu.Lock()
	m.sent[sholatSentKey(jid, day, prayer)] = true
	m.mu.Unlock()
}

// sholatSentKey membuat kunci unik untuk satu chat, tanggal, dan waktu sholat.
func sholatSentKey(jid string, day time.Time, prayer string) string {
	return fmt.Sprintf("%s|%s|%s", jid, day.Format("2006-01-02"), prayer)
}
