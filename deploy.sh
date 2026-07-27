#!/bin/bash
set -e
cd "$(dirname "$0")"

echo "=== DEPLOY BOT WA KE VPS ==="

# --- Build image ---
echo "[1/3] Build Docker image..."
docker compose build

# --- Save image ---
echo "[2/3] Export image..."
docker save botgodownloader-botgodownloader | gzip > botgodownloader.tar.gz
echo "  ✓ botgodownloader.tar.gz ($(du -h botgodownloader.tar.gz | cut -f1))"

# --- Upload + run di VPS ---
echo ""
echo "[3/3] Upload ke VPS & start..."
echo ""
echo "  scp botgodownloader.tar.gz docker-compose.yml root@<IP-VPS>:/opt/botgodownloader/"
echo ""
echo "  # SSH ke VPS lalu:"
echo "  cd /opt/botgodownloader"
echo "  docker load < botgodownloader.tar.gz"
echo "  docker compose up -d"
echo "  docker compose logs -f"
