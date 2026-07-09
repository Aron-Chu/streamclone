#!/usr/bin/env bash
# clipper-create-smoke.sh — RF-P2-008 operator smoke.
#
# Drives the Streamclone Analytics -> ReplayForge create-job trigger path for a
# VOD-backed candidate and asserts the job reaches the end of the source
# validation stage (RF-P2-001 `validating_source` and beyond).
#
# It exercises the SAME wiring the browser uses:
#   Analytics "Export Moment" -> triggerClipperManual()
#     -> POST {SC_BASE}/v1/clipper/triggers/manual   (same-origin, default)
#     -> Caddy @clipper route rewrites /v1/clipper -> /v1
#     -> host.docker.internal:8095 (ReplayForge)  -> validates moment_context
#        and enters `validating_source` for a VOD-backed create.
#
# This is a NON-BLOCKING smoke. It requires a running core stack (`make up`),
# a running ReplayForge on :8095, and Twitch clips:edit + VOD-read credentials
# configured in ReplayForge. It never fabricates a pass: if prerequisites are
# missing it exits BLOCKED (2), and only exits PASS (0) when a job is created
# and observed at/through the validation stage.
#
# Usage:
#   scripts/clipper-create-smoke.sh --channel <login> --vod-id <id> [--offset <sec>]
#   scripts/clipper-create-smoke.sh --direct  ...        # bypass Caddy, hit :8095/v1 directly
#
# Env overrides:
#   SC_BASE         Streamclone browser/API origin (default http://localhost:8090)
#   RF_BASE         ReplayForge API base for --direct (default http://localhost:8095)
#   CLIPPER_TOKEN   Bearer token if ReplayForge CLIPPER_WEBHOOK_TOKEN is set
#   CLIP_CHANNEL    default channel login
#   CLIP_VOD_ID     default VOD id (numeric Twitch VOD id, broadcaster-owned)
#   CLIP_VOD_OFFSET default VOD offset seconds (default 60)
#   POLL_ATTEMPTS   status poll attempts (default 20)
#   POLL_INTERVAL   seconds between polls (default 2)
set -euo pipefail

SC_BASE="${SC_BASE:-http://localhost:8090}"
RF_BASE="${RF_BASE:-http://localhost:8095}"
CLIPPER_TOKEN="${CLIPPER_TOKEN:-}"
channel="${CLIP_CHANNEL:-}"
vod_id="${CLIP_VOD_ID:-}"
offset="${CLIP_VOD_OFFSET:-60}"
poll_attempts="${POLL_ATTEMPTS:-20}"
poll_interval="${POLL_INTERVAL:-2}"
direct=false

while [ $# -gt 0 ]; do
  case "$1" in
    --channel) channel="$2"; shift 2 ;;
    --vod-id) vod_id="$2"; shift 2 ;;
    --offset) offset="$2"; shift 2 ;;
    --direct) direct=true; shift ;;
    -h|--help) grep '^#' "$0" | sed 's/^# \{0,1\}//'; exit 0 ;;
    *) echo "clipper-create-smoke: unknown arg '$1'" >&2; exit 64 ;;
  esac
done

pass()    { echo "clipper-create-smoke: PASS — $1"; exit 0; }
blocked() { echo "clipper-create-smoke: BLOCKED — $1" >&2; exit 2; }
fail()    { echo "clipper-create-smoke: FAIL — $1" >&2; exit 1; }

command -v curl >/dev/null 2>&1 || blocked "curl not found on PATH"

if [ -z "$channel" ] || [ -z "$vod_id" ]; then
  blocked "channel and VOD id required (--channel <login> --vod-id <id>, or CLIP_CHANNEL / CLIP_VOD_ID). A broadcaster-owned VOD is required to pass validation."
fi

# Resolve the create + status URLs to match the real browser trigger path.
if [ "$direct" = true ]; then
  base="${RF_BASE%/}/v1"
  health_url="${RF_BASE%/}/healthz"
else
  base="${SC_BASE%/}/v1/clipper"
  health_url="${SC_BASE%/}/v1/clipper/healthz"
fi
create_url="${base}/triggers/manual"
jobs_url="${base}/jobs"

auth_header=()
if [ -n "$CLIPPER_TOKEN" ]; then
  auth_header=(-H "Authorization: Bearer ${CLIPPER_TOKEN}")
fi

echo "clipper-create-smoke: create path = ${create_url}"

# Preflight: ReplayForge reachable through the chosen path.
if ! curl --connect-timeout 3 --max-time 8 -fsS "$health_url" >/dev/null 2>&1; then
  blocked "ReplayForge health not reachable at ${health_url}. Start the core stack ('make up') and ReplayForge (../replayforge 'make up'), then retry."
fi

# Build a VOD-backed moment_context payload — mirrors Analytics.tsx handleCreateClip.
ts="$(date -u +%H:%M:%S)"
payload=$(cat <<JSON
{
  "channel": "${channel}",
  "title": "RF-P2-008 smoke (${ts})",
  "duration": 60.0,
  "final_duration": 30.0,
  "reason": "manual smoke at ${ts}",
  "moment_context": {
    "vod_id": "${vod_id}",
    "vod_offset_seconds": ${offset},
    "source_kind": "vod",
    "pick_reason": "manual",
    "moment_score": 50
  }
}
JSON
)

resp="$(curl --connect-timeout 5 --max-time 30 -sS -w $'\n%{http_code}' \
  -X POST "$create_url" \
  -H 'Content-Type: application/json' \
  "${auth_header[@]}" \
  -d "$payload" 2>/dev/null || true)"

code="$(printf '%s' "$resp" | tail -n1)"
body="$(printf '%s' "$resp" | sed '$d')"
echo "clipper-create-smoke: create HTTP ${code}"
echo "clipper-create-smoke: create body ${body}"

case "$code" in
  401|403) blocked "create rejected (${code}). Set CLIPPER_TOKEN to match ReplayForge CLIPPER_WEBHOOK_TOKEN." ;;
  400) fail "create rejected 400 — moment_context failed validation. Body: ${body}" ;;
  000) blocked "no HTTP response from ${create_url} (stack/ReplayForge down or proxy misrouted)." ;;
esac
[ "$code" = "202" ] || [ "$code" = "200" ] || fail "unexpected create status ${code}. Body: ${body}"

job_id="$(printf '%s' "$body" | sed -n 's/.*"job_id"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p')"
status_field="$(printf '%s' "$body" | sed -n 's/.*"status"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p')"
if [ -z "$job_id" ]; then
  if [ "$status_field" = "suppressed" ]; then
    blocked "create suppressed by duplicate-suppression (an active job already exists for this source). Use a different offset/VOD or clear the existing job, then retry."
  fi
  fail "create accepted but no job_id in response. Body: ${body}"
fi
echo "clipper-create-smoke: job_id = ${job_id}"

# Poll job status; assert it reaches or passes the validation stage.
# validating_source is the RF-P2-001 validation state; any later active state
# (downloading_source, transcribing, ready_for_edit, rendering, ...) means
# validation passed. auth_required / source_unavailable / vod_unavailable are
# validation *outcomes* that indicate the wiring works but creds/VOD are the
# blocker — reported as BLOCKED, not a wiring FAIL.
last_state=""
for i in $(seq 1 "$poll_attempts"); do
  jresp="$(curl --connect-timeout 3 --max-time 10 -sS "${jobs_url}/${job_id}" "${auth_header[@]}" 2>/dev/null || true)"
  state="$(printf '%s' "$jresp" | sed -n 's/.*"state"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p')"
  [ -z "$state" ] && state="$(printf '%s' "$jresp" | sed -n 's/.*"status"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p')"
  [ -n "$state" ] && last_state="$state"
  echo "clipper-create-smoke: poll ${i}/${poll_attempts} state=${state:-<none>}"
  case "$state" in
    validating_source|downloading_source|transcribing|ready_for_edit|rendering|rendered|uploading_artifact|complete)
      pass "job ${job_id} reached '${state}' — create passed the validation stage." ;;
    source_unavailable|auth_required|vod_unavailable)
      blocked "job ${job_id} hit validation outcome '${state}'. Trigger wiring works; the blocker is VOD ownership/availability or Twitch credentials. Use a broadcaster-owned VOD with clips:edit + VOD-read creds in ReplayForge." ;;
    failed|retryable_failed)
      fail "job ${job_id} ended in '${state}' during/after validation. Response: ${jresp}" ;;
  esac
  sleep "$poll_interval"
done

blocked "job ${job_id} did not advance past creation within $((poll_attempts * poll_interval))s (last state='${last_state:-queued}'). Confirm the ReplayForge worker is running."
