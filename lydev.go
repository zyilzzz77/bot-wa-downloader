package main

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// lydevBaseURL adalah base URL API LYDEV Pay.
const lydevBaseURL = "https://pay.lydev.id"

// Batas nominal QRIS (mengikuti default server LYDEV Pay).
const (
	lydevMinAmount = 10000
	lydevMaxAmount = 10000000
)

// lydevPayment merepresentasikan objek payment dari LYDEV Pay.
// Fee & providerAmount bisa null, jadi dipakai pointer.
type lydevPayment struct {
	OrderID           string `json:"orderId"`
	Status            string `json:"status"`
	Amount            int    `json:"amount"`
	Fee               *int   `json:"fee"`
	ProviderAmount    *int   `json:"providerAmount"`
	Currency          string `json:"currency"`
	Description       string `json:"description"`
	ExternalReference string `json:"externalReference"`
	ExpiresAt         string `json:"expiresAt"`
	PaidAt            string `json:"paidAt"`
	CreatedAt         string `json:"createdAt"`
	CheckoutURL       string `json:"checkoutUrl"`
	QRURL             string `json:"qrUrl"`
}

// totalAmount mengembalikan total yang dibayar pelanggan (gross) — nominal
// tagihan QRIS memakai nilai ini.
func (p *lydevPayment) totalAmount() int {
	if p.ProviderAmount != nil {
		return *p.ProviderAmount
	}
	if p.Fee != nil {
		return p.Amount + *p.Fee
	}
	return p.Amount
}

// feeValue mengembalikan biaya layanan (0 bila tidak ada).
func (p *lydevPayment) feeValue() int {
	if p.Fee != nil {
		return *p.Fee
	}
	return 0
}

// CreateQRIS membuat tagihan QRIS baru lewat LYDEV Pay.
func CreateQRIS(amount int, description, apiKey string) (*lydevPayment, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("API key LYDEV Pay belum diset (LYDEV_API_KEY)")
	}
	if amount < lydevMinAmount {
		return nil, fmt.Errorf("nominal minimal %s", formatRupiah(lydevMinAmount))
	}
	if amount > lydevMaxAmount {
		return nil, fmt.Errorf("nominal maksimal %s", formatRupiah(lydevMaxAmount))
	}

	payload, err := json.Marshal(map[string]any{
		"amount":      amount,
		"currency":    "IDR",
		"description": description,
	})
	if err != nil {
		return nil, err
	}

	// Idempotency-Key unik per order; dipakai ulang saat retry agar aman.
	idemKey := newIdempotencyKey()

	var lastErr error
	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			time.Sleep(time.Duration(1<<attempt) * time.Second)
		}

		body, status, err := lydevRequest(http.MethodPost, lydevBaseURL+"/api/v1/payments", apiKey, idemKey, payload)
		if err != nil {
			lastErr = err
			continue
		}
		// 5xx (termasuk 503) aman di-retry dengan Idempotency-Key yang sama.
		if status >= http.StatusInternalServerError {
			lastErr = fmt.Errorf("API LYDEV Pay sedang bermasalah (HTTP %d)", status)
			continue
		}
		if status != http.StatusCreated && status != http.StatusOK {
			return nil, lydevError(body, status)
		}

		var payment lydevPayment
		if err := json.Unmarshal(body, &payment); err != nil {
			return nil, fmt.Errorf("gagal membaca respons LYDEV Pay: %w", err)
		}
		if payment.OrderID == "" {
			return nil, fmt.Errorf("API LYDEV Pay tidak mengembalikan order")
		}
		return &payment, nil
	}
	return nil, lastErr
}

// FetchPaymentStatus mengambil status terbaru sebuah payment.
func FetchPaymentStatus(orderID, apiKey string) (*lydevPayment, error) {
	apiURL := fmt.Sprintf("%s/api/v1/payments/%s/status", lydevBaseURL, url.PathEscape(orderID))

	body, status, err := lydevRequest(http.MethodGet, apiURL, apiKey, "", nil)
	if err != nil {
		return nil, err
	}
	if status != http.StatusOK {
		return nil, lydevError(body, status)
	}

	var payment lydevPayment
	if err := json.Unmarshal(body, &payment); err != nil {
		return nil, fmt.Errorf("gagal membaca respons LYDEV Pay: %w", err)
	}
	return &payment, nil
}

// FetchQRISImage mengunduh gambar PNG QRIS dari qrUrl.
func FetchQRISImage(qrURL, apiKey string) ([]byte, error) {
	body, status, err := lydevRequest(http.MethodGet, qrURL, apiKey, "", nil)
	if err != nil {
		return nil, err
	}
	if status != http.StatusOK {
		// Endpoint QR memakai body teks biasa, bukan JSON.
		msg := string(bytes.TrimSpace(body))
		if msg == "" {
			msg = fmt.Sprintf("HTTP %d", status)
		}
		return nil, fmt.Errorf("QR belum tersedia — %s", msg)
	}
	if len(body) == 0 {
		return nil, fmt.Errorf("QR kosong")
	}
	return body, nil
}

// lydevRequest menjalankan request ke API LYDEV Pay dengan header autentikasi.
func lydevRequest(method, apiURL, apiKey, idemKey string, payload []byte) ([]byte, int, error) {
	var reader io.Reader
	if payload != nil {
		reader = bytes.NewReader(payload)
	}

	req, err := http.NewRequest(method, apiURL, reader)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Accept", "application/json")
	if idemKey != "" {
		req.Header.Set("Idempotency-Key", idemKey)
	}
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("gagal menghubungi LYDEV Pay: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("gagal membaca respons LYDEV Pay: %w", err)
	}
	return body, resp.StatusCode, nil
}

// lydevError mengubah body error LYDEV Pay ({"error":"..."}) menjadi error Go.
func lydevError(body []byte, status int) error {
	var e struct {
		Error string `json:"error"`
	}
	if json.Unmarshal(body, &e) == nil && e.Error != "" {
		return fmt.Errorf("%s", e.Error)
	}
	return fmt.Errorf("API LYDEV Pay mengembalikan HTTP %d", status)
}

// newIdempotencyKey membuat Idempotency-Key unik dengan pola yang diizinkan
// ([A-Za-z0-9._:-]).
func newIdempotencyKey() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("wa-%d", time.Now().UnixNano())
	}
	return fmt.Sprintf("wa-%d-%s", time.Now().UnixNano(), hex.EncodeToString(b))
}

// parseLydevTime mengurai timestamp ISO 8601 dari LYDEV Pay.
func parseLydevTime(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339} {
		if t, err := time.Parse(layout, s); err == nil {
			return t
		}
	}
	return time.Time{}
}
