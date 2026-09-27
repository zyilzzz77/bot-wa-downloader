package main

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/host"
	"github.com/shirou/gopsutil/v4/load"
	"github.com/shirou/gopsutil/v4/mem"
	gnet "github.com/shirou/gopsutil/v4/net"
)

const (
	// speedTestURL mengunduh 5 MB dari Cloudflare untuk mengukur kecepatan.
	speedTestURL     = "https://speed.cloudflare.com/__down?bytes=5000000"
	speedTestTimeout = 15 * time.Second
	latencyTimeout   = 3 * time.Second
)

// latencyTargets adalah host:port untuk mengukur latency koneksi.
var latencyTargets = []string{"1.1.1.1:53", "8.8.8.8:53"}

// buildServerReport mengumpulkan metrik server lalu menyusunnya jadi teks.
func buildServerReport() string {
	var sb strings.Builder
	sb.WriteString("🖥 *Status Server*\n")

	if info, err := host.Info(); err == nil {
		fmt.Fprintf(&sb, "\n🏠 *Host:* %s\n", info.Hostname)
		fmt.Fprintf(&sb, "💻 *OS:* %s %s (%s/%s)\n",
			titleCase(info.Platform), info.PlatformVersion, info.KernelArch, runtime.GOOS)
		fmt.Fprintf(&sb, "🧠 *Kernel:* %s\n", info.KernelVersion)
		fmt.Fprintf(&sb, "⏱ *Uptime:* %s\n", formatUptime(info.Uptime))
	}

	sb.WriteString("\n⚙️ *CPU*\n")
	if infos, err := cpu.Info(); err == nil && len(infos) > 0 {
		fmt.Fprintf(&sb, "• Model: %s\n", strings.TrimSpace(infos[0].ModelName))
	}
	logical, _ := cpu.Counts(true)
	physical, _ := cpu.Counts(false)
	fmt.Fprintf(&sb, "• Core: %d logical, %d physical\n", logical, physical)
	if pct, err := cpu.Percent(time.Second, false); err == nil && len(pct) > 0 {
		fmt.Fprintf(&sb, "• Usage: %.1f%%\n", pct[0])
	}

	if avg, err := load.Avg(); err == nil {
		fmt.Fprintf(&sb, "\n📊 *Load Average:* %.2f %.2f %.2f\n", avg.Load1, avg.Load5, avg.Load15)
	}

	if vm, err := mem.VirtualMemory(); err == nil {
		sb.WriteString("\n🧮 *RAM*\n")
		fmt.Fprintf(&sb, "• Terpakai: %s / %s (%.1f%%)\n",
			humanBytes(int64(vm.Used)), humanBytes(int64(vm.Total)), vm.UsedPercent)
		fmt.Fprintf(&sb, "• Tersedia: %s\n", humanBytes(int64(vm.Available)))
	}

	if sw, err := mem.SwapMemory(); err == nil && sw.Total > 0 {
		fmt.Fprintf(&sb, "\n💾 *Swap:* %s / %s (%.1f%%)\n",
			humanBytes(int64(sw.Used)), humanBytes(int64(sw.Total)), sw.UsedPercent)
	}

	if usage, err := disk.Usage(rootDiskPath()); err == nil {
		fmt.Fprintf(&sb, "\n💽 *Disk (%s)*\n", rootDiskPath())
		fmt.Fprintf(&sb, "• Terpakai: %s / %s (%.1f%%)\n",
			humanBytes(int64(usage.Used)), humanBytes(int64(usage.Total)), usage.UsedPercent)
		fmt.Fprintf(&sb, "• Sisa: %s\n", humanBytes(int64(usage.Free)))
	}

	if counters, err := gnet.IOCounters(false); err == nil && len(counters) > 0 {
		sb.WriteString("\n🌐 *Network (total)*\n")
		fmt.Fprintf(&sb, "• ⬇️ Terima: %s\n", humanBytes(int64(counters[0].BytesRecv)))
		fmt.Fprintf(&sb, "• ⬆️ Kirim: %s\n", humanBytes(int64(counters[0].BytesSent)))
	}

	sb.WriteString("\n📡 *Internet*\n")
	if latency, ok := measureLatency(); ok {
		sb.WriteString("• Koneksi: ✅ online\n")
		fmt.Fprintf(&sb, "• Latency: %d ms\n", latency.Milliseconds())
	} else {
		sb.WriteString("• Koneksi: ❌ offline\n")
	}
	if mbps, err := speedTest(); err == nil {
		fmt.Fprintf(&sb, "• Download: %.1f Mbps\n", mbps)
	} else {
		fmt.Fprintf(&sb, "• Download: gagal diukur (%v)\n", err)
	}

	sb.WriteString("\n🤖 *Proses Bot*\n")
	fmt.Fprintf(&sb, "• Goroutine: %d\n", runtime.NumGoroutine())
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	fmt.Fprintf(&sb, "• Memori: %s (sys %s)\n", humanBytes(int64(ms.Alloc)), humanBytes(int64(ms.Sys)))

	return sb.String()
}

// rootDiskPath mengembalikan path disk yang dicek (drive sistem di Windows).
func rootDiskPath() string {
	if runtime.GOOS == "windows" {
		if drive := os.Getenv("SystemDrive"); drive != "" {
			return drive + "\\"
		}
		return "C:\\"
	}
	return "/"
}

// formatUptime memformat durasi uptime (detik) ke "3d 4h 12m".
func formatUptime(seconds uint64) string {
	days := seconds / 86400
	hours := (seconds % 86400) / 3600
	mins := (seconds % 3600) / 60

	parts := make([]string, 0, 3)
	if days > 0 {
		parts = append(parts, fmt.Sprintf("%dd", days))
	}
	if hours > 0 || days > 0 {
		parts = append(parts, fmt.Sprintf("%dh", hours))
	}
	parts = append(parts, fmt.Sprintf("%dm", mins))
	return strings.Join(parts, " ")
}

// measureLatency mengukur latency TCP ke target pertama yang bisa dihubungi.
func measureLatency() (time.Duration, bool) {
	for _, target := range latencyTargets {
		start := time.Now()
		conn, err := net.DialTimeout("tcp", target, latencyTimeout)
		if err == nil {
			conn.Close()
			return time.Since(start), true
		}
	}
	return 0, false
}

// speedTest mengukur kecepatan unduh (Mbps) dengan mengunduh file uji.
func speedTest() (float64, error) {
	client := &http.Client{Timeout: speedTestTimeout}

	start := time.Now()
	resp, err := client.Get(speedTestURL)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	n, err := io.Copy(io.Discard, resp.Body)
	elapsed := time.Since(start).Seconds()

	if n == 0 {
		if err != nil {
			return 0, err
		}
		return 0, fmt.Errorf("tidak ada data terunduh")
	}
	if elapsed <= 0 {
		return 0, fmt.Errorf("durasi tidak valid")
	}
	return float64(n) * 8 / 1e6 / elapsed, nil
}
