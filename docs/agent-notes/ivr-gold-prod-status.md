# Agent note — IVR / Gold prod status (2026-06-28)

Short runtime truth for agents. Full audit instructions: [`ivr-gold-audit-prompt.md`](./ivr-gold-audit-prompt.md).

## Is everything working?

| Layer | Status (2026-06-28) | Evidence |
|-------|---------------------|----------|
| **Hosted Pulse API** (`api.streampulse.stream`) | **Up** | `GET /v1/extension/health` → `ok:true`, `version:v0.3.0-rc4`, `hostedMode:true` |
| **Public hub aggregates** | **Up** (degraded roster) | `GET /v1/public/hub` → `databaseOk:true`, live pool active |
| **Gold GQL backfill (corpus)** | **Running in prod DB** | Hub `corpusPipeline.gold`: `running:1`, `queued:542`, `done:372` (historical + active queue) |
| **Top-500 metadata + live IRC** | **Prod** | Pulse API profile: `TOP500_METADATA_ENABLED`, `TIER0_ENABLED`, `CORPUS_WORKERS_ENABLED=0` on API host |
| **IVR shadow canary (`PROD_SHADOW_CANARY_ONLY`)** | **Not deployed** | Code committed locally; overlay env exists; no prod proof of `gold_ivr effective config` |
| **Hosted Layer-2 auth (S10 fix)** | **Code ready, prod pending deploy** | Gated: `/v1/analytics/streams/{id}`, `/replay-heatmap`, `/channels/{login}/live` (+ `writeStreamDetail` guard); prod still open until image rsync |
| **Emoteverse prototypes** | **Not wired to backend** | `streampulse-web/prototypes/emoteverse/*` uses synthetic `shared/data.js` only |
| **Emote Atlas (portal `/analytics/{login}/emotes`)** | **Partial / beta** | Backend `PortalChannelEmotes` + portal route exist in repo; depends on emote-history migrations and materialization |

**Degraded signals (expected on 8 GB VPS):** hub `coverage.state=degraded`, roster `liveCollectorDeficitRows`, gold queue backlog — not automatic IVR failures.

## IVR vs Gold — what is “in prod”?

```text
IN PROD TODAY
├── Top-500 Helix metadata sampler
├── Live IRC collector (partial realtime graph for tracked channels)
├── Gold VOD chat via GQL SyncHistoricalStream (corpus workers when host is in corpus mode)
└── Portal/extension read paths from Postgres rollups (GQL canonical + live verified)

NOT IN PROD (hold until audit + deploy)
├── GOLD_IVR_SHADOW_MODE canary (ludwig allowlist, artifacts only)
├── GOLD_IVR_LITE / peaks-only / canonical_replace writes
└── Migration 000050 chat_source stamping on prod Postgres (until migrate + deploy)
```

BearHost runs **one heavy mode at a time** on 8 GB: Pulse API (`bearhost-pulse-api.sh`, `GOLD_BACKFILL_ENABLED=false`) vs corpus workers (`bearhost-corpus-only.sh`). Gold job counts in `/v1/public/hub` reflect **Postgres queue state** across modes — confirm which compose profile is active on VPS before attributing running gold to the current host profile.

## Commit authorship (agents)

All commits must be **Aron-Chu `<aroncloudchu@gmail.com>` only** — no `Co-authored-by: Cursor`.

Cursor may auto-append co-author trailers on `git commit`. Rewrite with WSL:

```bash
export GIT_AUTHOR_NAME='Aron-Chu' GIT_AUTHOR_EMAIL='aroncloudchu@gmail.com'
export GIT_COMMITTER_NAME='Aron-Chu' GIT_COMMITTER_EMAIL='aroncloudchu@gmail.com'
parent=$(git rev-parse HEAD~1)
tree=$(git rev-parse 'HEAD^{tree}')
new=$(git commit-tree "$tree" -p "$parent" -F /path/to/message.txt)
git update-ref refs/heads/$(git branch --show-current) "$new"
```

See `.cursor/rules/commits.mdc`.

## Local proof before prod canary

```bash
bash scripts/bench/ivr-shadow-reconcile-proof.sh
bash scripts/bearhost-migration-000050-preflight.sh   # read-only prod gate
```

## Deploy checklist (IVR shadow only — not done)

1. Push streamclone commit; rsync/deploy BearHost corpus workers image.
2. **Preflight:** `bash scripts/bearhost-migration-000050-preflight.sh` → `MIGRATION_000050=PASS` (3 columns).
3. `make migrate` on VPS — apply **000050** if preflight fails.
3. Merge `profile-bearhost-corpus-ivr-shadow.env` **after** `profile-bearhost-corpus.env`; recreate `analytics-workers` only.
4. Confirm startup log: `gold_ivr effective config` with `shadow=true`, `lite=false`, `allowlist=[ludwig]`.
5. Run one ludwig gold job; verify artifacts under `runtime/ivr-shadow/` on worker volume; **no** new `chat_source=ivr` rollup rows.

## Related docs

- [`docs/bearhost-production.md`](../bearhost-production.md) — IVR shadow section
- [`streamclone-pulse/docs/pulse-extension/gold-ivr-gql-only-conclusion.md`](../../../streamclone-pulse/docs/pulse-extension/gold-ivr-gql-only-conclusion.md)
- [`scripts/bench/ivr-shadow-reconcile-proof.sh`](../../scripts/bench/ivr-shadow-reconcile-proof.sh)
