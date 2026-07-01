# Corpus hosted baseline — 2026-07-01 (read-only)

**Source:** `streamclone-hosted-data` MCP + `GET /v1/public/hub`
**Flag state:** `GOLD_VOD_SEGMENTS_ENABLED` expected **false** on prod (pre-canary).

## Checklist steps 1–4

| Check | Result |
|-------|--------|
| `to_regclass('gold_vod_segments')` | `gold_vod_segments` present |
| Segment ledger rows | **0** (empty — flag not enabled yet) |
| Public hub segment leak | **None** — 14 top-level keys; `corpusPipeline` aggregate only |

### `backfill_jobs` (snapshot)

| tier | status | count |
|------|--------|------:|
| gold | done | 457 |
| gold | failed | 67 |
| gold | queued | 459 |
| gold | running | 1 |
| gold | skipped | 371 |
| gold_full | done | 5 |
| silver | done | 6597 |
| silver | failed | 2816 |
| silver | skipped | 12750 |

### `top500_vod_inventory.gold_status`

| status | count |
|--------|------:|
| failed | 4 |
| not_queued | 21 |

## Next operator steps

1. Push/deploy PR 0B stack (`b3bc4b6` … `d76c1af`) to streampulse-vps corpus workers.
2. Run migrate on BearHost if not already at `000049`.
3. Canary: `GOLD_VOD_SEGMENTS_ENABLED=true` on **one** worker; complete checklist §5–9 in [`corpus-0b2-hosted-verify.md`](corpus-0b2-hosted-verify.md).
