#!/usr/bin/env bash
# Read-only Azure blob prefix inventory (Phase 0.6 migration audit).
# Lists metadata only — no download, copy, delete, or lifecycle changes.
#
# Prerequisites: Azure CLI (`az`), python3, connection string file.
# Usage:
#   export AZURE_CONN_FILE=~/.streamclone/azure-archive-connection-string  # optional
#   bash scripts/storage/azure-prefix-inventory.sh
#
# See docs/storage/azure-to-r2-migration.md and docs/storage/README.md.
set -euo pipefail

CONN_FILE="${AZURE_CONN_FILE:-$HOME/.streamclone/azure-archive-connection-string}"
if [[ -f "/mnt/c/Users/Aron/.streamclone/azure-archive-connection-string" && ! -f "$CONN_FILE" ]]; then
  CONN_FILE="/mnt/c/Users/Aron/.streamclone/azure-archive-connection-string"
fi

if [[ ! -f "$CONN_FILE" ]]; then
  echo "ERROR: connection string file not found: $CONN_FILE" >&2
  echo "Set AZURE_CONN_FILE or create ~/.streamclone/azure-archive-connection-string (never commit)." >&2
  exit 1
fi

export AZURE_STORAGE_CONNECTION_STRING
AZURE_STORAGE_CONNECTION_STRING="$(tr -d '\r\n' < "$CONN_FILE")"
if [[ -z "$AZURE_STORAGE_CONNECTION_STRING" ]]; then
  echo "ERROR: empty connection string from $CONN_FILE" >&2
  exit 1
fi

CONTAINER="${AZURE_CONTAINER:-streamclone-archive}"
PREFIX="${AZURE_PREFIX:-streamclone/}"

echo "== Azure read-only prefix inventory =="
echo "container=${CONTAINER} prefix=${PREFIX}"
echo "conn_file=${CONN_FILE} conn_len=${#AZURE_STORAGE_CONNECTION_STRING}"
echo ""

inventory_prefix() {
  local p="$1"
  local full="${PREFIX}${p}"
  local tmp
  tmp="$(mktemp)"
  echo "--- ${p} ---"
  if ! az storage blob list --container-name "$CONTAINER" --prefix "$full" -o json >"$tmp" 2>/dev/null; then
    echo "LIST_FAILED"
    rm -f "$tmp"
    return
  fi
  python3 - "$p" "$tmp" <<'PY'
import json, sys
from collections import Counter

prefix = sys.argv[1]
path = sys.argv[2]
with open(path, encoding="utf-8") as f:
    blobs = json.load(f)

count = len(blobs)
total_bytes = sum(b.get("properties", {}).get("contentLength") or 0 for b in blobs)
exts = Counter()
tiers = Counter()
mods = []
for b in blobs:
    name = b.get("name", "")
    if "." in name.rsplit("/", 1)[-1]:
        ext = "." + name.rsplit(".", 1)[-1]
    else:
        ext = "(none)"
    exts[ext] += 1
    tier = b.get("properties", {}).get("blobTier") or "unknown"
    tiers[tier] += 1
    lm = b.get("properties", {}).get("lastModified")
    if lm:
        mods.append(lm)

mod_min = min(mods) if mods else "n/a"
mod_max = max(mods) if mods else "n/a"
ext_str = ", ".join(f"{k}({v})" for k, v in exts.most_common(8))
tier_str = ", ".join(f"{k}({v})" for k, v in tiers.most_common())

print(f"count={count}")
print(f"bytes={total_bytes}")
print(f"modified_min={mod_min}")
print(f"modified_max={mod_max}")
print(f"extensions={ext_str or 'n/a'}")
print(f"tiers={tier_str or 'n/a'}")
if blobs[:2]:
    print("sample:")
    for b in blobs[:2]:
        props = b.get("properties", {})
        print(f"  {b.get('name')} bytes={props.get('contentLength')} tier={props.get('blobTier')} mod={props.get('lastModified')}")
PY
  rm -f "$tmp"
  echo ""
}

for p in rollups/ vod_chat/ emotes/snapshots/ postgres/nightly/ tt-detail/ channels/ vod_catalog/; do
  inventory_prefix "$p"
done

echo "== NOTE: root total may paginate at 5000 objects =="
echo "Use scripts/storage/azure-top-prefixes.sh and azure-extra-prefixes.sh for full domain coverage."
