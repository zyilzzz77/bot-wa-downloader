#!/usr/bin/env bash
#
# bench.sh (versi ringan) — turunan dari "bench.sh" by Teddysun.
# Asli: https://github.com/teddysun/across/blob/master/bench.sh
# Copyright (C) 2015 - 2026 Teddysun <i@teddysun.com>
#
# Dipangkas & disesuaikan agar aman/cepat dijalankan dari container Alpine:
#   - I/O test diperkecil (~256MB total, aslinya ~3GB)
#   - node speedtest dikurangi (default + 4 node terdekat)
#   - tanpa 'clear' dan tanpa warna ANSI
#
# Env:
#   SPEEDTEST_BIN  path binary Ookla speedtest (default ./speedtest-cli/speedtest)

export LC_ALL=C

SPEEDTEST_BIN="${SPEEDTEST_BIN:-./speedtest-cli/speedtest}"
SPEEDTEST_TIMEOUT="${SPEEDTEST_TIMEOUT:-60}"
IO_COUNT="${IO_COUNT:-256}" # 256 * 512KB = 128MB per run

next() { printf "%-68s\n" "-" | sed 's/\s/-/g'; }

# --- System info ---
system_info() {
    local cname cores freq tram uram swap up load opsy kern arch tcpctrl dtotal dused

    cname=$(awk -F: '/model name/ {name=$2} END {print name}' /proc/cpuinfo | sed 's/^[ \t]*//;s/[ \t]*$//')
    [ -z "$cname" ] && cname=$(awk -F: '/Hardware/ {print $2; exit}' /proc/cpuinfo | sed 's/^[ \t]*//;s/[ \t]*$//')

    cores=$(nproc 2>/dev/null || grep -c '^processor' /proc/cpuinfo)
    freq=$(awk -F'[ :]' '/cpu MHz/ {print $4; exit}' /proc/cpuinfo)

    tram=$(LANG=C free -m 2>/dev/null | awk '/Mem/ {print $2}')
    uram=$(LANG=C free -m 2>/dev/null | awk '/Mem/ {print $3}')
    swap=$(LANG=C free -m 2>/dev/null | awk '/Swap/ {print $2}')

    up=$(awk '{d=int($1/86400); h=int(($1%86400)/3600); m=int(($1%3600)/60); printf "%d days, %d hour %d min", d, h, m}' /proc/uptime)
    load=$(awk '{print $1", "$2", "$3}' /proc/loadavg)

    opsy=$(awk -F= '/^PRETTY_NAME=/{gsub(/^"|"$/, "", $2); print $2}' /etc/os-release 2>/dev/null)
    kern=$(uname -r)
    arch=$(uname -m)
    tcpctrl=$(sysctl -n net.ipv4.tcp_congestion_control 2>/dev/null)

    dtotal=$(df -B1 / 2>/dev/null | awk 'NR==2 {printf "%.1f GB", $2/1073741824}')
    dused=$(df -B1 / 2>/dev/null | awk 'NR==2 {printf "%.1f GB", $3/1073741824}')

    echo " CPU Model        : ${cname:-tidak terdeteksi}"
    echo " CPU Cores        : ${cores}${freq:+ @ ${freq} MHz}"
    echo " Total Disk       : ${dtotal} (${dused} Used)"
    echo " Total RAM        : ${tram:-?} MB (${uram:-?} MB Used)"
    if [ -n "$swap" ] && [ "$swap" != "0" ]; then
        echo " Total Swap       : ${swap} MB"
    fi
    echo " Uptime           : ${up}"
    echo " Load Average     : ${load}"
    echo " OS               : ${opsy:-unknown}"
    echo " Arch             : ${arch}"
    echo " Kernel           : ${kern}"
    [ -n "$tcpctrl" ] && echo " TCP CC           : ${tcpctrl}"
}

# --- I/O (ringan) ---
io_test() {
    local count="${1}"
    ( LANG=C dd if=/dev/zero of=benchtest_$$ bs=512k count="${count}" conv=fsync 2>&1 \
        && rm -f benchtest_$$ ) 2>&1 \
        | awk -F '[,，]' '{io=$NF} END {print io}' | sed 's/^[ \t]*//;s/[ \t]*$//'
}

print_io() {
    local r1 r2
    r1=$(io_test "${IO_COUNT}")
    r2=$(io_test "${IO_COUNT}")
    echo " I/O Speed (128MB) : ${r1}"
    echo " I/O Speed (128MB) : ${r2}"
}

# --- Speed test (Ookla) ---
speed_test() {
    local id="${1}" name="${2}" log dl up lat

    if [ ! -x "${SPEEDTEST_BIN}" ]; then
        printf " %-16s : speedtest binary tidak ditemukan\n" "${name}"
        return
    fi

    log=$(mktemp)
    if [ -z "${id}" ]; then
        timeout "${SPEEDTEST_TIMEOUT}" "${SPEEDTEST_BIN}" --progress=no --accept-license --accept-gdpr >"${log}" 2>&1
    else
        timeout "${SPEEDTEST_TIMEOUT}" "${SPEEDTEST_BIN}" --progress=no --server-id="${id}" --accept-license --accept-gdpr >"${log}" 2>&1
    fi

    dl=$(awk '/Download/ {print $3" "$4}' "${log}")
    up=$(awk '/Upload/ {print $3" "$4}' "${log}")
    lat=$(awk '/Latency/ {print $3" "$4}' "${log}")
    rm -f "${log}"

    if [ -n "${dl}" ] && [ -n "${up}" ]; then
        printf " %-16s : ↑ %-14s ↓ %-14s (%s)\n" "${name}" "${up}" "${dl}" "${lat}"
    else
        printf " %-16s : gagal\n" "${name}"
    fi
}

print_speed() {
    speed_test ''      'Speedtest.net'
    speed_test '13623' 'Singapore'
    speed_test '32155' 'Hong Kong'
    speed_test '48463' 'Tokyo'
    speed_test '7190'  'Los Angeles'
}

# --- Main ---
echo "=================== Bench.sh (ringan) ==================="
next
system_info
next
print_io
next
print_speed
next
echo " Selesai: $(date '+%Y-%m-%d %H:%M:%S %Z')"
