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
	"time"

	_ "modernc.org/sqlite"
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
	whatsmeowMediaImage = whatsmeow.MediaImage
	whatsmeowMediaVideo = whatsmeow.MediaVideo
	whatsmeowMediaAudio = whatsmeow.MediaAudio
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
	pairPhone := flag.String("pair", "", "Pairing dengan nomor HP (contoh: -pair 6281234567890)")
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

	// --- Mode: Pairing command ---
	if *pairPhone != "" {
		pairAndExit(waClient, *pairPhone)
		return
	}

	// --- Mode: Normal bot ---
	wrapper := &ClientWrapper{Client: waClient}

	waClient.AddEventHandler(func(evt interface{}) {
		switch v := evt.(type) {
		case *events.Message:
			handleMessage(wrapper, v)
		}
	})

	fmt.Println("╔══════════════════════════════════════════════╗")
	fmt.Println("║   BOT WA DOWNLOADER TIKTOK & INSTAGRAM      ║")
	fmt.Println("╚══════════════════════════════════════════════╝")
	fmt.Println()

	if waClient.Store.ID == nil {
		// Belum login — cek PHONE_NUMBER untuk auto-pairing
		phoneEnv := os.Getenv("PHONE_NUMBER")
		if phoneEnv == "" {
			fmt.Fprintln(os.Stderr, "❌ Bot belum login dan PHONE_NUMBER tidak diset.")
			fmt.Fprintln(os.Stderr, "   Jalankan command pairing:")
			fmt.Fprintln(os.Stderr, "   docker compose exec botgodownloader ./botgodownloader -pair 628xxx")
			fmt.Fprintln(os.Stderr, "")
			fmt.Fprintln(os.Stderr, "   Atau set PHONE_NUMBER di .env untuk auto-pairing.")
			os.Exit(1)
		}
		fmt.Printf("Auto-pairing dengan nomor: %s\n", phoneEnv)
		loginWithPairingPhone(waClient, phoneEnv)
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

// pairAndExit melakukan pairing code lalu keluar (untuk command -pair).
func pairAndExit(client *whatsmeow.Client, phone string) {
	fmt.Printf("Pairing code dengan nomor: %s\n", phone)
	loginWithPairingPhone(client, phone)
	fmt.Println("Session tersimpan. Restart bot untuk mulai:")
	fmt.Println("  docker compose restart botgodownloader")
}

// loginWithPairingPhone melakukan pairing code dengan nomor HP yang diberikan.
func loginWithPairingPhone(client *whatsmeow.Client, phone string) {
	if err := client.Connect(); err != nil {
		fmt.Fprintf(os.Stderr, "Gagal koneksi: %v\n", err)
		os.Exit(1)
	}

	ctx := context.Background()
	code, err := client.PairPhone(ctx, phone, true, whatsmeow.PairClientChrome, "Chrome (Linux)")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Gagal pairing: %v\n", err)
		os.Exit(1)
	}

	fmt.Println()
	fmt.Println("╔══════════════════════════════════╗")
	fmt.Printf("║   KODE PAIRING: %-16s  ║\n", code)
	fmt.Println("╚══════════════════════════════════╝")
	fmt.Println()
	fmt.Println("Buka WhatsApp di HP:")
	fmt.Println("  Perangkat Tertaut → Tautkan Perangkat → Tautkan dengan Nomor Telepon")
	fmt.Println("  Masukkan kode di atas.")
	fmt.Println()

	fmt.Println("Menunggu pairing disetujui di HP…")
	for client.Store.ID == nil {
		time.Sleep(1 * time.Second)
	}
	fmt.Println("✓ Pairing berhasil!")
}
