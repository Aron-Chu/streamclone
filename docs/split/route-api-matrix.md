# Route / API matrix (boundary split)

Caddy today (`deploy/Caddyfile`) includes an `@analytics` matcher:

```caddyfile
path_regexp analytics ^/v1/(analytics|extension|pulse|public|portal)(/|$)
```

**Target state:** public Streamclone Caddy serves **core routes only**. BFF paths removed in Step 4 (same PR as compose trim).

---

## Route ownership

| Route prefix | Current local owner | After split owner | Public Streamclone | Validation |
|--------------|--------------------|--------------------|--------------------|------------|
| `/v1/analytics/*` | analytics:8080 | **streampulse-backend** (hosted) | **Remove** | `curl :8090/v1/analytics/...` → 404 |
| `/v1/extension/*` | analytics BFF | **streampulse-backend** | **Remove** | Hosted extension health (private ops probe) |
| `/v1/pulse/*` | analytics | **streampulse-backend** | **Remove** | Extension smoke vs hosted API |
| `/v1/public/*` | hub, status | **streampulse-backend** | **Remove** | Portal hub against hosted API |
| `/v1/portal/analytics/*` | portal BFF | **streampulse-backend** | **Remove** | streampulse-web session page |
| `/v1/internal/ops/*` | ops launch API | **streampulse-backend** | **Remove** | Private ops SSH probe |
| `/v1/triggers/*`, `/studio`, clipper callbacks | analytics | **streampulse-backend** | **Remove** | ReplayForge integration (private) |
| `/v1/metadata/*`, catch-all `/v1/*` | metadata | **streamclone** | **Keep** | `make smoke` |
| `/v1/stream/*` | video | **streamclone** | **Keep** | `playback_probe` |
| `/v1/auth*`, `/v1/me`, `/v1/ws` | chat | **streamclone** | **Keep** | `twitch_auth_status` |
| `/v1/emotes*`, `/v1/channels/*/emotes*` | emote | **streamclone** | **Keep** | emote MCP |
| `/live/*` HLS | mediamtx via Caddy | **streamclone** | **Keep** | HLS curl / Playwright |
| `/`, SPA | frontend | **streamclone** | **Keep** | `GET /` |

No proxy from public Streamclone to hosted API — product boundary doc only.

---

## Local StreamPulse dev routing (after split)

| Surface | Today | After split |
|---------|-------|-------------|
| **streampulse-web** default | hosted API | Unchanged (hosted-first) |
| **streampulse-web `dev:local`** | often `:8090` on Streamclone | **streampulse-backend** local API port |
| **Extension Options → Advanced** | optional `localhost:8090` | Document: local BFF = backend `make up`; Streamclone `:8090` is watch-only |
| **Extension service worker** | `getBackendUrl()` hosted default | Local override = backend base URL |

Update **streamclone-pulse** `docs/website-portal/local-dev-runbook.md` in Step 6 — not this repo.

---

## Validation probes (gate)

```bash
# Core must work
make up && make smoke

# BFF must NOT be served locally (after Step 4)
curl -o /dev/null -w '%{http_code}' http://localhost:8090/v1/extension/health   # expect 404

# Portal local mode (after backend compose up — private checkout)
curl -fsS http://localhost:<backend-port>/v1/extension/health
```

Add `deploy/smoke/test-core-routes-only.sh` in Step 4.
