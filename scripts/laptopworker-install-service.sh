#!/usr/bin/env bash
# Install systemd user service: start Streamclone dev stack on boot + optional health timer.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
USER_NAME="$(id -un)"
UNIT_DIR="${HOME}/.config/systemd/user"
STACK="${ROOT}/scripts/laptopworker-stack.sh"

mkdir -p "$UNIT_DIR"

cat >"${UNIT_DIR}/streamclone-dev.service" <<EOF
[Unit]
Description=Streamclone laptopworker dev stack (core UI, no scraper)
After=docker.service network-online.target tailscaled.service
Wants=network-online.target

[Service]
Type=oneshot
RemainAfterExit=yes
WorkingDirectory=${ROOT}
ExecStartPre=/bin/bash -c 'for i in $(seq 1 60); do docker info >/dev/null 2>&1 && tailscale status >/dev/null 2>&1 && exit 0; sleep 2; done; echo docker/tailscale not ready; exit 1'
ExecStart=/usr/bin/sg docker -c "${STACK} start"
ExecStop=/usr/bin/sg docker -c "${STACK} stop"
TimeoutStartSec=900

[Install]
WantedBy=default.target
EOF

cat >"${UNIT_DIR}/streamclone-dev-health.timer" <<'EOF'
[Unit]
Description=Streamclone dev stack health check (every 10 min)

[Timer]
OnBootSec=5min
OnUnitActiveSec=10min
Persistent=true

[Install]
WantedBy=timers.target
EOF

cat >"${UNIT_DIR}/streamclone-dev-health.service" <<EOF
[Unit]
Description=Streamclone dev stack health probe

[Service]
Type=oneshot
WorkingDirectory=${ROOT}
ExecStart=/usr/bin/sg docker -c "${STACK} smoke"
EOF

systemctl --user daemon-reload
systemctl --user enable --now streamclone-dev.service
systemctl --user enable --now streamclone-dev-health.timer

if command -v loginctl >/dev/null 2>&1; then
  linger="$(loginctl show-user "$USER_NAME" -p Linger --value 2>/dev/null || true)"
  if [ "$linger" != "yes" ]; then
    echo "==> Enabling linger (boot without login) — sudo password required once..."
    if sudo -n loginctl enable-linger "$USER_NAME" 2>/dev/null; then
      echo "    linger enabled"
    else
      echo "    Run manually: sudo loginctl enable-linger $USER_NAME"
    fi
  fi
fi

systemctl --user reset-failed streamclone-dev.service 2>/dev/null || true
systemctl --user start streamclone-dev.service || {
  echo "Service start failed — if stack is already up, that's ok. Check: systemctl --user status streamclone-dev" >&2
}

cat <<EOF

Installed:
  streamclone-dev.service         — start stack on boot
  streamclone-dev-health.timer    — smoke every 10 min (logs: journalctl --user -u streamclone-dev-health)

After git push to master, from Windows:
  powershell -File scripts/laptopworker-remote.ps1 update

Status:
  systemctl --user status streamclone-dev.service
  bash scripts/laptopworker-stack.sh status

EOF
