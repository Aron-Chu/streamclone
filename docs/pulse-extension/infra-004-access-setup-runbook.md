# INFRA-004 — Cloudflare Access setup runbook

Status: operator runbook (Batch S). Implements Phase A of [`infra-004-admin-access-decision.md`](../../../streamclone-pulse/docs/website-portal/infra-004-admin-access-decision.md).

**Prerequisites:** Cloudflare account with `streampulse.stream` zone; BearHost tunnel `api.streampulse.stream` live; Grafana on VPS localhost `:3000`.

---

## 1. Identity provider + operator group

1. Cloudflare dashboard → **Zero Trust** → **Settings** → **Authentication**.
2. Add identity provider (Google, GitHub, or OIDC).
3. Create Access group **`streampulse-operators`** with allowed operator emails.

---

## 2. Access application — Grafana

| Field | Value |
|-------|--------|
| Application name | StreamPulse Grafana |
| Subdomain | `grafana` |
| Domain | `streampulse.stream` |
| Policy | Allow `streampulse-operators` only |
| Bypass | **None** |

After policy exists, add tunnel public hostname:

| Hostname | Service |
|----------|---------|
| `grafana.streampulse.stream` | `http://127.0.0.1:3000` |

Verify: anonymous `curl -sI https://grafana.streampulse.stream/` → **403** or **302** (not Grafana login HTML).

---

## 3. Access application — Pulse admin API

| Field | Value |
|-------|--------|
| Application name | StreamPulse Admin API |
| Hostname | `api.streampulse.stream` |
| Path | `/v1/admin/*` |
| Policy | Allow `streampulse-operators` only |

Copy the **Application AUD tag** from the Access app settings.

Set on BearHost analytics (secrets or env overlay — never commit):

```bash
PULSE_CF_ACCESS_TEAM_DOMAIN=your-team.cloudflareaccess.com
PULSE_CF_ACCESS_AUD=<application-aud-tag>
```

Optional break-glass (local/Tailscale only):

```bash
ADMIN_ARCHIVE_TOKEN=<long-random>
ADMIN_ARCHIVE_REQUIRE_TOKEN=true
```

---

## 4. Pages admin route (when `/admin` ships)

Cloudflare Pages → `streampulse.stream` → **Access** → policy on `/admin` and `/admin/*` for `streampulse-operators`.

Beta dashboard `/dashboard/*` must **not** require Access.

---

## 5. Deploy sequence

1. Create Access apps **before** exposing Grafana tunnel hostname publicly.
2. Set `PULSE_CF_ACCESS_*` on analytics; redeploy Pulse stack.
3. Update Caddy ([`Caddyfile.pulse-api`](../../deploy/Caddyfile.pulse-api)) — routes `/v1/admin/pulse/*` to analytics (archive admin remains 404 on Pulse host).
4. Run `deploy/smoke/test-013b-hosted.sh`.
5. Run `deploy/smoke/test-access-setup-checklist.sh` (Batch T1 verification).
6. Append §4.6 ledger row.

---

## 6. Rollback

| Issue | Action |
|-------|--------|
| Operators locked out | Time-boxed Access bypass policy or Tailscale SSH + archive token |
| Beta API blocked | Remove path-scoped Access app; keep API hostname open for extension |
| JWT validation bug | Revert to Caddy `@admin_pulse` 404 until fix deployed |

---

## 7. Local dev (no Access)

```bash
PULSE_HOSTED_MODE=false
PULSE_ADMIN_LOCAL_BYPASS=true
# or
ADMIN_ARCHIVE_TOKEN=dev-only-token
```

Hosted production: **never** set `PULSE_ADMIN_LOCAL_BYPASS=true`.
