#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
OUT="$ROOT/dist/gabutray-linux-amd64"
GO_BIN="${GO_BIN:-go}"

XRAY="$ROOT/third_party/bin/linux-amd64/xray"
TUN2SOCKS="$ROOT/third_party/bin/linux-amd64/tun2socks"
GEOIP="$ROOT/third_party/bin/linux-amd64/geoip.dat"
GEOSITE="$ROOT/third_party/bin/linux-amd64/geosite.dat"

if [[ ! -x "$XRAY" ]]; then
  echo "missing executable: $XRAY" >&2
  exit 1
fi
if [[ ! -x "$TUN2SOCKS" ]]; then
  echo "missing executable: $TUN2SOCKS" >&2
  exit 1
fi
if [[ ! -f "$GEOIP" ]]; then
  echo "missing file: $GEOIP" >&2
  exit 1
fi
if [[ ! -f "$GEOSITE" ]]; then
  echo "missing file: $GEOSITE" >&2
  exit 1
fi

rm -rf "$OUT"
mkdir -p "$OUT/engines"

GOOS=linux GOARCH=amd64 CGO_ENABLED=0 "$GO_BIN" build -o "$OUT/gabutray" "$ROOT/cmd/gabutray"
cp "$XRAY" "$OUT/engines/xray"
cp "$TUN2SOCKS" "$OUT/engines/tun2socks"
cp "$GEOIP" "$OUT/engines/geoip.dat"
cp "$GEOSITE" "$OUT/engines/geosite.dat"
cp "$ROOT/scripts/install.sh" "$OUT/install.sh"
cp "$ROOT/scripts/uninstall.sh" "$OUT/uninstall.sh"
cp "$ROOT/README.md" "$OUT/README.md"
chmod +x "$OUT/gabutray" "$OUT/engines/xray" "$OUT/engines/tun2socks" "$OUT/install.sh" "$OUT/uninstall.sh"

tar -C "$ROOT/dist" -czf "$ROOT/dist/gabutray-linux-amd64.tar.gz" "gabutray-linux-amd64"
(
  cd "$ROOT/dist"
  sha256sum "gabutray-linux-amd64.tar.gz" > "gabutray-linux-amd64.tar.gz.sha256"
)

echo "created: $ROOT/dist/gabutray-linux-amd64.tar.gz"
echo "created: $ROOT/dist/gabutray-linux-amd64.tar.gz.sha256"
cat "$ROOT/dist/gabutray-linux-amd64.tar.gz.sha256"
