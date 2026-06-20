# Environment guide

How Streamclone loads configuration for local dev, optional profiles, and release. **Do not commit** `.env`, `.env.local`, or token files.

Full variable reference: [`.env.example`](../.env.example). Security: [`docs/security.md`](security.md).

---

## File layering

| File | Role |
|------|------|
| `.env.dev` | Dev defaults copied to `.env` on first `make up` |
| `.env` | Active local config (gitignored) |
| `.env.local` | Optional overrides (gitignored) — API keys, proxy creds |
| `deploy/env/profile-*.env` | Feature profile overlays |
| `runtime/` | Clipper auth sync, generated runtime state |

Bootstrap:

```sh
make env                 # creates .env from .env.dev if missing
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
| `deploy/env/profile-pulse.env` | InfluxDB + Grafana live stats |
| `deploy/env/profile-full.env` | core + scraper |
| `deploy/env/profile-clipper.env` | ReplayForge URL hints (clipper not in compose) |
| `deploy/env/profile-archive.env` | Azure Blob export + Tier-0/Bronze/backfill worker defaults |
| `deploy/env/profile-local-hybrid.env` | Local UI + remote Azure scraper; **workers OFF** |
| `deploy/env/profile-azure-scraper.env` | Mode A Azure VM scraper-only |
| `deploy/env/profile-azure-workers.env` | Mode B Azure VM workers (merge with `profile-archive.env`) |

Azure hybrid plane runbook: [azure-archive-plane.md](azure-archive-plane.md).
| `deploy/env/profile-local-hybrid.env` | Local dev with **remote Azure scraper**; capture workers off for Mode B |
| `deploy/env/profile-azure-scraper.env` | Mode A Azure VM — scraper only |
| `deploy/env/profile-azure-workers.env` | Mode B Azure VM — analytics-workers + archive |

Makefile shortcuts:

```sh
make up                  # core (+ profiles from .env)
make up-scraper          # core + scraper profile
make hybrid-preflight    # Tailscale + remote Azure scraper smoke (Mode A)
make pulse-on / pulse-off   # Influx/Grafana toggle helpers
```

Hybrid / archive plane runbook: [azure-archive-plane.md](azure-archive-plane.md).

See [SERVICE_MAP.md](SERVICE_MAP.md) for services per profile.

### Network monitor (`/network`)

The Network activity page reads container rx/tx totals from setup-control and Prometheus rates from the Pulse stack.

- **Container I/O:** set `SETUP_CONTROL_TOKEN` in `.env` (generated on first `make up` / `ensure-setup-control.ps1`) so the browser can call `GET /diagnostics/network`. Without it, summary cards show a hint to run `scripts/ensure-setup-control.ps1`.
- **Prometheus metrics (chat WS, analytics bytes/s, Grafana ops):** start the **`pulse`** compose profile (`make pulse-on` or one-click **Start Pulse dashboards** in the UI). Metadata probes Prometheus only when Pulse is ready.

---

## Key variables (by domain)

| Domain | Variables | Doc |
|--------|-----------|-----|
| **Public URL** | `PUBLIC_ORIGIN`, `HLS_PUBLIC_BASE` | Tunnel / desktop install |
| **Twitch OAuth** | `TWITCH_CLIENT_ID`, `TWITCH_CLIENT_SECRET`, token import flags | `.kiro/steering/local-auth.md` |
| **Playback / HLS** | `HLS_LOW_LATENCY_ENABLED`, `MTX_*`, MediaMTX CDN secret | `.kiro/steering/playback.md` |
| **Scraper** | `SCRAPER_API_URL`, `SCRAPER_USE_IMAGES`, `SCRAPER_EPHEMERAL_BROWSER` | `docs/scraper-cloudflare-and-proxy.md` |
| **Proxy / Flame** | `PROXY_*`, `PROXY_API_KEY` | `deploy/env/proxy-flame.env.example` |
| **Pulse Wire** | `PULSE_WIRE_ENABLED`, `X_AUTH_TOKEN`, `MEDIA_MATCHER_URL` | `.kiro/steering/pulse-wire.md` |
| **Analytics TS** | `TIMESERIES_*`, `INFLUXDB_*` | `.kiro/steering/analytics.md` |
| **Azure hybrid / archive** | `SCRAPER_API_URL`, `TIER0_*`, `BRONZE_*`, `ARCHIVE_*`, `EMOTE_ROSTER_PRELOAD_*` | [azure-archive-plane.md](azure-archive-plane.md) |
| **Clipper** | `CLIPPER_SERVICE_URL`, `VITE_REPLAYFORGE_UI_URL` | `docs/agents-streamclone-and-replayforge.md` |

---

## Reload after `.env` changes

```sh
make reload-env          # recreates chat, metadata, analytics, emote, storygraph, frontend
make reload-env-if-stale # used by make up
```

Some services cache env at start; prefer `reload-env` over editing running containers.

---

## Release vs dev

| Context | Notes |
|---------|-------|
| **Dev repo** | `.env.dev`, `TWITCH_DEV_TOKEN_IMPORT_ENABLED` may be true |
| **Desktop install** | `%USERPROFILE%\streamclone` — not this checkout; fixes ship via git tag |
| **CI / release compose** | `deploy/docker-compose.release.yml`, `deploy/docker-compose.prod.yml` — validated by `make compose-config-check` |
| **Azure hybrid** | `deploy/docker-compose.azure-scraper.yml`, `deploy/docker-compose.azure-archive-plane.yml` — `make azure-scraper-config-check`, `make azure-archive-plane-config-check` |

Version tag: `VERSION` file must match git tag for `release-images.yml`.
