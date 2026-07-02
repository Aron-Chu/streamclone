# Migration reconciliation — 2026-07-02

## Problem

BearHost production `schema_migrations` is at **version 58**, `dirty=false`, but the git
checkout (through `2660a3a`) only shipped migrations through **000050**. A normal
`bearhost-pulse-redeploy-remote.sh` fails at `migrate` because golang-migrate cannot find
files for versions 51–58.

Migrations 051–058 were applied on BearHost from an uncommitted working tree that was never
pushed to `origin/master`. The remote `/opt/streamclone/app/migrations/` directory also
stopped at `000050` after rsync.

## Resolution (forward-only)

Added recovery migrations **000051–000058** to the repo:

| Version | Purpose |
|--------:|---------|
| 051 | `public_emote_provider_hourly_rollups` |
| 052 | `public_emote_materialization_runs` |
| 053–055 | Emote Atlas snapshot tables (retired) |
| 056 | `pulse_collector_leases` |
| 057 | Atlas materialization bookkeeping (retired) |
| 058 | Drop Atlas tables (forward-only retirement) |

All `up` migrations use `CREATE TABLE IF NOT EXISTS` / `DROP TABLE IF EXISTS` so:

- **Existing v58 DB:** `migrate up` is a no-op (version already recorded).
- **Fresh install:** tables are created in order; 058 drops retired Atlas tables.
- **pg_restore from BearHost:** schema matches; subsequent `migrate up` succeeds without error.

## Do not

- Edit or delete rows in `schema_migrations` on production unless doing a controlled `migrate force` after operator review.
- Run `migrate down` across 058 on production (Atlas retirement is forward-only).
- Assume recovery stubs match every index/constraint on BearHost — after cutover, diff with:

```sql
SELECT table_name FROM information_schema.tables
WHERE table_schema = 'public'
  AND table_name LIKE 'public_emote%' OR table_name = 'pulse_collector_leases'
ORDER BY 1;
```

## Deploy checklist

1. Rsync repo including `migrations/000051`–`000058`.
2. `docker compose … up migrate` — must exit 0 with `version=58`.
3. Run `bearhost-analytics-predeploy-gate` (or VPS equivalent) before analytics recreate.
4. For streampulse-vps SoT: restore BearHost `pg_dump -Fc` first, then migrate, then smoke.

## Redis

Redis holds ephemeral cache keys only (public hub cache, emote hot paths). No pg_dump-style
migration is required; an empty Redis on the new VPS is acceptable. Warm caches rebuild from
Postgres on first hub requests.
