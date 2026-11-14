package main

import (
	"context"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/types"
)

// editOrSend mengedit pesan status kalau ada, atau mengirim pesan baru.
func editOrSend(client *ClientWrapper, ctx context.Context, chatJID types.JID, processingMsg *whatsmeow.SendResponse, text string) {
	if processingMsg != nil {
		client.EditText(ctx, chatJID, processingMsg.ID, text)
		return
	}
	client.SendText(ctx, chatJID, text)
}
