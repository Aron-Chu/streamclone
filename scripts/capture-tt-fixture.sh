#!/usr/bin/env bash
# Capture a TwitchTracker stream detail HTML fixture for parser tests.
set -euo pipefail

LOGIN=""
STREAM_ID=""
SCRAPER_URL="${SCRAPER_URL:-http://127.0.0.1:8000/v2/scrape}"
API_KEY="${SCRAPER_API_KEY:-local-dev-key}"
OUTPUT_DIR=""
TIMEOUT_MS="${TIMEOUT_MS:-120000}"

usage() {
  echo "Usage: $0 --login LOGIN --stream-id STREAM_ID [--output-dir DIR]"
  exit 1
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --login) LOGIN="$2"; shift 2 ;;
    --stream-id) STREAM_ID="$2"; shift 2 ;;
    --output-dir) OUTPUT_DIR="$2"; shift 2 ;;
    --scraper-url) SCRAPER_URL="$2"; shift 2 ;;
    *) usage ;;
  esac
done

[[ -n "$LOGIN" && -n "$STREAM_ID" ]] || usage

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
OUTPUT_DIR="${OUTPUT_DIR:-$REPO_ROOT/docs/benchmarks/tt-fixtures}"
mkdir -p "$OUTPUT_DIR"

PAGE_URL="https://twitchtracker.com/${LOGIN}/streams/${STREAM_ID}"
OUT_PATH="$OUTPUT_DIR/${LOGIN}-${STREAM_ID}.html"

echo "Capturing TwitchTracker fixture: $PAGE_URL"
echo "Output: $OUT_PATH"

payload=$(jq -n \
  --arg url "$PAGE_URL" \
  --arg apiKey "$API_KEY" \
  --argjson timeoutMs "$TIMEOUT_MS" \
  '{url: $url, apiKey: $apiKey, useProxy: false, maxAgeMs: 0, timeoutMs: $timeoutMs}')

curl -sfS -X POST "$SCRAPER_URL" \
  -H 'Content-Type: application/json' \
  -d "$payload" \
  -o "$OUT_PATH"

if ! grep -q 'id="ecs"' "$OUT_PATH" 2>/dev/null; then
  echo "WARN: meta#ecs not found in captured HTML" >&2
fi

echo "Saved $(wc -c < "$OUT_PATH") bytes"
