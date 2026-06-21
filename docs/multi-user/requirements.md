# Multi-User Platform — Requirements

Status: **proposal / not started** (2026-06). Companion docs: [../tiers-scraper-and-social-spread.md](../tiers-scraper-and-social-spread.md), [../scraping-archive/requirements.md](../scraping-archive/requirements.md#multi-user-data-access-read-plane), [../bearhost-production.md](../bearhost-production.md), [../security.md](../security.md), [.kiro/steering/local-auth.md](../../.kiro/steering/local-auth.md).

Related code (today): `internal/chat/auth/`, `internal/analytics/`, `internal/storygraph/`, `cmd/analytics/`, `frontend/src/api.ts`, `deploy/Caddyfile.bearhost`, `deploy/docker-compose.bearhost-prod.yml`.

Planned code: `internal/auth/`, user-layer migrations, shared middleware, job deduplication in analytics workers.

---

## Product vision

**PulseWire / Streamclone becomes a shared public intelligence platform with optional user accounts for personalization, saved views, follows, alerts, API keys, and operator controls — not isolated per-user scraping.**

“Multiple users” **must not** mean every user gets their own scraper, Postgres database, worker config, or TwitchTracker scrape load. The expensive planes (browser scraping, VOD/chat backfill, rollup computation) stay **global and deduplicated**.

### Chosen model: **Model B**

```text
Shared global corpus  +  thin personal user layer
```

**Not** Model C (true multi-tenant SaaS with per-org isolated analytics facts) until there are paying teams that require isolated configs.

| Layer | Scope | Examples |
|-------|-------|----------|
| **Global (shared)** | One corpus for all users | channels, streams, viewer rollups, chat rollups, emote stats, PulseWire stories, social items, ban/news events |
| **Personal (per user)** | UI, prefs, ownership of requests | saved channels, dashboards, follow-aware filters, bookmarks, alerts, API keys, job subscriptions, preferences |
| **Operational** | Platform control | tracked roster, ingest sources, worker caps, moderation, audit log |

Cross-ref read-plane scaling (GCS/API gateway): [scraping-archive requirements — Multi-user data access](../scraping-archive/requirements.md#multi-user-data-access-read-plane). This document covers **identity, personalization, job fairness, and route protection** on the hot path (Postgres + existing services).

---

## TL;DR

| Topic | Decision |
|-------|----------|
| Product shape | Shared intelligence platform; login optional for **browse**, required for **personalization** and **writes** |
| Auth for viewers | Twitch OAuth → internal `users` row → Redis session cookie |
| Auth for API clients | Scoped API keys (hash stored, never raw key) |
| Auth for operators | Separate from “logged in with Twitch”; setup token + allowlisted Twitch IDs → later admin role + audit |
| Analytics data | Global rollups; user sync requests **dedupe** to shared `sync_jobs` |
| PulseWire | Global ingest; per-user filters, mutes, saves, alerts only |
| Near-term deploy | BearHost VPS; HTTPS + domain before production login hardening |
| Defer | Username/password, per-user scrapers, per-user Postgres, `tenant_id` on fact tables, billing |

---

## Goals & non-goals

### Goals

- Allow **many anonymous readers** on public analytics and PulseWire without destabilizing BearHost (rate limits, route map).
- Allow **logged-in Twitch users** to personalize around follows, saved channels/streams/dashboards, and (later) alerts.
- Allow **operators/admins** to run syncs, manage roster/ingest, and moderate PulseWire with auditable actions.
- Allow **API consumers** to read analytics/PulseWire with scoped keys and usage limits.
- **Deduplicate** user-triggered sync/backfill jobs so N users requesting the same stream produce one worker run.
- Extract identity from chat into **shared `internal/auth`** used by chat, analytics, storygraph, and admin routes.
- Document public vs protected routes in [../security.md](../security.md).

### Non-goals (explicit)

- Custom username/password authentication.
- Per-user scraper containers or per-user worker configs.
- Per-user Postgres databases or full tenant isolation on rollup tables.
- Per-user TwitchTracker / Reddit / LSF ingest.
- Billing, org subscriptions, or complex team RBAC (Phase 5+ only if needed).
- Replacing Twitch OAuth as the primary **viewer** identity provider.

---

## Current state (as-built)

| Area | Today |
|------|-------|
| **Production runtime** | BearHost VPS (`141.11.243.103`), Caddy `:80`, build-on-VPS from rsynced repo ([bearhost-production.md](../bearhost-production.md)) |
| **Data** | Single shared Postgres; global worker roster (`TIER0_ROSTER_TOP_N`, `ALWAYS_TRACKED_CHANNELS`) |
| **Viewer auth** | Twitch OAuth / dev import under `internal/chat/auth/`; cookie `streamclone_session`; Redis-backed session; `/v1/me`, `/v1/me/followed` on **chat** service |
| **Analytics API** | Read routes effectively **public** when exposed through Caddy; no session middleware |
| **PulseWire** | Optional `storygraph` profile; public reader when enabled; operator routes use `X-Streamclone-Setup-Token` / Bearer setup token |
| **Scraper** | Internal Docker network only; optional `SCRAPER_API_KEY` for service calls |
| **“Session” in analytics** | Twitch **stream session** (broadcast), not user login |

**Gap:** Identity is chat-owned; analytics and PulseWire are not account-aware; no user tables, job deduplication, quotas, or unified RBAC.

---

## Access layers

Login is **optional for browsing** but **required for personalization and write actions**.

### Anonymous

| Capability | Allowed |
|------------|---------|
| Browse public analytics (channels, streams, rollups) | Yes (rate-limited) |
| Browse PulseWire public feed | Yes (rate-limited) |
| Watch public playback pages | If enabled by deploy |
| Trigger sync/backfill | **No** |
| Save channels/streams/dashboards | **No** |
| Chat send / follows | **No** |

### Logged-in viewer (Twitch OAuth)

| Capability | Allowed |
|------------|---------|
| All anonymous read capabilities | Yes (higher rate limits) |
| My follows, follow-aware channel picker | Yes |
| Saved channels, streams, dashboards | Yes |
| User preferences (layout, theme, default window) | Yes |
| PulseWire Following tab, mutes, saved stories | Yes |
| Request limited sync jobs (deduped, quota) | Yes (Phase 3) |
| Alert rules | Yes (Phase 4) |
| Chat send (if chat enabled) | Yes |

### Operator

| Capability | Allowed |
|------------|---------|
| Force backfill / retry jobs | Yes |
| Manage global tracked channel roster | Yes |
| PulseWire ingest controls, story suppress/merge | Yes |
| View job queue and scraper health | Yes |
| Emergency access via setup token | Yes |

**Requirement MU1:** Operator privileges SHALL NOT be granted solely by “any Twitch login.”

### Admin

| Capability | Allowed |
|------------|---------|
| All operator capabilities | Yes |
| Issue/revoke API keys for others | Yes |
| Assign operator/admin roles | Yes |
| Adjust worker caps and system config | Yes |
| All actions audited | Yes |

**Requirement MU2:** Admin assignment SHALL be invite-only (DB role or env allowlist), not self-service.

### API key principal

| Capability | Allowed |
|------------|---------|
| Scoped read (analytics, PulseWire, streams) | Per key scopes |
| Mutations / admin | Only if scope includes `admin:*` |

---

## Auth architecture

### Principle: Twitch OAuth is one identity provider, not the whole platform

On successful Twitch OAuth:

```text
User → Twitch OAuth callback
     → validate token, fetch Twitch user id + display name + avatar
     → upsert users row (twitch_user_id unique)
     → create Redis session bound to users.id
     → Set-Cookie streamclone_session (HttpOnly)
     → frontend GET /v1/me
```

### Target package layout

Extract and generalize from `internal/chat/auth/`:

```text
internal/auth/
  session/          # cookie sign/verify, Redis store
  oauth/twitch/     # OAuth callback, token exchange
  rbac/             # roles, RequireRole, RequireScope
  apikeys/          # issue, verify, hash, prefix lookup
  middleware.go     # OptionalUser, RequireUser, RequireRole, RequireScope
  principal.go      # Principal struct on request context
```

Chat, analytics, storygraph, and future admin handlers **import the same middleware**.

**Requirement MU3:** All services exposing `/v1/me/*` or personalized routes SHALL use shared `internal/auth` middleware by Phase 2 complete.

### Session cookie (production)

| Attribute | Value |
|-----------|-------|
| Name | `streamclone_session` (keep for migration) or `streamclone_sid` |
| HttpOnly | `true` |
| Secure | `true` (requires HTTPS) |
| SameSite | `Lax` |
| Path | `/` |

**Requirement MU4:** Production login (non-loopback) SHALL NOT be advertised until HTTPS + registered Twitch redirect URIs are configured.

Raw IP HTTP (BearHost today) remains valid for smoke and anonymous browse; login is dev/smoke until domain cutover.

### Operator / admin authorization (phased)

| Phase | Mechanism |
|-------|-----------|
| Phase 1 | Protect admin routes with setup token (existing pattern) + Caddy rate limits |
| Phase 2 | `OPERATOR_TWITCH_USER_IDS` env allowlist → sets `users.role = operator` on login |
| Phase 3 | DB `role` column; admin promotes users; `audit_events` on all admin actions |
| Phase 5+ | Optional OIDC for team; organizations |

### API keys

Separate from Twitch sessions. Store **hash + prefix** only.

```text
Authorization: Bearer sk_live_<prefix>_<secret>
```

Scopes (initial):

```text
analytics:read
pulsewire:read
streams:read
alerts:write
admin:jobs
admin:ingest
```

**Requirement MU5:** API keys SHALL be revocable, scope-bound, and rate-limited per key.

---

## Request identity (`Principal`)

Every HTTP request resolves to:

```go
type Principal struct {
    UserID    *uuid.UUID  // nil if anonymous
    Role      string      // anonymous | viewer | operator | admin
    Scopes    []string    // from API key if applicable
    IsAPIKey  bool
    IsService bool        // scraper, setup-control, internal
}
```

Middleware behavior:

| Route group | Middleware |
|-------------|------------|
| `/v1/analytics/*` (read) | `OptionalUser` + rate limit |
| `/v1/pulsewire/*` (read) | `OptionalUser` + rate limit |
| `/v1/me/*` | `RequireUser` |
| `/v1/analytics/sync-requests` | `RequireUser` + quota |
| `/v1/admin/*` | `RequireRole(operator)` or setup token |
| `/v1/internal/*` | `RequireServiceToken` |

Personalization: when `Principal.UserID` is set, handlers may attach user-specific overlays (saved flags, follow filters); **response bodies still read global facts**.

---

## Data model

### Global tables (unchanged ownership)

Existing analytics and PulseWire fact tables remain **without `tenant_id`**. All users read the same:

- channel / stream / rollup tables
- `social_items`, story graph tables
- directory samples, emote stats

### New: identity & personal layer

#### `users`

```sql
users (
  id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  twitch_user_id  text UNIQUE NOT NULL,
  display_name    text NOT NULL DEFAULT '',
  avatar_url      text,
  role            text NOT NULL DEFAULT 'viewer',  -- viewer | operator | admin
  created_at      timestamptz NOT NULL DEFAULT now(),
  last_seen_at    timestamptz
);
```

Optional later: `user_identities` if adding non-Twitch providers (Phase 5+).

#### `user_preferences`

```sql
user_preferences (
  user_id                 uuid PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
  default_time_window     text,
  default_chart_layout    jsonb,
  theme                   text,
  created_at              timestamptz NOT NULL DEFAULT now(),
  updated_at              timestamptz NOT NULL DEFAULT now()
);
```

#### `user_saved_channels`

```sql
user_saved_channels (
  user_id     uuid REFERENCES users(id) ON DELETE CASCADE,
  channel_id  text NOT NULL,
  label       text,
  created_at  timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (user_id, channel_id)
);
```

#### `user_saved_streams`

```sql
user_saved_streams (
  user_id     uuid REFERENCES users(id) ON DELETE CASCADE,
  stream_id   text NOT NULL,
  note        text,
  created_at  timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (user_id, stream_id)
);
```

#### `user_saved_dashboards`

```sql
user_saved_dashboards (
  id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id     uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  name        text NOT NULL,
  config      jsonb NOT NULL DEFAULT '{}',
  created_at  timestamptz NOT NULL DEFAULT now(),
  updated_at  timestamptz NOT NULL DEFAULT now()
);
```

#### PulseWire personal layer

```sql
user_pulsewire_preferences (
  user_id             uuid PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
  muted_terms         text[] NOT NULL DEFAULT '{}',
  muted_entities      text[] NOT NULL DEFAULT '{}',
  preferred_sources   text[] NOT NULL DEFAULT '{}',
  default_tab         text,
  created_at          timestamptz NOT NULL DEFAULT now(),
  updated_at          timestamptz NOT NULL DEFAULT now()
);

user_saved_stories (
  user_id     uuid REFERENCES users(id) ON DELETE CASCADE,
  story_id    uuid NOT NULL,  -- FK to story cluster id when wired
  note        text,
  created_at  timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (user_id, story_id)
);

user_alert_rules (
  id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id     uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  rule_type   text NOT NULL,   -- e.g. entity_mention, ban, keyword_spike
  entity_id   text,
  keywords    text[] NOT NULL DEFAULT '{}',
  delivery    text NOT NULL DEFAULT 'in_app',  -- in_app | webhook (future)
  enabled     boolean NOT NULL DEFAULT true,
  created_at  timestamptz NOT NULL DEFAULT now()
);
```

#### `api_keys`

```sql
api_keys (
  id                  uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  owner_user_id       uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  name                text NOT NULL,
  key_prefix          text NOT NULL,
  key_hash            text NOT NULL,
  scopes              text[] NOT NULL DEFAULT '{}',
  rate_limit_per_min  int NOT NULL DEFAULT 60,
  created_at          timestamptz NOT NULL DEFAULT now(),
  last_used_at        timestamptz,
  revoked_at          timestamptz
);
```

#### `audit_events`

```sql
audit_events (
  id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  actor_user_id   uuid REFERENCES users(id),
  action          text NOT NULL,
  target_type     text,
  target_id       text,
  metadata        jsonb NOT NULL DEFAULT '{}',
  created_at      timestamptz NOT NULL DEFAULT now()
);
```

#### Job deduplication (Phase 3)

```sql
sync_jobs (
  id                      uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  job_type                text NOT NULL,
  channel_id              text,
  stream_id               text,
  vod_id                  text,
  dedupe_key              text UNIQUE NOT NULL,
  status                  text NOT NULL DEFAULT 'pending',
  priority                int NOT NULL DEFAULT 40,
  requested_by_user_id    uuid REFERENCES users(id),
  created_at              timestamptz NOT NULL DEFAULT now(),
  started_at              timestamptz,
  finished_at             timestamptz,
  error                   text
);

sync_job_subscribers (
  job_id      uuid REFERENCES sync_jobs(id) ON DELETE CASCADE,
  user_id     uuid REFERENCES users(id) ON DELETE CASCADE,
  created_at  timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (job_id, user_id)
);
```

**Dedupe rule:** Same `(job_type, channel_id, stream_id, vod_id)` → one row; additional users insert into `sync_job_subscribers` only.

**Requirement MU6:** User-triggered jobs SHALL NOT spawn duplicate worker runs when a pending or running job with the same `dedupe_key` exists.

---

## Analytics multi-user behavior

### Core rule

Multiple users **share the same analytics corpus**. Workers write global rollups once; all users read the same minute charts and stream metadata.

### User-triggered sync flow

```text
User A requests sync for stream S
  → compute dedupe_key
  → INSERT sync_jobs OR attach subscriber if exists
  → worker picks job by priority
  → writes global rollups
  → subscribers see status via GET /v1/me/sync-requests

User B requests same stream S
  → attach to existing job (no second worker run)
```

### Personal analytics features (Phase 2+)

| Feature | Source |
|---------|--------|
| My streamers | `user_saved_channels` + Helix follows |
| My saved VODs/streams | `user_saved_streams` |
| My chart layout | `user_preferences` + `user_saved_dashboards` |
| Follow-aware channel picker | `/v1/me/followed` + global channel list |
| Recently viewed | client or `user_recent_streams` (optional P2) |

### Quotas & priority

| Principal | Sync requests | Read rate | Job priority |
|-----------|---------------|-----------|--------------|
| Anonymous | **None** | 60/min/IP | — |
| Viewer | 5/hour/user (configurable) | 300/min/user | 40 |
| Operator | Unlimited manual | High | 100 |
| Global roster / Tier-0 | N/A (system) | N/A | 80 (tracked), 60 (top-N) |

**Requirement MU7:** Anonymous clients SHALL NOT trigger backfill or sync mutations.

**Requirement MU8:** Worker queue SHALL respect priority ordering (operator > always-tracked > top-N > viewer requests).

---

## PulseWire multi-user behavior

### Core rule

**Global ingest, personal lens.** One storygraph worker set ingests Reddit/LSF/bans/social; users filter and save on top.

| Global (shared) | Personal (per user) |
|-----------------|---------------------|
| `social_items`, stories, entities, edges, rank snapshots | preferences, mutes, saved stories, alert rules |
| Operator moderation actions | Following tab (join to Helix follows) |

### User features

| Feature | Phase |
|---------|-------|
| Following tab (stories mentioning followed channels) | 2 |
| Mute terms / entities | 2 |
| Save story | 2 |
| Alerts (ban, mention, spike) | 4 |

### Operator features

| Feature | Auth |
|---------|------|
| Add/remove ingest source | operator |
| Force ingest | operator |
| Suppress / merge stories | operator + audit |
| Source trust promote/demote | operator + audit |
| Scraper failure dashboard | operator |

**Requirement MU9:** PulseWire ingest SHALL remain a single global pipeline; no per-user social scrapers.

---

## API surface

### Public read (anonymous + rate limit)

```text
GET /v1/analytics/channels
GET /v1/analytics/streams/:id
GET /v1/analytics/streams/:id/rollups
GET /v1/pulsewire/stories
GET /v1/pulsewire/stories/:id
```

Existing route shapes SHOULD be reused where possible ([scraping-archive API sketch](../scraping-archive/requirements.md#public--shared-data-api-sketch)).

### Auth

```text
GET  /v1/auth/twitch/login
GET  /v1/auth/twitch/callback
POST /v1/auth/logout
GET  /v1/me
GET  /v1/me/followed
```

**Note:** Today `/v1/me` lives on chat; Phase 2 may centralize on a dedicated auth route or keep chat as issuer with shared session validation everywhere.

### Personal (RequireUser)

```text
GET    /v1/me/preferences
PUT    /v1/me/preferences
GET    /v1/me/saved-channels
POST   /v1/me/saved-channels
DELETE /v1/me/saved-channels/:channelId
GET    /v1/me/saved-streams
POST   /v1/me/saved-streams
DELETE /v1/me/saved-streams/:streamId
GET    /v1/me/saved-dashboards
POST   /v1/me/saved-dashboards
PUT    /v1/me/saved-dashboards/:id
DELETE /v1/me/saved-dashboards/:id
GET    /v1/me/pulsewire/preferences
PUT    /v1/me/pulsewire/preferences
GET    /v1/me/saved-stories
POST   /v1/me/saved-stories
GET    /v1/me/alerts
POST   /v1/me/alerts
PUT    /v1/me/alerts/:id
DELETE /v1/me/alerts/:id
```

### Job requests (RequireUser, Phase 3)

```text
POST /v1/analytics/sync-requests
GET  /v1/analytics/sync-requests/:id
GET  /v1/me/sync-requests
```

### Operator / admin (RequireRole or setup token)

```text
POST /v1/admin/analytics/backfill
POST /v1/admin/pulsewire/ingest
POST /v1/admin/pulsewire/stories/:id/suppress
GET  /v1/admin/jobs
GET  /v1/admin/system/health
POST /v1/admin/api-keys
DELETE /v1/admin/api-keys/:id
```

---

## Rate limiting & abuse protection

Redis-backed counters keyed by:

- IP address (anonymous)
- `user_id` (logged-in)
- `api_key_id` (API clients)
- Route group
- Job type (sync requests)

| Limit | Default |
|-------|---------|
| Anonymous analytics reads | 60/min/IP |
| Logged-in reads | 300/min/user |
| PulseWire story reads | 120/min/IP |
| Viewer sync requests | 5/hour/user |
| Admin job mutations | operator role only |

**Requirement MU10:** Rate limits SHALL be enforceable at app middleware with Redis; Caddy may add edge limits for anonymous floods.

Scraper protection: viewer-requested jobs never bypass global `SCRAPER_MAX_CONCURRENT`; quotas prevent one user exhausting the pool.

---

## Frontend requirements

| Area | Change |
|------|--------|
| Auth context | Single provider from `GET /v1/me`; anonymous vs viewer states |
| Login CTA | Shown for save/sync/alert actions; not blocking public browse |
| Analytics | Follow-aware picker; saved channels/streams; saved dashboards |
| PulseWire | Following tab; mutes; saved stories |
| Operator panel | Hidden unless `role` is operator/admin |
| API keys UI | Admin/settings page (Phase 4) |

**Requirement MU11:** Frontend SHALL degrade gracefully when logged out (read-only public UI).

---

## Deployment & security (BearHost + future HTTPS)

| Item | Requirement |
|------|-------------|
| Postgres, Redis, scraper, workers | Not published to host (existing BearHost overlay) |
| UFW | 22, 80, 443 only |
| Production login | HTTPS + domain + Twitch redirect URIs |
| Admin routes | App middleware + optional VPN/Tailscale/Cloudflare Access later |
| Audit | All `/v1/admin/*` mutations write `audit_events` |
| Docs | Extend [../security.md](../security.md) with public/private route map |

Phase 1 can ship on IP HTTP with **anonymous browse + operator token** before Twitch login is production-ready.

---

## Implementation phases

### Phase 1 — Safe public app

**Goal:** Many readers without breaking VPS.

| Deliverable | Requirement IDs |
|-------------|-----------------|
| Domain + HTTPS cutover path documented and executed | MU4 |
| Public vs operator route map in security.md | MU1 |
| Rate limiting on public read APIs | MU10 |
| Operator/admin route protection (setup token + env allowlist stub) | MU1, MU2 |
| No exposure of internal ports | (existing BearHost) |
| Audit events table + logging for admin mutations | MU2 |

**Acceptance tests**

- Anonymous `GET /v1/analytics/streams/:id` succeeds with 429 after burst limit.
- `POST /v1/admin/*` without credentials returns 401.
- Scraper/Postgres ports not reachable from WAN.
- Smoke: [bearhost-production.md](../bearhost-production.md) gates still pass.

### Phase 2 — Twitch login + personal UI

**Goal:** “My version” of the shared product.

| Deliverable | Requirement IDs |
|-------------|-----------------|
| `internal/auth` extracted; chat uses shared session | MU3 |
| `users` + preferences + saved channels/streams/dashboards migrations | — |
| `/v1/me/*` personal routes | MU11 |
| Follow-aware analytics picker | — |
| PulseWire Following tab + mutes + saved stories | MU9 |
| `OPERATOR_TWITCH_USER_IDS` → role on login | MU1 |

**Acceptance tests**

- OAuth flow creates `users` row; repeat login updates `last_seen_at`.
- Saved channel persists across sessions; other users do not see it.
- Operator allowlist grants admin UI; normal viewer does not.

### Phase 3 — User-triggered jobs

**Goal:** Request sync without duplicating worker load.

| Deliverable | Requirement IDs |
|-------------|-----------------|
| `sync_jobs` + `sync_job_subscribers` | MU6 |
| `POST /v1/analytics/sync-requests` with dedupe | MU6, MU7 |
| Quotas + priority queue in workers | MU8 |
| Job status UI | — |

**Acceptance tests**

- Two users request same stream → one job row, two subscribers, one worker execution.
- Anonymous sync request → 401.
- Viewer over quota → 429.

### Phase 4 — Alerts & API keys

**Goal:** Value beyond browsing.

| Deliverable | Requirement IDs |
|-------------|-----------------|
| `user_alert_rules` + in-app delivery | — |
| `api_keys` issue/revoke + scope enforcement | MU5 |
| Usage tracking (`last_used_at`) | MU5 |

**Acceptance tests**

- API key with `analytics:read` only cannot call admin routes.
- Revoked key returns 401 immediately.

### Phase 5 — Organizations (only if needed)

**Goal:** Teams with isolated **configs**, not isolated **facts**.

| Deliverable | Notes |
|-------------|-------|
| `organizations`, `memberships` | — |
| Org-owned watchlists, dashboards, alerts, API keys | No `tenant_id` on rollup tables |
| Optional OIDC | Enterprise |
| Billing | Out of scope until product decision |

---

## Files likely to change

| Area | Paths |
|------|-------|
| Auth extraction | `internal/auth/**`, refactor `internal/chat/auth/**` |
| Migrations | `migrations/*_multi_user_*.sql` |
| Analytics API | `internal/analytics/api/**`, `cmd/analytics/**` |
| Workers / jobs | `internal/analytics/sync.go`, `internal/analytics/backfill_worker.go`, new job store |
| Storygraph | `internal/storygraph/api/**`, personal route handlers |
| Frontend | `frontend/src/api.ts`, auth context, Analytics/PulseWire pages |
| Deploy | `deploy/Caddyfile*`, `deploy/env/profile-bearhost-prod.env`, [../security.md](../security.md) |
| Tests | `internal/chat/auth/*_test.go` → `internal/auth/*_test.go`, API integration tests |

---

## Risks & rollback

| Risk | Mitigation |
|------|------------|
| Session break during auth refactor | Keep cookie name/format compatible; feature-flag new middleware |
| Public API abuse on BearHost | Phase 1 rate limits before promoting URL |
| User sync requests overload scraper | Quotas + dedupe + low viewer priority |
| Twitch OAuth on HTTP IP | Defer production login until HTTPS |
| Scope creep to multi-tenant | Explicit non-goals; org tables only in Phase 5 |

**Rollback:** Phases are forward-only migrations; personal tables are additive. Disable viewer sync via env (`USER_SYNC_REQUESTS_ENABLED=false`) without dropping global data.

---

## Requirement index

| ID | Summary |
|----|---------|
| MU1 | Operator ≠ any Twitch login |
| MU2 | Admin invite-only + audited |
| MU3 | Shared `internal/auth` middleware |
| MU4 | HTTPS before production login |
| MU5 | API keys: hash, scopes, revoke |
| MU6 | Job deduplication |
| MU7 | Anonymous cannot trigger sync |
| MU8 | Worker priority queue |
| MU9 | Global PulseWire ingest only |
| MU10 | Redis rate limits |
| MU11 | Frontend anonymous degrade |

---

## Open questions (resolve before Phase 2 coding)

1. **Auth route host:** Keep OAuth on chat service vs new `auth` microservice in compose?
2. **Cookie domain:** Single domain for Caddy vs subdomain split (API vs UI).
3. **Migrate existing sessions:** One-time invalidation on deploy vs dual-read period.
4. **PulseWire on BearHost:** Enable `storygraph` profile on VPS in Phase 1 or 2?
5. **Alert delivery:** In-app only for Phase 4, or webhook MVP?

---

## Next step

Break Phase 1 into tracked tasks (migrations optional in P1; rate limits + route map + security doc first). Do not start Phase 2 auth extraction until Phase 1 acceptance tests pass on BearHost or staging with HTTPS plan confirmed.
