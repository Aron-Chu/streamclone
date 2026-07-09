# Dependency inventory (boundary split)

Generated 2026-07-09 from `rg` audits on public Streamclone. Gate for Step 3+ code move.

**Decision record:** `PACKAGE_SCOPE=streampulse` · `PACKAGE_ALIAS_POLICY=lockstep` (no npm alias shims; coordinated rename in one merge window).

---

## Go: analytics → core (reverse deps)

Command:

```bash
rg 'streamclone/internal/analytics' internal/chat internal/emote internal/metadata internal/video internal/config
```

**Result:** zero matches — clean split boundary; core packages do not import analytics.

---

## Go: analytics outbound deps

Command:

```bash
rg 'streamclone/internal/(chat|emote|config|emoteimage|redact|boundaryguard)' internal/analytics cmd/analytics cmd/backfill
```

**Hits (representative):**

| Import | Used by (sample) | Decision |
|--------|------------------|----------|
| `internal/config` | `api.go`, `ingestcore/config.go`, `hosted_*`, `pulse_*`, `top500_*`, … | **Copy** hosted/admission subset to **streampulse-backend** (cannot import public `internal/` from sibling repo) |
| `internal/chat/batch`, `parse`, `tokenize`, `enrich` | `rollup.go`, `collector.go`, `trie_tokenizer.go`, `ingestcore/engine.go`, `sync_gql_parallel.go` | **Move with analytics** to backend |
| `internal/emote/flags` | `store.go` | **Move with analytics** or duplicate flags package in backend |
| `internal/emoteimage` | `store.go`, `hosted_emote_urls_test.go`, `sync_gql_parallel.go` | **Move with analytics** |
| `internal/redact` | `job_reconcile.go`, `clip_replayforge.go`, `job_mirror_callback.go` | **Extract** to `pkg/redact` in public streamclone **or duplicate** in backend |
| `internal/boundaryguard` | *(no hits in internal/analytics)* | **Stay public**; backend copies if needed |

**Invalid pattern:** backend repo importing `streamclone/internal/*` — **forbidden**. Resolve via copy or `pkg/` extract before scaffold.

---

## Go: cmd/analytics entry imports

Command:

```bash
rg 'streamclone/' cmd/analytics cmd/backfill cmd/archive
```

**Sample `cmd/analytics/main.go` imports:** `internal/analytics`, `internal/analytics/chatreplay`, `internal/analytics/heatmap`, `internal/analytics/ingestcore`, `internal/archive`, `internal/chat/batch`, `internal/chat/enrich`, `internal/chat/ircconn`, `internal/config`, `internal/emote/render`, `internal/emote/seeder`, `internal/httpx`, `internal/log`, `internal/metrics`, `internal/redisutil`, …

**Decision:** entire `cmd/analytics`, `cmd/backfill`, `cmd/archive`, `internal/analytics/**`, `internal/archive/**` → **streampulse-backend**.

---

## npm / frontend

Command:

```bash
rg '@streamclone/pulse|packages/pulse' frontend packages
```

**Frontend hits:**

| File | Package | After split |
|------|---------|-------------|
| `frontend/package.json` | `@streamclone/pulse-core` → `file:../packages/pulse-core` | **Remove** when VOD moment UI trimmed (Step 5); inline `frontend/src/utils/vodLink.ts` or drop feature |
| `frontend/src/components/Analytics.tsx`, `channel/VodMomentsPanel.tsx`, … | `@streamclone/pulse-core` | Remove with Analytics tab / VOD UI trim |
| `packages/pulse-core`, `pulse-charts`, `analytics-console` | inter-package | **Move entire trees** to **streampulse-backend** |

**streamclone-pulse:** `streampulse-web/package.json` `file:` deps retarget to `../streampulse-backend/packages/*` after backend exists; rename scope `@streamclone/*` → `@streampulse/*` (lockstep).

---

## Summary table

| Artifact | Stay public | Copy to backend | New shared |
|----------|-------------|-----------------|------------|
| `cmd/metadata`, `video`, `chat`, `emote` | ✓ | | |
| `internal/video`, `chat`, `emote`, `metadata` | ✓ | | |
| `cmd/analytics`, `backfill`, `archive` | | ✓ | |
| `internal/analytics/**` | | ✓ | |
| `packages/pulse-*`, `analytics-console` | | ✓ | |
| `internal/config` (hosted caps) | partial | ✓ subset | optional `pkg/config` extract |
| `internal/redact`, `boundaryguard` | ✓ today | duplicate or | `pkg/*` extract |
| `frontend` watch UI | ✓ (no pulse deps) | | `vodLink.ts` if VOD deeplinks kept |

**Sign-off:** zero TBD rows; zero invalid cross-repo `internal/` imports planned.
