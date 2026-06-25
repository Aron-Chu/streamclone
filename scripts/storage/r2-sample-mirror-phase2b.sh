#!/usr/bin/env bash
# Phase 2B — mirror tiny sample rows from sample-manifest-phase2a.csv to R2 staging.
#
# Allowed artifact_type: analytics_rollups, analytics_stream, bronze_vod_catalog,
#   emote_snapshot (only if present in manifest).
# Forbidden prefixes: postgres/nightly/, vod_chat/, tt-detail/
#
# Default: print plan and exit. Set EXECUTE=1 to run mirror + verification.
#
# Env (never commit):
#   AZURE_CONN_FILE, R2_STAGING_ENV_FILE, CLOUDFLARE_ACCOUNT_ID
# Optional: MANIFEST_CSV, MIRROR_LOG_CSV, R2_BUCKET, CONCURRENCY (default 3)
#
# See docs/storage/azure-to-r2-migration.md § Phase 2B.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
EXECUTE="${EXECUTE:-0}"
R2_BUCKET="${R2_BUCKET:-streampulse-artifacts-staging}"
CLOUDFLARE_ACCOUNT_ID="${CLOUDFLARE_ACCOUNT_ID:-51dd8007b22ac92482388d8b6cdbb6e3}"
AZURE_CONTAINER="${AZURE_CONTAINER:-streamclone-archive}"
AZURE_PREFIX="${AZURE_PREFIX:-streamclone}"
CONCURRENCY="${CONCURRENCY:-3}"
MANIFEST_CSV="${MANIFEST_CSV:-$ROOT/docs/storage/sample-manifest-phase2a.csv}"
MIRROR_LOG_CSV="${MIRROR_LOG_CSV:-$ROOT/docs/storage/sample-mirror-phase2b.csv}"
TEMP_DIR="${TEMP_DIR:-/tmp/r2-phase2b-mirror-$$}"
CONN_FILE="${AZURE_CONN_FILE:-$HOME/.streamclone/azure-archive-connection-string}"
if [[ -f "/mnt/c/Users/Aron/.streamclone/azure-archive-connection-string" && ! -f "$CONN_FILE" ]]; then
  CONN_FILE="/mnt/c/Users/Aron/.streamclone/azure-archive-connection-string"
fi
R2_ENDPOINT="${R2_ENDPOINT:-https://${CLOUDFLARE_ACCOUNT_ID}.r2.cloudflarestorage.com}"
ENV_FILE="${R2_STAGING_ENV_FILE:-$HOME/.streamclone/r2-staging-s3.env}"
if [[ -f "/mnt/c/Users/Aron/.streamclone/r2-staging-s3.env" && ! -f "$ENV_FILE" ]]; then
  ENV_FILE="/mnt/c/Users/Aron/.streamclone/r2-staging-s3.env"
fi
if [[ -f "$ENV_FILE" ]]; then
  set -a
  # shellcheck disable=SC1090
  source "$ENV_FILE"
  set +a
fi
export AWS_DEFAULT_REGION="${AWS_DEFAULT_REGION:-auto}"

ALLOWED_TYPES="analytics_rollups analytics_stream bronze_vod_catalog emote_snapshot"

r2_put() {
  local file="$1"
  local key="$2"
  if [[ -n "${AWS_ACCESS_KEY_ID:-}" && -n "${AWS_SECRET_ACCESS_KEY:-}" ]]; then
    aws s3 cp "$file" "s3://${R2_BUCKET}/${key}" --endpoint-url "$R2_ENDPOINT" --only-show-errors
  else
    (cd "$ROOT" && CLOUDFLARE_ACCOUNT_ID="$CLOUDFLARE_ACCOUNT_ID" npx wrangler r2 object put "${R2_BUCKET}/${key}" --file "$file" --remote)
  fi
}

r2_get() {
  local key="$1"
  local dest="$2"
  if [[ -n "${AWS_ACCESS_KEY_ID:-}" && -n "${AWS_SECRET_ACCESS_KEY:-}" ]]; then
    aws s3 cp "s3://${R2_BUCKET}/${key}" "$dest" --endpoint-url "$R2_ENDPOINT" --only-show-errors
    aws s3api head-object --bucket "$R2_BUCKET" --key "$key" --endpoint-url "$R2_ENDPOINT" \
      --query ContentLength --output text
  else
    (cd "$ROOT" && CLOUDFLARE_ACCOUNT_ID="$CLOUDFLARE_ACCOUNT_ID" npx wrangler r2 object get "${R2_BUCKET}/${key}" --file "$dest" --remote)
    wc -c < "$dest" | tr -d ' '
  fi
}

mirror_one() {
  local idx="$1"
  local artifact_type="$2"
  local natural_key="$3"
  local gcs_uri="$4"
  local expected_bytes="$5"
  local expected_etag="$6"
  local r2_key="$7"
  local row_file="${TEMP_DIR}/row-${idx}.result"

  # Derive Azure blob path from gcs_uri (after container name).
  local azure_blob
  azure_blob="${gcs_uri#*${AZURE_CONTAINER}/}"
  if [[ "$azure_blob" == "$gcs_uri" || -z "$azure_blob" ]]; then
    echo "fail,bad_gcs_uri" > "$row_file"
    return 1
  fi

  for forbidden in postgres/nightly vod_chat tt-detail; do
    if [[ "$azure_blob" == *"${forbidden}"* ]]; then
      echo "skip,forbidden_prefix" > "$row_file"
      return 0
    fi
  done

  local ok=0
  local status="fail"
  local gzip_ok="n/a"
  local azure_unchanged="no"
  local sha256=""
  local r2_bytes=""
  local verified_at
  verified_at="$(date -u +"%Y-%m-%dT%H:%M:%SZ")"
  local local_file="${TEMP_DIR}/obj-${idx}.bin"
  local r2_file="${TEMP_DIR}/obj-${idx}.r2.bin"

  local azure_bytes_meta before_etag
  if ! read -r azure_bytes_meta before_etag < <(
    az storage blob show --container-name "$AZURE_CONTAINER" --name "$azure_blob" \
      --query "[properties.contentLength, properties.etag]" -o tsv 2>/dev/null | tr -d '\r'
  ); then
    echo "fail,azure_metadata" > "$row_file"
    return 1
  fi
  before_etag="${before_etag//\"/}"

  if ! az storage blob download --container-name "$AZURE_CONTAINER" --name "$azure_blob" \
      --file "$local_file" --only-show-errors >/dev/null 2>&1; then
    echo "fail,azure_download" > "$row_file"
    return 1
  fi

  local actual_bytes
  actual_bytes="$(wc -c < "$local_file" | tr -d ' ')"
  sha256="$(sha256sum "$local_file" | awk '{print $1}')"

  if [[ "$actual_bytes" != "$azure_bytes_meta" ]]; then
    echo "fail,azure_size_mismatch meta=${azure_bytes_meta} actual=${actual_bytes}" > "$row_file"
    return 1
  fi

  if [[ "$r2_key" == *.gz ]]; then
    if gzip -t "$local_file" 2>/dev/null; then gzip_ok="yes"; else gzip_ok="no"; fi
  fi

  if ! r2_put "$local_file" "$r2_key"; then
    echo "fail,r2_upload" > "$row_file"
    return 1
  fi

  if ! r2_bytes="$(r2_get "$r2_key" "$r2_file")"; then
    echo "fail,r2_download" > "$row_file"
    return 1
  fi

  local r2_sha
  r2_sha="$(sha256sum "$r2_file" | awk '{print $1}')"
  if [[ "$sha256" != "$r2_sha" ]] || ! cmp -s "$local_file" "$r2_file"; then
    echo "fail,sha256_or_bytes_mismatch" > "$row_file"
    return 1
  fi

  if [[ "$r2_bytes" != "$actual_bytes" ]]; then
    echo "fail,r2_size_mismatch" > "$row_file"
    return 1
  fi

  local after_etag after_bytes
  read -r after_bytes after_etag < <(
    az storage blob show --container-name "$AZURE_CONTAINER" --name "$azure_blob" \
      --query "[properties.contentLength, properties.etag]" -o tsv 2>/dev/null | tr -d '\r'
  )
  after_etag="${after_etag//\"/}"
  if [[ "$before_etag" == "$after_etag" && "$after_bytes" == "$azure_bytes_meta" ]]; then
    azure_unchanged="yes"
  else
    echo "fail,azure_etag_or_size_changed" > "$row_file"
    return 1
  fi

  status="ok"
  printf '%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n' \
    "$artifact_type" "$natural_key" "$gcs_uri" "$R2_BUCKET" "$r2_key" \
    "$actual_bytes" "$r2_bytes" "$sha256" "$gzip_ok" "$azure_unchanged" "$status" "$verified_at" \
    > "$row_file"
  rm -f "$local_file" "$r2_file"
}

export -f mirror_one r2_put r2_get
export ROOT R2_BUCKET AZURE_CONTAINER AZURE_PREFIX TEMP_DIR R2_ENDPOINT CLOUDFLARE_ACCOUNT_ID
export AWS_ACCESS_KEY_ID AWS_SECRET_ACCESS_KEY AWS_DEFAULT_REGION AZURE_STORAGE_CONNECTION_STRING

echo "== Phase 2B sample mirror =="
echo "execute=${EXECUTE} manifest=${MANIFEST_CSV} bucket=${R2_BUCKET} concurrency=${CONCURRENCY}"
echo "log=${MIRROR_LOG_CSV}"
echo ""

if [[ ! -f "$MANIFEST_CSV" ]]; then
  echo "ERROR: manifest not found: $MANIFEST_CSV" >&2
  exit 1
fi

# Build work list via Python (robust CSV; manifest may be UTF-8 or UTF-16 from Windows).
WORK_TSV="${TEMP_DIR}.work.tsv"
python3 - "$MANIFEST_CSV" "$WORK_TSV" <<'PY'
import csv, sys

def read_manifest(path):
    raw = open(path, "rb").read()
    if raw.startswith(b"\xff\xfe"):
        payload = raw[2:]
        if len(payload) % 2 == 1:
            payload = payload[:-1]
        text = payload.decode("utf-16-le")
    elif raw.startswith(b"\xfe\xff"):
        payload = raw[2:]
        if len(payload) % 2 == 1:
            payload = payload[:-1]
        text = payload.decode("utf-16-be")
    else:
        text = raw.decode("utf-8-sig")
    rows = list(csv.DictReader(text.splitlines()))
    if not rows or "artifact_type" not in rows[0]:
        raise SystemExit(f"manifest missing artifact_type column: {path}")
    return rows

allowed = {"analytics_rollups", "analytics_stream", "bronze_vod_catalog", "emote_snapshot"}
manifest, out = sys.argv[1], sys.argv[2]
rows = []
for i, row in enumerate(read_manifest(manifest)):
        t = row["artifact_type"].strip()
        if t not in allowed:
            continue
        r2_key = row["proposed_r2_key"].strip()
        if any(x in r2_key for x in ("postgres/nightly", "vod_chat", "tt-detail")):
            continue
        etag = row["etag"].strip().strip('"')
        rows.append((
            str(i + 1),
            t,
            row["natural_key"].strip(),
            row["gcs_uri"].strip(),
            row["byte_size"].strip(),
            etag,
            r2_key,
        ))
with open(out, "w", encoding="utf-8") as f:
    for r in rows:
        f.write("\t".join(r) + "\n")
print(f"selected_rows={len(rows)}", file=sys.stderr)
PY

SELECTED="$(wc -l < "$WORK_TSV" | tr -d ' ')"
echo "selected_objects=${SELECTED}"
echo ""

if [[ "$EXECUTE" != "1" ]]; then
  echo "PREPARED ONLY — set EXECUTE=1 to mirror and verify."
  head -5 "$WORK_TSV" | while IFS=$'\t' read -r idx t nk _ _ _ rk; do
    echo "  [$idx] $t $nk -> $rk"
  done
  rm -f "$WORK_TSV"
  exit 0
fi

export AZURE_STORAGE_CONNECTION_STRING
AZURE_STORAGE_CONNECTION_STRING="$(tr -d '\r\n' < "$CONN_FILE")"
mkdir -p "$TEMP_DIR"

echo "artifact_type,natural_key,azure_uri,r2_bucket,r2_key,azure_bytes,r2_bytes,sha256,gzip_ok,azure_unchanged,status,verified_at" > "$MIRROR_LOG_CSV"

FAIL=0
RUNNING=0
PIDS=()
while IFS=$'\t' read -r idx artifact_type natural_key gcs_uri expected_bytes expected_etag r2_key; do
  while (( RUNNING >= CONCURRENCY )); do
    if wait -n 2>/dev/null; then :; else wait "${PIDS[0]}"; unset 'PIDS[0]'; PIDS=("${PIDS[@]}") || true; fi
    ((RUNNING--)) || true
  done
  mirror_one "$idx" "$artifact_type" "$natural_key" "$gcs_uri" "$expected_bytes" "$expected_etag" "$r2_key" &
  PIDS+=("$!")
  ((RUNNING++)) || true
done < "$WORK_TSV"

for pid in "${PIDS[@]}"; do
  wait "$pid" || FAIL=1
done

# Merge row results in index order.
python3 - "$TEMP_DIR" "$WORK_TSV" "$MIRROR_LOG_CSV" "$R2_BUCKET" <<'PY'
import csv, os, sys

temp_dir, work_tsv, log_csv, r2_bucket = sys.argv[1], sys.argv[2], sys.argv[3], sys.argv[4]
header = ["artifact_type","natural_key","azure_uri","r2_bucket","r2_key",
          "azure_bytes","r2_bytes","sha256","gzip_ok","azure_unchanged","status","verified_at"]
rows = []
fail = 0
with open(work_tsv, encoding="utf-8") as f:
    for line in f:
        parts = line.rstrip("\n").split("\t")
        idx = parts[0]
        rf = os.path.join(temp_dir, f"row-{idx}.result")
        if not os.path.isfile(rf):
            fail += 1
            rows.append([parts[1], parts[2], parts[3], r2_bucket, parts[6],
                         parts[4], "", "", "", "no", "fail,missing_result", ""])
            continue
        body = open(rf, encoding="utf-8").read().strip()
        if body.startswith("fail,") or body.startswith("skip,"):
            fail += 1
            rows.append([parts[1], parts[2], parts[3], r2_bucket, parts[6],
                         parts[4], "", "", "", "no", body, ""])
            continue
        fields = body.split("\t")
        if len(fields) != 12 or fields[10] != "ok":
            fail += 1
            rows.append([parts[1], parts[2], parts[3], r2_bucket, parts[6],
                         parts[4], "", "", "", "no", "fail,invalid_result", ""])
            continue
        rows.append(fields)
with open(log_csv, "w", newline="", encoding="utf-8") as out:
    w = csv.writer(out)
    w.writerow(header)
    w.writerows(rows)
ok = sum(1 for r in rows if r[10] == "ok")
print(f"summary ok={ok} total={len(rows)} fail={fail}")
sys.exit(0 if ok == len(rows) and fail == 0 else 1)
PY
RESULT=$?

rm -rf "$TEMP_DIR" "$WORK_TSV"

if [[ "$RESULT" -ne 0 ]]; then
  echo "ERROR: one or more objects failed — see $MIRROR_LOG_CSV" >&2
  exit 1
fi

echo "phase2b_mirror_complete log=$MIRROR_LOG_CSV"
