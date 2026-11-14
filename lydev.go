package main

import (
	"encoding/json"
	"fmt"
	"net/http"
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
