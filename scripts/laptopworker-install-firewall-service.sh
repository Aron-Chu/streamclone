#!/usr/bin/env bash
# Systemd unit: reapply laptopworker firewall after docker/tailscale on boot.
set -euo pipefail

if [ "${EUID:-$(id -u)}" -ne 0 ]; then
  exec sudo bash "$0" "$@"
fi

UNIT="/etc/systemd/system/streamclone-laptopworker-firewall.service"

if [ ! -x /usr/local/sbin/streamclone-laptopworker-firewall ]; then
  echo "install helpers first: bash scripts/laptopworker-install-helpers.sh" >&2
  exit 1
fi

tee "$UNIT" >/dev/null <<'EOF'
[Unit]
Description=Streamclone laptopworker tailnet firewall (UFW + iptables)
After=docker.service network-online.target tailscaled.service
Wants=docker.service network-online.target

[Service]
Type=oneshot
RemainAfterExit=yes
ExecStart=/usr/local/sbin/streamclone-laptopworker-firewall

[Install]
WantedBy=multi-user.target
EOF

systemctl daemon-reload
systemctl enable streamclone-laptopworker-firewall.service
systemctl restart streamclone-laptopworker-firewall.service
echo "enabled streamclone-laptopworker-firewall.service"
