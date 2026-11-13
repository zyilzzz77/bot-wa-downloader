package main

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"
)

// wib adalah zona waktu Indonesia Barat (UTC+7, tanpa DST).
var wib = time.FixedZone("WIB", 7*60*60)

// sholatPrayers mendefinisikan urutan dan label waktu sholat.
var sholatPrayers = []struct {
	key   string
	label string
	icon  string
}{
	{"shubuh", "Shubuh", "🌙"},
	{"dzuhur", "Dzuhur", "☀️"},
	{"ashr", "Ashr", "🌤"},
	{"maghrib", "Maghrib", "🌇"},
	{"isya", "Isya", "🌃"},
}

// --- Model jadwal sholat ---

// prayerTime adalah satu waktu sholat pada tanggal tertentu.
type prayerTime struct {
	Name string
	At   time.Time
}

// sholatSchedule adalah jadwal sholat satu kota untuk satu hari.
type sholatSchedule struct {
	City     string
	DateText string
	Day      time.Time
	Prayers  []prayerTime
}

// --- API sholat neoxr ---

// sholatRegion adalah satu pilihan wilayah dari respons API.
type sholatRegion struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// sholatResponse merepresentasikan respons dari API sholat.
// Field "data" berisi array objek seperti {"shubuh": "04:11"}.
type sholatResponse struct {
	Creator    string              `json:"creator"`
	Status     bool                `json:"status"`
	Message    string              `json:"msg"`
	Date       string              `json:"date"`
	City       string              `json:"city"`
	Data       []map[string]string `json:"data"`
	RegionList []sholatRegion      `json:"region_list"`
}

// FetchSholat mengambil jadwal sholat hari ini untuk sebuah kota.
func FetchSholat(city, apiKey string) (*sholatSchedule, error) {
	apiURL := fmt.Sprintf(
		"https://api.neoxr.eu/api/sholat?q=%s&apikey=%s",
		url.QueryEscape(city),
		apiKey,
	)

	body, err := callAPIWithRetry(apiURL, "sholat")
	if err != nil {
		return nil, err
	}

	var apiResp sholatResponse
	if err := decodeAPIResponse(body, "sholat", &apiResp); err != nil {
		return nil, err
	}

	if !apiResp.Status {
		msg := apiResp.Message
		if auth, ok := apiAuthMessage(msg); ok {
			msg = auth
		} else if strings.Contains(strings.ToLower(msg), "region") ||
			strings.Contains(strings.ToLower(msg), "not found") {
			msg = "Kota tidak dikenali"
		}
		if msg == "" {
			msg = "kota tidak dikenali"
		}
		if saran := suggestSholatRegions(apiResp.RegionList, city); saran != "" {
			return nil, fmt.Errorf("%s\n\nMungkin maksudmu:\n%s", msg, saran)
		}
		return nil, errors.New(msg)
	}

	now := time.Now().In(wib)
	day := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, wib)

	schedule := &sholatSchedule{
		City:     apiResp.City,
		DateText: apiResp.Date,
		Day:      day,
	}
	for _, item := range apiResp.Data {
		for _, p := range sholatPrayers {
			raw := item[p.key]
			if raw == "" {
				continue
			}
			parsed, err := time.ParseInLocation("15:04", raw, wib)
			if err != nil {
				continue
			}
			schedule.Prayers = append(schedule.Prayers, prayerTime{
				Name: p.label,
				At:   time.Date(day.Year(), day.Month(), day.Day(), parsed.Hour(), parsed.Minute(), 0, 0, wib),
			})
		}
	}

	if len(schedule.Prayers) == 0 {
		return nil, fmt.Errorf("API sholat tidak mengembalikan jadwal untuk %q", city)
	}
	return schedule, nil
}

// suggestSholatRegions mencari nama wilayah yang mirip dengan kata kunci
// agar pengguna tahu nama kota yang diterima API.
func suggestSholatRegions(regions []sholatRegion, query string) string {
	q := normalizeCity(query)
	if q == "" {
		return ""
	}

	var matches []string
	for _, r := range regions {
		name := normalizeCity(r.Name)
		if strings.Contains(name, q) || strings.Contains(q, name) {
			matches = append(matches, r.Name)
			if len(matches) == 5 {
				break
			}
		}
	}
	if len(matches) == 0 {
		return ""
	}

	var sb strings.Builder
	for _, name := range matches {
		fmt.Fprintf(&sb, "• %s\n", titleCase(name))
	}
	return strings.TrimRight(sb.String(), "\n")
}

// normalizeCity menormalkan nama kota untuk keperluan cache dan pencocokan.
func normalizeCity(s string) string {
	return strings.Join(strings.Fields(strings.ToLower(s)), " ")
}

// sameDay memeriksa apakah dua waktu jatuh pada tanggal yang sama di WIB.
func sameDay(a, b time.Time) bool {
	ay, am, ad := a.In(wib).Date()
	by, bm, bd := b.In(wib).Date()
	return ay == by && am == bm && ad == bd
}
