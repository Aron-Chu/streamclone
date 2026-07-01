# Corpus PR 0B-2 — hosted verification checklist (read-only)

**Purpose:** Post-deploy verification after `GOLD_VOD_SEGMENTS_ENABLED` canary on streampulse-vps corpus workers.
**Rules:** Hosted MCP / `app_readonly` is **SELECT only**. No requeue, claim, update, or delete via MCP.

**Prerequisites:**

- PR 0B-1 merged (`000049` migration chain on disk).
- PR 0B-2 deployed (`feat(analytics): wire gold_vod_segments into parallel GQL fetch`).
- Migration `000049` applied on BearHost Postgres (normal migrate path).
- Corpus worker profile has `GOLD_VOD_SEGMENTS_ENABLED=false` until this checklist completes baseline steps 1–4.

---

## 1. Migration present

```sql
SELECT to_regclass('public.gold_vod_segments');
```

Expect: `gold_vod_segments` (not null).

---

## 2. Baseline segment ledger (before enabling flag)

```sql
SELECT status, count(*)::int
FROM gold_vod_segments
GROUP BY 1
ORDER BY 1;
```

Record counts. Expect zero or historical test rows only before first flagged Gold job.

---

## 3. Backfill queue health

```sql
SELECT tier, status, count(*)::int
FROM backfill_jobs
GROUP BY 1, 2
ORDER BY 1, 2;
```

---

## 4. Top-500 Gold inventory status

```sql
SELECT gold_status, count(*)::int
FROM top500_vod_inventory
GROUP BY 1
ORDER BY 1;
```

---

## 5. Canary enable (operator — not MCP)

On **one** streampulse-vps corpus worker only:

```env
GOLD_VOD_SEGMENTS_ENABLED=true
```

Redeploy/restart analytics workers. Trigger or wait for one Gold `backfill_jobs` run (gold / gold_full tier).

---

## 6. After canary Gold job — segment activity

```sql
SELECT status, count(*)::int
FROM gold_vod_segments
GROUP BY 1
ORDER BY 1;

SELECT lease_owner, status, count(*)::int
FROM gold_vod_segments
GROUP BY 1, 2
ORDER BY 1, 2;

SELECT vod_id,
       min(start_offset_seconds) AS min_off,
       max(end_offset_seconds) AS max_off,
       count(*)::int AS segments
FROM gold_vod_segments
GROUP BY vod_id
ORDER BY segments DESC
LIMIT 10;
```

Expect after a successful Gold GQL fetch:

- Non-zero `queued` → `running` → `done` transitions for the canary VOD.
- `lease_owner` like `gold-<hostname>` while running; cleared on `done`.
- No indefinite `running` rows with `lease_expires_at < now()` after worker restart (reclaim path — verify in PR 0B-3).

---

## 7. Public API — no segment leak

```bash
curl -sS https://api.streampulse.stream/v1/public/hub | jq 'keys'
```

Manual / automated check:

- Response must **not** include: `gold_vod_segments`, `lease_owner`, segment status, per-stream IDs, logins, job errors, worker IDs.
- `corpusPipeline` remains aggregate counts only.
- `TestPublicHubResponseOmitsSensitiveKeys` passes in CI.

---

## 8. Rollback

Set `GOLD_VOD_SEGMENTS_ENABLED=false` on canary worker; restart. Migration stays applied. In-memory GQL + checkpoints continue unchanged.

---

## 9. Soak gate (before wider enable)

- [ ] 24h canary: no growth in `dead_letter` without operator action.
- [ ] Grafana: `analytics_vod_gql_throttle_total` / `analytics_vod_gql_backoff_seconds_total` stable vs baseline.
- [ ] No false `backfill_jobs.status=done` with unresolved segments (full gate in PR 0B-3).
