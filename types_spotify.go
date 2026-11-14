package main

// --- Spotify API response types ---

// SpotifyResponse merepresentasikan respons dari API spotify.
// Untuk playlist, daftar lagu ada di "tracks"; untuk satu track, hasilnya di "data".
type SpotifyResponse struct {
	Creator string         `json:"creator"`
	Status  bool           `json:"status"`
	Message string         `json:"msg"`
	Data    SpotifyData    `json:"data"`
	Tracks  []SpotifyTrack `json:"tracks"`
}

// SpotifyData berisi metadata track/playlist. Untuk track, "url" adalah link
// unduhan audio; "preview" adalah potongan mp3 30 detik.
type SpotifyData struct {
	Cover     string `json:"cover"`
	Thumbnail string `json:"thumbnail"`
	Title     string `json:"title"`
	Artist    string `json:"artist"`
	Duration  string `json:"duration"`
	Preview   string `json:"preview"`
	URL       string `json:"url"`
}

// SpotifyTrack adalah satu lagu dalam playlist.
type SpotifyTrack struct {
	Cover    string `json:"cover"`
	Title    string `json:"title"`
	Artists  string `json:"artists"`
	Album    string `json:"album"`
	Duration string `json:"duration"`
	URL      string `json:"url"`
}

// SpotifyItem adalah satu track siap diunduh.
type SpotifyItem struct {
	Title       string
	Artists     string
	Duration    string
	URL         string // link track Spotify
	DownloadURL string // link audio langsung (kalau sudah tersedia)
}

// SpotifyListing adalah hasil API spotify: satu track atau isi sebuah playlist.
type SpotifyListing struct {
	Title      string
	Cover      string
	IsPlaylist bool
	Tracks     []SpotifyItem
}
