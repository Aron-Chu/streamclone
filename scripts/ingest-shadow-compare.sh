#!/usr/bin/env bash
# Compare ingest-core shadow artifacts against tolerance gates.
set -euo pipefail

DIR="${INGEST_SHADOW_ARTIFACT_DIR:-runtime/ingest-shadow}"
FILE="${DIR}/latest.jsonl"
TOLERANCE="${1:-2}"
MIN_SAMPLES="${2:-100}"

if [[ ! -f "${FILE}" ]]; then
  echo "FAIL: shadow artifact missing: ${FILE}" >&2
  exit 1
fi

python3 - "${FILE}" "${TOLERANCE}" "${MIN_SAMPLES}" <<'PY'
import json, sys

path, tol, min_samples = sys.argv[1], float(sys.argv[2]), int(sys.argv[3])
match = mismatch = 0
with open(path, encoding="utf-8") as fh:
    for line in fh:
        line = line.strip()
        if not line:
            continue
        rec = json.loads(line)
        if rec.get("match"):
            match += 1
        else:
            mismatch += 1
total = match + mismatch
if total < min_samples:
    print(f"WARN: only {total} samples (need {min_samples})")
if total == 0:
    sys.exit(1)
pct = 100.0 * match / total
print(f"shadow_compare total={total} match={match} mismatch={mismatch} match_pct={pct:.2f}")
if pct < 100.0 - tol:
    print(f"FAIL: match_pct {pct:.2f} below tolerance")
    sys.exit(1)
print("PASS")
PY
