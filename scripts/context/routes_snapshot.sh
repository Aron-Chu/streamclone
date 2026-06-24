#!/usr/bin/env bash
# Write compact Caddy route summary for agents (no secrets).
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
OUT="${ROOT}/runtime/context"
mkdir -p "$OUT"
CADDY="${ROOT}/deploy/Caddyfile"

{
  echo "# Caddy routes snapshot — $(date -u +%Y-%m-%dT%H:%M:%SZ)"
  echo "# Source: deploy/Caddyfile (+ local-tunnel overlay may extend)"
  echo
  if [[ -f "$CADDY" ]]; then
    grep -E '^\s*@(chat|video|emote|analytics|storygraph|hls|metadata|clipper|admin)' "$CADDY" 2>/dev/null || true
    grep -E 'reverse_proxy|path_regexp|path ' "$CADDY" | head -40
  else
    echo "(Caddyfile missing)"
  fi
} > "${OUT}/routes.txt"

echo "wrote ${OUT}/routes.txt"
