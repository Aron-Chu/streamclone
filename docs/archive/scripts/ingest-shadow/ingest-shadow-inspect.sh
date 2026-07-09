#!/usr/bin/env bash
# Inspect ingest-core shadow JSONL artifacts (ShadowRecord schema).
# Usage: INGEST_SHADOW_ARTIFACT_DIR=... bash scripts/ingest-shadow-inspect.sh [OUT_DIR]
set -euo pipefail

DIR="${INGEST_SHADOW_ARTIFACT_DIR:-runtime/ingest-shadow}"
FILE="${DIR}/latest.jsonl"
OUT_DIR="${1:-runtime/evidence/shadow-inspect-$(date -u +%Y%m%dT%H%M%SZ)}"
mkdir -p "${OUT_DIR}"

if [[ ! -f "${FILE}" ]]; then
  echo "FAIL: shadow artifact missing: ${FILE}" >&2
  exit 1
fi

{
  echo "artifact_dir=${DIR}"
  echo "artifact_file=${FILE}"
  ls -lah "${DIR}" 2>/dev/null || true
  stat "${FILE}" 2>/dev/null || true
  echo "line_count=$(wc -l < "${FILE}")"
} | tee "${OUT_DIR}/artifact-meta.txt"

python3 - "${FILE}" "${OUT_DIR}" <<'PY'
import json, sys, collections
from pathlib import Path

path, out_dir = sys.argv[1], Path(sys.argv[2])
records = []
with open(path, encoding="utf-8") as fh:
    for line in fh:
        line = line.strip()
        if not line:
            continue
        records.append(json.loads(line))

closed = open_ = match = mismatch = 0
closed_match = closed_mismatch = open_match = open_mismatch = 0
reasons = collections.Counter()
first_mismatch = first_closed_mismatch = first_match = None
mismatches = []
matches = []

def key_closed(rec):
    k = rec.get("key") or {}
    return bool(k.get("Closed") or k.get("closed"))

for rec in records:
    is_closed = key_closed(rec)
    if is_closed:
        closed += 1
    else:
        open_ += 1
    if rec.get("match"):
        match += 1
        matches.append(rec)
        if first_match is None:
            first_match = rec
        if is_closed:
            closed_match += 1
        else:
            open_match += 1
    else:
        mismatch += 1
        mismatches.append(rec)
        reason = rec.get("reason") or "unknown"
        bucket = reason.split(":", 1)[0] if ":" in reason else reason.split("=")[0]
        reasons[bucket] += 1
        if first_mismatch is None:
            first_mismatch = rec
        if is_closed and first_closed_mismatch is None:
            first_closed_mismatch = rec
        if is_closed:
            closed_mismatch += 1
        else:
            open_mismatch += 1

lines = [
    f"total_records={len(records)}",
    f"closed_records={closed}",
    f"open_records={open_}",
    f"match_count={match}",
    f"mismatch_count={mismatch}",
    f"closed_match_count={closed_match}",
    f"closed_mismatch_count={closed_mismatch}",
    f"open_match_count={open_match}",
    f"open_mismatch_count={open_mismatch}",
    "",
    "reason_histogram:",
]
for r, n in reasons.most_common():
    lines.append(f"  {r}={n}")

summary = "\n".join(lines)
print(summary)
(out_dir / "summary.txt").write_text(summary + "\n", encoding="utf-8")

def dump(name, obj):
    if obj is None:
        (out_dir / name).write_text("(none)\n", encoding="utf-8")
        return
    (out_dir / name).write_text(json.dumps(obj, indent=2, default=str) + "\n", encoding="utf-8")

dump("first_mismatch.json", first_mismatch)
dump("first_closed_mismatch.json", first_closed_mismatch)
dump("first_match.json", first_match)

with open(out_dir / "first_5_mismatches.jsonl", "w", encoding="utf-8") as fh:
    for rec in mismatches[:5]:
        fh.write(json.dumps(rec, default=str) + "\n")

with open(out_dir / "first_5_matches.jsonl", "w", encoding="utf-8") as fh:
    for rec in matches[:5]:
        fh.write(json.dumps(rec, default=str) + "\n")

# Key field samples for diagnosis
with open(out_dir / "key_samples.tsv", "w", encoding="utf-8") as fh:
    fh.write("closed\tstream_id\tchannel\tminute\tmatch\treason\tlegacy_chat\tshadow_chat\n")
    for rec in records[:50]:
        k = rec.get("key") or {}
        fh.write(
            f"{key_closed(rec)}\t{k.get('StreamID', k.get('streamID', ''))}\t{k.get('Channel', k.get('channel', ''))}\t{k.get('Minute', k.get('minute', ''))}\t"
            f"{rec.get('match')}\t{rec.get('reason','')}\t{rec.get('legacyChat')}\t{rec.get('shadowChat')}\n"
        )

print(f"DONE: {out_dir}")
PY

echo "Inspect complete: ${OUT_DIR}"
