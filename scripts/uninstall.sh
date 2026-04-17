#!/usr/bin/env bash
set -euo pipefail

systemctl disable --now gabutrayd.service 2>/dev/null || true
rm -f /etc/systemd/system/gabutrayd.service
systemctl daemon-reload
rm -f /usr/local/bin/gabutray
rm -rf /opt/gabutray
echo "uninstalled Gabutray"
