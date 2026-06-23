#!/usr/bin/env bash
# Stop SSH tunnels to BearHost Grafana (ports 3000/3001).
# Pass --quiet when called from tunnel-start (no "done" line).
set -euo pipefail
QUIET=0
if [[ "${1:-}" == "--quiet" ]]; then
  QUIET=1
fi

pkill -f 'ssh.*-L.*3000:127.0.0.1:3000' 2>/dev/null || true
pkill -f 'ssh.*-L.*3001:127.0.0.1:3000' 2>/dev/null || true
if pgrep -af 'ssh.*300[01]:127.0.0.1:3000' >/dev/null 2>&1; then
  echo "bearhost-grafana-tunnel-stop: some tunnels still running" >&2
  pgrep -af 'ssh.*300[01]:127.0.0.1:3000'
  exit 1
fi
if [[ "$QUIET" -eq 0 ]]; then
  echo "bearhost-grafana-tunnel-stop: done"
fi
