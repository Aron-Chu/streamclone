#!/usr/bin/env bash
# Stop SSH tunnels to BearHost Grafana (ports 3000/3001).
set -euo pipefail
pkill -f 'ssh.*-L.*3000:127.0.0.1:3000' 2>/dev/null || true
pkill -f 'ssh.*-L.*3001:127.0.0.1:3000' 2>/dev/null || true
if pgrep -af 'ssh.*300[01]:127.0.0.1:3000' >/dev/null 2>&1; then
  echo "bearhost-grafana-tunnel-stop: some tunnels still running" >&2
  pgrep -af 'ssh.*300[01]:127.0.0.1:3000'
  exit 1
fi
echo "bearhost-grafana-tunnel-stop: done"
