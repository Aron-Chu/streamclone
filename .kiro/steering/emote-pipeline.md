# Emote Pipeline Steering

Emotes are local-first: provider sync, asset rendering, Redis dictionaries, chat tokenization, and frontend rendering.

## Boundaries

- API/store/worker: `internal/emote/*`
- Rollup/Pulse image URLs: `internal/emoteimage` (provider-aware CDN vs local `/emotes/{id}/…`)
- Chat parsing: `internal/chat/parse`, `internal/chat/enrich`
- Frontend rendering: `frontend/src/emote*.ts*`
- Assets: S3-compatible object storage, rendered WebP scales. MinIO remains
  the local/default store; an external store is an optional migration target,
  not a channel-capacity dependency.

## Data Rules

- PostgreSQL is durable source of truth.
- Redis dictionaries use `channel:emotes:{login}`.
- Redis channel dictionaries are bounded hot caches: rebuilds refresh
  `EMOTE_DICTIONARY_TTL` (default 24h), and startup attaches that TTL to legacy
  no-expiry keys so dormant channels age out without deleting PostgreSQL truth.
  The first legacy sweep runs after the initial active-roster preload attempt,
  uses `EXPIRE NX`, deterministic TTL jitter, and paced batches so historical
  keys do not create one concentrated expiration/rebuild wave.
- Production keeps roster preloading enabled with `EMOTE_ROSTER_PRELOAD_TOP_N`
  at least as large as the live collector ceiling and a preload interval shorter
  than the dictionary TTL, so active/live churn stays warm while dormant keys
  expire.
- Rendered keys are `{emote_id}/{scale}.webp`.
- Object-store migration is provider-neutral. Enable
  `EMOTE_OBJECT_SECONDARY_ENABLED` with a direct S3-compatible endpoint,
  dual-write while copying/verifying existing objects, then set
  `EMOTE_OBJECT_SECONDARY_PRIMARY=true` for cutover. Read-through fallback
  promotes misses into the primary store. Deletes always target both stores so
  removed assets cannot reappear through fallback.
- Secondary buckets stay private. Keep access keys environment-driven and do
  not point either object-store client back through the public emote API/CDN;
  that would create a recursive proxy path.
- Build the private migration manifest with
  `go run ./cmd/emote-object-manifest` (metadata JSONL) and repeat with
  `--sha256` for the sequential full-byte verification pass. ETags are provider
  metadata and are never treated as content hashes.
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
