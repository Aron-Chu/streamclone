# Corpus hosted baseline — 2026-07-02 (post canary-blocker deploy)

**Deploy SHA:** `2660a3a` (`fix/corpus-canary-blockers` merged to `master`)
**Commits:**
- `7197575` fix(analytics): speed up public hub and isolate mixed live rollups
- `2660a3a` fix(analytics): heartbeat and reclaim stale gold vod segments

**Scope:** Hub perf + mixed live-graph parity on hosted API; segment heartbeat/reclaim on streampulse-vps corpus worker. **Post-cutover (2026-07-02):** hosted API and Postgres SoT are on streampulse-vps (`23.173.152.156`), not BearHost.

---

## Public hub (blocker cleared)

| Check | Result | Pass |
|-------|--------|:----:|
| `GET /v1/public/hub?activityWindow=30m` | HTTP **200**, ~0.3–1.5s | yes (<10s) |
| `GET /v1/public/hub?activityWindow=7d` | HTTP **200**, ~0.3–0.6s | yes (<15s) |
| Top-level keys | `activity`, `corpus`, `corpusPipeline`, `coverage`, `emoteIntel`, `generatedAt`, `liveChannels`, `moments`, `poolSize`, `topEmotes`, `topMovers` | yes |
| Segment / lease leak | No `gold_vod_segments`, `lease_owner`, segment keys | yes |

`GET /v1/extension/health` → **200** after analytics manual start (see deploy notes).

---

## Segment ledger (BearHost Postgres, read-only)

| status | count |
|--------|------:|
| done | 6408 |
| queued | 1373 |
| running | 24 |
| skipped | 5633 |
| failed / dead_letter | 0 |

| Check | Result | Pass |
|-------|--------|:----:|
| `to_regclass('public.gold_vod_segments')` | present | yes |
| False done (done gold jobs + unresolved segments) | **0** rows | yes |
| Stale `running` (`lease_expires_at < now()`) | **24** (pre-reclaim; see below) | pending |

---

## VPS canary env (`streampulse-analytics-workers`)

```
GOLD_VOD_SEGMENTS_ENABLED=true
ANALYTICS_VOD_GQL_CONCURRENCY=2
BACKFILL_GOLD_WORKER_COUNT=1
BACKFILL_GOLD_WORKER_ENABLED=true
```

Single worker; flag not widened beyond canary host.

---

## Deploy notes

### BearHost API

- `bearhost-rsync-to-vps.sh` succeeded; analytics image rebuilt with `2660a3a`.
- `bearhost-pulse-redeploy-remote.sh` **failed** on `migrate` exit 1 (DB at version 58; checkout migrations stop at `000050`; duplicate `000055` down on remote). Analytics + pulse-caddy were left in `Created` state → **manually started** (`docker start streamclone-analytics-1 streamclone-emote-1 streamclone-pulse-caddy`). API recovered.
- **Follow-up (ops):** reconcile BearHost migration tree vs applied version 58 before next pulse redeploy; do not rely on manual `docker start`.

### streampulse-vps corpus

- `CANARY_GOLD_VOD_SEGMENTS=1 scripts/streampulse-vps-corpus-deploy.sh` completed; `streampulse-analytics-workers` rebuilt/restarted.
- Reclaimer logs show `dial tcp 100.93.173.75:5432: connect: connection refused` intermittently after BearHost postgres recreate — worker **unhealthy**, stale reclaim not yet applied.
- **Follow-up (ops):** restore Tailscale Postgres reachability from VPS → BearHost; restart worker; expect `stale_running` to drop after `StartStaleGoldVODSegmentReclaimer` startup + interval (~3m).

---

## Go / no-go

| Next step | Status |
|-----------|--------|
| Continue 24h canary soak | **Go** for hub + ledger hygiene; monitor stale reclaim after PG connectivity fix |
| PR 2 coverage/gap linkage | **Hold** until soak §9 passes |
| Second worker / widen flag | **No** |
