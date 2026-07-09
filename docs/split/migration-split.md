# Migration split (boundary split)

128 migration files today under `migrations/` (64 `.up.sql` + paired `.down.sql`). **Never edit applied migrations** in either repo.

Field names for the production migration row: [`migration-state.md`](migration-state.md) (`pre_cutover_schema_version`, `pre_cutover_schema_dirty`). **Do not** use placeholder version numbers in public docs until ops records them.

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
| **Hosted production** (existing PG) | backend `migrations/` | `schema_migrations_backend` (override) or default after cutover lock | Ops sets `version` / `dirty` to recorded `pre_cutover_schema_version` / `pre_cutover_schema_dirty` without re-running historical SQL |
| **Backend local dev** | backend bundle | same as prod policy | Empty DB applies full copied history + `100000_+` |
| **Fresh public desktop** | core-only subset | default public table | Core migrations only from empty DB |
| **Upgraded pre-split desktop** | core-only | existing public table | New core files numbered **> `pre_cutover_schema_version`** (ops-recorded) |

---

## Fresh vs upgraded paths

| Scenario | Repo | Steps |
|----------|------|-------|
| **Fresh desktop install** | streamclone | Core-only migrate bundle; empty DB; no backend analytics files in public tree after final deletion PR |
| **Backend developer** | streampulse-backend | Full copied history; empty local PG; verify migrate up from zero |
| **Old desktop upgrade (pre-split)** | streamclone | Simulate DB already at ops-recorded `pre_cutover_schema_version`; apply next **core-only** migration numbered above that version; confirm golang-migrate does not downgrade or re-apply removed analytics files |

---

## Numbering after split

**Upgraded installs win:** new public core migrations must be numbered **greater than** ops-recorded `pre_cutover_schema_version`, even though many lower analytics numbers are removed from the public bundle.

| Repo | Rule |
|------|------|
| **streamclone** | **Fresh install:** core-only subset. **Upgraded install:** next core file **> `pre_cutover_schema_version`**, not “continue from last core 000NNN” |
| **streampulse-backend** | Copied history keeps original `000NNN`; **new** backend-only files start at **`100000_`** prefix |

---

## Anti-patterns (forbidden)

- Inserting one row per historical migration into golang-migrate state tables
- Re-running analytics `.up.sql` on production during cutover
- New public core migrations numbered ≤ ops-recorded `pre_cutover_schema_version` on upgraded installs
- Public Streamclone migrate job against hosted production DSN after cutover
- Publishing guessed `version` / `dirty` values in this public repo (use [`migration-state.md`](migration-state.md) field names only)

---

## Checklist

### Public (this repo)

- [x] Ownership class table and numbering rules documented
- [x] [`migration-state.md`](migration-state.md) field template (no live prod values)
- [ ] Public fresh-DB core-only migration test (after core subset is defined in deletion PR)
- [ ] Upgraded-install simulation documented with ops-recorded `pre_cutover_schema_version` (blocked on ops query)

### Ops-owned (streampulse-ops)

- [ ] Production `SELECT version, dirty FROM schema_migrations` archived (`pre_cutover_*` fields)
- [ ] Backend baseline and rollback anchor updated with recorded row
- [ ] Backend fresh-DB migration test from empty
