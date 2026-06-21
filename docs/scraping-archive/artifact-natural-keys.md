# Artifact natural keys

Authoritative contract for `archive_exports.(artifact_type, natural_key)` upserts. Writers must use helpers in `internal/archive/natural_keys.go`.

## Canonical patterns

| artifact_type | natural_key pattern | blob path (hive) | on re-run |
|---------------|---------------------|------------------|-----------|
| `bronze_vod_catalog` | `{login}:{date}` | `vod_catalog/v1/login={login}/date={date}/part-000.jsonl.gz` | overwrite same date |
| `channel_identity` | `{channel_id}:{date}` | `channels/identity/login={login}/date={date}/identity.json.gz` | overwrite same date |
| `provider_crosswalk` | `{login}:{date}` | `channels/crosswalk/login={login}/date={date}/crosswalk.json.gz` | overwrite same date |
| `bronze_top500` | `roster:{date}` | `channels/top500.json.gz` (legacy) / `rosters/tier0/date={date}/part-000.json.gz` | overwrite same date |
| `bronze_vod_index` | `vod_index:{login}` (legacy) | `channels/vod_index/{login}.jsonl.gz` | overwrite |
| `analytics_rollups` | `{stream_id}:twitchtracker` | `rollups/stream_id={id}/part-000.jsonl.gz` | overwrite |
| `tt_detail_html` | `{stream_id}:twitchtracker` | `tt-detail/{login}/{stream_id}/page.html.gz` | overwrite |
| `tt_chart_json` | `{stream_id}:{fetched_at_unix}` | `raw/twitchtracker/stream_id={id}/chart.json.gz` | version by fetch time |
| `emote_snapshot` | `{provider}:{login}:{date}` | `emotes/snapshots/provider={p}/login={login}/date={date}/part-000.json.gz` | overwrite same date |
| `emote_snapshot_global` | `7tv:global:{date}` | `emotes/snapshots/provider=7tv/login=global/date={date}/part-000.json.gz` | overwrite same date |
| `emote_changelog` | `{provider}:{login}:{event}:{unix}` | `emotes/changelog/...` | append (immutable event) |
| `gold_lite` | `{stream_id}` | `rollups/chat/stream_id={id}/minute.jsonl.gz` | overwrite |
| `gold_full` | `{stream_id}:part:{part_no}` | `vod_chat/stream_id={id}/messages.jsonl.gz` | overwrite part |
| `pulsewire_raw` | `{source}:{date}:{part_no}` | `pulsewire/...` | overwrite part |

## Legacy → canonical mapping

| legacy natural_key | canonical | dual-write |
|--------------------|-----------|------------|
| `vod_index:{login}` | `bronze_vod_catalog` with `{login}:{date}` | one release |
| `rollups:{stream_id}` | `{stream_id}:twitchtracker` on `analytics_rollups` | one release |
| `emote_snapshot:{provider}:{login}:{date}` | same pattern on `emote_snapshot` type | n/a |

## Rules

- Daily snapshots version by **date** in the key.
- Per-stream canonical artifacts idempotent on `stream_id` + type.
- Raw fetch artifacts version by **fetched_at** or part number.
