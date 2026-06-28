# Chat benchmark planning scaffold (local-only)

`chat_benchmark_plan.py` supports **Batch P1-MAX Lane D** and future `BENCH-STORAGE-001` planning.

`ivr_chat_benchmark.py` implements **BENCH-IVR-001** (logs.ivr.fi vs GQL) — evidence-only; no production writes.

## Safety defaults

- **`--no-network` is the default** (implicit when `--allow-live` is omitted).
- `chat_benchmark_plan.py`: reads fixture markdown metadata only; **no Twitch, GQL, IRC, gold, or scraper calls**.
- `ivr_chat_benchmark.py`: `--allow-live` required for IVR/GQL; kill switches on bytes/messages/duration; GQL lane read-only (no queue/sync/rollup writes).

## Usage

```powershell
cd c:\Users\Aron\twitch-7tv-clone

python scripts/bench/chat_benchmark_plan.py --help

python scripts/bench/chat_benchmark_plan.py `
  --fixture scripts/bench/fixtures/sample-light.md `
  --no-network
```

## BENCH-STORAGE-001 fixture selection

Pick light, medium, and heavy candidates from existing confirmed archive exports and rollups only:

```powershell
bash -lc 'bash scripts/bench/bench-storage-fixture-candidates.sh --target bearhost --csv > runtime/bench-storage-fixture-candidates.csv'
```

The selector is read-only. It does not fetch Twitch/GQL, enqueue silver/gold, mirror objects, mutate Postgres, or change hosted caps. Prefer candidates with `has_vod_chat_payload=true`; `has_vod_chat_export=true` with a tiny byte count only proves an archive row exists, not that it is useful for raw chat compression. If a class only has rollup exports, treat it as a readback/storage-layout candidate until a VOD chat export is approved.

Run readback validation against the selected rollup artifacts:

```powershell
bash scripts/bench/bench-storage-readback.sh --candidate-csv runtime/bench-storage-fixture-candidates.csv --artifact rollups --out-csv runtime/bench-storage-readback-rollups.csv
```

Readback validation performs HTTP GETs for selected artifact URLs, then falls back to Azure CLI readback for private Azure Blob URLs when `AZURE_CONN_FILE` is available. Use `--source r2 --mirror-csv runtime/sample-mirror-phase2b-dryrun.csv` after R2 mirror execute.

### Hosted rollout lane scripts

Operator checklists live under `scripts/hosted-rollout/` — see [`scripts/hosted-rollout/README.md`](../hosted-rollout/README.md).

### Bounded VOD chat fixture plan

```powershell
bash scripts/bench/bench-vod-chat-fixture-plan.sh
```

Read-only plan for one light/medium/heavy export each. `EXECUTE=1` is blocked in-script; operator enqueues gold manually after approval.

### BENCH-IVR-001 (IVR vs GQL)

```powershell
# Dry-run validation (no network)
python scripts/bench/ivr_chat_benchmark.py `
  --fixture ..\streamclone-pulse\docs\pulse-extension\fixtures `
  --fixture-id bench-ivr-jynxzi-gql-hot-chat `
  --lane all `
  --window smoke `
  --dry-run

# IVR smoke on positive control (Ludwig)
python scripts/bench/ivr_chat_benchmark.py `
  --fixture ..\streamclone-pulse\docs\pulse-extension\fixtures `
  --fixture-id bench-ivr-ludwig-positive-control `
  --allow-live `
  --lane all `
  --window smoke `
  --output-dir runtime/bench-evidence

# GQL smoke on Jynxzi hot-chat baseline
python scripts/bench/ivr_chat_benchmark.py `
  --fixture ..\streamclone-pulse\docs\pulse-extension\fixtures `
  --fixture-id bench-ivr-jynxzi-gql-hot-chat `
  --allow-live `
  --lane gql `
  --window smoke `
  --output-dir runtime/bench-evidence

# Unit tests (no network)
python -m unittest scripts/bench/tests/test_ivr_chat_benchmark.py
```

Fixture docs live in **streamclone-pulse** — see [`docs/pulse-extension/evidence/bench-ivr-001.md`](../../streamclone-pulse/docs/pulse-extension/evidence/bench-ivr-001.md).

### PROD_SHADOW_CANARY_ONLY runtime proof

End-to-end shadow → GQL → reconcile against local Docker stack (no Azure archive):

```bash
bash scripts/bench/ivr-shadow-reconcile-proof.sh
```

Checks migration 000050, writes artifacts under `runtime/ivr-shadow/`, confirms `gql/canonical` DB rows and zero `ivr` rollup rows. BearHost overlay: `deploy/env/profile-bearhost-corpus-ivr-shadow.env`.

## Fixture format

BENCH-IVR-001 uses strict JSON fixtures. Markdown files are evidence and operator notes only.

Canonical fixture files live in `streamclone-pulse/docs/pulse-extension/fixtures/`:

- `bench-ivr-jynxzi-gql.json` — GQL-only hot-chat baseline; `ivr_expected=false`.
- `bench-ivr-ludwig-positive.json` — IVR-covered positive control; `ivr_expected=true`.

The parser rejects duplicate `fixture_id`s, unknown keys, legacy generic table keys such as `smoke_from`, invalid time ranges, and lane/fixture mismatches.

## Does not complete

This scaffold does **not** complete runtime benchmark tasks (`BENCH-002`–`005`, `LOAD-CHAT-001`, `LOAD-GOLD-001`) without explicit operator approval.

`BENCH-IVR-001` long runs remain **HOLD** until dry-run output and positive-control IVR smoke are reviewed. GQL remains read-only and does not enqueue or mutate production state.
