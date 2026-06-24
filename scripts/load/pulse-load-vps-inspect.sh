#!/usr/bin/env bash
# Remote helper for LOAD-001 — inspect env without printing secrets.
set -euo pipefail
echo "==> health localhost"
curl -s http://127.0.0.1:8090/v1/extension/health | python3 -m json.tool 2>/dev/null | head -25 || curl -s http://127.0.0.1:8090/v1/extension/health | head -c 400
echo ""
echo "==> analytics env flags (no secret values)"
docker inspect streamclone-analytics-1 --format '{{range .Config.Env}}{{println .}}{{end}}' \
  | grep -E '^PULSE_(HOSTED|BETA|MAX_ACTIVE)|^STREAMCLONE_VERSION' \
  | sed 's/=.*KEYS.*/=***redacted***/' || true
