package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// stickerFilter menyetel media ke kanvas sticker WhatsApp 512x512 tanpa
// mengubah rasio aspek, sisa ruang diisi transparan.
const stickerFilter = "scale=512:512:force_original_aspect_ratio=decrease,format=rgba,pad=512:512:(ow-iw)/2:(oh-ih)/2:color=0x00000000"

// convertToStickerWebP mengubah gambar (brat) atau video (bratvid) menjadi WebP
// yang siap dikirim sebagai sticker WhatsApp. Untuk animasi, video dipotong
// 3 detik pada 15 fps dan di-loop.
func convertToStickerWebP(data []byte, animated bool) ([]byte, error) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		return nil, fmt.Errorf("ffmpeg tidak ditemukan di sistem — wajib untuk membuat sticker")
	}

	inputExt := "png"
	if animated {
		inputExt = "mp4"
	}

	inFile, err := os.CreateTemp("", "sticker-in-*."+inputExt)
	if err != nil {
		return nil, fmt.Errorf("gagal membuat file sementara: %w", err)
	}
	defer os.Remove(inFile.Name())
	if _, err := inFile.Write(data); err != nil {
		inFile.Close()
		return nil, fmt.Errorf("gagal menulis file sementara: %w", err)
	}
	inFile.Close()

	outFile, err := os.CreateTemp("", "sticker-out-*.webp")
	if err != nil {
		return nil, fmt.Errorf("gagal membuat file sementara: %w", err)
	}
	outName := outFile.Name()
	outFile.Close()
	defer os.Remove(outName)

	args := []string{"-y", "-hide_banner", "-loglevel", "error", "-i", inFile.Name()}
	if animated {
		args = append(args,
			"-t", "3",
			"-vf", "fps=15,"+stickerFilter,
			"-c:v", "libwebp",
			"-lossless", "0",
			"-q:v", "50",
			"-compression_level", "4",
			"-loop", "0",
			"-an",
		)
	} else {
		args = append(args,
			"-vf", stickerFilter,
			"-c:v", "libwebp",
			"-lossless", "0",
			"-q:v", "75",
			"-preset", "default",
		)
	}
	args = append(args, "-f", "webp", outName)

	cmd := exec.Command("ffmpeg", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("ffmpeg gagal mengonversi ke sticker: %w (%s)", err, strings.TrimSpace(stderr.String()))
	}

	webp, err := os.ReadFile(outName)
	if err != nil {
		return nil, fmt.Errorf("gagal membaca hasil sticker: %w", err)
	}
	if len(webp) == 0 {
		return nil, fmt.Errorf("ffmpeg tidak menghasilkan sticker")
	}
	return webp, nil
}
