package main

import (
	"fmt"
	"net/url"
)

// --- Spotify downloader ---

// spotifyCover memilih cover/thumbnail yang tersedia.
func (d SpotifyData) spotifyCover() string {
	if d.Cover != "" {
		return d.Cover
	}
	return d.Thumbnail
}

// FetchSpotify memanggil API spotify dan mengembalikan satu track atau isi
// playlist. Untuk playlist, tiap track masih perlu diunduh satu per satu
// (DownloadSpotifyTrack) karena listing tidak menyertakan link audio.
func FetchSpotify(spotifyURL, apiKey string) (*SpotifyListing, error) {
	apiURL := fmt.Sprintf(
		"https://api.neoxr.eu/api/spotify?url=%s&apikey=%s",
		url.QueryEscape(spotifyURL),
		apiKey,
	)

	body, err := callAPIWithRetry(apiURL, "spotify")
	if err != nil {
		return nil, err
	}

	var apiResp SpotifyResponse
	if err := decodeAPIResponse(body, "Spotify", &apiResp); err != nil {
		return nil, err
	}

	if !apiResp.Status {
		return nil, apiStatusError(apiResp.Message,
			"API Spotify menolak permintaan — pastikan link valid dan playlist publik.")
	}

	listing := &SpotifyListing{
		Title: apiResp.Data.Title,
		Cover: apiResp.Data.spotifyCover(),
	}

	if len(apiResp.Tracks) > 0 {
		listing.IsPlaylist = true
		for _, t := range apiResp.Tracks {
			if t.URL == "" {
				continue
			}
			listing.Tracks = append(listing.Tracks, SpotifyItem{
				Title:    t.Title,
				Artists:  t.Artists,
				Duration: t.Duration,
				URL:      t.URL,
			})
		}
		if len(listing.Tracks) == 0 {
			return nil, fmt.Errorf("playlist Spotify tidak berisi track yang bisa diunduh")
		}
		return listing, nil
	}

	if apiResp.Data.URL == "" {
		return nil, fmt.Errorf("API Spotify tidak mengembalikan link download — coba lagi")
	}
	listing.Tracks = append(listing.Tracks, SpotifyItem{
		Title:       apiResp.Data.Title,
		Artists:     apiResp.Data.Artist,
		Duration:    apiResp.Data.Duration,
		URL:         spotifyURL,
		DownloadURL: apiResp.Data.URL,
	})
	return listing, nil
}

// DownloadSpotifyTrack memanggil API untuk satu track dan mengembalikan link
// audio beserta judul, artis, dan durasi (fallback ke metadata playlist bila
// API tidak mengirimkannya).
func DownloadSpotifyTrack(trackURL, fallbackTitle, apiKey string) (downloadURL, title, artists, duration string, err error) {
	apiURL := fmt.Sprintf(
		"https://api.neoxr.eu/api/spotify?url=%s&apikey=%s",
		url.QueryEscape(trackURL),
		apiKey,
	)

	body, err := callAPIWithRetry(apiURL, "spotify")
	if err != nil {
		return "", "", "", "", err
	}

	var apiResp SpotifyResponse
	if err := decodeAPIResponse(body, "Spotify", &apiResp); err != nil {
		return "", "", "", "", err
	}

	if !apiResp.Status {
		return "", "", "", "", apiStatusError(apiResp.Message,
			"track Spotify gagal diunduh — coba lagi nanti.")
	}
	if apiResp.Data.URL == "" {
		return "", "", "", "", fmt.Errorf("API Spotify tidak mengembalikan link download — coba lagi")
	}

	title = apiResp.Data.Title
	if title == "" {
		title = fallbackTitle
	}
	return apiResp.Data.URL, title, apiResp.Data.Artist, apiResp.Data.Duration, nil
}
