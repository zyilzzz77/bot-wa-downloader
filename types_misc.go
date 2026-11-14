package main

// --- Brat (sticker) API response types ---

// BratResponse merepresentasikan respons dari API brat / bratvid.
type BratResponse struct {
	Creator string   `json:"creator"`
	Status  bool     `json:"status"`
	Message string   `json:"msg"`
	Data    BratData `json:"data"`
}

// BratData berisi file gambar (brat) atau video (bratvid) hasil generator.
type BratData struct {
	URL  string `json:"url"`
	Mime string `json:"mime"`
}

// --- Terabox API response types ---

// TeraboxResponse merepresentasikan respons dari API terabox.
type TeraboxResponse struct {
	Creator string        `json:"creator"`
	Status  bool          `json:"status"`
	Message string        `json:"msg"`
	Data    []TeraboxFile `json:"data"`
}

// TeraboxFile berisi satu file dari share link Terabox.
type TeraboxFile struct {
	ServerFilename string `json:"server_filename"`
	Size           string `json:"size"`
	Bytes          string `json:"bytes"`
	Dlink          string `json:"dlink"`
	URL            string `json:"url"`
	Duration       int    `json:"duration"`
}

// DocumentFile adalah file generik (selain video/gambar/audio) beserta nama filenya.
type DocumentFile struct {
	URL      string
	FileName string
}

// DownloadResult adalah hasil download yang sudah diolah dan siap dikirim.
type DownloadResult struct {
	Platform  string         // "tiktok", "instagram", "threads", atau "terabox"
	Creator   string         // info kreator dari API
	Videos    []string       // URL video
	Images    []string       // URL gambar
	Audio     []string       // URL audio
	Documents []DocumentFile // file generik (terabox non-video)
}
