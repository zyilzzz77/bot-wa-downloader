package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"go.mau.fi/whatsmeow/types"
)

// --- Helper download file dengan progres ---

const (
	// downloadProgressDelay adalah jeda sebelum pesan progres pertama dikirim,
	// agar unduhan yang cepat tidak menambah pesan baru.
	downloadProgressDelay = 2 * time.Second
	// downloadProgressInterval adalah jeda minimum antar edit pesan progres.
	downloadProgressInterval = 3 * time.Second
)

// downloadFileWithProgress mengunduh file sambil melaporkan progres (persentase,
// kecepatan, dan estimasi waktu tersisa) lewat pesan WhatsApp yang diedit
// berkala. Pesan hanya muncul bila unduhan berjalan lambat, sehingga unduhan
// kecil/cepat tidak menambah pesan. Progres tetap di-update saat koneksi
// tersendat, sehingga estimasi ikut menyesuaikan.
func downloadFileWithProgress(client *ClientWrapper, ctx context.Context, chatJID types.JID, rawURL, label string) ([]byte, string, error) {
	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, "", fmt.Errorf("link unduhan tidak valid")
	}
	req.Header.Set("User-Agent", apiUserAgent)

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("gagal mengunduh: %s", networkErrorText(err))
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, "", fmt.Errorf("server media menolak unduhan (HTTP %d) — coba lagi nanti", resp.StatusCode)
	}

	mimeType := resp.Header.Get("Content-Type")
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}

	total := resp.ContentLength
	start := time.Now()

	var downloaded int64
	var msgID string

	stop := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()

		timer := time.NewTimer(downloadProgressDelay)
		defer timer.Stop()
		select {
		case <-stop:
			return
		case <-timer.C:
		}

		first := client.SendText(ctx, chatJID,
			progressText(label, atomic.LoadInt64(&downloaded), total, time.Since(start)))
		if first == nil {
			return
		}
		msgID = first.ID

		ticker := time.NewTicker(downloadProgressInterval)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
				client.EditText(ctx, chatJID, msgID,
					progressText(label, atomic.LoadInt64(&downloaded), total, time.Since(start)))
			}
		}
	}()

	var buf bytes.Buffer
	chunk := make([]byte, 32*1024)
	for {
		n, readErr := resp.Body.Read(chunk)
		if n > 0 {
			buf.Write(chunk[:n])
			atomic.AddInt64(&downloaded, int64(n))
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			close(stop)
			wg.Wait()
			if msgID != "" {
				client.EditText(ctx, chatJID, msgID,
					fmt.Sprintf("❌ *Gagal mendownload %s* — koneksi terputus", label))
			}
			return nil, "", fmt.Errorf("gagal membaca: %w", readErr)
		}
	}

	close(stop)
	wg.Wait()

	if msgID != "" {
		client.EditText(ctx, chatJID, msgID,
			fmt.Sprintf("✅ *%s* terunduh — %.1f MB dalam %s",
				label, float64(buf.Len())/(1024*1024), humanDuration(time.Since(start).Seconds())))
	}

	return buf.Bytes(), mimeType, nil
}

// progressText menyusun teks progres unduhan: bar, persentase, ukuran,
// kecepatan, dan estimasi waktu tersisa.
func progressText(label string, downloaded, total int64, elapsed time.Duration) string {
	secs := elapsed.Seconds()
	if secs <= 0 {
		secs = 0.001
	}
	speed := float64(downloaded) / secs

	var sb strings.Builder
	fmt.Fprintf(&sb, "⬇️ *Mendownload %s…*\n\n", label)

	if total > 0 {
		pct := float64(downloaded) / float64(total) * 100
		fmt.Fprintf(&sb, "%s  %.0f%%\n", progressBar(pct), pct)
		fmt.Fprintf(&sb, "📦 %s / %s\n", humanBytes(downloaded), humanBytes(total))
	} else {
		fmt.Fprintf(&sb, "📦 %s terunduh\n", humanBytes(downloaded))
	}

	fmt.Fprintf(&sb, "🚀 %s/detik", humanBytes(int64(speed)))

	if total > 0 && speed > 0 && total > downloaded {
		eta := float64(total-downloaded) / speed
		fmt.Fprintf(&sb, "\n⏳ Estimasi: %s", humanDuration(eta))
	}
	return sb.String()
}
