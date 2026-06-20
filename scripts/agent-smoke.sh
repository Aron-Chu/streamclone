#!/usr/bin/env bash
# Post-edit agent validation: stack_health snapshot + core API smoke.
# Requires the local stack (make up). Exit non-zero on failure.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

PY="${CODEGRAPH_PY:-$ROOT/.codegraph/.venv/bin/python}"
BASE_URL="${STREAMCLONE_BASE_URL:-http://localhost:8090}"

fail() {
  echo "agent-smoke: $1" >&2
  exit 1
}

if [[ ! -x "$PY" ]]; then
  fail "Python venv missing — run: make codegraph-install"
fi

echo "agent-smoke: stack_health ($BASE_URL)"
"$PY" - <<'PY' "$ROOT" "$BASE_URL"
import json, sys
from pathlib import Path

root = Path(sys.argv[1])
base = sys.argv[2]
sys.path.insert(0, str(root / "tools" / "stack"))
from stack_mcp import stack_health  # noqa: E402

result = stack_health(base)
print(json.dumps({"warnings": result.get("warnings"), "proxy": result.get("proxy_root", {}).get("status")}, indent=2))
auth = result.get("auth_debug") or {}
if not auth.get("ok"):
    print(f"agent-smoke: auth/debug failed: {auth}", file=sys.stderr)
    sys.exit(1)
for name, svc in (result.get("service_health") or {}).items():
    if name == "scraper":
        continue
    if not svc.get("ok") and svc.get("status") not in (404, None):
        print(f"agent-smoke: {name} unhealthy: {svc}", file=sys.stderr)
        sys.exit(1)
PY

echo "agent-smoke: core API smoke"
bash "$ROOT/scripts/smoke-core.sh"

echo "agent-smoke: passed"
