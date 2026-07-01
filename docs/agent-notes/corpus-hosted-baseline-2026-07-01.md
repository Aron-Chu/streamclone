# Corpus hosted baseline — 2026-07-01 (read-only)

**Source:** `streamclone-hosted-data` MCP + `GET /v1/public/hub`
**Flag state:** `GOLD_VOD_SEGMENTS_ENABLED` expected **false** on prod (pre-canary).

**Refresh:** Re-run steps 1–4 after merging `feat/corpus-0b-safe-batch` (live graph isolation). Hosted MCP was unavailable during the 2026-07-01 safe-batch pass — use queries in [`corpus-0b-safe-batch.md`](corpus-0b-safe-batch.md) if MCP is down.

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

1. Merge/deploy [PR #31](https://github.com/Aron-Chu/streamclone/pull/31) (`feat/corpus-0b-gold-segments`) to streampulse-vps corpus workers.
2. Confirm BearHost Postgres at migration `000049`+ (hosted baseline verified 2026-07-01).
3. Canary: `GOLD_VOD_SEGMENTS_ENABLED=true` on **one** worker; complete checklist §5–9 in [`corpus-0b2-hosted-verify.md`](corpus-0b2-hosted-verify.md).
