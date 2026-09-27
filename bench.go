package main

import (
	"bytes"
	"context"
	_ "embed"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

//go:embed scripts/bench.sh
var benchScript string

const (
	// benchTimeout adalah batas maksimal jalannya benchmark.
	benchTimeout = 5 * time.Minute
	// benchSpeedtestPath adalah lokasi binary Ookla speedtest di image.
	benchSpeedtestPath = "/app/speedtest-cli/speedtest"
)

// ansiEscape mencocokkan kode warna ANSI supaya output bersih di WhatsApp.
var ansiEscape = regexp.MustCompile(`\x1b\[[0-9;]*[A-Za-z]`)

// runBenchmark menjalankan script bench.sh (versi ringan) dan mengembalikan outputnya.
func runBenchmark() (string, error) {
	if _, err := exec.LookPath("bash"); err != nil {
		return "", fmt.Errorf("bash tidak ditemukan di sistem")
	}

	ctx, cancel := context.WithTimeout(context.Background(), benchTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "bash", "-s")
	cmd.Stdin = strings.NewReader(strings.ReplaceAll(benchScript, "\r\n", "\n"))

	// I/O test menulis di working dir; pakai /app (image) bila ada.
	workDir := "/app"
	if _, err := os.Stat(workDir); err != nil {
		workDir = "."
	}
	cmd.Dir = workDir
	cmd.Env = append(cmd.Environ(),
		"LC_ALL=C",
		"SPEEDTEST_BIN="+benchSpeedtestPath,
	)

	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out

	runErr := cmd.Run()

	text := strings.TrimSpace(ansiEscape.ReplaceAllString(out.String(), ""))

	if ctx.Err() == context.DeadlineExceeded {
		return text, fmt.Errorf("benchmark melebihi batas waktu %s", benchTimeout)
	}
	if runErr != nil && text == "" {
		return "", fmt.Errorf("benchmark gagal: %w", runErr)
	}
	return text, runErr
}
