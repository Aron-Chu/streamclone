#!/usr/bin/env bash
# One-shot laptopworker QoL from Windows: scripts\laptopworker-remote.cmd setup
# Enter laptop sudo password once at your desk — then ufw/grub/power work without walking over.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"
# shellcheck source=laptopworker-env.sh
source "$ROOT/scripts/laptopworker-env.sh"

echo "==> Streamclone laptopworker remote setup"
echo "    Repo: $ROOT"

if ! laptopworker_required_files "$ROOT"; then
  echo "Missing laptopworker files — run git pull on laptop first." >&2
  exit 1
fi

laptopworker_ensure_scripts_executable "$ROOT"

if ! sudo -n true 2>/dev/null; then
  echo
  echo "Enter your laptop sudo password once (typed here at your Windows desk, not at the machine):"
  sudo -v
fi

echo "==> Installing passwordless sudo for laptopworker scripts..."
bash "$ROOT/scripts/laptopworker-install-sudoers.sh"

echo "==> linger (boot stack without console login)"
if [ "$(loginctl show-user "$(id -un)" -p Linger --value 2>/dev/null || echo no)" != "yes" ]; then
  sudo loginctl enable-linger "$(id -un)"
fi
loginctl show-user "$(id -un)" -p Linger

echo "==> Always-on power posture"
bash "$ROOT/scripts/laptopworker-power-config.sh"

echo "==> Tailnet firewall (UFW + DOCKER-USER)"
bash "$ROOT/scripts/laptopworker-ufw-tailnet.sh"

echo "==> Boot into Ubuntu by default (GRUB + UEFI)"
bash "$ROOT/scripts/laptopworker-setup-boot.sh"

if [ ! -f "${HOME}/.config/systemd/user/streamclone-dev.service" ]; then
  echo "==> Installing systemd user service..."
  bash "$ROOT/scripts/laptopworker-stack.sh" install-service
else
  systemctl --user daemon-reload
  systemctl --user enable streamclone-dev.service streamclone-dev-health.timer 2>/dev/null || true
  laptopworker_ensure_scripts_executable "$ROOT"
  systemctl --user reset-failed streamclone-dev.service 2>/dev/null || true
  systemctl --user start streamclone-dev.service || bash "$ROOT/scripts/laptopworker-stack.sh" start
fi

echo
bash "$ROOT/scripts/laptopworker-stack.sh" boot-check

cat <<EOF

Setup complete. From Windows (no sudo prompts next time):
  scripts\\laptopworker-remote.cmd smoke
  scripts\\laptopworker-remote.cmd boot-check
  scripts\\laptopworker-remote.cmd ufw-tailnet   # re-apply if Docker rules lost

Reboot test (optional): ssh -t aron@laptopworker sudo reboot
  then: scripts\\laptopworker-remote.cmd smoke

EOF
