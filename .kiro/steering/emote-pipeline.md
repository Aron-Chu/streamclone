# Emote Pipeline Steering

Emotes are local-first: provider sync, asset rendering, Redis dictionaries, chat tokenization, and frontend rendering.

## Boundaries

- API/store/worker: `internal/emote/*`
- Chat parsing: `internal/chat/parse`, `internal/chat/enrich`
- Frontend rendering: `frontend/src/emote*.ts*`
- Assets: MinIO/S3-compatible storage, rendered WebP scales

## Data Rules

- PostgreSQL is durable source of truth.
- Redis dictionaries use `channel:emotes:{login}`.
- Rendered keys are `{emote_id}/{scale}.webp`.
- Chat fan-out should use prepared dictionaries and bounded work.

## Guardrails

- Preserve zero-width emote behavior.
- Keep provider credentials environment-driven.
- Use libvips/`vips thumbnail` for WebP variants.
- Avoid duplicating tokenizer logic across packages.

## Checks

```sh
go test ./internal/emote/...
go test ./internal/chat/parse ./internal/chat/enrich
cd frontend && npm run build
```
