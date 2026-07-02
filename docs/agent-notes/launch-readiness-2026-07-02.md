# Launch readiness — 2026-07-02

Cross-repo snapshot after R2 emote cutover and pre-launch hardening. **Backend corpus-auth deploy completed 2026-07-02** on streampulse-vps; portal UI still needs Cloudflare Pages deploy.

## Deploy record (2026-07-02)

| Step | Result |
|------|--------|
| `go test ./internal/analytics/...` | **PASS** (after heatmap window validation order fix) |
| Analytics + emote rebuild on `/opt/streamclone/app` | **DONE** |
| Caddy route fix (`deploy/Caddyfile.pulse-api`) | **DONE** — `@internal_corpus_ops` proxies gaps/workers/inventory to analytics |
| Hosted corpus mutation probes | **401** unauthenticated |
| Hosted `/v1/internal/corpus/readiness` | **404** at Caddy edge (blocked; not public) |
| Public hub | No `recentAdmissions` / `rows`; `coverage=critical`, `collectorTracking=0` |
| Portal guest `/v1/portal/analytics/channels/xqc/streams` | **200** (not 401; sanitized aggregate payload) |

**Root cause:** `/v1/internal/corpus/gaps/*` and related mutation paths were not proxied to analytics — Caddy fell through to the default `@root` 200 responder. Analytics auth middleware was correct but never reached.

**Operator scripts:** `scripts/tmp/vps-corpus-auth-hardening-deploy.sh`, `scripts/tmp/vps-caddy-corpus-routes-fix.sh`, promoted read-only probes in `scripts/hosted-launch-probes.sh`

## Risk table

| Order | Risk | Owner | Status | Mitigation |
|------:|------|-------|--------|------------|
| 1 | Unauthenticated `/v1/internal/corpus/*` mutation on hosted API | Backend ops | **Closed (2026-07-02)** | Caddy `@internal_corpus_ops` + `AdminArchiveAuthMiddleware`; hosted probes 401 |
| 2 | Public hub calling raw readiness endpoints | Portal | **Fixed (local)** | `publicHub.ts` uses `/v1/public/hub` + stats/status fallback only |
| 3 | Critical coverage UI overclaims live IRC | Portal | **Fixed (local)** | `/analytics` banner + `HubDataHealthBanner` when `collectorTracking=0` |
| 4 | Live collector admission regression (`collector≈1/10`, admission disabled) | Backend ops | **Open** | Restore top-200 caps directly — 24h soak already passed (~113 active). See dual-VPS doc |
| 5 | Portal guest route payload leaks | Backend | **Hardened (tests)** | `/v1/portal/analytics/*` guest-safe + JSON forbidden-key tests |
| 6 | R2 `archive/` public read | Infra | **Guarded** | `S3_PUBLIC_READ=false`; emotes via API/proxy; verify Cloudflare bucket policy |
| 7 | Cutover mirror/runtime bucket drift | Infra | **Guarded** | Cutover script fails emote smoke unless `ALLOW_EMOTE_SMOKE_FAIL=1` |
| 8 | Extension store listing | Product | **Deferred** | Beta via `/docs#extension`; see chrome-web-store checklist |
| 9 | CDN custom domain | Infra | **Deferred** | EMOTE-R2-005 — align `CDN_PUBLIC_BASE` when ready |
| 10 | Heatmap nil-store guard changed validation order | Backend | **Fixed** | Window param validated before `store_unavailable` in `heatmap_handler.go` |

## Cloudflare tunnel token rotation

The reusable rotation script is promoted at `scripts/cloudflared-tunnel-token-rotate.sh`.
It mints a new connector token via the Cloudflare API, transfers it to the target host over
SSH, reinstalls `cloudflared`, and runs the hosted health probe without printing the token.

```bash
CLOUDFLARE_API_TOKEN=... \
CF_ACCOUNT_ID=... \
CF_TUNNEL_NAME=streampulse-bearhost \
VPS_HOST=root@23.173.152.156 \
VPS_SSH_KEY=/path/to/key \
bash scripts/cloudflared-tunnel-token-rotate.sh
```

## Public source and KPI contract (local)

Launch-readiness LB-04 is handled locally by keeping the public JSON stable while removing
`hub.corpus.momentsDetected` from rendered portal KPI rows. The backend field is currently
bookmark-backed, not a true detected-peak counter; do not restore it as "Moments detected"
until `internal/analytics/public_api.go` gives it peak-count semantics and tests pin that SQL.

Source labels for public/client surfaces:

| Source | Label | Surface |
|---|---|---|
| `live_irc` | Live IRC | live hub/channel collector rows and current peaks |
| `corpus_historical` | Corpus historical | `/v1/public/hub/moments` historical bucket moments |
| `gql_gold` | Gold VOD corpus | completed VOD/gold-corpus minutes when explicitly returned |
| `vod_synced` | VOD synced | channel/session analytics backed by synced VOD chat |
| `partial` | Partial IRC | incomplete live or VOD-linked coverage |

Public source contract:

- `/v1/public/hub.activity.points` is live-IRC activity only; corpus/gold/IVR imports are filtered from the public hub activity chart.
- `/v1/public/hub/moments` returns historical bucket moments with `source="corpus_historical"` and `hubGeneratedAt` so the portal can detect skew against the rendered chart snapshot.
- Chart buckets and bucket moments may currently use different source sets; the portal must label that mismatch rather than implying both are live or both are corpus-backed.
- `/v1/portal/analytics/*` and extension live panels must use backend-provided `sources`, `dataSourceBadges`, or `source` fields. Missing source metadata is an unknown/partial state, not Live IRC.

Coverage labels use the portal contract in `../streamclone-pulse/docs/website-portal/analytics-command-center-layout.md`.

## IRC admission restore (streampulse-vps)

**Do not re-run the 24h top-200 soak** — it already passed (~113 active tracking).

Operator target (requires explicit deploy approval):

```bash
PULSE_COLLECTOR_ENABLED=true
PULSE_TOP500_ADMISSION_ENABLED=true
PULSE_TOP500_ADMISSION_TOP_N=200
PULSE_MAX_ACTIVE_CHANNELS=200
MAX_CONCURRENT_TRACKED_CHANNELS=200
TIER0_ENABLED=true
```

Watch public hub `coverage.collectorTracking` / `collectorActive`. Rollback: `PULSE_TOP500_ADMISSION_ENABLED=false` (legacy `PULSE_TOP_ROSTER_*` alias still works).

Next scale step after stable top-200: `TOP_N=500`, caps 300 → 500 with metrics — see [`dual-vps-production-2026-07-02.md`](dual-vps-production-2026-07-02.md).

## BearHost private workers

BearHost (`141.11.243.103`) runs **`deploy/docker-compose.bearhost-worker.yml`** against VPS SoT over Tailscale — no public tunnel. Start: `bash scripts/bearhost-worker.sh`.

## Safe limited admission (streampulse-vps) — superseded

The prior cap=10 guidance was a regression guard, not the operating target. Use the top-200 block above instead.

## Deploy (operator)

Backend-only deploy closes the hosted corpus-auth blocker and backend guest-safe route behavior. It does **not** publish portal UI changes (`publicHub.ts` readiness removal or `/analytics` partial-coverage banner); those require a separate Cloudflare Pages deploy.

Before running the focused deploy script, verify the live compose checkout path. Production uses **`/opt/streamclone/app`** (`REMOTE_DIR=/opt/streamclone/app`).

```bash
# Dry run steps
bash scripts/tmp/vps-corpus-auth-hardening-deploy.sh

# Execute
EXECUTE=1 bash scripts/tmp/vps-corpus-auth-hardening-deploy.sh
```

Pre-deploy gate:

```bash
go test ./internal/analytics/... -count=1
```

If `TestPropWindowParamValidation_InvalidReturns400` fails with `503`, move heatmap `window` query validation ahead of the nil-store guard in `internal/analytics/heatmap_handler.go`.

## Hosted probes after backend deploy

Run the combined hosted truth-plane probe first. It checks public hub/admission metadata,
queue age thresholds, optional readiness JSON, and, when `PULSE_PROBE_SSH_TARGET` is set,
remote `DEPLOYED_SHA` age plus `cloudflared` systemd health:

```bash
PULSE_SMOKE_BASE_URL=https://api.streampulse.stream bash scripts/hosted-launch-probes.sh

# Optional operator-only remote checks:
PULSE_PROBE_SSH_TARGET=root@23.173.152.156 \
PULSE_PROBE_SSH_KEY=/path/to/key \
bash scripts/hosted-launch-probes.sh
```

All unauthenticated internal corpus routes must fail closed:

```bash
curl -s -o /dev/null -w '%{http_code}\n' -X POST \
  https://api.streampulse.stream/v1/internal/corpus/gaps/requeue \
  -H 'Content-Type: application/json' -d '{"segmentKeys":[]}'

curl -s -o /dev/null -w '%{http_code}\n' \
  https://api.streampulse.stream/v1/internal/corpus/readiness

curl -s -o /dev/null -w '%{http_code}\n' \
  'https://api.streampulse.stream/v1/internal/corpus/gaps?vod_id=test'

curl -s -o /dev/null -w '%{http_code}\n' \
  https://api.streampulse.stream/v1/internal/corpus/workers

curl -s -o /dev/null -w '%{http_code}\n' -X POST \
  https://api.streampulse.stream/v1/internal/corpus/inventory/vod-test/sync-gold-status \
  -H 'Content-Type: application/json' -d '{}'
```

Expected: mutation/list/worker routes return `401`; `/v1/internal/corpus/readiness` returns `404` at Caddy edge (intentionally blocked from public).

Optional server-side token smoke (do not print the token): run from the VPS with `X-Admin-Archive-Token` sourced from the server secret and verify the status is **not** `401`. This proves token/env wiring in addition to fail-closed behavior.

Public hub sanity must still show sanitized launch fields:

```bash
curl -s 'https://api.streampulse.stream/v1/public/hub?activityWindow=7d' | jq '{
  hasRecentAdmissions: has("recentAdmissions"),
  hasRows: has("rows"),
  moments: (.livePulseMoments|length),
  featured: .featuredSession.state,
  coverage: .coverage.state,
  collectorTracking: .corpusPipeline.roster.collectorTracking
}'
```

Expected: `hasRecentAdmissions=false`, `hasRows=false`, moments and `featuredSession` present. `collectorTracking=0` is allowed until live admission is intentionally enabled.

R2 archive privacy remains a separate pre-broad-launch verification: confirm Cloudflare bucket policy does not make `archive/` public, or split public emotes and private archive buckets before widening access.

## Rollback (emote + archive)

1. Revert VPS `.env` `S3_*` to MinIO endpoints/bucket/prefix.
2. Set `ARCHIVE_DUAL_WRITE=false` on `analytics-workers`.
3. `docker compose -f deploy/docker-compose.streampulse-vps-production.yml up -d emote analytics pulse-caddy analytics-workers`

## Verification run locally

```bash
# Backend
go test ./internal/analytics/...

# Portal
npm test --prefix ../streamclone-pulse/streampulse-web -- publicHub streamcloneAnalytics emoteAssetUrl
```

## Launch recommendation

| Surface | Recommendation |
|---------|----------------|
| **Public analytics API** | **Soft launch OK** — corpus auth closed; coverage still critical/honest (`collectorTracking=0`) |
| **Public analytics UI (`/analytics`)** | Needs Cloudflare Pages deploy before local `publicHub.ts` and partial-coverage banner fixes are live |
| **Extension beta** | OK — unpacked / docs install path |
| **Chrome Web Store / AMO** | Not ready — complete checklist, privacy policy, screenshots |
| **CDN (`cdn.streampulse.stream`)** | Performance polish only — EMOTE-R2-005 follow-up |
