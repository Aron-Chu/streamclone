---
name: streamclone-emote-pipeline
description: Debug Streamclone 7TV/FFZ emote ensure, Redis dictionaries, and WebP processing jobs. Use for emotes stuck processing, missing chat emotes, or worker failures.
---

# Streamclone emote pipeline

Read `.kiro/steering/emote-pipeline.md` first.

## Flow reminder

1. `POST /v1/channels/{login}/emotes/ensure` → Postgres + background seed
2. Worker claims `processing_jobs` → libvips WebP → MinIO `{emote_id}/{scale}.webp`
3. Redis dict: `channel:emotes:{login}` field `emote_name` → JSON `{"u":"url","zw":false}`
4. Deltas on `emotes:delta:{login}`

## Diagnostics

1. MCP **`streamclone-data`** → `data_status`
2. MCP **`streamclone-data`** → `emote_jobs`
3. MCP **`streamclone-data`** → `redis_channel_emotes(login="<channel>")`
4. MCP **`streamclone-stack`** → `compose_logs(service="emote")`

## Example SQL (via postgres_query)

```sql
SELECT status, COUNT(*) FROM processing_jobs GROUP BY status
```

## Code lookup

**`streamclone-codegraph`**: blast radius on ensure/seed handlers in emote service.

## Tests

Package-scoped Go tests for emote/internal packages touched by the change.
