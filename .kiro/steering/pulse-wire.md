# Pulse Wire Steering

**Pulse Wire** is the optional streamer news wire at `/pulse-wire` (Story Graph engine). It is **not** Grafana **Pulse** (`make pulse`, emote/chat metrics). User setup: [docs/options.md](../../docs/options.md).

## Boundaries

- Go service: `cmd/storygraph`, `internal/storygraph/*`, `internal/social/*`
- API: `/v1/pulse-wire/*`, `/v1/channels/{login}/spread`, `POST /v1/channels/{login}/spread/backfill` (Caddy → `storygraph:8080`)
- Frontend: `frontend/src/components/pulsewire/`, `frontend/src/components/channel/SocialSpreadPanel.tsx`, `frontend/src/pulseWireApi.ts`
- Gate: `PULSE_WIRE_ENABLED=false` by default; Core Watch has no dependency on storygraph

## Two UI modes (do not conflate)

| Surface | Route | Ranking |
|---------|-------|---------|
| **Trending tab** (default) | `/pulse-wire` | Reddit hot + clip view counts + StreamerBans (`/community`, `/clips/top`, `/bans`); LSF flair chips via `/community/flairs?window=7d` and `?flair=` filter |
| **Cross-platform tab** (advanced) | `/pulse-wire?tab=wire` | Window-native story scores; **2+ source stories only** |

**Trending streamers** (Cross-platform sidebar) = most story/evidence activity. **Rising streamers** = directory viewer/rank momentum (`directory_samples`). Different metrics.

## Current Rules

- **Reader-first Trending** — no Wire fallback column, no corroboration checklist on the default tab. Hot-only landing (legacy `?view=clips|drama|funny|bans` strips to hot). LSF flair chip row under the edition header (`All` + `/community/flairs`); selecting a flair sets `?flair=` and forces `window=7d`. Community cards (Trending hero + thread grid) open Reddit on full-card click — no separate “Open thread” button.
- **Cross-platform tab** hidden until the feed has at least one story with `sourceCount >= 2`; deep links still work.
- **Missing evidence** — reader mode shows partial-spread gaps only (`sourceCount < 2` → none). Full analyst checklist requires `?analyst=1` or the Cross-platform toggle.
- `published` does not mean `Breaking`. Single-source published stories are `Developing`; multi-source
  stories without Pulse/Twitch origin are `Needs origin`.
- Cross-platform UI composes `LeadStoryDesk`, `WireStoryLanes`, and `WireReaderRail`; Source Health is
  reader-visible, while operator tools (including the missing-evidence queue) stay in the drawer.
- Visual target: compact header, lead proof desk, evidence spread, timeline, previews, lower lanes,
  and right rail. Do not present scaffolded Pulse-origin widgets as real data until backend paths exist.

- Reader-first UI: headlines, sources, links — not operator panels on the main path.
- Windows are UTC-native: `today`, `24h` (default), `7d`; rank model `window-native-v1`.
- Honest empty states: warming, scraper down, filters with no matches.
- YouTube ingest runs **last** and skips when shared scraper is unhealthy (Reddit LSF priority).
- Twitch-only single-source clusters are penalized in Wire ranking when other sources are configured.
- Do not conflate Pulse Wire with Helm `make pulse` or `make helm-pulse-wire` (Analytics → Influx only).

## Shared scraper

Reddit LSF, YouTube, and Analytics TwitchTracker share `streamclone-scraper`. Scraper failures stack: Trending stale/empty, YouTube skipped, Analytics charts may queue. Chat/7TV sync does not depend on Reddit.

## Codegraph Hints

- `get_ast_chunk("PulseWirePage")` — tab shell, Trending vs Wire
- `get_ast_chunk("ListFeed")` — Wire feed ranking
- `get_ast_chunk("ListCommunityPosts")` — Trending community
- `get_blast_radius("ingestAll")` — ingest cycle
- `get_ast_chunk("ComputeWindowScore")` — window-native ranking
- `get_ast_chunk("sampleDirectory")` — rising leaderboard sampler

## Social spread Phase 1 (channel Pulse)

Channel `/c/:login` Pulse tab includes **Social spread** — entity-linked Pulse Wire stories scoped to that streamer. LSF threads in Stream Pulse stay the reliable Reddit source until parity is signed off.

| Piece | Behavior |
|-------|----------|
| `GET /v1/channels/{login}/spread` | Primary `items` (ranked by window source count / credibility), `probableItems` (max 3, title/flair match), `meta` (entityKnown, aliases, lastIngestAt, unresolvedMentionCount, backfill state) |
| `POST .../spread/backfill` | Async per-login ingest: Reddit LSF search (7d), Helix clips, reattach unresolved mentions; 5m cooldown; in-flight dedup; `202 warming` |
| Ingest | After global Reddit hot, tiered per-login search for `ALWAYS_TRACKED_CHANNELS` + top 25 directory seeds (max 3 logins/cycle, round-robin) |
| Aliases | Flair/display from Reddit ingest merge into `streamer_entities.aliases`; resolver checks aliases before giving up |
| UI | `SocialSpreadPanel` — source health empty state, **Check for stories** CTA, match reason on channel cards, **Possible matches** section, polls while warming |

Phase 2 backlog (not implemented): Reddit `/new`+`/top`, source expansion fan-out, `social_item_entities`, embeddings, and a stronger Wire lead desk.

## Runtime Probes

```sh
curl http://localhost:8090/v1/pulse-wire/source-health
curl "http://localhost:8090/v1/pulse-wire/feed?window=24h&sort=rank"
curl "http://localhost:8090/v1/pulse-wire/community?window=24h&sort=hot"
curl "http://localhost:8090/v1/pulse-wire/community/flairs?window=7d&limit=20"
curl "http://localhost:8090/v1/pulse-wire/community?window=7d&flair=News"
curl http://localhost:8090/v1/channels/xqc/spread
curl -X POST http://localhost:8090/v1/channels/xqc/spread/backfill
```

## Checks

```sh
go test ./internal/storygraph/...
cd frontend && npm run build
```

For scraper or ingest changes, also check `GET /v1/pulse-wire/source-health` after one ingest cycle.

## Docs

- Env and optional tier: [docs/options.md](../../docs/options.md)
- Tier detachment, scraper, proxies, Social spread: [docs/tiers-scraper-and-social-spread.md](../../docs/tiers-scraper-and-social-spread.md)
- Install: [docs/install-desktop.md](../../docs/install-desktop.md)
- Scraper / proxy: [docs/scraper-cloudflare-and-proxy.md](../../docs/scraper-cloudflare-and-proxy.md)
