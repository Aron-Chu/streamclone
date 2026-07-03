# Environment guide

How Streamclone loads configuration for local dev, optional profiles, and release. **Do not commit** `.env`, `.env.local`, or token files.

Full variable reference: [`.env.example`](../.env.example). Security: [`docs/security.md`](security.md).

---

## File layering

| File | Role |
|------|------|
| `.env.example` | Tracked base template |
| `deploy/env/profile-dev.env` | Tracked dev defaults (merged into `.env`) |
| `.env` | Active local config (gitignored) |
| `.env.local` | Optional overrides (gitignored) — API keys, proxy creds |
| `deploy/env/profile-*.env` | Feature profile overlays |
| `runtime/` | Clipper auth sync, generated runtime state |

Bootstrap:

```sh
make env                 # creates .env from .env.example + profile-dev if missing
make validate-env PROFILE=core
make up                  # merges profiles from .env via scripts/lib/env.sh
```

---

## Compose profiles

Set in `.env` (e.g. `STREAMCLONE_PROFILE=full`) or pass `--profile` to compose.

| Profile file | Enables |
|--------------|---------|
| `deploy/env/profile-core.env` | Default stack; no scraper |
| `deploy/env/profile-scraper.env` | TwitchTracker scraper service |
| `deploy/env/profile-full.env` | core + scraper |
| `deploy/env/profile-archive.env` | Azure Blob export + Tier-0/Bronze/backfill worker defaults |
| `deploy/env/profile-local-hybrid.env` | Local UI + remote Azure scraper; **workers OFF** (Mode B) |
| `deploy/env/profile-azure-scraper.env` | Mode A Azure VM — scraper only |
| `deploy/env/profile-azure-workers.env` | Mode B Azure VM — analytics-workers + archive |

Azure hybrid plane runbook: [azure-archive-plane.md](azure-archive-plane.md).

Deprecated: `STREAMCLONE_PROFILE=clipper` maps to core compose + core env (ReplayForge runs outside compose).

Makefile shortcuts:

```sh
make up                  # core (+ profiles from .env)
make up-scraper          # core + scraper profile
make hybrid-preflight    # Tailscale + remote Azure scraper smoke (Mode A)
```

Hybrid / archive plane runbook: [azure-archive-plane.md](azure-archive-plane.md).

See [SERVICE_MAP.md](SERVICE_MAP.md) for services per profile.

### Stack monitor (`/network`)

The Stack activity page reads container rx/tx totals from setup-control.

- **Container I/O:** set `SETUP_CONTROL_TOKEN` in `.env` (generated on first `make up` / `ensure-setup-control.ps1`) so the browser can call `GET /diagnostics/network`. Without it, summary cards show a hint to run `scripts/ensure-setup-control.ps1`.

---

## Key variables (by domain)

| Domain | Variables | Doc |
|--------|-----------|-----|
| **Public URL** | `PUBLIC_ORIGIN`, `HLS_PUBLIC_BASE` | Tunnel / desktop install |
| **Twitch OAuth** | `TWITCH_CLIENT_ID`, `TWITCH_CLIENT_SECRET`, token import flags | `.kiro/steering/local-auth.md` |
| **Playback / HLS** | `HLS_LOW_LATENCY_ENABLED`, `MTX_*`, MediaMTX CDN secret | `.kiro/steering/playback.md` |
| **Scraper** | `SCRAPER_API_URL`, `SCRAPER_USE_IMAGES`, `SCRAPER_EPHEMERAL_BROWSER` | `docs/scraper-cloudflare-and-proxy.md` |
| **Proxy / Flame** | `PROXY_*`, `PROXY_API_KEY` | `deploy/env/proxy-flame.env.example` |
| **Analytics TS** | `TIMESERIES_*`, `INFLUXDB_*` | `.kiro/steering/analytics.md` |
| **Azure hybrid / archive** | `SCRAPER_API_URL`, `TIER0_*`, `BRONZE_*`, `ARCHIVE_*`, `EMOTE_ROSTER_PRELOAD_*` | [azure-archive-plane.md](azure-archive-plane.md) |
| **Clipper** | `CLIPPER_SERVICE_URL`, `VITE_REPLAYFORGE_UI_URL` | `docs/agents-streamclone-and-replayforge.md` |

---

## Reload after `.env` changes

```sh
make reload-env          # recreates chat, metadata, analytics, emote, frontend
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
| **Production deploy** | Private **streampulse-ops** + pinned `IMAGE_TAG` — see [production-artifact-contract.md](production-artifact-contract.md) |
| **Azure hybrid** | `deploy/docker-compose.azure-scraper.yml`, `deploy/docker-compose.azure-archive-plane.yml` — `make azure-scraper-config-check`, `make azure-archive-plane-config-check` |

Version tag: `VERSION` file must match git tag for `release-images.yml`.
