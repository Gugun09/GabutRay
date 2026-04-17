#!/usr/bin/env bash
set -euo pipefail

ENABLE=0
if [[ "${1:-}" == "--enable" ]]; then
  ENABLE=1
fi

SRC="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PREFIX="/opt/gabutray"

install -d "$PREFIX/engines"
install -m 0755 "$SRC/gabutray" "$PREFIX/gabutray"
install -m 0755 "$SRC/engines/xray" "$PREFIX/engines/xray"
install -m 0755 "$SRC/engines/tun2socks" "$PREFIX/engines/tun2socks"
install -m 0644 "$SRC/engines/geoip.dat" "$PREFIX/engines/geoip.dat"
install -m 0644 "$SRC/engines/geosite.dat" "$PREFIX/engines/geosite.dat"
ln -sf "$PREFIX/gabutray" /usr/local/bin/gabutray

cat >/etc/systemd/system/gabutrayd.service <<'UNIT'
[Unit]
Description=Gabutray Xray/V2Ray TUN daemon
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=/opt/gabutray/gabutray daemon
Restart=on-failure
RestartSec=2
RuntimeDirectory=gabutray

[Install]
WantedBy=multi-user.target
UNIT

systemctl daemon-reload
if [[ "$ENABLE" == "1" ]]; then
  systemctl enable --now gabutrayd.service
fi

echo "installed Gabutray to $PREFIX"
