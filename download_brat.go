package main

import (
	"fmt"
	"net/url"
)

// --- Brat (sticker) ---

// GenerateBrat meminta generator brat (gambar) atau bratvid (video animasi)
// dari API neoxr. Mengembalikan URL media beserta MIME-nya.
func GenerateBrat(text, apiKey string, animated bool) (string, string, error) {
	endpoint := "brat"
	if animated {
		endpoint = "bratvid"
	}

	apiURL := fmt.Sprintf(
		"https://api.neoxr.eu/api/%s?text=%s&apikey=%s",
		endpoint,
		url.QueryEscape(text),
		apiKey,
	)

	body, err := callAPIWithRetry(apiURL, endpoint)
	if err != nil {
		return "", "", err
	}

	var apiResp BratResponse
	if err := decodeAPIResponse(body, endpoint, &apiResp); err != nil {
		return "", "", err
	}

	if !apiResp.Status {
		return "", "", apiStatusError(apiResp.Message,
			fmt.Sprintf("API %s menolak permintaan — coba lagi nanti.", endpoint))
	}
	if apiResp.Data.URL == "" {
		return "", "", fmt.Errorf("API %s tidak mengembalikan media — coba lagi", endpoint)
	}

	return apiResp.Data.URL, apiResp.Data.Mime, nil
}
