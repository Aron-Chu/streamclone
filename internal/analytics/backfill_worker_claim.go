package analytics

import (
	"strings"
)

// IsGoldWorkerTierFilter reports whether the filter targets gold-tier jobs only.
func IsGoldWorkerTierFilter(tierFilter []string) bool {
	if len(tierFilter) == 0 {
		return false
	}
	for _, t := range tierFilter {
		t = strings.ToLower(strings.TrimSpace(t))
		switch t {
		case "gold", "gold_full", "gold_lite":
			continue
		default:
			return false
		}
	}
	return true
}

// ClaimNextSQL returns the SELECT used to claim the next queued job for a tier filter.
// Empty tierFilter uses legacy FIFO across all tiers.
func ClaimNextSQL(tierFilter []string) string {
	if len(tierFilter) == 0 {
		return `
		SELECT id, tier, stream_id, login, egress_slot, attempt, export_status, status, next_run_at, COALESCE(error,'')
		FROM backfill_jobs
		WHERE status = 'queued' AND next_run_at <= now()
		ORDER BY next_run_at ASC, id ASC
		FOR UPDATE SKIP LOCKED
		LIMIT 1`
	}
	if IsGoldWorkerTierFilter(tierFilter) {
		return `
		SELECT gold.id, gold.tier, gold.stream_id, gold.login, gold.egress_slot, gold.attempt, gold.export_status, gold.status, gold.next_run_at, COALESCE(gold.error,'')
		FROM backfill_jobs gold
		WHERE gold.status = 'queued' AND gold.next_run_at <= now()
		  AND gold.tier IN ('gold','gold_full','gold_lite')
		  AND EXISTS (
			SELECT 1 FROM backfill_jobs silver
			WHERE silver.tier = 'silver'
			  AND silver.status = 'done'
			  AND silver.export_status = 'confirmed'
			  AND EXISTS (
				WITH RECURSIVE canonical_path(stream_id, depth, path) AS (
					SELECT silver.stream_id, 0, ARRAY[silver.stream_id]
					UNION ALL
					SELECT next_id, canonical_path.depth + 1, canonical_path.path || next_id
					FROM canonical_path
					CROSS JOIN LATERAL (
						SELECT COALESCE(
							(SELECT NULLIF(canonical_stream_id, '') FROM analytics_stream_aliases WHERE alias_stream_id = canonical_path.stream_id),
							(SELECT NULLIF(canonical_stream_id, '') FROM analytics_streams WHERE stream_id = canonical_path.stream_id)
						) AS next_id
					) resolved
					WHERE next_id IS NOT NULL
					  AND next_id <> ALL(canonical_path.path)
					  AND canonical_path.depth < 32
				)
				SELECT 1 FROM canonical_path WHERE stream_id = gold.stream_id
			  )
		  )
		ORDER BY gold.next_run_at ASC, gold.id ASC
		FOR UPDATE OF gold SKIP LOCKED
		LIMIT 1`
	}
	return `
		SELECT id, tier, stream_id, login, egress_slot, attempt, export_status, status, next_run_at, COALESCE(error,'')
		FROM backfill_jobs
		WHERE status = 'queued' AND next_run_at <= now()
		  AND tier = ANY($1::text[])
		ORDER BY next_run_at ASC, id ASC
		FOR UPDATE SKIP LOCKED
		LIMIT 1`
}

func normalizeTierFilter(tierFilter []string) []string {
	if len(tierFilter) == 0 {
		return nil
	}
	out := make([]string, 0, len(tierFilter))
	for _, t := range tierFilter {
		t = strings.ToLower(strings.TrimSpace(t))
		if t != "" {
			out = append(out, t)
		}
	}
	return out
}
