#!/usr/bin/env bash
# Always-on worker posture: lid closed + AC does not suspend. Run once on laptopworker (sudo).
set -euo pipefail

if ! command -v sudo >/dev/null 2>&1; then
  echo "sudo is required" >&2
  exit 1
fi

CONF="/etc/systemd/logind.conf"
echo "==> Configuring logind ($CONF)..."

sudo mkdir -p /etc/systemd/logind.conf.d
sudo tee /etc/systemd/logind.conf.d/99-streamclone-worker.conf >/dev/null <<'EOF'
[Login]
HandleLidSwitch=ignore
HandleLidSwitchExternalPower=ignore
HandleLidSwitchDocked=ignore
IdleAction=ignore
EOF

sudo systemctl restart systemd-logind

echo "==> Masking system sleep targets..."
for unit in sleep.target suspend.target hibernate.target hybrid-sleep.target; do
  sudo systemctl mask "$unit" 2>/dev/null || true
done

echo "==> Disabling unattended-upgrades auto-reboot..."
sudo tee /etc/apt/apt.conf.d/99-streamclone-no-auto-reboot >/dev/null <<'EOF'
Unattended-Upgrade::Automatic-Reboot "false";
EOF

echo "==> Power posture applied."
echo "    Lid close: ignore | idle suspend: off | sleep targets: masked"
echo "    Keep AC connected. Wi-Fi power save may still drop — prefer ethernet if flaky."
