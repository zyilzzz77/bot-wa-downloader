package main

import (
	"fmt"
	"io"
	"log"
	"net/url"
	"regexp"
	"strings"
)

// --- Google Drive downloader ---

// gdriveIDRegex mengambil file ID dari link Google Drive.
var gdriveIDRegex = regexp.MustCompile(`(?:/file/d/|/folders/|[?&]id=)([\w-]+)`)

// gdriveFileNameRegex mengambil nama file dari header Content-Disposition.
var gdriveFileNameRegex = regexp.MustCompile(`filename\*?=(?:UTF-8'')?"?([^";]+)"?`)

// gdriveFileID mengembalikan file ID dari sebuah link Google Drive.
func gdriveFileID(link string) string {
	if m := gdriveIDRegex.FindStringSubmatch(link); m != nil {
		return m[1]
	}
	return ""
}

// gdriveDirectURL membangun link unduhan resmi Google Drive untuk sebuah file ID.
// Dipakai saat link dari API ternyata halaman HTML, bukan file.
func gdriveDirectURL(fileID string) string {
	return fmt.Sprintf(
		"https://drive.usercontent.google.com/download?id=%s&export=download&confirm=t",
		fileID,
	)
}

// fetchGDriveFile mengunduh isi sebuah link dan mengembalikan byte, nama file, serta MIME-nya.
func fetchGDriveFile(downloadURL string) ([]byte, string, string, error) {
	resp, err := httpClient.Get(downloadURL)
	if err != nil {
		return nil, "", "", fmt.Errorf("gagal menghubungi Google Drive: %w", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", "", fmt.Errorf("gagal membaca file: %w", err)
	}

	disposition := resp.Header.Get("Content-Disposition")
	fileName := ""
	if m := gdriveFileNameRegex.FindStringSubmatch(disposition); m != nil {
		fileName = strings.TrimSpace(m[1])
	}

	return data, fileName, resp.Header.Get("Content-Type"), nil
}

// FetchGDrive memanggil API gdrive neoxr dan mengembalikan daftar file untuk
// diunduh. Link file menghasilkan satu item, link folder menghasilkan semua
// file di dalamnya (sub-folder dilewati dan dilaporkan lewat FolderCount).
func FetchGDrive(gdriveURL, apiKey string) (*GDriveListing, error) {
	apiURL := fmt.Sprintf(
		"https://api.neoxr.eu/api/gdrive?url=%s&verify=0&apikey=%s",
		url.QueryEscape(gdriveURL),
		apiKey,
	)

	body, err := callAPIWithRetry(apiURL, "gdrive")
	if err != nil {
		return nil, err
	}

	var apiResp GDriveResponse
	if err := decodeAPIResponse(body, "Google Drive", &apiResp); err != nil {
		return nil, err
	}

	if !apiResp.Status {
		return nil, apiStatusError(apiResp.Message,
			"API Google Drive menolak permintaan — pastikan link valid dan bisa diakses publik.")
	}

	data := apiResp.Data
	listing := &GDriveListing{
		Name:        data.Name,
		IsFolder:    data.Type == "folder" || len(data.Files) > 0 || data.FileCount > 0,
		FolderCount: data.FolderCount,
	}

	if listing.IsFolder {
		for _, f := range data.Files {
			if item, ok := gdriveItemFromFile(f); ok {
				listing.Items = append(listing.Items, item)
			}
		}
		if len(listing.Items) == 0 {
			return nil, fmt.Errorf("folder Google Drive tidak berisi file yang bisa diunduh")
		}
		return listing, nil
	}

	item := GDriveItem{
		Name:     data.Name,
		MimeType: data.MimeType,
		Size:     data.SizeBytes,
		URL:      data.DownloadURL,
	}
	if item.URL == "" {
		item.URL = data.URL
	}
	if item.URL == "" && data.ID != "" {
		item.URL = gdriveDirectURL(data.ID)
	}
	if item.URL == "" {
		return nil, fmt.Errorf("API Google Drive tidak mengembalikan link download — coba lagi")
	}

	listing.Items = append(listing.Items, item)
	return listing, nil
}

// gdriveItemFromFile mengubah satu file hasil listing folder menjadi item unduhan.
func gdriveItemFromFile(f GDriveFile) (GDriveItem, bool) {
	dl := f.DownloadURL
	if dl == "" && f.ID != "" {
		dl = gdriveDirectURL(f.ID)
	}
	if dl == "" {
		return GDriveItem{}, false
	}
	return GDriveItem{Name: f.Name, URL: dl, MimeType: f.MimeType, Size: f.SizeBytes}, true
}

// DownloadGDriveFile mengunduh satu item Google Drive dan mengembalikan isi
// file beserta nama file dan MIME-nya. Kalau respons berupa halaman HTML
// (link tidak dikenali API), unduhan diulang lewat endpoint download resmi.
func DownloadGDriveFile(item GDriveItem) ([]byte, string, string, error) {
	data, fileName, mimeType, err := fetchGDriveFile(item.URL)
	if err != nil {
		return nil, "", "", err
	}

	if strings.HasPrefix(mimeType, "text/html") {
		fileID := gdriveFileID(item.URL)
		if fileID == "" {
			return nil, "", "", fmt.Errorf("Google Drive mengembalikan halaman web, bukan file — pastikan link bisa diakses publik")
		}

		log.Printf("[gdrive] Dapat halaman HTML — ulangi lewat endpoint download resmi...")
		data, fileName, mimeType, err = fetchGDriveFile(gdriveDirectURL(fileID))
		if err != nil {
			return nil, "", "", err
		}
	}

	if strings.HasPrefix(mimeType, "text/html") {
		return nil, "", "", fmt.Errorf("Google Drive tidak mengembalikan file — pastikan link bisa diakses publik")
	}
	if fileName == "" {
		fileName = item.Name
	}
	if fileName == "" {
		fileName = "gdrive-file"
	}
	if mimeType == "" || mimeType == "application/octet-stream" {
		mimeType = item.MimeType
	}

	return data, fileName, mimeType, nil
}
