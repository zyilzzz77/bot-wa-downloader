package main

import (
	"fmt"
	"net/url"
	"path"
	"strings"
)

// teraboxExtVideo adalah ekstensi yang dikirim sebagai video WA.
var teraboxExtVideo = map[string]bool{
	".mp4": true, ".mov": true, ".mkv": true, ".webm": true,
	".avi": true, ".3gp": true, ".m4v": true,
}

// teraboxExtImage adalah ekstensi yang dikirim sebagai gambar WA.
var teraboxExtImage = map[string]bool{
	".jpg": true, ".jpeg": true, ".png": true, ".webp": true, ".gif": true,
}

// teraboxExtAudio adalah ekstensi yang dikirim sebagai audio WA.
var teraboxExtAudio = map[string]bool{
	".mp3": true, ".ogg": true, ".wav": true, ".m4a": true, ".opus": true,
}

// DownloadTerabox mengambil data file Terabox melalui API neoxr.
// Video/gambar/audio dipetakan ke Videos/Images/Audio berdasarkan ekstensi,
// sisanya (zip, pdf, dsb) masuk ke Documents beserta nama filenya.
func DownloadTerabox(teraboxURL, apiKey string) (*DownloadResult, error) {
	apiURL := fmt.Sprintf(
		"https://api.neoxr.eu/api/terabox?url=%s&apikey=%s",
		url.QueryEscape(teraboxURL),
		apiKey,
	)

	body, err := callAPIWithRetry(apiURL, "terabox")
	if err != nil {
		return nil, err
	}

	var apiResp TeraboxResponse
	if err := decodeAPIResponse(body, "Terabox", &apiResp); err != nil {
		return nil, err
	}

	if !apiResp.Status {
		return nil, apiStatusError(apiResp.Message,
			"API Terabox menolak permintaan — pastikan link valid dan bisa diakses publik.")
	}
	if len(apiResp.Data) == 0 {
		return nil, fmt.Errorf("API Terabox tidak mengembalikan file — pastikan link valid")
	}

	result := &DownloadResult{
		Platform: "terabox",
		Creator:  apiResp.Creator,
	}

	for _, f := range apiResp.Data {
		dlURL := f.Dlink
		if dlURL == "" {
			dlURL = f.URL
		}
		if dlURL == "" {
			continue
		}
		name := f.ServerFilename
		if name == "" {
			name = "terabox-file"
		}
		ext := strings.ToLower(path.Ext(name))

		switch {
		case teraboxExtVideo[ext] || (ext == "" && f.Duration > 0):
			result.Videos = append(result.Videos, dlURL)
		case teraboxExtImage[ext]:
			result.Images = append(result.Images, dlURL)
		case teraboxExtAudio[ext]:
			result.Audio = append(result.Audio, dlURL)
		default:
			result.Documents = append(result.Documents, DocumentFile{URL: dlURL, FileName: name})
		}
	}

	if len(result.Videos) == 0 && len(result.Images) == 0 &&
		len(result.Audio) == 0 && len(result.Documents) == 0 {
		return nil, fmt.Errorf("API Terabox tidak mengembalikan link download — pastikan link valid")
	}

	return result, nil
}
