---
description: Work on provider emote sync, rendering, dictionaries, chat tokenization, or emote frontend rendering.
---

# Emote Pipeline

Read `AGENTS.md`, `.kiro/steering/emote-pipeline.md`, and `.kiro/steering/tech.md`.

## Flow

Provider data -> PostgreSQL -> asset worker -> WebP scales -> Redis dictionaries -> chat/frontend rendering.

## Diagnostics

- `redis_channel_emotes`
- `emote_jobs`
- `postgres_query`

## Tests

```sh
go test ./internal/emote/...
go test ./internal/emoteimage/...
go test ./internal/chat/parse ./internal/chat/enrich
cd frontend && npm run build
```
