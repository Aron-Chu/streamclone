# Identity model MVP (2026-07)

Product decision for StreamPulse hosted beta. Do not widen private surfaces until this model is reflected in UI copy and route guards.

## Principals

| Principal | How established | Used for |
|-----------|-----------------|----------|
| **Guest** | No beta key; public portal/extension default | Read-only `/analytics`, public hub, channel analytics |
| **Beta-key** | Extension options or portal `localStorage` beta key | Protect, watchlist, bookmarks, `/dashboard/*`, clip queue |
| **Guest (hosted API)** | Device fingerprint / anonymous principal on backend | Rate limits only — not user-facing "account" |

OAuth / Twitch account identity is **later** — not MVP.

## Route matrix

| Route | Auth | Notes |
|-------|------|-------|
| `/analytics` | None | Canonical public hub |
| `/analytics/:login` | None | Public read-only channel console |
| `/login`, `/setup` | Redirect → `/analytics` | Legacy; do not re-expose as product login |
| `/dashboard`, `/dashboard/clips` | `RequireAuth` (beta key) | Private workspace — not linked from public hub |
| `/v1/pulse/clips` | Hosted auth + clip principal | Never advertise on public pages until P1-006 durable playback |

## UI rules

- Public hub must not show "Sign in" or clip CTAs that imply a launched clip product.
- Extension copy may mention beta key for **extension options only**, not as a requirement to view `/analytics`.
- Protect / bookmarks errors ("requires beta key") are acceptable on **gated actions**, not on hub landing.

## Validation

```bash
cd streamclone-pulse/streampulse-web
rg -n "/dashboard|/clips|Sign in|beta key" src/routes/analytics src/ui/components/analytics src/routes/public
# Expect no public analytics links to /dashboard/clips
```
