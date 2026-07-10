#!/usr/bin/env bash
# Quick check that agent instructions + hooks + codegraph are present.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$ROOT"
FAIL=0

check() {
  if [[ -e "$1" ]]; then
    echo "OK  $1"
  else
    echo "MISS $1"
    FAIL=1
  fi
}

echo "=== Agent rules ==="
for f in AGENTS.md \
  .cursor/rules/agents-router.mdc \
  .cursor/rules/backend-go.mdc \
  .cursor/rules/frontend-react.mdc \
  .cursor/rules/scraper-workers.mdc \
  .cursor/rules/db-migrations.mdc \
  .cursor/hooks.json; do
  check "$f"
done

echo
echo "=== Code graph ==="
DB=".codegraph/streamclone.kuzu"
if [[ -f "$DB" ]]; then
  age=$(( $(date +%s) - $(stat -c %Y "$DB" 2>/dev/null || stat -f %m "$DB") ))
  echo "OK  $DB (age ${age}s)"
  if (( age > 86400 )); then
    echo "WARN codegraph older than 24h — run: make codegraph"
  fi
else
  echo "MISS $DB — run: make mcp-setup"
  FAIL=1
fi

echo
echo "=== Context snapshots (optional) ==="
if [[ -f runtime/context/routes.txt ]]; then
  echo "OK  runtime/context/routes.txt"
else
  echo "SKIP runtime/context/ — run: make context-snapshots (needs stack for DB/Grafana)"
fi

echo
echo "=== Product boundary preflight ==="
if bash scripts/check-product-boundary.sh --preflight 2>/dev/null; then
  echo "OK  product-boundary-preflight"
else
  echo "WARN product-boundary-preflight hit(s) — run: make product-boundary-strict"
fi

echo
echo "=== Context contract (sdlc pointer) ==="
if bash scripts/ci-context-contract.sh; then
  echo "OK  ci-context-contract"
else
  echo "FAIL ci-context-contract"
  FAIL=1
fi

echo
echo "=== Streamclone MCP (WSL) ==="
if bash scripts/mcp-preflight.sh >/tmp/context-verify-mcp.txt 2>&1; then
  echo "OK  streamclone MCP preflight"
  grep 'handshake' /tmp/context-verify-mcp.txt | sed 's/^/    /'
else
  echo "FAIL streamclone MCP preflight"
  tail -5 /tmp/context-verify-mcp.txt | sed 's/^/    /'
  FAIL=1
fi
if command -v python3 >/dev/null 2>&1; then
  echo '{}' | python3 .cursor/hooks/codegraph-stale-hint.py >/dev/null && echo "OK  codegraph-stale-hint.py" || echo "FAIL codegraph-stale-hint.py"
else
  echo "SKIP python3"
fi

exit "$FAIL"
