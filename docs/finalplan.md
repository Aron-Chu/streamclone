# Streamclone Decoupling Final Plan

Status: architecture / implementation plan
Goal: make Streamclone durable, live-first, and less dependent on TwitchTracker for future data while keeping Core Watch stable.

## Implementation status (2026-06-20)

| Phase | Scope | Status |
|-------|--------|--------|
| **A** | Azure archive export/restore, manifest, retention guard, backup gzip | **Verified** — Writer + restore CLI + sync export hook + hourly incremental ticker (`ARCHIVE_EXPORT_INTERVAL`); nightly pg_dump path documented |
| **B** | Canonical stream sessions + dedupe | **Verified** — migration `000031`, `ResolveOrCreateSession`, startup prefetch stub cleanup for always-tracked channels |
| **C** | Tier-0 roster + Helix viewer sampler (top 200) | **Verified** — migration `000032`, roster + sampler when `TIER0_ENABLED=true`; enable via `profile-archive.env` |
| **D** | Post-end queue + `backfill_jobs` gap-fill | **Verified** — migration `000033`, `post_end.go`, `backfill_worker.go`; E2E matrix in `docs/benchmarks/archive-e2e-20260620.md` |
| **Phase 2 Bronze** | Helix VOD index + TT summary bulk index | **Verified** — migration `000034`, `BronzeIndexer`, `cmd/backfill bronze run-once`; see E2E matrix |
| **E–G** | Selective VOD chat gold, emote archive, service decouple | **Deferred** — architecture unchanged; not in this milestone (see phase bodies below) |

**E2E caveats (2026-06-20 closeout):** P0 archive blockers closed per [`docs/benchmarks/archive-e2e-20260620.md`](benchmarks/archive-e2e-20260620.md). `/v1/streams?limit=200` uses **Helix-primary** metadata when `TWITCH_OAUTH_CLIENT_ID/SECRET` are in synthesized `.env` (run `scripts/ensure-oauth-env.ps1` or `make twitch-sync`); GQL-only stacks fall back to hardened multi-page GQL. Silver backfill requires scraper profile + `ANALYTICS_TT_SYNC_TIMEOUT_MS=120000` (see `profile-archive.env`).

**Wave 0 operator checklist:** merge `ARCHIVE_AZURE_*` from Terraform/`~/.streamclone/azure-archive-connection-string` into `.env.local`; verify blob access with the connection string (not `az login`):

```powershell
$cs = Get-Content -Raw $env:USERPROFILE\.streamclone\azure-archive-connection-string
az storage blob list --container-name streamclone-archive --prefix streamclone/ --connection-string $cs
```

See [azure-archive-setup.md](azure-archive-setup.md) for deploy and [restore rollups](azure-archive-setup.md#restore-rollups).

## Summary

Streamclone should not depend on TwitchTracker as the primary source for streams we can observe live. The system should collect top-streamer data continuously through Twitch APIs, store durable archives in Azure Blob Storage, and use TwitchTracker only as a historical backfill or gap-fill source.

This plan does not claim full TwitchTracker independence for old streams that were never captured live. For historical viewer-minute charts before our archive existed, TwitchTracker or another third-party tracker remains necessary.

## Core principles

1. Core Watch must work without scraper, Pulse Wire, ReplayForge, or Grafana.
2. Postgres is hot/queryable state, not the only durable archive.
3. Azure Blob Storage is the durable source for raw archived data and restore paths.
4. Live Helix samples become the primary viewer timeline for future streams.
5. TwitchTracker is used only for missed streams, weak coverage, enrichment, and validation.
6. GQL VOD chat is selective and expensive, not run blindly for every top-200 stream.
7. Browser scraper remains a sibling service with strict queue/concurrency limits.
8. No proxy-by-default. Proxies are an optional fallback for blocked social/browser targets.

---

# Phase A — Stop losing work

## Goal

Make local Postgres disposable without forcing re-scrapes from TwitchTracker or Twitch GQL after every reset.

## Build

### 1. Archive export manifest

Create an `archive_exports` table:

```text
id
stream_id
vod_id
export_type
gcs_uri          # stores Azure blob HTTPS URI (legacy column name)
content_hash
row_count
byte_size
created_at
source_version
status
```

Export types:

```text
viewer_rollups
viewer_samples
stream_session
tt_detail_json
vod_chat_raw_jsonl
chat_rollups
emote_snapshots
emote_rollups
```

### 2. Export on sync completion

Whenever Analytics sync completes, export raw and derived artifacts to Azure Blob Storage:

```text
https://ststreamclone3lf6tt.blob.core.windows.net/streamclone-archive/streamclone/rollups/stream_id={id}/part-000.jsonl.gz
https://ststreamclone3lf6tt.blob.core.windows.net/streamclone-archive/streamclone/tt-detail/{login}/{stream_id}/page.html.gz
https://ststreamclone3lf6tt.blob.core.windows.net/streamclone-archive/streamclone/vod_chat/stream_id={id}.jsonl.gz
https://ststreamclone3lf6tt.blob.core.windows.net/streamclone-archive/streamclone/emotes/snapshots/{login}/date={date}.json
```

(Account and container come from `ARCHIVE_AZURE_*` env; prefix defaults to `streamclone/`.)

### 3. Retention guard

If archive mode is enabled, do not purge local rows unless export exists and hash verification passed.

### 4. Restore path

Add a restore command that can rebuild local Postgres state from Azure Blob without hitting TwitchTracker or Twitch GQL (`go run ./cmd/archive restore --stream-id <id>` — see [azure-archive-setup.md](azure-archive-setup.md#restore-rollups)).

## Acceptance criteria

* A completed sync can be exported to Azure Blob Storage.
* A local database reset can restore viewer rollups from Azure Blob (`archive restore --stream-id`).
* Retention cleanup refuses to delete unexported completed sync data when `ARCHIVE_PROTECT_RETENTION=true`.
* Reopening an archived stream does not trigger TwitchTracker if exported viewer data exists.

---

# Phase B — Fix stream identity and dedupe

## Goal

Prevent Tier 0 and backfill jobs from creating duplicate stream rows, placeholder rows, and conflicting TwitchTracker/live rows.

## Build

### 1. Canonical stream session table

Each stream should resolve to one canonical session:

```text
canonical_stream_id
twitch_stream_id
login
vod_id
started_at
ended_at
duration
title
category_segments
source_confidence
viewer_source
```

Viewer source values:

```text
live
tt
merged
restored
unknown
```

### 2. Dedupe rules

Merge rows when they share:

```text
same login
overlapping start/end window
same Twitch stream_id when available
same VOD id when available
same TwitchTracker stream id when available
```

### 3. Placeholder cleanup

Prefetch stubs should be upgraded into real canonical sessions instead of creating duplicate rows.

## Acceptance criteria

* One stream session does not appear as multiple Analytics rows.
* TwitchTracker backfill updates an existing live session instead of creating a duplicate.
* VOD discovery updates the canonical session with `vod_id`.
* Date-slug routing opens the correct canonical stream.

---

# Phase C — Tier 0 top-200 live archive

## Goal

Reduce future TwitchTracker dependency by continuously collecting live viewer samples for top streamers.

## Build

### 1. Tracked streamer roster

Create a `tracked_streamers` table:

```text
twitch_user_id
login
display_name
priority_tier
last_seen_live_at
last_rank
is_always_tracked
archive_policy
created_at
updated_at
```

Priority tiers:

```text
P0: manually always tracked
P1: top 50 live
P2: top 200 live
P3: recently top 200
P4: user requested or social spike
```

### 2. Top-200 directory sampler

Every 5 minutes:

```text
fetch top live directory
update tracked_streamers
refresh top-200 roster
record rank and category
```

### 3. Viewer sampler

For live tracked streamers:

```text
sample viewer_count every 30–60 seconds
store raw viewer_samples
roll up to viewer_minute_rollups
update stream session start/end/title/category
```

### 4. Offline cadence

For offline tracked streamers:

```text
check every 5–10 minutes
detect new live sessions
avoid unnecessary high-frequency polling
```

## Acceptance criteria

* Top-200 live streamers get viewer samples without visiting the channel page.
* A stream that was captured live can render a viewer chart without TwitchTracker.
* `shouldSkipTracker` skips TT when live coverage is good.
* Coverage percentage is visible in sync/debug status.

---

# Phase D — Post-end queue and TwitchTracker gap-fill

## Goal

Use TwitchTracker after stream end only when useful.

## Build

### 1. Post-end job

When a stream ends:

```text
wait 10–30 minutes
check live viewer sample coverage
if coverage is good: skip TT
if coverage has gaps: queue TT detail
if user requested full sync: queue higher priority
```

### 2. TT scrape result storage

For each TT detail scrape, store:

```text
raw extracted JSON
viewer points
category segments
scrape status
parse version
source URL
GCS export URI
```

### 3. Merge policy

Use TT as:

```text
gap-fill for missing viewer minutes
validation against local live samples
fallback for streams not captured live
```

Do not let TT overwrite good live samples unless explicitly configured.

## Acceptance criteria

* TT is skipped for streams with strong live coverage.
* TT fills missing minutes only.
* TT detail JSON is exported to GCS.
* Failed TT jobs do not block chat/GQL sync.

---

# Phase E — Selective VOD chat archive

> **Deferred** — not in the current archive milestone. Blob path convention below uses the same Azure prefix as Phase A when implemented.

## Goal

Archive VOD chat only for streams that justify the cost.

## Build

### 1. Gold selection policy

Run full GQL VOD chat for:

```text
P0 always-tracked streamers
user-requested streams
top daily streams by peak viewers
streams with clip spikes
streams with LSF/social stories
streams with unusual chat velocity
streams before VOD expiration
```

### 2. Raw-first storage

Always store raw comments before tokenization:

```text
vod_id
stream_id
comment_id
offset_seconds
created_at
commenter_login
message_text
message_fragments
badges
raw_json
fetched_at
```

### 3. Segment checkpointing

Persist segment state so failed syncs can resume:

```text
segment_start
segment_end
last_cursor
pages_fetched
comments_fetched
retry_count
status
```

### 4. Export raw chat to GCS

Raw VOD chat should be compressed and exported:

```text
https://{account}.blob.core.windows.net/streamclone-archive/streamclone/vod_chat/stream_id={id}/comments.jsonl.zst
```

Postgres keeps rollups and recent queryable data.

## Acceptance criteria

* Full chat sync resumes after failure.
* Raw comments are exported to GCS.
* Rollups can be rebuilt from raw GCS archive.
* GQL chat is not automatically run for every top-200 stream.

---

# Phase F — Emote snapshot archive

> **Deferred** — not in the current archive milestone.

## Goal

Tokenize historical chat using the emote set closest to the message timestamp.

## Build

### 1. Provider snapshots

For each tracked channel, snapshot:

```text
provider
channel_id
login
emote_set_id
emote_id
emote_name
image_url
animated
zero_width
snapshot_time
valid_from
valid_to
raw_json
hash
```

Providers:

```text
7TV
FFZ
BTTV
Twitch
```

### 2. Cadence

While live:

```text
P0/P1: every 5 minutes
P2: every 15 minutes
```

Offline:

```text
P0/P1: hourly
P2: every 6–12 hours
```

### 3. 7TV EventAPI listener

Use provider events to snapshot immediately when emotes are added, removed, renamed, or the active set changes.

### 4. Tokenization confidence

Each chat tokenization run should record:

```text
exact_snapshot
nearest_before_snapshot
nearest_after_snapshot
current_snapshot_fallback
unknown
```

## Acceptance criteria

* Emote snapshots are available for live-tracked streams.
* VOD chat tokenization uses nearest historical snapshot.
* Old streams without snapshots are marked estimated instead of silently wrong.
* Emote snapshots are exported to GCS.

---

# Phase G — Service decouple

> **Deferred** — not in the current archive milestone (Core Watch detachment rules unchanged).

## Goal

Keep heavy/failure-prone features outside Core Watch.

## Keep in Core

```text
frontend
Caddy
metadata
video / MediaMTX
chat
emote
analytics API
analytics rollups
Postgres hot DB
Redis
```

## Keep detached

```text
streamclone-scraper
Pulse Wire / storygraph
x-ingest
media-matcher
ReplayForge
Grafana / observability
```

## Rules

* Core Watch must work if scraper is down.
* Analytics chat/GQL sync must work if TT scrape fails.
* Pulse Wire failure must not break channel pages.
* ReplayForge failure must not break playback or analytics.
* Scraper must have queue priority and concurrency caps.

## Scraper priority

```text
1. user-requested TT sync
2. recent P0/P1 stream TT gap-fill
3. Pulse Wire Reddit fallback
4. YouTube fallback
5. old bulk TT backfill
```

## Acceptance criteria

* Scraper outage produces honest degraded UI, not hanging sync.
* Pulse Wire can be disabled without breaking Core Watch.
* Browser jobs have concurrency caps and cache.
* Shared scraper is not proxy-by-default.

---

# Explicit non-goals

This plan does not attempt to:

```text
eliminate TwitchTracker for old streams never captured live
run full GQL chat for every top-200 stream
store actual VOD video files
make proxies mandatory
merge scraper back into the monolith
move Core Watch into Pulse Wire
replace LSF panel before Social spread reaches parity
build embeddings/multi-entity story graph in this phase
```

---

# Recommended implementation order

## P0

```text
archive_exports table
Azure Blob export worker
restore smoke test
stream session dedupe
```

## P1

```text
tracked_streamers table
top-200 directory roster
viewer sampler
coverage-based TT skip
post-end TT gap-fill queue
```

## P2

```text
selective VOD chat archive
raw GQL export
emote snapshots
historical tokenization confidence
```

## P3

```text
full Pulse Wire detachment
scraper queue priority
ReplayForge worker isolation
Grafana/observability stack
```

---

# Final operating model

For future streams:

```text
Helix live samples are the primary viewer timeline.
TwitchTracker is validation and gap-fill.
GQL VOD chat is selective.
Emote snapshots preserve historical tokenization accuracy.
Azure Blob Storage stores durable archive data.
Postgres stores hot queryable data.
```

For past streams never captured live:

```text
TwitchTracker remains necessary for viewer-minute charts.
GQL remains necessary for VOD chat.
Archive export prevents re-scraping after data is captured once.
```
