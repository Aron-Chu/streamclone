# Emote Pipeline Steering

Emotes are local-first: provider sync, asset rendering, Redis dictionaries, chat tokenization, and frontend rendering.

## Boundaries

- API/store/worker: `internal/emote/*`
- Rollup/Pulse image URLs: `internal/emoteimage` (provider-aware CDN vs local `/emotes/{id}/…`)
- Chat parsing: `internal/chat/parse`, `internal/chat/enrich`
- Frontend rendering: `frontend/src/emote*.ts*`
- Assets: MinIO/S3-compatible storage, rendered WebP scales

## Data Rules

- PostgreSQL is durable source of truth.
- Redis dictionaries use `channel:emotes:{login}`.
- Rendered keys are `{emote_id}/{scale}.webp`.
- Chat fan-out should use prepared dictionaries and bounded work.
- Analytics rollups key emotes as `provider:id:name`. The middle `id` is the **chat fragment id** (Twitch numeric or `emotesv2_*` for native emotes; local emote-service UUID for synced 7TV/FFZ/BTTV). Do not assume every rollup id is a MinIO object key.
- Resolve rollup/Pulse thumbnail URLs through `emoteimage.URL(provider, id, scale)` — Twitch → `static-cdn.jtvnw.net`, synced third-party UUID → `/emotes/{uuid}/{scale}.webp`, legacy provider ids → provider CDN. Grafana Flux dashboards mirror the Twitch branch only; in-app Pulse uses the Go resolver.

## Guardrails

- Preserve zero-width emote behavior.
- Keep provider credentials environment-driven.
- Use libvips/`vips thumbnail` for WebP variants.
- Avoid duplicating tokenizer logic across packages.

## Checks

```sh
go test ./internal/emote/...
go test ./internal/emoteimage/...
go test ./internal/chat/parse ./internal/chat/enrich
cd frontend && npm run build
```
