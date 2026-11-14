package main

import (
	"fmt"
	"net/url"
)

// SearchPlay mencari lagu di YouTube melalui API play neoxr.
// Mengembalikan info lagu beserta URL audio mp3-nya.
func SearchPlay(query, apiKey string) (*PlayResponse, error) {
	apiURL := fmt.Sprintf(
		"https://api.neoxr.eu/api/play?q=%s&apikey=%s",
		url.QueryEscape(query),
		apiKey,
	)

	body, err := callAPIWithRetry(apiURL, "play")
	if err != nil {
		return nil, err
	}

	var apiResp PlayResponse
	if err := decodeAPIResponse(body, "Play", &apiResp); err != nil {
		return nil, err
	}

	if !apiResp.Status {
		return nil, apiStatusError(apiResp.Message, "lagu tidak ditemukan — coba kata kunci lain")
	}
	if apiResp.Data.URL == "" {
		return nil, fmt.Errorf("API Play tidak mengembalikan link audio — coba lagi")
	}

	return &apiResp, nil
}
