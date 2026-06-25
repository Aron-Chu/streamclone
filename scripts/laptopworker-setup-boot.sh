#!/usr/bin/env bash
# Prefer Ubuntu on reboot: GRUB default + UEFI boot order (dual-boot laptops).
set -euo pipefail

_self="$(readlink -f "$0" 2>/dev/null || realpath "$0" 2>/dev/null || echo "$0")"
if [ "${EUID:-$(id -u)}" -ne 0 ]; then
  exec sudo -n bash "$_self" "$@" 2>/dev/null || exec sudo bash "$_self" "$@"
fi

echo "==> GRUB default (Ubuntu first)"
if [ -f /etc/default/grub ]; then
  if grep -q '^GRUB_DEFAULT=' /etc/default/grub; then
    sed -i 's/^GRUB_DEFAULT=.*/GRUB_DEFAULT=0/' /etc/default/grub
  else
    echo 'GRUB_DEFAULT=0' >> /etc/default/grub
  fi
  if grep -q '^GRUB_TIMEOUT=' /etc/default/grub; then
    sed -i 's/^GRUB_TIMEOUT=.*/GRUB_TIMEOUT=2/' /etc/default/grub
  else
    echo 'GRUB_TIMEOUT=2' >> /etc/default/grub
  fi
  if command -v update-grub >/dev/null 2>&1; then
    update-grub
  elif command -v grub-mkconfig >/dev/null 2>&1; then
    grub-mkconfig -o /boot/grub/grub.cfg
  fi
  echo "    GRUB_DEFAULT=0 GRUB_TIMEOUT=2"
else
  echo "    /etc/default/grub missing — skip"
fi

if ! command -v efibootmgr >/dev/null 2>&1; then
  echo "==> efibootmgr not installed — skip UEFI order"
  exit 0
fi

echo "==> UEFI boot order (Ubuntu before Windows)"
mapfile -t lines < <(efibootmgr 2>/dev/null | grep -E '^Boot[0-9]{4}' || true)
ubuntu_num=""
windows_num=""
others=()

for line in "${lines[@]}"; do
  num="${line#Boot}"
  num="${num%%\**}"
  num="${num%% *}"
  lower="$(echo "$line" | tr '[:upper:]' '[:lower:]')"
  if [ -z "$ubuntu_num" ] && [[ "$lower" == *ubuntu* || "$lower" == *grub* || "$lower" == *shim* ]]; then
    ubuntu_num="$num"
  elif [ -z "$windows_num" ] && [[ "$lower" == *windows* ]]; then
    windows_num="$num"
  else
    others+=("$num")
  fi
done

if [ -z "$ubuntu_num" ]; then
  echo "    No Ubuntu EFI entry found — check firmware boot order manually if needed"
  efibootmgr 2>/dev/null | head -15 || true
  exit 0
fi

order="$ubuntu_num"
if [ -n "$windows_num" ]; then
  order="${order},${windows_num}"
fi
for n in "${others[@]}"; do
  [ "$n" = "$ubuntu_num" ] || [ "$n" = "$windows_num" ] && continue
  order="${order},${n}"
done

echo "    efibootmgr -o ${order}"
efibootmgr -o "$order" >/dev/null
efibootmgr | head -8
