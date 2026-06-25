#!/usr/bin/env bash
# Install root-owned /usr/local/sbin helpers (not user-writable checkout paths).
set -euo pipefail

if [ "${EUID:-$(id -u)}" -ne 0 ]; then
  exec sudo bash "$0" "$@"
fi

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
SBIN_SRC="${ROOT}/scripts/laptopworker/sbin"
DEST="/usr/local/sbin"

if [ ! -d "$SBIN_SRC" ]; then
  echo "missing $SBIN_SRC" >&2
  exit 1
fi

for name in streamclone-laptopworker-firewall streamclone-laptopworker-power streamclone-laptopworker-boot; do
  src="${SBIN_SRC}/${name}"
  if [ ! -f "$src" ]; then
    echo "missing $src" >&2
    exit 1
  fi
  install -o root -g root -m 755 "$src" "${DEST}/${name}"
  echo "installed ${DEST}/${name}"
done
