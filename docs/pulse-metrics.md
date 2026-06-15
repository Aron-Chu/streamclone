# Pulse Metrics

Pulse is an optional dashboard layer for Streamclone. Postgres remains the source of truth for stream metadata, sync state, VOD chat, date routing, replay heatmap inputs, and in-app Analytics details. InfluxDB and Grafana are local dashboard add-ons fed by best-effort rollup export.

Normal Analytics must keep working when `TIMESERIES_ENABLED=false`, when InfluxDB is down, or when Grafana has never been started.

## Runtime Boundary

| Layer | Owns |
|-------|------|
| Postgres Analytics | Stream rows, minute rollups, sync checkpoints, VOD chat, date routes, app detail views |
| InfluxDB Pulse | Best-effort time-series copies of minute rollups for Grafana exploration |
| Grafana Pulse | Optional dashboards, leaderboards, data-quality investigation, local export visibility |

The current supported Pulse target is InfluxDB 2.7 with Flux and Grafana 11.5. The Helm chart under `.local/helm-pulse` and `charts/pulse` remains a developer sandbox. The user-facing release path is the optional Docker Compose `pulse` profile.

## Canonical Metrics

| Metric | Definition |
|--------|------------|
| `chat_per_min` | `sum(chat_count) / minutes_with_rollup_data` |
| `emotes_per_min` | `sum(total_emote_count) / minutes_with_rollup_data` |
| `seventv_per_min` | `sum(seventv_emote_count) / minutes_with_rollup_data` |
| `provider_share_pct` | `seventv_emote_count / total_emote_count * 100` for the selected stream/range; `0` when total emotes are zero |
| `reaction_score_0_100` | Bounded 0-100 activity score. Current summary API uses `(total_emote_count + seventv_emote_count) / (chat_count + total_emote_count) * 100`; replay moment scoring remains canonical for per-minute peaks. |
| `viewer_momentum_5m` | Percent change between earliest and latest available viewer sample in the selected rollup window; future versions may narrow this to an explicit five-minute trailing window. |
| `data_coverage_pct` | `minutes_with_rollup_data / expected_stream_minutes * 100`, clamped to 0-100 |
| `sync_health_state` | One of `synced`, `viewer_only`, `chat_only`, `stats_only`, `partial`, or `missing` based on Postgres rollup coverage and stream summaries |

## Influx Export Rules

- Export only aggregate rollups. Do not export chat message text, usernames, badges, raw VOD comments, OAuth data, or other per-user payloads.
- Keep tags bounded: `channel_login`, `stream_id`, `provider`, and `emote_id` are safe; long mutable display strings should not become high-volume tags in future schema revisions.
- Treat Influx writes as best-effort. Failed export must not fail sync, stream detail, or app Analytics.
- Any future in-app Influx read path must be gated by export status, lag checks, Postgres parity checks, backfill coverage, and automatic Postgres fallback.

## APIs

Postgres-backed summary endpoints define the app-facing metric contract:

- `GET /v1/analytics/streams/{streamId}/summary?channel=`
- `GET /v1/analytics/channels/{login}/streams/ranked?sort=&period=`
- `GET /v1/analytics/timeseries/status`

Grafana should mirror these definitions where practical. It must not become the only place where a metric is defined.
