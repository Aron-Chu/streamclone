# Low-Latency Playback — Requirements

Status: **proposal / not started**. Related code: `internal/video/token`, `internal/video/usher`, `internal/video/orchestrator` (`proxy.go`, `latency.go`, `orchestrator.go`), `internal/video/worker`, `deploy/mediamtx.yml`, `deploy/Caddyfile*`, `frontend/src/playback.ts`, `frontend/src/playbackMath.ts`.

---

## TL;DR of the feasibility call

The legacy Streamlink-as-blueprint idea — *"build a custom Twitch-aware adaptive HLS relay so we beat Streamlink"* — is **mostly already done and partly mis-aimed for this product**:

- Streamlink is only a **fallback backend**. The `direct_hls` backend already does native Go extraction (`token` + `usher`) and serves a custom, ad-filtered manifest via `/v1/stream/proxy`. "Replace Streamlink as extractor" is ~80% shipped.
- The roadmap assumes **mpv with a tiny buffer**. Streamclone is a **browser** app that re-originates through **MediaMTX → hls.js**. mpv cache/catch-up tricks do not apply; the hls.js equivalents already exist in `playback.ts`.
- The dominant end-to-end latency is the **re-origination hop** (`ffmpeg → RTMP → MediaMTX mpegts HLS @ 2s × 15 → Caddy → hls.js`), **not** the upstream `--hls-live-edge`. So a server-side adaptive edge controller upstream of ffmpeg buys little and adds instability risk.

**Conclusion:** Do **not** rebuild the literal roadmap. Pursue the *reframed* scope below, where the headline win is the **transport hop**, supported by drift control and a two-way adaptive latency mode.

### Explicitly out of scope

- Rewriting Streamlink/extraction from scratch — already native via `token`/`usher`/`direct_hls`.
- An mpv-based relay or any mpv-specific tuning — wrong player for this product.
- A bespoke "ringbuffer → stdin/FIFO → player" transport — the browser needs HTTP-origin (HLS/LL-HLS) or WebRTC, not a pipe.
- Static `--hls-live-edge 1` as a default — flex setting; unstable, and not where the latency lives.

---

## Goals & non-goals

**Goals**

- Cut steady-state glass-to-glass live delay from the current ~8–12s toward **3–6s** with stability comparable to or better than today.
- Make latency mode **recover upward** (not just degrade), and recover from drift after a stall by jumping to live instead of accumulating delay.
- Keep the existing dual-backend reliability (`direct_hls` → `streamlink` fallback) and honest error surfacing intact.

**Non-goals**

- Beating Twitch's own publish delay (impossible from the viewer side).
- Sub-second latency as a hard requirement for all channels/networks (WebRTC is a best-effort enhancement, not a guarantee).
- Changing the extraction layer (`token`/`usher`) behavior.

---

## Baseline (what exists today)

| Layer | Today |
|------|-------|
| Extraction | Native Go `token` (GQL `PlaybackAccessTokenLive`) + `usher` master-playlist parse. No Streamlink needed. |
| Transport backends | `direct_hls` (Go manifest proxy + ad drop → ffmpeg copy → RTMP) and `streamlink` (fallback). |
| Re-origination | MediaMTX `hlsVariant: mpegts`, `hlsSegmentDuration: 2s`, `hlsSegmentCount: 15`. |
| Delivery | Caddy `:8090` → MediaMTX `:8888`, `Authorization: Bearer` + `hlsCDNSecret`. |
| Browser | hls.js with latency modes (`instant`/`fast`/`stable`), `lowLatencyMode`, `maxLiveSyncPlaybackRate` ≤1.3x, per-fragment metrics, stall-based **one-way** downgrade. |
| Server latency control | `parseLatencyMode` → static live-edge 1/2/3; `waitForHLS` readiness probe; no per-segment timing telemetry. |

---

## Requirements

EARS-style. Priority: **P0** = headline latency win, **P1** = stability/control, **P2** = observability/nice-to-have.

### R1 — Low-latency re-origination variant (P0)

The system SHALL provide a low-latency re-origination path that reduces the MediaMTX buffering contribution below the current `2s × 15` mpegts window.

- R1.1 The system SHALL support MediaMTX `hlsVariant: lowLatency` (LL-HLS with `EXT-X-PART`) behind a config flag, with a fallback to `mpegts` when a client/codec cannot negotiate parts.
- R1.2 The system SHALL allow configurable `hlsSegmentDuration` and `hlsSegmentCount` (and part target where LL-HLS is active) via env, defaulting to a shorter live window than today.
- R1.3 Caddy and the `hlsCDNSecret`/`Authorization: Bearer` contract SHALL continue to work for LL-HLS part requests (no 401 regressions on `/live/*`).
- R1.4 hls.js `lowLatencyMode` SHALL consume parts when present; `playback.ts` SHALL set `liveSyncDuration`/part-aware sync targets rather than fixed segment counts when LL-HLS is active.

### R2 — Optional WebRTC (WHEP) passthrough (P0, best-effort)

The system SHOULD offer a WebRTC/WHEP delivery path from MediaMTX for sub-second latency, selectable per session and gated by capability detection.

- R2.1 MediaMTX WebRTC SHALL be exposed through the existing proxy boundary (no raw service ports to the browser).
- R2.2 The frontend SHALL feature-detect WHEP support and fall back to LL-HLS, then mpegts HLS, with no user-visible hard failure.
- R2.3 Diagnostics SHALL report which transport (`webrtc` / `ll-hls` / `hls-mpegts`) is active.

### R3 — Drift / stale-segment control (P1)

The player SHALL prefer a small jump-to-live over accumulating delay after a stall.

- R3.1 When `behindLiveSec` exceeds an adaptive threshold (function of target latency + segment/part duration), the player SHALL snap toward live edge rather than play back-buffer. (Extends existing `calculateLiveEdge`/`jumpLive`.)
- R3.2 The server manifest proxy MAY trim stale media entries beyond the live window for the `direct_hls` path, reusing the existing `proxyPlaylist` rewrite pipeline (do not regress `filterTwitchAdSegments`).
- R3.3 Drift recovery SHALL be rate-limited so it does not oscillate (no repeated micro-seeks within a short window).

### R4 — Two-way adaptive latency mode (P1)

The latency mode SHALL recover toward lower-latency settings when conditions are stable, not only downgrade.

- R4.1 The client SHALL downgrade `instant → fast → stable` on repeated stalls (existing behavior preserved).
- R4.2 The client SHALL attempt an **upgrade** back toward the user-selected mode after a stable window (e.g. N seconds without stalls and healthy buffer), bounded by the user's selected ceiling.
- R4.3 Upgrade/downgrade transitions SHALL be debounced and surfaced in metrics (`effectiveLatencyMode`).

### R5 — Latency-aware quality downshift (P1)

The system SHOULD reduce quality before dropping frames when segment/part fetch time is risky.

- R5.1 When download time per segment/part exceeds a fraction of its media duration over a short window, the player SHALL cap to a lower hls.js level (extends `capLevelToPlayerSize`).
- R5.2 Quality SHALL recover upward under the same stability gate as R4.2.
- R5.3 Manual quality selection SHALL override automatic downshift (respect the existing requested-vs-loaded-quality separation from the playback steering rule).

### R6 — Per-segment telemetry (P2)

The system SHALL expose timing telemetry sufficient to validate the latency wins and drive R4/R5 decisions.

- R6.1 The browser SHALL record (already partially present): fragment/part download bytes+duration, bandwidth estimate, buffer ahead, `behindLiveSec`, stalls, recovery attempts, first-frame ms.
- R6.2 The server `direct_hls` path SHOULD record playlist reload RTT and segment first-byte/total fetch time, exported via the existing diagnostics endpoint and Prometheus metrics.
- R6.3 Diagnostics SHALL report the active transport (R2.3), effective latency mode, live-edge/part target, and measured end-to-end delay (reuse `computeEndToEndLiveDelaySec`).

### R7 — Reliability & compatibility (P0 guardrail)

The changes SHALL NOT regress existing reliability guarantees.

- R7.1 The `direct_hls` → `streamlink` fallback and supervisor restart/backoff SHALL remain intact.
- R7.2 VOD relay playback SHALL be unaffected (its config path in `playback.ts` stays `lowLatencyMode: false`).
- R7.3 Honest error surfacing for offline/unavailable/auth-blocked/warming streams SHALL be preserved.
- R7.4 Every new behavior SHALL be flag-gated and default to current behavior until validated.

---

## Acceptance criteria

- A/B on ≥3 representative channels (incl. one 1080p60) shows median glass-to-glass delay reduced vs. the current `streamlink` default, at equal or fewer stalls per 10 min.
- LL-HLS path degrades cleanly to mpegts HLS where parts are unavailable; WebRTC degrades cleanly to LL-HLS.
- After an induced 5s network stall, the player returns to within target latency within one stability window (no permanent drift).
- No 401 regressions on `/live/*` through Caddy; `make compose-config-check` passes.

---

## Risks / open questions

- **MediaMTX LL-HLS maturity** for republished Twitch content (part alignment after `-c copy` remux) needs a spike before committing R1 defaults.
- **WebRTC behind Caddy** on Windows/WSL localhost may need ICE/host-candidate config; validate before promising R2.
- The single biggest win may be R1 alone; R3–R5 are refinements. Sequence R1 first and **re-measure before** building the adaptive controller, to avoid adding instability for marginal gains.

---

## Suggested checks

```sh
go test ./internal/video/...
cd frontend && npm run build
make compose-config-check
# Manual: curl -w "%{http_code}" http://localhost:8090/live/{channel}/index.m3u8
```
