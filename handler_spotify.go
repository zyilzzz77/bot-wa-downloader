package main

import (
	"context"
	"fmt"
	"log"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/types"
)

// --- Spotify ---

// handleSpotify mengunduh satu track atau seluruh track dalam playlist Spotify
// lalu mengirimnya sebagai audio, sambil memperbarui satu pesan status.
func handleSpotify(client *ClientWrapper, chatJID types.JID, spotifyURL string) {
	log.Printf("[spotify] Menerima link %s", spotifyURL)
	ctx := context.Background()

	processingMsg := client.SendText(ctx, chatJID, "⏳ *Memproses Spotify…*")

	listing, err := FetchSpotify(spotifyURL, apiKey)
	if err != nil {
		log.Printf("[spotify] ❌ Gagal: %v", err)
		editOrSend(client, ctx, chatJID, processingMsg,
			fmt.Sprintf("❌ Gagal download *Spotify*\n\n%s", err.Error()))
		return
	}

	if !listing.IsPlaylist {
		handleSpotifySingle(client, ctx, chatJID, processingMsg, listing.Tracks[0])
		return
	}

	handleSpotifyPlaylist(client, ctx, chatJID, processingMsg, listing)
}

// handleSpotifySingle mengunduh dan mengirim satu track Spotify.
func handleSpotifySingle(client *ClientWrapper, ctx context.Context, chatJID types.JID, processingMsg *whatsmeow.SendResponse, item SpotifyItem) {
	title, err := downloadAndSendSpotifyTrack(client, ctx, chatJID, item)
	if err != nil {
		log.Printf("[spotify] ❌ Gagal: %v", err)
		editOrSend(client, ctx, chatJID, processingMsg,
			fmt.Sprintf("❌ Gagal download *Spotify*\n\n%s", err.Error()))
		return
	}

	artists := item.Artists
	duration := item.Duration
	summary := fmt.Sprintf("✅ *Track Spotify*\n\n🎵 *Judul:* %s", title)
	if artists != "" {
		summary += fmt.Sprintf("\n🎤 *Artis:* %s", artists)
	}
	if duration != "" {
		summary += fmt.Sprintf("\n⏱ *Durasi:* %s", duration)
	}
	summary += "\n\n📥 _Audio terkirim._"
	editOrSend(client, ctx, chatJID, processingMsg, summary)
}

// handleSpotifyPlaylist mengunduh semua track playlist satu per satu.
func handleSpotifyPlaylist(client *ClientWrapper, ctx context.Context, chatJID types.JID, processingMsg *whatsmeow.SendResponse, listing *SpotifyListing) {
	total := len(listing.Tracks)
	title := listing.Title
	if title == "" {
		title = "Playlist"
	}

	header := fmt.Sprintf("🎧 *Spotify Playlist*\n\n📀 *Judul:* %s\n🎵 *Jumlah track:* %d", title, total)
	editOrSend(client, ctx, chatJID, processingMsg, header+"\n\n📥 _Mengirim audio…_")

	sent := 0
	var failed []string
	for i, item := range listing.Tracks {
		editOrSend(client, ctx, chatJID, processingMsg,
			fmt.Sprintf("%s\n\n📥 _Mengunduh %d/%d — %s…_", header, i+1, total, item.Title))

		if _, err := downloadAndSendSpotifyTrack(client, ctx, chatJID, item); err != nil {
			log.Printf("[spotify] ❌ Gagal %s: %v", item.Title, err)
			failed = append(failed, item.Title)
			continue
		}
		sent++
	}

	final := fmt.Sprintf("%s\n\n✅ _Selesai — %d/%d track terkirim._", header, sent, total)
	if len(failed) > 0 {
		const maxListed = 5
		final += fmt.Sprintf("\n\n❌ *Gagal:* %d track", len(failed))
		for i, name := range failed {
			if i == maxListed {
				final += fmt.Sprintf("\n• …dan %d lainnya", len(failed)-maxListed)
				break
			}
			final += fmt.Sprintf("\n• %s", name)
		}
	}
	editOrSend(client, ctx, chatJID, processingMsg, final)
	log.Printf("[spotify] ✓ Playlist selesai: %d/%d track terkirim", sent, total)
}

// downloadAndSendSpotifyTrack memastikan link audio track tersedia, mengunduh
// audionya, lalu mengirimnya ke chat. Mengembalikan judul yang dipakai.
func downloadAndSendSpotifyTrack(client *ClientWrapper, ctx context.Context, chatJID types.JID, item SpotifyItem) (string, error) {
	downloadURL := item.DownloadURL
	title := item.Title

	if downloadURL == "" {
		var err error
		downloadURL, title, _, _, err = DownloadSpotifyTrack(item.URL, item.Title, apiKey)
		if err != nil {
			return title, err
		}
	}

	data, mimeType, err := downloadFileWithProgress(client, ctx, chatJID, downloadURL, "audio — "+title)
	if err != nil {
		return title, err
	}

	log.Printf("[spotify] ✓ %s — %dKB (%s)", title, len(data)/1024, mimeType)
	uploadAndSendAudio(client, ctx, chatJID, data, mimeType)
	return title, nil
}
