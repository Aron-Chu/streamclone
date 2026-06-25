#!/usr/bin/env bash
# Always-on worker posture: lid closed + AC does not suspend.
set -euo pipefail

_self="$(readlink -f "$0" 2>/dev/null || realpath "$0" 2>/dev/null || echo "$0")"
if [ "${EUID:-$(id -u)}" -ne 0 ]; then
  exec sudo -n bash "$_self" "$@" 2>/dev/null || exec sudo bash "$_self" "$@"
fi

echo "==> Configuring logind..."
mkdir -p /etc/systemd/logind.conf.d
tee /etc/systemd/logind.conf.d/99-streamclone-worker.conf >/dev/null <<'EOF'
[Login]
HandleLidSwitch=ignore
HandleLidSwitchExternalPower=ignore
HandleLidSwitchDocked=ignore
IdleAction=ignore
EOF

systemctl restart systemd-logind

echo "==> Masking system sleep targets..."
for unit in sleep.target suspend.target hibernate.target hybrid-sleep.target; do
  systemctl mask "$unit" 2>/dev/null || true
done

echo "==> Disabling unattended-upgrades auto-reboot..."
tee /etc/apt/apt.conf.d/99-streamclone-no-auto-reboot >/dev/null <<'EOF'
Unattended-Upgrade::Automatic-Reboot "false";
EOF

echo "==> Power posture applied (lid ignore, sleep masked, no auto-reboot)."
