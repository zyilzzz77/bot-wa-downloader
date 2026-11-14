package main

import (
	"fmt"
	"strings"
)

// progressBar membuat bar progres 10 segmen dari persentase.
func progressBar(pct float64) string {
	const segments = 10
	filled := int(pct / 100 * segments)
	if filled < 0 {
		filled = 0
	}
	if filled > segments {
		filled = segments
	}
	return strings.Repeat("▰", filled) + strings.Repeat("▱", segments-filled)
}

// humanBytes memformat ukuran byte ke satuan yang mudah dibaca.
func humanBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	units := []string{"KB", "MB", "GB", "TB"}
	value := float64(n)
	i := -1
	for value >= unit && i < len(units)-1 {
		value /= unit
		i++
	}
	return fmt.Sprintf("%.1f %s", value, units[i])
}

// humanDuration memformat durasi (detik) ke teks ringkas.
func humanDuration(seconds float64) string {
	if seconds < 0 {
		return "?"
	}
	total := int(seconds + 0.5)
	if total < 60 {
		return fmt.Sprintf("%ds", total)
	}
	return fmt.Sprintf("%dm %ds", total/60, total%60)
}
