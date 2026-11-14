package main

import (
	"context"
	"fmt"
	"log"
	"strings"

	"go.mau.fi/whatsmeow/types"
)

// --- Command /bench (benchmark server ala bench.sh) ---

// parseBenchCommand memeriksa command bench (dengan atau tanpa slash).
func parseBenchCommand(text string) bool {
	switch strings.ToLower(strings.TrimSpace(text)) {
	case "bench", "/bench", "!bench":
		return true
	}
	return false
}

// handleBench menjalankan benchmark lalu mengirim hasilnya ke chat.
func handleBench(client *ClientWrapper, chatJID types.JID) {
	log.Printf("[bench] Menjalankan benchmark")
	ctx := context.Background()

	processingMsg := client.SendText(ctx, chatJID,
		"⏳ *Menjalankan benchmark…*\n\n_Info sistem + I/O + speedtest. Butuh 1–2 menit._")

	report, err := runBenchmark()
	if err != nil {
		log.Printf("[bench] ⚠️ %v", err)
		if report == "" {
			report = err.Error()
		} else {
			report = fmt.Sprintf("%s\n\n⚠️ %v", report, err)
		}
	}

	text := "📊 *Benchmark Server*\n\n```\n" + report + "\n```"
	if processingMsg != nil {
		client.EditText(ctx, chatJID, processingMsg.ID, text)
	} else {
		client.SendText(ctx, chatJID, text)
	}
	log.Printf("[bench] ✓ Selesai")
}
