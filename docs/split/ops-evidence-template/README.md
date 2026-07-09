# Ops evidence template (public fallback)

Canonical copies live in private **streampulse-ops** `docs/deployments/`:

- `boundary-split-rollback.md`
- `boundary-split-migration-baseline.md`
- `boundary-split-digest-checklist.md`

Public Streamclone must **not** record production `PRE_SPLIT_MAX`, image digests, or DSN values. Use `migration-state.md` for the field checklist only.

## Operator query (run from ops checkout)

```sql
SELECT version, dirty FROM schema_migrations;
```

Archive result in ops `boundary-split-migration-baseline.md` before Step 7 public deletion.
