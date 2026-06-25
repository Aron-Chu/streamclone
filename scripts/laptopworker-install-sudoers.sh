#!/usr/bin/env bash
# Passwordless sudo for root-owned /usr/local/sbin helpers only.
set -euo pipefail

if [ "${EUID:-$(id -u)}" -ne 0 ]; then
  exec sudo bash "$0" "$@"
fi

USER_NAME="${SUDO_USER:-${LAPTOPWORKER_USER:-aron}}"
DROP="/etc/sudoers.d/streamclone-laptopworker"
TMP="$(mktemp)"

cleanup() { rm -f "$TMP"; }
trap cleanup EXIT

cat >"$TMP" <<EOF
# Streamclone laptopworker — NOPASSWD for root-owned helpers only (not git checkout).
${USER_NAME} ALL=(ALL) NOPASSWD: /usr/bin/loginctl enable-linger ${USER_NAME}
${USER_NAME} ALL=(ALL) NOPASSWD: /usr/local/sbin/streamclone-laptopworker-firewall
${USER_NAME} ALL=(ALL) NOPASSWD: /usr/local/sbin/streamclone-laptopworker-power
${USER_NAME} ALL=(ALL) NOPASSWD: /usr/local/sbin/streamclone-laptopworker-boot
${USER_NAME} ALL=(ALL) NOPASSWD: /usr/bin/systemctl start streamclone-laptopworker-firewall.service
${USER_NAME} ALL=(ALL) NOPASSWD: /usr/bin/systemctl restart streamclone-laptopworker-firewall.service
EOF

cp "$TMP" "$DROP"
chmod 440 "$DROP"
visudo -cf "$DROP"
echo "Installed $DROP"
