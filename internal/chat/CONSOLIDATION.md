# IRC ingest consolidation (phase 1)

## Current state

Three independent Twitch IRC WebSocket pools exist today:

| Consumer | Entry | Pool |
|----------|-------|------|
| Live chat | `cmd/chat/main.go` | `ircconn.Manager` (read + optional authenticated send) |
| Analytics collector | `cmd/analytics/main.go` | `ircconn.Manager` (read only) |
| Clipper spikes | `clipper/liveclipper/irc.py` | `IRCMonitor` (Python, separate process) |

Each pool dials `wss://irc-ws.chat.twitch.tv:443`, requests `twitch.tv/tags`, and JOINs channels independently. This duplicates upstream connections and JOIN traffic.

## Phase 1 (this change)

- `internal/chat/pubsub` exposes Redis IRC bus helpers:
  - `IRCBusChannel` (`irc:lines`) — global raw-line fan-out
  - `IRCBusKey(channel)` — per-channel variant for selective subscribers
  - `PublishIRCLine` / `SubscribeIRCLines` — consolidation bus API
- **No behavior change** — existing `ircconn.Manager` pools in chat and analytics are untouched.

## Target path (phase 2+)

1. **Single reader pool** in `cmd/chat` (or a dedicated `cmd/irc` service) publishes every raw IRC line via `pubsub.PublishIRCLine`.
2. **Chat handler** stays local: parse → enrich → `pubsub.Publish` on `chat:{channel}` (unchanged WebSocket contract).
3. **Analytics** replaces its `ircconn.Manager` with `pubsub.SubscribeIRCLines` → `collector.HandleIRCLine`.
4. **Clipper** subscribes to the same bus (Go sidecar or Redis bridge) instead of opening a third pool.
5. **JOIN/PART** remain refcounted in the single pool; analytics watch/unwatch and chat subscribe/unsubscribe coordinate through one joiner.

## Migration constraints

- Do not break authenticated chat send (`ircconn.SenderManager`) — send path can stay on the consolidated pool.
- Preserve `HandleIRCLine` parsing in analytics; only change the transport.
- Clipper Python process may need a thin Redis subscriber or a shared Go relay until clipper moves to the bus natively.
