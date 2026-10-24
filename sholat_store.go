package main

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"go.mau.fi/whatsmeow/types"
)

// --- Penyimpanan langganan ---

// sholatSubscription adalah langganan satu chat.
type sholatSubscription struct {
	City string `json:"city"`
}

// sholatStore adalah bentuk file penyimpanan langganan.
type sholatStore struct {
	Chats map[string]sholatSubscription `json:"chats"`
}

// sholatManager menyimpan langganan per chat, cache jadwal, dan penanda
// pengingat yang sudah terkirim.
type sholatManager struct {
	mu        sync.Mutex
	path      string
	chats     map[string]string // JID chat -> nama kota
	schedules map[string]*sholatSchedule
	sent      map[string]bool
	lastDay   time.Time
	client    *ClientWrapper
}

// sholatMgr adalah instance global yang dipakai handler dan penjadwal.
var sholatMgr = newSholatManager(filepath.Join("data", "sholat.json"))

// newSholatManager membuat manager dan memuat langganan dari disk.
func newSholatManager(path string) *sholatManager {
	m := &sholatManager{
		path:      path,
		chats:     map[string]string{},
		schedules: map[string]*sholatSchedule{},
		sent:      map[string]bool{},
	}
	m.load()
	return m
}

// load membaca daftar langganan dari file JSON.
func (m *sholatManager) load() {
	data, err := os.ReadFile(m.path)
	if err != nil {
		if !os.IsNotExist(err) {
			log.Printf("[sholat] gagal membaca %s: %v", m.path, err)
		}
		return
	}

	var store sholatStore
	if err := json.Unmarshal(data, &store); err != nil {
		log.Printf("[sholat] file langganan rusak: %v", err)
		return
	}
	for jid, sub := range store.Chats {
		if sub.City != "" {
			m.chats[jid] = sub.City
		}
	}
	log.Printf("[sholat] %d chat berlangganan pengingat sholat", len(m.chats))
}

// saveLocked menyimpan daftar langganan ke disk. Pemanggil wajib memegang lock.
func (m *sholatManager) saveLocked() {
	store := sholatStore{Chats: map[string]sholatSubscription{}}
	for jid, city := range m.chats {
		store.Chats[jid] = sholatSubscription{City: city}
	}

	data, err := json.MarshalIndent(store, "", "  ")
	if err != nil {
		log.Printf("[sholat] gagal menyusun data langganan: %v", err)
		return
	}

	dir := filepath.Dir(m.path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		log.Printf("[sholat] gagal membuat direktori %s: %v", dir, err)
		return
	}

	tmp := m.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		log.Printf("[sholat] gagal menulis %s: %v", tmp, err)
		return
	}
	if err := os.Rename(tmp, m.path); err != nil {
		log.Printf("[sholat] gagal menyimpan %s: %v", m.path, err)
	}
}

// subscribe menyimpan (atau memperbarui) kota untuk sebuah chat.
func (m *sholatManager) subscribe(chat types.JID, city string) {
	m.mu.Lock()
	m.chats[chat.String()] = city
	m.saveLocked()
	m.mu.Unlock()
}

// unsubscribe menghapus langganan sebuah chat. Mengembalikan false jika chat
// memang belum berlangganan.
func (m *sholatManager) unsubscribe(chat types.JID) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.chats[chat.String()]; !ok {
		return false
	}
	delete(m.chats, chat.String())
	m.saveLocked()
	return true
}

// cityFor mengembalikan kota langganan sebuah chat.
func (m *sholatManager) cityFor(chat types.JID) (string, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	city, ok := m.chats[chat.String()]
	return city, ok
}

// cacheSchedule menyimpan jadwal ke cache tanpa memanggil API.
func (m *sholatManager) cacheSchedule(schedule *sholatSchedule) {
	m.mu.Lock()
	m.schedules[normalizeCity(schedule.City)] = schedule
	m.mu.Unlock()
}

// schedule mengembalikan jadwal hari ini untuk sebuah kota, memakai cache
// bila masih berlaku dan mengambil ulang (termasuk saat pergantian hari)
// bila belum ada atau sudah kedaluwarsa.
func (m *sholatManager) schedule(city string) (*sholatSchedule, error) {
	key := normalizeCity(city)
	today := time.Now().In(wib)

	m.mu.Lock()
	cached := m.schedules[key]
	m.mu.Unlock()

	if cached != nil && sameDay(cached.Day, today) {
		return cached, nil
	}

	schedule, err := FetchSholat(city, apiKey)
	if err != nil {
		if cached != nil {
			log.Printf("[sholat] gagal memperbarui jadwal %q, pakai cache: %v", city, err)
			return cached, nil
		}
		return nil, err
	}

	m.cacheSchedule(schedule)
	return schedule, nil
}
