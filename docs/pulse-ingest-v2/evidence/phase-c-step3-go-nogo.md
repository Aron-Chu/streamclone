# Phase C gates + Phase D GO/NO-GO — template

Run after shadow soak:

```bash
bash scripts/ingest-phase-c-gates.sh 2 1000
```

## Current status (pre-shadow deploy)

| Gate | Status |
|------|--------|
| Shadow compare ≥99% | **N/A** — shadow not running |
| Drops ~0 | **N/A** |
| Redis delta flat | **N/A** — needs Step 0 VPS baseline |
| Memory flat | **N/A** |
| Rollup p95 vs baseline | **N/A** |
| Hub/moments healthy | **PARTIAL** — hub OK; moments Cache-Control **missing on prod** |
| Artifacts bounded | **N/A** |
| Phase C env proof | **N/A** |

## Phase D recommendation

**NO-GO** until:

1. Step 0 VPS baseline complete
2. Moments Cache-Control deployed
3. Docker limits applied + stable (15–30 min per service)
4. Phase C shadow env running ≥12h full fleet
5. `scripts/ingest-phase-c-gates.sh` exits 0
6. `scripts/ingest-shadow-compare.sh 2 1000` PASS

## Phase C safety assertion (required in gates output)

```text
INGEST_CORE_ENABLED=0
INGEST_CORE_DUAL_READ_MODE=1
INGEST_CORE_SHADOW_MODE=1
WritesProduction() false — legacy collector sole PG writer
```
