package main

// --- Instagram API response types ---

// InstagramResponse merepresentasikan respons dari API ig.
type InstagramResponse struct {
	Creator string           `json:"creator"`
	Status  bool             `json:"status"`
	Message string           `json:"msg"`
	Data    []InstagramMedia `json:"data"`
}

// InstagramMedia berisi satu item media Instagram.
type InstagramMedia struct {
	Type string `json:"type"` // "mp4", "jpg", "jpeg", "png", dsb.
	URL  string `json:"url"`
}

// --- Threads API response types ---

// ThreadsResponse merepresentasikan respons dari API threads.
// Bentuknya sama dengan Instagram: array {type, url}.
type ThreadsResponse struct {
	Creator string           `json:"creator"`
	Status  bool             `json:"status"`
	Message string           `json:"msg"`
	Data    []InstagramMedia `json:"data"`
}

// --- Play (YouTube search) API response types ---

// PlayResponse merepresentasikan respons dari API play.
type PlayResponse struct {
	Creator         string   `json:"creator"`
	Status          bool     `json:"status"`
	Message         string   `json:"msg"`
	ID              string   `json:"id"`
	Title           string   `json:"title"`
	Thumbnail       string   `json:"thumbnail"`
	Duration        string   `json:"duration"`
	DurationSeconds int      `json:"duration_seconds"`
	Channel         string   `json:"channel"`
	Views           string   `json:"views"`
	Data            PlayData `json:"data"`
}

// PlayData berisi file audio hasil pencarian lagu.
type PlayData struct {
	Filename  string `json:"filename"`
	Quality   string `json:"quality"`
	Size      string `json:"size"`
	Extension string `json:"extension"`
	URL       string `json:"url"`
}

// --- YouTube download API response types ---

// YouTubeResponse merepresentasikan respons dari API youtube.
// Bentuknya sama dengan PlayResponse, plus field publish & fduration untuk audio.
type YouTubeResponse struct {
	Creator   string   `json:"creator"`
	Status    bool     `json:"status"`
	Message   string   `json:"msg"`
	ID        string   `json:"id"`
	Title     string   `json:"title"`
	Thumbnail string   `json:"thumbnail"`
	Duration  string   `json:"duration"`
	FDuration string   `json:"fduration"`
	Channel   string   `json:"channel"`
	Views     string   `json:"views"`
	Data      PlayData `json:"data"`
}

// DurationText mengembalikan durasi siap tampil.
// Video memakai field "duration", audio memakai "fduration".
func (y *YouTubeResponse) DurationText() string {
	if y.FDuration != "" {
		return y.FDuration
	}
	return y.Duration
}
