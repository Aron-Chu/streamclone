#!/usr/bin/env bash
# Detailed closed-minute mismatch report for ingest shadow parity debugging.
#
# Usage:
#   bash scripts/ingest-shadow-mismatch-report.sh [OUT_DIR]
#
# Writes:
#   ${OUT_DIR}/mismatch-report.txt
#   ${OUT_DIR}/mismatch-table.md (first 10 closed mismatches)
set -euo pipefail

OUT_DIR="${1:-runtime/evidence/ingest-shadow-mismatch-$(date -u +%Y%m%dT%H%M%SZ)}"
DIR="${INGEST_SHADOW_ARTIFACT_DIR:-runtime/ingest-shadow}"
FILE="${DIR}/latest.jsonl"
mkdir -p "${OUT_DIR}"

if [[ ! -f "${FILE}" ]]; then
  echo "FAIL: shadow artifact missing: ${FILE}" >&2
  exit 1
fi

python3 - "${FILE}" "${OUT_DIR}" <<'PY'
import json, sys, os
from datetime import datetime, timedelta

path = sys.argv[1]
out_dir = sys.argv[2]
report_path = os.path.join(out_dir, "mismatch-report.txt")
table_path = os.path.join(out_dir, "mismatch-table.md")

with open(path, encoding="utf-8") as fh:
    records = [json.loads(line) for line in fh if line.strip()]

def is_closed(rec):
    k = rec.get("key") or {}
    return bool(k.get("Closed") or k.get("closed"))

def stream_channel_key(rec):
    k = rec.get("key") or {}
    return (
        k.get("StreamID") or k.get("streamID") or "",
        (k.get("Channel") or k.get("channel") or "").lower(),
        k.get("Minute") or k.get("minute") or "",
    )

def minute_dt(s):
    if not s:
        return None
    try:
        if s.endswith("Z"):
            s = s[:-1] + "+00:00"
        return datetime.fromisoformat(s.replace("Z", "+00:00"))
    except Exception:
        return None

def field(rec, *names, default=0):
    for n in names:
        if n in rec and rec[n] is not None:
            return rec[n]
    return default

index = {}
for rec in records:
    if not is_closed(rec):
        continue
    index[stream_channel_key(rec)] = rec

closed = [r for r in records if is_closed(r)]
mismatches = [r for r in closed if not r.get("match")]
matches = [r for r in closed if r.get("match")]

def chat_diff_pct(rec):
    leg = field(rec, "legacyChat", "LegacyChat")
    sh = field(rec, "shadowChat", "ShadowChat")
    if leg == 0:
        return 100.0 if sh else 0.0
    return abs(sh - leg) / leg * 100.0

def classify(rec):
    leg_c = field(rec, "legacyChat", "LegacyChat")
    sh_c = field(rec, "shadowChat", "ShadowChat")
    leg_e = field(rec, "legacyEmotes", "LegacyEmotes", "legacyTotalEmotes", "LegacyTotalEmotes")
    sh_e = field(rec, "shadowEmotes", "ShadowEmotes", "shadowTotalEmotes", "ShadowTotalEmotes")
    leg_v = field(rec, "legacyViewers", "LegacyViewers")
    sh_v = field(rec, "shadowViewers", "ShadowViewers")
    if leg_c == sh_c and leg_e == sh_e and leg_v != sh_v:
        return "viewer_only"
    dt = minute_dt((rec.get("key") or {}).get("Minute") or (rec.get("key") or {}).get("minute"))
    if dt:
        sid, ch, _ = stream_channel_key(rec)
        for delta in (-1, 1):
            adj = dt + timedelta(minutes=delta)
            adj_key = (sid, ch, adj.strftime("%Y-%m-%dT%H:%M:%SZ"))
            if adj_key in index:
                adj_rec = index[adj_key]
                adj_leg = field(adj_rec, "legacyChat", "LegacyChat")
                adj_sh = field(adj_rec, "shadowChat", "ShadowChat")
                if sh_c > leg_c and adj_leg > adj_sh:
                    return "boundary_shift_suspect"
                if sh_c < leg_c and adj_leg < adj_sh:
                    return "boundary_shift_suspect"
    if sh_c > leg_c or sh_e > leg_e:
        return "over_count"
    if sh_c < leg_c or sh_e < leg_e:
        return "under_count"
    return "other"

lines = []
lines.append(f"total_closed={len(closed)}")
lines.append(f"closed_match={len(matches)}")
lines.append(f"closed_mismatch={len(mismatches)}")
lines.append("")

table = []
table.append("# First 10 closed mismatches")
table.append("")
table.append("| key | legacy_chat | shadow_chat | legacy_emote | shadow_emote | legacy_viewer | shadow_viewer | chat_delta | emote_delta | diff_pct | classification | recordedAt |")
table.append("| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | --- | --- |")

for i, rec in enumerate(mismatches[:10]):
    k = rec.get("key") or {}
    leg_c = field(rec, "legacyChat", "LegacyChat")
    sh_c = field(rec, "shadowChat", "ShadowChat")
    leg_e = field(rec, "legacyEmotes", "LegacyEmotes", "legacyTotalEmotes", "LegacyTotalEmotes")
    sh_e = field(rec, "shadowEmotes", "ShadowEmotes", "shadowTotalEmotes", "ShadowTotalEmotes")
    leg_v = field(rec, "legacyViewers", "LegacyViewers")
    sh_v = field(rec, "shadowViewers", "ShadowViewers")
    cls = classify(rec)
    pct = chat_diff_pct(rec)
    key_str = f"{k.get('Channel') or k.get('channel','?')}|{k.get('Minute') or k.get('minute','?')}"
    recorded = rec.get("recordedAt") or rec.get("RecordedAt") or ""
    lines.append(
        f"mismatch[{i}] key={key_str} legacy_chat={leg_c} shadow_chat={sh_c} "
        f"legacy_emote={leg_e} shadow_emote={sh_e} legacy_viewer={leg_v} shadow_viewer={sh_v} "
        f"chat_delta={sh_c-leg_c} emote_delta={sh_e-leg_e} diff_pct={pct:.2f} class={cls} recordedAt={recorded}"
    )
    table.append(
        f"| {key_str} | {leg_c} | {sh_c} | {leg_e} | {sh_e} | {leg_v} | {sh_v} | {sh_c-leg_c} | {sh_e-leg_e} | {pct:.2f} | {cls} | {recorded} |"
    )

    # adjacent-minute check
    dt = minute_dt(k.get("Minute") or k.get("minute"))
    if dt:
        sid, ch, _ = stream_channel_key(rec)
        for label, delta in (("prev", -1), ("next", 1)):
            adj = dt + timedelta(minutes=delta)
            adj_key = (sid, ch, adj.strftime("%Y-%m-%dT%H:%M:%SZ"))
            adj_rec = index.get(adj_key)
            if adj_rec:
                lines.append(
                    f"  adjacent_{label} minute={adj.isoformat()}Z "
                    f"legacy_chat={field(adj_rec,'legacyChat','LegacyChat')} "
                    f"shadow_chat={field(adj_rec,'shadowChat','ShadowChat')} "
                    f"match={adj_rec.get('match')}"
                )

with open(report_path, "w", encoding="utf-8") as fh:
    fh.write("\n".join(lines) + "\n")
with open(table_path, "w", encoding="utf-8") as fh:
    fh.write("\n".join(table) + "\n")

print(f"wrote {report_path}")
print(f"wrote {table_path}")
print(f"closed_mismatch={len(mismatches)}")
PY

cat "${OUT_DIR}/mismatch-report.txt"
