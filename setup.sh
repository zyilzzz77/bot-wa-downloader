#!/bin/bash
set -e

echo "== Setup botgodownloader =="
echo ""

# Check env vars
MISSING=""
[ -z "$NEOXR_API_KEY" ] && MISSING="$MISSING NEOXR_API_KEY"

if [ -n "$MISSING" ]; then
  echo "ERROR: Environment variable belum diset:$MISSING"
  echo ""
  echo "Cara pakai:"
  echo "  export NEOXR_API_KEY=api_key_kamu"
  echo "  ./setup.sh"
  echo ""
  echo "Setelah setup, pairing WhatsApp:"
  echo "  docker compose exec botgodownloader ./botgodownloader -pair 628xxx"
  exit 1
fi

# Generate .env
cat > .env <<EOF
NEOXR_API_KEY=$NEOXR_API_KEY
EOF

# PHONE_NUMBER optional — untuk auto-pairing
if [ -n "$PHONE_NUMBER" ]; then
  echo "PHONE_NUMBER=$PHONE_NUMBER" >> .env
fi

echo "✓ .env berhasil dibuat!"
echo "  NEOXR_API_KEY = ${NEOXR_API_KEY:0:6}..."
[ -n "$PHONE_NUMBER" ] && echo "  PHONE_NUMBER   = $PHONE_NUMBER (auto-pairing)" || echo "  PHONE_NUMBER   = (pairing manual)"
echo ""
echo "Selanjutnya:"
echo "  docker compose up -d --build"
echo ""
echo "Pairing WhatsApp (kalau belum set PHONE_NUMBER):"
echo "  docker compose exec botgodownloader ./botgodownloader -pair 628xxx"
echo ""
echo "Cek log:"
echo "  docker compose logs -f"
