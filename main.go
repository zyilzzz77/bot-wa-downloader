package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	_ "modernc.org/sqlite"
	"github.com/mdp/qrterminal/v3"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	waLog "go.mau.fi/whatsmeow/util/log"
	"google.golang.org/protobuf/proto"
)

// --- Konfigurasi ---

var apiKey string

func init() {
	apiKey = os.Getenv("NEOXR_API_KEY")
	if apiKey == "" {
		fmt.Fprintf(os.Stderr, "WARNING: NEOXR_API_KEY tidak diset. Downloader tidak akan berfungsi.\n")
	}
}

// --- Constanta Media Type ---

const (
	whatsmeowMediaImage    = whatsmeow.MediaImage
	whatsmeowMediaVideo    = whatsmeow.MediaVideo
	whatsmeowMediaAudio    = whatsmeow.MediaAudio
	whatsmeowMediaDocument = whatsmeow.MediaDocument
)

// --- Wrapper WhatsApp Client ---

// ClientWrapper membungkus *whatsmeow.Client dengan method helper.
type ClientWrapper struct {
	*whatsmeow.Client
}

// SendText mengirim pesan teks dan mengembalikan respon untuk keperluan edit.
func (c *ClientWrapper) SendText(ctx context.Context, jid types.JID, text string) *whatsmeow.SendResponse {
	resp, err := c.SendMessage(ctx, jid, &waE2E.Message{
		ExtendedTextMessage: &waE2E.ExtendedTextMessage{
			Text: proto.String(text),
		},
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "[send] gagal kirim teks: %v\n", err)
		return nil
	}
	return &resp
}

// EditText mengedit pesan yang sudah dikirim sebelumnya.
func (c *ClientWrapper) EditText(ctx context.Context, jid types.JID, msgID, newText string) {
	editMsg := c.BuildEdit(jid, types.MessageID(msgID), &waE2E.Message{
		ExtendedTextMessage: &waE2E.ExtendedTextMessage{
			Text: proto.String(newText),
		},
	})
	_, err := c.SendMessage(ctx, jid, editMsg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[edit] gagal edit pesan: %v\n", err)
	}
}

// --- Entry Point ---

func main() {
	flag.Parse()

	ctx := context.Background()

	// Buat direktori data
	dataDir := "data"
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Gagal membuat direktori data: %v\n", err)
		os.Exit(1)
	}
	dbPath := filepath.Join(dataDir, "whatsapp.db")

	// Inisialisasi SQLite store
	rawDB, err := sql.Open("sqlite", fmt.Sprintf("file:%s?_journal_mode=WAL&_busy_timeout=5000", dbPath))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Gagal membuka database: %v\n", err)
		os.Exit(1)
	}
	if _, err := rawDB.Exec("PRAGMA foreign_keys = ON"); err != nil {
		fmt.Fprintf(os.Stderr, "Gagal enable foreign keys: %v\n", err)
		os.Exit(1)
	}
	container := sqlstore.NewWithDB(rawDB, "sqlite", waLog.Stdout("db", "DEBUG", true))
	if err := container.Upgrade(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "Gagal upgrade database: %v\n", err)
		os.Exit(1)
	}

	deviceStore, err := container.GetFirstDevice(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Gagal mendapatkan device: %v\n", err)
		os.Exit(1)
	}

	waClient := whatsmeow.NewClient(deviceStore, waLog.Stdout("wa", "INFO", true))

	// --- Mode: Normal bot ---
	wrapper := &ClientWrapper{Client: waClient}

	waClient.AddEventHandler(func(evt interface{}) {
		switch v := evt.(type) {
		case *events.Message:
			handleMessage(wrapper, v)
		}
	})

	fmt.Println("╔══════════════════════════════════════════════╗")
	fmt.Println("║ BOT WA DOWNLOADER TT, IG, TERABOX & THREADS ║")
	fmt.Println("╚══════════════════════════════════════════════╝")
	fmt.Println()

	if waClient.Store.ID == nil {
		fmt.Println("Belum login — scan QR di bawah pakai HP bot:")
		fmt.Println("  WhatsApp → Pengaturan / Perangkat Tertaut → Tautkan Perangkat")
		fmt.Println()
		qrChan, _ := waClient.GetQRChannel(ctx)
		if err := waClient.Connect(); err != nil {
			fmt.Fprintf(os.Stderr, "Gagal koneksi: %v\n", err)
			os.Exit(1)
		}
		for evt := range qrChan {
			if evt.Event == "code" {
				qrterminal.GenerateHalfBlock(evt.Code, qrterminal.L, os.Stdout)
				fmt.Println()
			} else {
				fmt.Printf("QR: %s\n", evt.Event)
			}
		}
		fmt.Println("✓ Login QR berhasil!")
	} else {
		if err := waClient.Connect(); err != nil {
			fmt.Fprintf(os.Stderr, "Gagal koneksi: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("✓ Bot sudah login dan siap digunakan!")
	}

	fmt.Println("Tekan Ctrl+C untuk keluar…")

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	<-sigCh

	fmt.Println("\nMematikan bot…")
	waClient.Disconnect()
	fmt.Println("Bot berhenti.")
}
