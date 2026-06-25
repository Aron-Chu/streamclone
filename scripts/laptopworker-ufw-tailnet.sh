#!/usr/bin/env bash
# Tailnet-only firewall for laptopworker.
# Primary: Caddy binds to Tailscale IP only (compose overlay).
# Backup: DOCKER-USER drops :8090 except on tailscale0 (Docker bypasses UFW INPUT).
set -euo pipefail

if ! command -v sudo >/dev/null 2>&1; then
  echo "sudo is required" >&2
  exit 1
fi

if ! command -v ufw >/dev/null 2>&1; then
  echo "==> Installing ufw..."
  sudo apt-get update -qq
  sudo DEBIAN_FRONTEND=noninteractive apt-get install -y -qq ufw
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
sudo ufw default deny incoming
sudo ufw default allow outgoing
sudo ufw allow in on tailscale0 to any port 22 proto tcp comment 'Tailscale SSH'

if [ "${LAPTOPWORKER_UFW_ALLOW_LAN_SSH:-}" = "1" ]; then
  echo "==> Opt-in: allowing OpenSSH on all interfaces (LAPTOPWORKER_UFW_ALLOW_LAN_SSH=1)"
  sudo ufw allow OpenSSH comment 'LAN SSH opt-in'
fi

sudo ufw --force enable

echo "==> Applying DOCKER-USER rules for :8090 (loopback + tailscale0 only)"
if ! sudo iptables -C DOCKER-USER -i lo -p tcp --dport 8090 -j ACCEPT 2>/dev/null; then
  sudo iptables -I DOCKER-USER 1 -i lo -p tcp --dport 8090 -j ACCEPT
fi
if ! sudo iptables -C DOCKER-USER -i tailscale0 -p tcp --dport 8090 -j ACCEPT 2>/dev/null; then
  sudo iptables -I DOCKER-USER 2 -i tailscale0 -p tcp --dport 8090 -j ACCEPT
fi
if ! sudo iptables -C DOCKER-USER -p tcp --dport 8090 -j DROP 2>/dev/null; then
  sudo iptables -I DOCKER-USER 3 -p tcp --dport 8090 -j DROP
fi

echo
echo "==> Caddy publishes 8090:80 (all interfaces); DOCKER-USER limits access to lo + tailscale0"
echo "    Run once after Docker is up: bash scripts/laptopworker-stack.sh ufw-tailnet"
echo
sudo ufw status verbose

cat <<EOF

Tailnet posture applied.
- Caddy publishes 8090:80; DOCKER-USER blocks LAN/WAN except lo + tailscale0
- UFW: SSH on tailscale0 only (set LAPTOPWORKER_UFW_ALLOW_LAN_SSH=1 for LAN console SSH)
- Verify from Windows: http://laptopworker:8090

EOF
