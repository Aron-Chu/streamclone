#!/usr/bin/env bash
# Compare ingest-core shadow artifacts (ShadowRecord JSONL) against tolerance gates.
#
# Usage:
#   bash scripts/ingest-shadow-compare.sh [--closed-only] [--diagnose] TOLERANCE MIN_SAMPLES
#
# Phase C production gate uses --closed-only (closed minute rollups only).
set -euo pipefail

CLOSED_ONLY=0
DIAGNOSE=0
ARGS=()
for arg in "$@"; do
  case "${arg}" in
    --closed-only) CLOSED_ONLY=1 ;;
    --diagnose) DIAGNOSE=1 ;;
    *) ARGS+=("${arg}") ;;
  esac
done

TOLERANCE="${ARGS[0]:-2}"
MIN_SAMPLES="${ARGS[1]:-100}"

DIR="${INGEST_SHADOW_ARTIFACT_DIR:-runtime/ingest-shadow}"
FILE="${DIR}/latest.jsonl"

if [[ ! -f "${FILE}" ]]; then
  echo "FAIL: shadow artifact missing: ${FILE}" >&2
  exit 1
fi

python3 - "${FILE}" "${TOLERANCE}" "${MIN_SAMPLES}" "${CLOSED_ONLY}" "${DIAGNOSE}" <<'PY'
import json, sys, collections

path = sys.argv[1]
tol = float(sys.argv[2])
min_samples = int(sys.argv[3])
closed_only = sys.argv[4] == "1"
diagnose = sys.argv[5] == "1"

records = []
with open(path, encoding="utf-8") as fh:
    for line in fh:
        line = line.strip()
        if not line:
            continue
        records.append(json.loads(line))

def is_closed(rec):
    k = rec.get("key") or {}
    return bool(k.get("Closed") or k.get("closed"))

def chat_match(rec):
    leg = rec.get("legacyChat") or rec.get("LegacyChat") or 0
    sh = rec.get("shadowChat") or rec.get("ShadowChat") or 0
    if leg == sh:
        return True
    if leg == 0 and sh == 0:
        return True
    if leg == 0:
        return False
    return abs(sh - leg) / leg * 100.0 <= tol

def emote_match(rec):
    leg = rec.get("legacyEmotes") or rec.get("LegacyEmotes") or rec.get("legacyTotalEmotes") or 0
    sh = rec.get("shadowEmotes") or rec.get("ShadowEmotes") or rec.get("shadowTotalEmotes") or 0
    return leg == sh

def top_emote_key_mismatch(rec):
    leg = rec.get("legacyTopEmotes") or rec.get("LegacyTopEmotes") or {}
    sh = rec.get("shadowTopEmotes") or rec.get("ShadowTopEmotes") or {}
    if not isinstance(leg, dict):
        leg = {}
    if not isinstance(sh, dict):
        sh = {}
    return leg != sh

closed = open_ = match = mismatch = 0
closed_match = closed_mismatch = open_match = open_mismatch = 0
closed_chat_match = closed_emote_match = closed_both_match = 0
top_emote_key_mismatch_count = 0
reasons = collections.Counter()
first_mismatch = first_closed_mismatch = first_match = None
gate_records = []

for rec in records:
    closed_flag = is_closed(rec)
    if closed_flag:
        closed += 1
    else:
        open_ += 1

    matched = bool(rec.get("match"))
    if matched:
        match += 1
        if first_match is None:
            first_match = rec
        if closed_flag:
            closed_match += 1
        else:
            open_match += 1
    else:
        mismatch += 1
        reason = rec.get("reason") or "unknown"
        bucket = reason.split(":", 1)[0] if ":" in reason else reason.split("=")[0]
        reasons[bucket] += 1
        if first_mismatch is None:
            first_mismatch = rec
        if closed_flag and first_closed_mismatch is None:
            first_closed_mismatch = rec
        if closed_flag:
            closed_mismatch += 1
        else:
            open_mismatch += 1

    if closed_flag:
        if chat_match(rec):
            closed_chat_match += 1
        if emote_match(rec):
            closed_emote_match += 1
        if chat_match(rec) and emote_match(rec):
            closed_both_match += 1
        if top_emote_key_mismatch(rec):
            top_emote_key_mismatch_count += 1

    if closed_only:
        if closed_flag:
            gate_records.append(rec)
    else:
        gate_records.append(rec)

gate_match = sum(1 for r in gate_records if r.get("match"))
gate_mismatch = len(gate_records) - gate_match
gate_total = len(gate_records)

def rate(num, den):
    return 100.0 * num / den if den else 0.0

print(f"total_records={len(records)}")
print(f"closed_records={closed}")
print(f"open_records={open_}")
print(f"match_count={match}")
print(f"mismatch_count={mismatch}")
print(f"closed_match_count={closed_match}")
print(f"closed_mismatch_count={closed_mismatch}")
print(f"open_match_count={open_match}")
print(f"open_mismatch_count={open_mismatch}")
print(f"closed_chat_match_count={closed_chat_match}")
print(f"closed_total_emote_match_count={closed_emote_match}")
print(f"closed_both_chat_and_emote_match_count={closed_both_match}")
print(f"closed_chat_match_rate={rate(closed_chat_match, closed):.2f}")
print(f"closed_total_emote_match_rate={rate(closed_emote_match, closed):.2f}")
print(f"closed_both_chat_and_emote_match_rate={rate(closed_both_match, closed):.2f}")
print(f"top_emote_key_mismatch_count={top_emote_key_mismatch_count}")
print(f"gate_mode={'closed_only' if closed_only else 'all'}")
print(f"gate_total={gate_total}")
print(f"gate_match={gate_match}")
print(f"gate_mismatch={gate_mismatch}")

if diagnose or gate_total == 0 or (gate_total > 0 and gate_match / gate_total < 1.0 - tol / 100.0):
    print("reason_histogram:")
    for r, n in reasons.most_common():
        print(f"  {r}={n}")
    if first_mismatch:
        print(f"first_mismatch={json.dumps(first_mismatch, default=str)}")
    if first_closed_mismatch:
        print(f"first_closed_mismatch={json.dumps(first_closed_mismatch, default=str)}")
    if first_match:
        print(f"first_match={json.dumps(first_match, default=str)}")

if gate_total < min_samples:
    print(f"WARN: only {gate_total} gate samples (need {min_samples})")
if gate_total == 0:
    print("FAIL: no gate samples")
    sys.exit(1)

if closed_only:
    gate_chat_match = sum(1 for r in gate_records if chat_match(r))
    pct = 100.0 * gate_chat_match / gate_total
    print(f"shadow_compare gate_total={gate_total} gate_match={gate_match} gate_mismatch={gate_mismatch} match_pct={pct:.2f} gate_metric=closed_chat_match_rate")
    if pct < 100.0 - tol:
        print(f"FAIL: closed_chat_match_rate {pct:.2f} below tolerance")
        sys.exit(1)
    print("PASS")
    sys.exit(0)

pct = 100.0 * gate_match / gate_total
print(f"shadow_compare gate_total={gate_total} gate_match={gate_match} gate_mismatch={gate_mismatch} match_pct={pct:.2f}")
if pct < 100.0 - tol:
    print(f"FAIL: match_pct {pct:.2f} below tolerance")
    sys.exit(1)
print("PASS")
PY
