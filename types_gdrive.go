package main

// --- Google Drive API response types ---

// GDriveResponse merepresentasikan respons dari API gdrive.
type GDriveResponse struct {
	Creator string     `json:"creator"`
	Status  bool       `json:"status"`
	Message string     `json:"msg"`
	Data    GDriveData `json:"data"`
}

// GDriveData bisa berupa satu file (type "file") atau isi folder (type "folder").
type GDriveData struct {
	Type        string         `json:"type"`
	Name        string         `json:"name"`
	ID          string         `json:"id"`
	ViewURL     string         `json:"viewUrl"`
	URL         string         `json:"url"`         // link unduhan lama (single file)
	DownloadURL string         `json:"downloadUrl"` // link unduhan langsung
	MimeType    string         `json:"mimeType"`
	Size        string         `json:"size"`
	SizeBytes   int64          `json:"sizeBytes"`
	FileCount   int            `json:"fileCount"`
	FolderCount int            `json:"folderCount"`
	Files       []GDriveFile   `json:"files"`
	Folders     []GDriveFolder `json:"folders"`
}

// GDriveFile adalah satu file hasil listing Google Drive.
type GDriveFile struct {
	Name        string `json:"name"`
	MimeType    string `json:"mimeType"`
	Size        string `json:"size"`
	SizeBytes   int64  `json:"sizeBytes"`
	Extension   string `json:"extension"`
	DownloadURL string `json:"downloadUrl"`
	ViewURL     string `json:"viewUrl"`
	ID          string `json:"id"`
}

// GDriveFolder adalah sub-folder di dalam folder Google Drive.
type GDriveFolder struct {
	Name    string `json:"name"`
	ID      string `json:"id"`
	ViewURL string `json:"viewUrl"`
}

// GDriveItem adalah satu file siap unduh dari Google Drive.
type GDriveItem struct {
	Name     string
	URL      string
	MimeType string
	Size     int64
}

// GDriveListing adalah hasil API gdrive: satu file atau isi sebuah folder.
type GDriveListing struct {
	Name        string
	IsFolder    bool
	Items       []GDriveItem
	FolderCount int // jumlah sub-folder yang dilewati
}
