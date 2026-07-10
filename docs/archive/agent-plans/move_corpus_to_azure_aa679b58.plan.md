---
name: Move corpus to Azure
overview: Relocate the 24/7 corpus compute plane (analytics-workers + scraper) to a small Azure VM that connects back to BearHost's existing Postgres and the shared Azure Blob, so no data moves (zero data-loss), corpus runs continuously, and its status stays observable via /v1/corpus/readiness. The continuous host also finally enables IVR shadow validation - the real (but staged, not config-flip) path to relieving Gold's GQL rate-limit ceiling.
todos: []
isProject: false
---

# Move corpus to an Azure instance (zero data loss)

## Core principle: move compute, not data

Postgres (source of truth) stays on **BearHost**, and the corpus artifacts already live in **shared Azure Blob** (`ststreamclone3lf6tt` / `streamclone-archive`, see [profile-archive.env](deploy/env/profile-archive.env)). The Azure VM only runs the **worker processes** and writes to that same DB + blob over the network. Because nothing is migrated, there is no data-loss surface.

```mermaid
flowchart LR
  CF["Cloudflare Tunnel<br/>api.streampulse.stream"] --> A
  subgraph hostA [BearHost - unchanged]
    A["analytics API + pulse-caddy:8090"]
    PG[("Postgres - source of truth")]
    A --> PG
  end
  subgraph hostB [Azure VM B2ats_v2 - new corpus 24/7]
    W["analytics-workers"]
    SC["scraper - Camoufox"]
    MD["metadata"]
    RB[("redis - local")]
    W --> RB
    W --> SC
    W --> MD
  end
  W -->|"DATABASE_URL over Tailscale"| PG
  W -->|"artifacts"| AZ[("Azure Blob - shared, unchanged")]
  A -->|"reads corpus counts + readiness"| PG
```



## Key facts that shape the plan

- `analytics-workers` in [docker-compose.bearhost-prod.yml](deploy/docker-compose.bearhost-prod.yml) hardcodes `DATABASE_URL: postgres://app:app@postgres:5432/streamclone` (L39) and hard-depends on local `postgres`, `migrate`, `metadata`, `scraper` (L81-91). To run on Azure against remote Postgres, we add an **override compose file** that rewrites `DATABASE_URL` and removes the `postgres`/`migrate` dependencies (we keep local `redis`, `metadata`, `scraper`).
- Corpus enable gate is `CORPUS_WORKERS_ENABLED=1` plus the tier flags in [profile-bearhost-corpus.env](deploy/env/profile-bearhost-corpus.env).
- Preflight ([bearhost-corpus-preflight.sh](scripts/bearhost-corpus-preflight.sh)) requires the Azure secret file at `/etc/streamclone/secrets/azure-archive-connection-string` and Twitch creds in `.env`.
- Status is observable through `GET /v1/corpus/readiness` ([corpus_readiness.go](internal/analytics/corpus_readiness.go) L247-258), which reads the shared Postgres, so the BearHost API reflects Azure worker progress with no code change.

## Phase 1 - Provision the Azure corpus VM

- Create a **B2ats_v2 (2 vCPU / 8 GB, AMD x86)** Ubuntu VM in one of your allowed student regions, ideally the **same region as the blob account** to avoid egress.
- Install Docker + compose plugin; clone the streamclone repo + sibling `streamclone-scraper` to `/opt/streamclone/app` (scraper builds from sibling per [docker-compose.bearhost-prod.yml](deploy/docker-compose.bearhost-prod.yml) L93-98).
- Place secrets: copy `/etc/streamclone/secrets/azure-archive-connection-string` and a `.env` with Twitch creds so preflight passes.

## Phase 2 - Secure DB link (BearHost Postgres <- Azure)

- Install **Tailscale** on the Azure VM and confirm BearHost is on the tailnet.
- Bind/allow Postgres on BearHost to the **tailnet interface only** (never public UFW). Keep `app/app` or rotate to a dedicated `corpus` role.
- Decision (recommended start): **Tailscale-remote to BearHost Postgres** = zero migration, zero data loss. Note the documented reliability caveat ([docs/azure-archive-plane.md]) about remote PG over tailnet; if drops occur, the fallback is a managed Postgres later (separate effort, would require a one-time data move).

## Phase 3 - Repo changes (new files, no edits to applied config)

- New `deploy/docker-compose.corpus-remote.yml` override:
  - `analytics-workers.environment.DATABASE_URL` -> `postgres://<user>@<bearhost-tailnet-ip>:5432/streamclone?sslmode=disable`
  - `depends_on:` reduced to `redis`, `metadata`, `scraper` (drop `postgres`, `migrate`).
- New `deploy/env/profile-corpus-remote.env` (or reuse `profile-bearhost-corpus.env`) with `CORPUS_WORKERS_ENABLED=1`, `BACKFILL_ENABLED=true`, `BRONZE_ENABLED=true`, `GOLD_BACKFILL_ENABLED=true`, `SILVER_AUTO_ENQUEUE_ENABLED=true`, `ARCHIVE_ENABLED=true`.
- New `scripts/azure-corpus-up.sh` that brings up only `analytics-workers`, `scraper`, `metadata`, `redis` with the corpus profile + remote override (modeled on [bearhost-restart-workers-corpus.sh](scripts/bearhost-restart-workers-corpus.sh) but targeting the Azure host and skipping local `postgres`/`migrate`).

## Phase 4 - Turn off corpus on BearHost (free the 8 GB)

- Ensure BearHost runs **Pulse mode** ([bearhost-pulse-api.sh](scripts/bearhost-pulse-api.sh)) so `analytics-workers`/`scraper` are stopped there and `CORPUS_WORKERS_ENABLED=0` on the API container. BearHost API + tunnel stay exactly as-is.
- Migrations: the live schema is current EXCEPT migration **000050** (`chat_source` / `source_confidence`), documented as **pending on prod** ([docs/agent-notes/ivr-gold-prod-status.md]). It is forward-only/additive (no data loss). Apply it once to the shared Postgres before enabling any IVR mode in Phase 7. Run `migrate` from one host only; both sides stay on compatible image versions.

## Phase 5 - Maximize Gold throughput (the real goal of the move)

Relocating corpus only buys 24/7 uptime; loading "as much Gold as possible" needs the Gold dials turned up, but Gold is bounded by Twitch GQL rate limits, not CPU. Tune within that ceiling:

- Loosen the Gold gate so more VODs qualify: lower `GOLD_MIN_PEAK_VIEWERS` (default 5000) and `GOLD_MIN_DURATION_MINUTES` (default 60) in the corpus env ([docker-compose.bearhost-prod.yml](deploy/docker-compose.bearhost-prod.yml) L63-64).
- Consider raising `BACKFILL_GOLD_WORKER_COUNT` only if the GQL RPM budget (`GOLD_GLOBAL_GQL_RPM`, `GOLD_PER_VOD_GQL_RPM`) is not saturated; more workers past the RPM ceiling just idle and risk GQL blocks.
- Keep `SCRAPER_MAX_CONCURRENT=1` and `ANALYTICS_VOD_GQL_CONCURRENCY=1` on the 8 GB Azure box; throughput comes from continuous uptime, not parallelism.
- Watch `GoldSegments` / rate-limit component in `/v1/corpus/readiness` to confirm we are GQL-bound, not worker-bound, before adding capacity.
- The structural relief for the GQL ceiling is IVR (Phase 7), not more workers. Tuning here only widens the candidate set within the existing GQL budget.

## Phase 6 - Continuity + status awareness (the "are we aware" requirement)

- After Azure corpus is up, `running` startup-reclaim requeues any interrupted jobs ([backfill_worker.go] `ReclaimRunningOnStartup`) so the existing queued Gold job and any in-flight work continue safely.
- Verify continuously via:
  - `GET https://api.streampulse.stream/v1/corpus/readiness` (silver/gold queue counts, eligibility, oldest-queued age, worker capacity, archive component).
  - `GET /v1/public/hub` corpusPipeline counts move as Azure drains the queue.
  - On Azure host: `docker logs streamclone-analytics-workers | grep -E 'silver|gold|bronze|backfill'`.
- If the stale "1 queued - oldest 1d" Gold job still does NOT drain once Azure workers are confirmed running, it is a claim-gate problem, not a worker problem: check it has a silver sibling `done` + `export_status=confirmed` (or a `top500_vod_inventory` row) and `next_run_at <= now()` ([backfill_worker_claim.go]); otherwise it stays unclaimed forever. (Carried over from the coverage/chart-fixes plan Phase 3.)
- Optional: a small `scripts/azure-corpus-status.sh` wrapper that curls readiness + tails worker logs in one command.

## Safety / rollback

- No data is copied or deleted; if the Azure link is unstable, stop the Azure workers and run `bearhost-corpus-only.sh` on BearHost to resume corpus locally exactly as before. Source of truth (Postgres + Azure Blob) is identical in both modes.

## Phase 7 - IVR as the real GQL rate-limit relief (staged, not a config flip)

Gold throughput is capped by Twitch GQL RPM. IVR (`logs.ivr.fi` Rustlog IRC logs) hits a different host and does not consume GQL quota, so it is the strategic lever - but today it only runs as a pre-GQL accelerator that NEVER replaces GQL ([backfill_worker.go](internal/analytics/backfill_worker.go) L276-292; gold always `forceChat=true`). The 24/7 Azure corpus host is the correct place to mature it.

Current reality (do not over-promise):
- IVR is OFF in prod; Gold is GQL-only. Shadow canary coded but not deployed.
- Benchmarks REJECTED promotion: ~2.3x-4.4x speedup vs the >=5x bar, per-minute volume below the 95% gold-lite threshold; coverage sparse (Ludwig verified; Jynxzi/shroud/summit1g miss `/list`).
- `GOLD_IVR_CANONICAL_REPLACE` is a safety blocker, not a working mode; "IVR primary, GQL gap-fill only" is NOT built.

Staged path on the Azure corpus host (each step gated by the prior):
1. Apply migration 000050 (Phase 4), then deploy IVR shadow canary using [profile-bearhost-corpus-ivr-shadow.env](deploy/env/profile-bearhost-corpus-ivr-shadow.env) (`GOLD_IVR_ENABLED=true`, `GOLD_IVR_SHADOW_MODE=true`, allowlist `ludwig`, lite/peaks/canonical_replace off). Confirm zero `chat_source='ivr'` rollups written. This is finally possible to run continuously because corpus is now 24/7.
2. Widen the shadow allowlist across representative corpus channels to gather real coverage + quality data (artifacts under `runtime/ivr-shadow/`).
3. Only if quality re-passes: enable lite/peaks for provisional fast-fill (GQL still canonical).
4. The actual GQL-call reduction requires NEW logic (skip/segment GQL where IVR coverage is sufficient) plus wiring the unused `GOLD_*_GQL_RPM` enforcement and `gqlPriorityFromIVR` recommendations into scheduling. Treat as a separate engineering effort after shadow validation.

Net: this plan makes IVR validation possible and continuous; it does not assume IVR reduces GQL load yet.

## Cross-plan coordination (other plans this session)

- The new files this plan adds (`deploy/docker-compose.corpus-remote.yml`, `deploy/env/profile-corpus-remote.env`, `scripts/azure-corpus-*.sh`) must be added to the protected list in the **dead-code-cleanup** plan so a later cleanup pass does not flag them as orphans.
- The **coverage and chart fixes** plan's 2-host split (its Phase 2) is superseded by this Azure-specific plan. Its remaining independent items (chart bar color, BTTV/FFZ rendering, staged IRC cap raise) stay in that plan.

## Out of scope (separate follow-ups, tracked in coverage/chart-fixes plan)

- IRC scaling toward 100 on the freed BearHost (CAP-001 staged raise).
- BTTV/FFZ rollup rendering fix and the chart bar color (frontend + ingestion path; independent of this move, though long-window provider data improves as Gold drains).

[{"id": "provision-vm", "content": "Provision Azure B2ats_v2 Ubuntu VM (allowed region, ideally same region as blob), install Docker + compose, clone streamclone + streamclone-scraper, place Azure secret file and .env with Twitch creds so corpus preflight passes"}, {"id": "db-link", "content": "Install Tailscale on the Azure VM, confirm BearHost is on the tailnet, bind BearHost Postgres to the tailnet interface only (no public exposure)"}, {"id": "compose-override", "content": "Add deploy/docker-compose.corpus-remote.yml overriding analytics-workers DATABASE_URL to the BearHost tailnet Postgres and reducing depends_on to redis/metadata/scraper (drop postgres/migrate)"}, {"id": "env-profile", "content": "Add deploy/env/profile-corpus-remote.env (or reuse profile-bearhost-corpus.env) with CORPUS_WORKERS_ENABLED=1 and all tier flags enabled"}, {"id": "up-script", "content": "Add scripts/azure-corpus-up.sh to bring up analytics-workers + scraper + metadata + redis on the Azure host with the corpus profile and remote override"}, {"id": "bearhost-pulse-mode", "content": "Ensure BearHost is in Pulse mode (corpus workers off there) so the 8GB box is freed; leave API + Cloudflare tunnel unchanged"}, {"id": "apply-migration-000050", "content": "Apply pending forward-only migration 000050 (chat_source/source_confidence) once to the shared Postgres before any IVR mode"}, {"id": "gold-throughput-tune", "content": "Loosen Gold gate (GOLD_MIN_PEAK_VIEWERS, GOLD_MIN_DURATION_MINUTES) within the GQL RPM ceiling; only raise BACKFILL_GOLD_WORKER_COUNT if not GQL-saturated"}, {"id": "status-verify", "content": "Verify continuity and status via /v1/corpus/readiness, /v1/public/hub corpusPipeline counts, and Azure worker logs; confirm queued jobs resume after startup reclaim; if stale gold job still not draining, check the claim gate (silver done+confirmed or top500_vod_inventory, next_run_at<=now)"}, {"id": "ivr-shadow-canary", "content": "Deploy IVR shadow canary on the 24/7 Azure corpus host (profile-bearhost-corpus-ivr-shadow.env, ludwig allowlist, shadow-only); confirm zero chat_source='ivr' rollups; gather coverage/quality data toward GQL-relief decision"}, {"id": "status-helper", "content": "Optional: add scripts/azure-corpus-status.sh to curl corpus readiness and tail worker logs in one command"}]
