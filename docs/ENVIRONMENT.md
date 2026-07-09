# Environment guide

How Streamclone loads configuration for local dev and release. **Do not commit** `.env`, `.env.local`, or token files.

Full variable reference: [`.env.example`](../.env.example). Security: [`docs/security.md`](security.md).

**Scope:** core compose profile (watch, playback, chat, emotes). Hosted production env and ingest overlays live in private **streampulse-ops**. See [streampulse-product-boundary.md](streampulse-product-boundary.md).

---

## File layering

| File | Role |
|------|------|
| `.env.example` | Tracked base template |
| `deploy/env/profile-dev.env` | Tracked dev defaults (merged into `.env`) |
| `.env` | Active local config (gitignored) |
| `.env.local` | Optional overrides (gitignored) — API keys |
| `deploy/env/profile-*.env` | Feature profile overlays |
| `runtime/` | Generated runtime state |

Bootstrap:

```sh
make env                 # creates .env from .env.example + profile-dev if missing
make validate-env PROFILE=core
make up                  # merges profiles from .env via scripts/lib/env.sh
```

---

## Compose profiles

Set in `.env` or pass `--profile` to compose.

| Profile file | Enables |
|--------------|---------|
| `deploy/env/profile-core.env` | Default stack — metadata, video, chat, emote, frontend |
| `deploy/env/profile-scraper.env` | *(legacy — boundary split)* TwitchTracker scraper |
| `deploy/env/profile-full.env` | *(legacy)* core + scraper |

Deprecated: `STREAMCLONE_PROFILE=clipper` maps to core compose (ReplayForge runs outside compose).

Makefile shortcuts:

```sh
make up                  # core (+ profiles from .env)
```

See [SERVICE_MAP.md](SERVICE_MAP.md) for services per profile.

### Stack monitor (`/network`)

The Stack activity page reads container rx/tx totals from setup-control.

- **Container I/O:** set `SETUP_CONTROL_TOKEN` in `.env` (generated on first `make up` / `ensure-setup-control.ps1`).

---

## Key variables (by domain)

| Domain | Variables | Doc |
|--------|-----------|-----|
| **Public URL** | `PUBLIC_ORIGIN`, `HLS_PUBLIC_BASE` | Tunnel / desktop install |
| **Twitch OAuth** | `TWITCH_CLIENT_ID`, `TWITCH_CLIENT_SECRET`, token import flags | `.kiro/steering/local-auth.md` |
| **Playback / HLS** | `HLS_LOW_LATENCY_ENABLED`, `MTX_*`, MediaMTX CDN secret | `.kiro/steering/playback.md` |
| **Clipper** | `CLIPPER_SERVICE_URL`, `VITE_REPLAYFORGE_UI_URL` | `docs/agents-streamclone-and-replayforge.md` |

---

## Reload after `.env` changes

```sh
make reload-env          # recreates chat, metadata, emote, frontend
make reload-env-if-stale # used by make up
```

Some services cache env at start; prefer `reload-env` over editing running containers.

---

## Release vs dev

| Context | Notes |
|---------|-------|
| **Dev repo** | `.env.example` + `profile-dev.env`, `TWITCH_DEV_TOKEN_IMPORT_ENABLED` may be true |
| **Desktop install** | `%USERPROFILE%\streamclone` — not this checkout; fixes ship via git tag |
| **CI / release compose** | `deploy/docker-compose.release.yml`, `deploy/docker-compose.prod.yml` — validated by `make compose-config-check` |
| **Hosted production** | Private **streampulse-ops** — see [streampulse-product-boundary.md](streampulse-product-boundary.md) |

Version tag: `VERSION` file must match git tag for `release-images.yml`.
