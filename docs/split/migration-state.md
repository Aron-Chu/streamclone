# Migration state (public template)

Production `schema_migrations` row values are **ops-owned**. Do not commit guessed version numbers in the public Streamclone repo. After cutover lock, the canonical record lives in **private streampulse-ops** (for example `docs/deployments/boundary-split-rollback.md`).

Use the field names below consistently in split docs and checklists.

| Field | Source | Stored where |
|-------|--------|--------------|
| `pre_cutover_schema_version` | `SELECT version FROM schema_migrations` on hosted PG at cutover lock | streampulse-ops only |
| `pre_cutover_schema_dirty` | `SELECT dirty FROM schema_migrations` (must be `false` before force-set) | streampulse-ops only |
| `pre_cutover_captured_at` | UTC timestamp when the query was run | streampulse-ops only |
| `pre_cutover_evidence_ref` | Ops path or digest tying the row to rollback anchor | streampulse-ops only |

## Ops query (run in private ops context)

```sql
SELECT version, dirty FROM schema_migrations;
```

## How public docs reference this

- **Upgraded pre-split desktop:** next core migration file number must be **greater than** `pre_cutover_schema_version` (once ops has recorded it).
- **Hosted production baseline:** backend migrate uses ops-recorded `pre_cutover_schema_version` / `pre_cutover_schema_dirty` without re-running removed public analytics SQL.
- **This repo:** link to [`migration-split.md`](migration-split.md) for ownership classes; never paste live `version` values here.

## Status

| Step | Owner | Public checklist |
|------|-------|------------------|
| Prod query captured | streampulse-ops | [ ] |
| Rollback anchor updated | streampulse-ops | [ ] |
| Backend fresh-DB migrate test | streampulse-backend | [ ] |
| Public core-only fresh-DB test | streamclone | [ ] |
| Upgraded-install simulation | streamclone + ops | [ ] |
