#!/usr/bin/env bash
# Passwordless sudo for laptopworker remote ops (run once from laptopworker-setup-remote.sh).
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
USER_NAME="$(id -un)"
DROP="/etc/sudoers.d/streamclone-laptopworker"
TMP="$(mktemp)"

cleanup() { rm -f "$TMP"; }
trap cleanup EXIT

cat >"$TMP" <<EOF
# Streamclone laptopworker — remote setup from Windows (scripts/laptopworker-remote.cmd setup)
# Managed by ${ROOT}/scripts/laptopworker-install-sudoers.sh
${USER_NAME} ALL=(ALL) NOPASSWD: /usr/bin/loginctl enable-linger ${USER_NAME}
${USER_NAME} ALL=(ALL) NOPASSWD: /bin/bash ${ROOT}/scripts/laptopworker-ufw-tailnet.sh
${USER_NAME} ALL=(ALL) NOPASSWD: /bin/bash ${ROOT}/scripts/laptopworker-power-config.sh
${USER_NAME} ALL=(ALL) NOPASSWD: /bin/bash ${ROOT}/scripts/laptopworker-setup-boot.sh
${USER_NAME} ALL=(ALL) NOPASSWD: /usr/sbin/iptables
EOF

sudo cp "$TMP" "$DROP"
sudo chmod 440 "$DROP"
sudo visudo -cf "$DROP"
echo "Installed $DROP (passwordless laptopworker sudo for remote control)"
