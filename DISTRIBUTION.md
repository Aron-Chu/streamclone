# Distribution & cloud storage

How to share Streamclone with others and how to use Google Cloud ($10/mo credit + 5TB storage) without running the heavy stack on GCP.

## Pick a tier

| Tier | Who | Entry | GCP |
|------|-----|-------|-----|
| **A — Local** | Solo dev | `make up` → `http://localhost:8090` | None |
| **B — Tunnel** | Phone test, friends, HTTPS OAuth | Local stack + Cloudflare Quick Tunnel (`deploy/LOCAL_HTTPS_OAUTH.md`) | Optional backups |
| **C — Public VM** | Small always-on site | `docker-compose.prod.yml` on Oracle free tier or VPS (`deploy/FREE_DEPLOYMENT.md`) | Skip compute |
| **D — Hybrid** | You + long-term archives | Tier A/B/C for serving; **GCS for cold storage** | **Recommended** |

## Sharing with others

1. Publish the git repo (or GHCR images later).
2. Recipients: `cp .env.example .env`, set `CURATOR_API_TOKEN` and `AUTH_COOKIE_SECRET`, then `make up`.
3. **Never commit** `.env`, Twitch tokens, clipper tokens, or MinIO secrets.
4. Point Windows users at `.kiro/steering/windows-dev.md` (localhost / `wslrelay` issues).
5. Legal/ToS notice in `README.md` stays prominent for any public distribution.

## Deploy so others can use it (recommended path)

**Tier C — public VM** is the right default for “friends / small community”:

1. Oracle Cloud **Always Free** Ampere (4 OCPU / 24 GB RAM) or a $6–12/mo VPS (Hetzner, DigitalOcean).
2. DuckDNS or your own domain → point at the VM.
3. On the VM: `docker compose -f deploy/docker-compose.yml -f deploy/docker-compose.prod.yml up -d --build`
4. Full walkthrough: [`deploy/FREE_DEPLOYMENT.md`](deploy/FREE_DEPLOYMENT.md)

Expose only **80/443** (and **22** from your IP). Do not publish Redis, Postgres, RTMP `1935`, or internal service ports.

For a quick demo without a VM, use **Tier B** (Cloudflare Quick Tunnel) — see [`deploy/LOCAL_HTTPS_OAUTH.md`](deploy/LOCAL_HTTPS_OAUTH.md). Your PC must stay on; not ideal for “always available” sharing.

## Gemini / Google AI subscription vs hosting

A **Gemini Advanced / Google AI Pro** subscription (consumer) does **not** include free VM hosting for this app. It covers AI chat/features in Google products, not GCE/Cloud Run credits for a Docker stack.

What *does* help on Google:

| Program | What you get | Good for Streamclone? |
|---------|----------------|------------------------|
| **Google Cloud free trial** | ~$300 credit, 90 days | Test a **large** GCE VM briefly; migrate to Oracle free after |
| **GCP always-free tier** | Tiny e2-micro, 5 GB GCS, etc. | **Too small** for full stack; OK for GCS backups only |
| **Google Cloud Skills Boost / Qwiklabs** | Time-limited lab credits | Learning, not production |
| **[Google Cloud for Students](https://cloud.google.com/edu/students)** | Periodic $300 credits via partner programs | Short-term VM tests; check current eligibility |
| **Gemini API free tier** | Limited API calls | Optional future feature (captions/reframe), **not** app hosting |

**Practical split:** run the app on **Oracle free VM or cheap VPS**; use GCP/Gemini only for **optional AI APIs** (Speech-to-Text, Vertex) if local Whisper is not enough.

## GCP: what fits $10/mo

**Do not** run the full compose stack on Cloud Run or a 1–2 GB GCE instance. Video workers, MediaMTX, Postgres, Redis, and clipper ASR need **8–16+ GB RAM** locally or on a proper VM.

**Do use GCP for:**

- **GCS bucket** (your 5TB) — backups and archives
- Optional **Cloud Scheduler** + tiny function to trigger backup scripts (~$1/mo total)
- Optional **Gemini / Speech API** for clipper upgrades (pay-per-use; stay within free API quotas when possible)

Typical spend: **$1–5/mo** if compute stays off GCP and egress is limited.

## GCS bucket layout (suggested)

```text
gs://streamclone-<your-id>/
  backups/postgres/YYYY-MM-DD/streamclone.sql.gz
  backups/clipper-sqlite/YYYY-MM-DD/clipper.sqlite
  emotes/minio-mirror/
  clipper/output/
  analytics/exports/
```

**Backup cadence**

| Data | Source | Method | Frequency |
|------|--------|--------|-----------|
| Postgres | `pg-data` volume | `pg_dump -Fc` → `gsutil cp` | Daily |
| MinIO emotes | `minio-data` | `gsutil -m rsync` | Daily incremental |
| Clipper DB | `clipper-data/clipper.sqlite` | file copy | Daily |
| Clipper renders | `clipper-data/output` | `gsutil rsync` before local retention purge | Weekly |

Enable GCS lifecycle: Standard → Nearline (30d) → Coldline (90d). Set a **$8 budget alert**.

## Monthly checklist

- [ ] Stack healthy: `docker compose ps`, smoke one channel on `:8090`
- [ ] Disk: `clipper-data`, `minio-data`, `pg-data`
- [ ] GCS backup timestamps fresh
- [ ] GCP billing under $10
- [ ] Quarterly: restore test from one postgres dump

## AI agents & contributors

See **`AGENTS.md`** for token-efficient navigation:

- Domain steering: `.kiro/steering/`
- Code graph: `make codegraph` + Cursor MCP `streamclone-codegraph`
- Scoped rules: `.cursor/rules/`

## Cleanup roadmap (readability)

Completed in repo hygiene pass: removed `scratch/`, frontend debug scripts, dead API exports.

**Next refactors** (separate PRs):

1. Extract shared `SourcePills` / `count()` from `Channel.tsx` + `Analytics.tsx`
2. Extract `ClipJobList` from `Analytics.tsx` + `ClipStudio.tsx`
3. Shared `internal/helix` client (analytics + metadata duplication)
4. Split god-files: `Channel.tsx`, `Analytics.tsx`, `metadata/api/api.go`
5. YAML-anchor duplicate `VITE_*` blocks in compose files
