package main

import (
	"context"
	"log"
	"strings"

	"go.mau.fi/whatsmeow/types"
)

// --- Command /server (status server) ---

// parseServerCommand memeriksa command server (dengan atau tanpa slash).
func parseServerCommand(text string) bool {
	switch strings.ToLower(strings.TrimSpace(text)) {
	case "server", "/server", "!server":
		return true
	}
	return false
}

// handleServer mengirim ringkasan status server ke chat.
func handleServer(client *ClientWrapper, chatJID types.JID) {
	log.Printf("[server] Mengambil status server")
	ctx := context.Background()

	processingMsg := client.SendText(ctx, chatJID, "⏳ *Mengambil status server…*")

	report := buildServerReport()

	if processingMsg != nil {
		client.EditText(ctx, chatJID, processingMsg.ID, report)
	} else {
		client.SendText(ctx, chatJID, report)
	}
	log.Printf("[server] ✓ Status terkirim")
}
