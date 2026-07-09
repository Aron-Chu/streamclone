# Streamclone — Data sources & tools inventory

| | |
|---|---|
| **Status** | Draft v1 |
| **Companion** | [`roadmapping.md`](roadmapping.md) (when to build) · sibling [`live-coverage-requirements.md`](../../streamclone-pulse/docs/pulse-extension/live-coverage-requirements.md) (coverage rules) |

Split every external input into **core** (product depends on it), **enrichment** (nice-to-have), or **risky/manual-only** (never block correctness).

---

## 1. Core sources (first-class dependencies)

| Source | Use | Pulse path |
|--------|-----|------------|
| **Twitch IRC live chat** | Real-time rollups | Path A — live tracking |
| **Twitch Helix Streams / Videos** | Live state, `vodId`, metadata | Go-live, VOD resolution, BFF |
| **Twitch GQL VideoComments** | VOD chat replay | Path B — `SyncPulseMissedChat` |
| **7TV / BTTV / FFZ emotes** | Tokenization, spike labels | Emote service + enricher |
| **Postgres minute rollups** | Primary analytics store | Collector + backfill merge |
| **Redis BFF cache** | Pulse payload (`ext:pulse:v2:*`) | Extension/portal polling |

**Twitch has no official historical chat API** — capture live or use VOD replay when published.

---

## 2. Enrichment (optional, non-blocking)

| Source | Use | Notes |
|--------|-----|-------|
| Twitch clips (Helix) | Correlate peaks with clips | Not chat history |
| Category / title changes | Moment context | EventSub or Helix poll |
| Polls / predictions / hype train | “Why chat spiked” | EventSub where authorized |
| Stream schedule | Pre-warm protected tracking | Helix schedule |
| TwitchTracker / SullyGnome | Viewer chart fallback | Gap-fill only; honest labeling |
| Pulse Wire / social | External spread | Separate from chat graph |

---

## 3. Risky / manual-only

| Source | Verdict |
|--------|---------|
| Third-party chat archive sites | Not core |
| Deleted-VOD recovery scrapers | Admin/research only |
| Extension DOM/GQL VOD scrape | **Hint only** — backend Helix is source of truth |
| Paid scraping actors for VOD chat | Research, not product |
| Unofficial Kick/Pusher scraping | Wait for official API |

---

## 4. Agent & dev tools (Codex / Cursor)

| Tool | Purpose |
|------|---------|
| **streamclone-codegraph** MCP | Symbols, blast radius, call chains |
| **streamclone-stack** MCP | Health, ports, compose logs |
| **streamclone-data** MCP | SELECT-only Postgres/Redis |
| **make context-snapshots** | Offline runtime summaries |
| **StreamPulse backend/ops skills** | private **streampulse-backend** / **streampulse-ops** / **streamclone-pulse** — not this repo |

See [streampulse-product-boundary.md](streampulse-product-boundary.md) and [`CODEX.md`](CODEX.md).

---

## 5. Two backfill pipelines (do not confuse)

| Pipeline | Code | BearHost Pulse profile |
|----------|------|------------------------|
| **Pulse extension backfill** | `pulse_backfill.go`, `sync_pulse_missed.go` | User-triggered; `PULSE_MAX_BACKFILLS=1` |
| **Archive gold/silver** | `backfill_worker.go`, `gold_enqueuer.go` | **Disabled** (`BACKFILL_ENABLED=false`) |

---

## Revision history

| Date | Change |
|------|--------|
| 2026-06-23 | Initial source inventory (split from roadmap notes) |
