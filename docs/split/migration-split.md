# Migration split (boundary split)

128 migration files today under `migrations/` (64 `.up.sql` + paired `.down.sql`). **Never edit applied migrations** in either repo.

---

## Ownership classes

| Class | Migration examples | Owner repo | Desktop install needs? |
|-------|-------------------|------------|------------------------|
| **Core** | `000001_init`, `000002_emotes`, `000003_provider_emotes`, `000004_channel_emote_providers`, `000011_local_follows`, `000013_chat_logs`, `000014_chat_logs_search` | **streamclone** | Yes |
| **Analytics / Pulse** | `000005_analytics`, `000008_*checkpoints*`, `000012_vod_chat`, `000019_pulse_*`, `000039_watchlist`, `000042_roster`, `000056_collector_leases`, `000060_rollups*`, `000061_peaks`, … | **streampulse-backend** | No |
| **Storygraph / Pulse Wire** | `000015_story_graph*`, `000016_story_graph_social`, `000020_story_*`, … | **streampulse-backend** | No |
| **Corpus / archive / backfill** | `000030_archive*`, `000033_backfill`, `000034_bronze`, `000049_gold_vod`, `000057_atlas*`, … | **streampulse-backend** | No |
| **Clipper / ReplayForge** | `000062_auto_clipper*`, `000063_replayforge*` | **streampulse-backend** | No |

Full file list: `ls migrations/*.up.sql` — classify any unlisted file as backend unless it matches core table above.

---

## `schema_migrations` strategy

`golang-migrate` stores **one version row** + `dirty` flag — not one row per historical file.

| Install | Migration path | Table | Baseline |
|---------|----------------|-------|----------|
| **Hosted production** (existing PG) | backend `migrations/` | `schema_migrations_backend` (override) or default after cutover lock | Ops force `version=PRE_SPLIT_MAX`, `dirty=false` without re-running historical SQL |
| **Backend local dev** | backend bundle | same as prod | Empty DB applies full copied history + `100000_+` |
| **Fresh public desktop** | core-only subset | default public table | Core migrations only |
| **Upgraded pre-split desktop** | core-only | existing public table | New core files numbered **> PRE_SPLIT_MAX** |

**PRE_SPLIT_MAX:** archive from production before cutover → private ops `docs/deployments/boundary-split-rollback.md`.

---

## Numbering after split

| Repo | Rule |
|------|------|
| **streamclone** | Continue core sequence; freeze historical analytics numbers in public bundle removal |
| **streampulse-backend** | Copied history keeps original `000NNN`; **new** backend-only files start at **`100000_`** prefix |

---

## Anti-patterns (forbidden)

- Inserting one row per historical migration into golang-migrate state tables
- Re-running analytics `.up.sql` on production during cutover
- New public core migrations numbered ≤ `PRE_SPLIT_MAX` on upgraded installs
- Public Streamclone migrate job against hosted production DSN after cutover

---

## Checklist

- [ ] Production `SELECT version, dirty FROM schema_migrations` archived in ops
- [ ] Backend baseline documented with `PRE_SPLIT_MAX`
- [ ] Backend fresh-DB migration test from empty
- [ ] Public fresh-DB core-only migration test
- [ ] Existing-install simulation at `version = PRE_SPLIT_MAX`
