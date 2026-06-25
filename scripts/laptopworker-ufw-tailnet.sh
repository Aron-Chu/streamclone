#!/usr/bin/env bash
# Tailnet-only firewall for laptopworker (UFW + DOCKER-USER).
set -euo pipefail

if [ "${EUID:-$(id -u)}" -ne 0 ]; then
  exec sudo -n bash "$0" "$@" 2>/dev/null || exec sudo bash "$0" "$@"
fi

if ! command -v ufw >/dev/null 2>&1; then
  echo "==> Installing ufw..."
  apt-get update -qq
  DEBIAN_FRONTEND=noninteractive apt-get install -y -qq ufw
fi

if ! ip link show tailscale0 >/dev/null 2>&1; then
  echo "tailscale0 not found — run: sudo tailscale up --hostname=laptopworker" >&2
  exit 1
fi

ts_ip="$(tailscale ip -4 2>/dev/null | head -1 || true)"
if [ -z "$ts_ip" ]; then
  echo "tailscale ip -4 unavailable" >&2
  exit 1
fi

echo "==> Applying UFW baseline (SSH on tailscale0 only by default)..."
ufw default deny incoming
ufw default allow outgoing
ufw allow in on tailscale0 to any port 22 proto tcp comment 'Tailscale SSH'

if [ "${LAPTOPWORKER_UFW_ALLOW_LAN_SSH:-}" = "1" ]; then
  echo "==> Opt-in: allowing OpenSSH on all interfaces (LAPTOPWORKER_UFW_ALLOW_LAN_SSH=1)"
  ufw allow OpenSSH comment 'LAN SSH opt-in'
fi

ufw --force enable

echo "==> Applying DOCKER-USER rules for :8090 (loopback + tailscale0 only)"
if ! iptables -C DOCKER-USER -i lo -p tcp --dport 8090 -j ACCEPT 2>/dev/null; then
  iptables -I DOCKER-USER 1 -i lo -p tcp --dport 8090 -j ACCEPT
fi
if ! iptables -C DOCKER-USER -i tailscale0 -p tcp --dport 8090 -j ACCEPT 2>/dev/null; then
  iptables -I DOCKER-USER 2 -i tailscale0 -p tcp --dport 8090 -j ACCEPT
fi
if ! iptables -C DOCKER-USER -p tcp --dport 8090 -j DROP 2>/dev/null; then
  iptables -I DOCKER-USER 3 -p tcp --dport 8090 -j DROP
fi

echo
echo "==> Caddy publishes 8090:80; DOCKER-USER limits access to lo + tailscale0"
echo
ufw status verbose

cat <<EOF

Tailnet posture applied.
- DOCKER-USER blocks LAN/WAN except lo + tailscale0
- Verify from Windows: http://laptopworker:8090

EOF
