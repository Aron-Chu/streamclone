package analytics

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

// SessionCleanupReport summarizes one-shot prefetch stub / alias cleanup.
type SessionCleanupReport struct {
	Logins              []string `json:"logins"`
	DryRun              bool     `json:"dryRun,omitempty"`
	OrphanAliasesMerged []string `json:"orphanAliasesMerged"`
	AliasesFlattened    []string `json:"aliasesFlattened"`
	AliasCyclesRepaired []string `json:"aliasCyclesRepaired"`
	BackfillJobsRekeyed []string `json:"backfillJobsRekeyed"`
	StubSessionsMerged  int      `json:"stubSessionsMerged"`
	Errors              []string `json:"errors,omitempty"`
}

type SessionCleanupOptions struct {
	DryRun bool
}

type orphanedSessionAlias struct {
	AliasStreamID     string
	CanonicalStreamID string
	Login             string
}

type sessionAliasRow struct {
	AliasStreamID     string
	CanonicalStreamID string
	Login             string
}

type backfillJobRekey struct {
	ID                int64
	StreamID          string
	CanonicalStreamID string
	Status            string
}

// CleanupSessionStubs removes orphaned alias rows and merges overlapping prefetch
// stubs for the given logins using canonical session rules.
func (s *Store) CleanupSessionStubs(ctx context.Context, logins []string) (SessionCleanupReport, error) {
	return s.CleanupSessionStubsWithOptions(ctx, logins, SessionCleanupOptions{})
}

// CleanupSessionStubsWithOptions repairs alias chains/cycles and overlapping
// prefetch stubs for the given logins. Dry-run reports planned repairs only.
func (s *Store) CleanupSessionStubsWithOptions(ctx context.Context, logins []string, opts SessionCleanupOptions) (SessionCleanupReport, error) {
	report := SessionCleanupReport{Logins: normalizeLoginList(logins), DryRun: opts.DryRun}
	if s == nil || s.db == nil {
		return report, fmt.Errorf("analytics store unavailable")
	}
	if len(report.Logins) == 0 {
		return report, nil
	}

	flattened, cycles, err := s.normalizeSessionAliases(ctx, report.Logins, opts.DryRun)
	if err != nil {
		report.Errors = append(report.Errors, err.Error())
	} else {
		report.AliasesFlattened = flattened
		report.AliasCyclesRepaired = cycles
	}

	jobRekeys, err := s.listBackfillJobRekeys(ctx, report.Logins)
	if err != nil {
		report.Errors = append(report.Errors, err.Error())
	} else {
		for _, rekey := range jobRekeys {
			report.BackfillJobsRekeyed = append(report.BackfillJobsRekeyed, fmt.Sprintf("%d:%s->%s", rekey.ID, rekey.StreamID, rekey.CanonicalStreamID))
			if opts.DryRun {
				continue
			}
			if err := s.applyBackfillJobRekey(ctx, rekey); err != nil {
				report.Errors = append(report.Errors, fmt.Sprintf("rekey backfill job %d %s -> %s: %v", rekey.ID, rekey.StreamID, rekey.CanonicalStreamID, err))
			}
		}
	}

	orphans, err := s.listOrphanedSessionAliases(ctx, report.Logins)
	if err != nil {
		return report, err
	}
	for _, row := range orphans {
		if opts.DryRun {
			report.OrphanAliasesMerged = append(report.OrphanAliasesMerged, fmt.Sprintf("%s->%s", row.AliasStreamID, row.CanonicalStreamID))
			continue
		}
		if err := s.purgeOrphanedAliasStream(ctx, row.AliasStreamID, row.CanonicalStreamID); err != nil {
			report.Errors = append(report.Errors, fmt.Sprintf(
				"merge orphan alias %s -> %s (%s): %v",
				row.AliasStreamID, row.CanonicalStreamID, row.Login, err,
			))
			continue
		}
		report.OrphanAliasesMerged = append(report.OrphanAliasesMerged, row.AliasStreamID)
	}

	merged, err := s.cleanupPrefetchStubs(ctx, report.Logins, opts.DryRun)
	report.StubSessionsMerged = merged
	if err != nil {
		report.Errors = append(report.Errors, err.Error())
	}
	return report, nil
}

// ResolveSessionCleanupLogins returns explicit logins when provided; otherwise unions env and DB always-tracked rows.
func (s *Store) ResolveSessionCleanupLogins(ctx context.Context, explicit []string, envTracked []string) ([]string, error) {
	if len(explicit) > 0 {
		return normalizeLoginList(explicit), nil
	}
	merged := append([]string{}, envTracked...)
	if s != nil && s.db != nil {
		dbLogins, err := s.GetAlwaysTracked(ctx)
		if err != nil {
			return nil, err
		}
		merged = append(merged, dbLogins...)
	}
	out := normalizeLoginList(merged)
	if len(out) == 0 {
		return nil, fmt.Errorf("no logins selected for session cleanup")
	}
	return out, nil
}

func normalizeLoginList(logins []string) []string {
	seen := make(map[string]struct{}, len(logins))
	out := make([]string, 0, len(logins))
	for _, login := range logins {
		normalized := normalizeLogin(login)
		if normalized == "" {
			continue
		}
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		out = append(out, normalized)
	}
	sort.Strings(out)
	return out
}

func (s *Store) listOrphanedSessionAliases(ctx context.Context, logins []string) ([]orphanedSessionAlias, error) {
	rows, err := s.db.Query(ctx, `
		SELECT
			st.stream_id,
			COALESCE(NULLIF(st.canonical_stream_id, ''), st.stream_id),
			st.login
		FROM analytics_streams st
		WHERE st.login = ANY($1)
		  AND st.stream_id <> COALESCE(NULLIF(st.canonical_stream_id, ''), st.stream_id)
		ORDER BY st.login, st.started_at ASC`,
		logins,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []orphanedSessionAlias
	for rows.Next() {
		var row orphanedSessionAlias
		if err := rows.Scan(&row.AliasStreamID, &row.CanonicalStreamID, &row.Login); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func (s *Store) normalizeSessionAliases(ctx context.Context, logins []string, dryRun bool) ([]string, []string, error) {
	rows, err := s.db.Query(ctx, `
		SELECT
			a.alias_stream_id,
			a.canonical_stream_id,
			COALESCE(sa.login, sc.login, '')
		FROM analytics_stream_aliases a
		LEFT JOIN analytics_streams sa ON sa.stream_id = a.alias_stream_id
		LEFT JOIN analytics_streams sc ON sc.stream_id = a.canonical_stream_id
		WHERE sa.login = ANY($1) OR sc.login = ANY($1)
		ORDER BY COALESCE(sa.login, sc.login, ''), a.alias_stream_id`, logins)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	var aliases []sessionAliasRow
	for rows.Next() {
		var row sessionAliasRow
		if err := rows.Scan(&row.AliasStreamID, &row.CanonicalStreamID, &row.Login); err != nil {
			return nil, nil, err
		}
		aliases = append(aliases, row)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}

	var flattened []string
	var cycles []string
	seenCycle := map[string]bool{}
	for _, row := range aliases {
		resolved, err := s.resolveSessionAliasForCleanup(ctx, row.AliasStreamID)
		if err != nil {
			return flattened, cycles, fmt.Errorf("resolve alias %s: %w", row.AliasStreamID, err)
		}
		if resolved.CycleDetected {
			cycleIDs := uniqueStrings(resolved.Cycle)
			sort.Strings(cycleIDs)
			cycleKey := strings.Join(cycleIDs, "->")
			if seenCycle[cycleKey] {
				continue
			}
			seenCycle[cycleKey] = true
			target, err := s.pickCanonicalStreamID(ctx, s.db, cycleIDs, row.CanonicalStreamID)
			if err != nil {
				return flattened, cycles, fmt.Errorf("choose cycle canonical %s: %w", cycleKey, err)
			}
			cycles = append(cycles, fmt.Sprintf("%s=>%s", cycleKey, target))
			if !dryRun {
				if err := s.linkSessionAlias(ctx, row.AliasStreamID, target); err != nil {
					return flattened, cycles, fmt.Errorf("repair cycle %s: %w", cycleKey, err)
				}
			}
			continue
		}
		if resolved.CanonicalID == "" || resolved.CanonicalID == row.CanonicalStreamID {
			continue
		}
		flattened = append(flattened, fmt.Sprintf("%s->%s", row.AliasStreamID, resolved.CanonicalID))
		if !dryRun {
			if err := s.linkSessionAlias(ctx, row.AliasStreamID, resolved.CanonicalID); err != nil {
				return flattened, cycles, fmt.Errorf("flatten alias %s -> %s: %w", row.AliasStreamID, resolved.CanonicalID, err)
			}
		}
	}
	return flattened, cycles, nil
}

func (s *Store) resolveSessionAliasForCleanup(ctx context.Context, streamID string) (canonicalResolution, error) {
	return resolveCanonicalStream(ctx, s.db, streamID)
}

func (s *Store) listBackfillJobRekeys(ctx context.Context, logins []string) ([]backfillJobRekey, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id, stream_id, status
		FROM backfill_jobs
		WHERE login = ANY($1)
		  AND status IN ('queued', 'failed', 'done')
		ORDER BY id ASC`, logins)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []backfillJobRekey
	for rows.Next() {
		var row backfillJobRekey
		if err := rows.Scan(&row.ID, &row.StreamID, &row.Status); err != nil {
			return nil, err
		}
		canonicalID, err := s.ResolveCanonicalStreamID(ctx, row.StreamID)
		if err != nil {
			return nil, err
		}
		if canonicalID == "" || canonicalID == row.StreamID {
			continue
		}
		row.CanonicalStreamID = canonicalID
		out = append(out, row)
	}
	return out, rows.Err()
}

func (s *Store) applyBackfillJobRekey(ctx context.Context, rekey backfillJobRekey) error {
	if rekey.ID == 0 || rekey.StreamID == "" || rekey.CanonicalStreamID == "" || rekey.StreamID == rekey.CanonicalStreamID {
		return nil
	}
	tag, err := s.db.Exec(ctx, `
		UPDATE backfill_jobs
		SET stream_id = $2, updated_at = now()
		WHERE id = $1
		  AND NOT (
			status IN ('queued', 'running')
			AND EXISTS (
				SELECT 1 FROM backfill_jobs existing
				WHERE existing.id <> backfill_jobs.id
				  AND existing.stream_id = $2
				  AND existing.status IN ('queued', 'running')
			)
		  )`, rekey.ID, rekey.CanonicalStreamID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() > 0 {
		return nil
	}
	_, err = s.db.Exec(ctx, `
		UPDATE backfill_jobs
		SET status='skipped',
		    export_status='skipped',
		    error='[alias cleanup] duplicate active canonical backfill job exists',
		    updated_at=now()
		WHERE id=$1
		  AND status IN ('queued', 'running')`, rekey.ID)
	return err
}

func (s *Store) purgeOrphanedAliasStream(ctx context.Context, aliasID, canonicalID string) error {
	if aliasID == "" || canonicalID == "" || aliasID == canonicalID {
		return nil
	}
	var rollupCount int
	if err := s.db.QueryRow(ctx, `
		SELECT COUNT(*) FROM analytics_minute_rollups WHERE stream_id = $1`, aliasID,
	).Scan(&rollupCount); err != nil {
		return err
	}
	if rollupCount == 0 {
		_, err := s.db.Exec(ctx, `
			DELETE FROM analytics_streams
			WHERE stream_id = $1 AND stream_id <> $2`, aliasID, canonicalID,
		)
		return err
	}
	return s.linkSessionAlias(ctx, aliasID, canonicalID)
}

func shouldMergeStubPair(a, b sessionCandidate) bool {
	if !sessionsMatch(a, b) {
		return false
	}
	if a.IsPlaceholder || b.IsPlaceholder {
		return true
	}
	return isPlaceholderStreamTitle(a.Title) || isPlaceholderStreamTitle(b.Title)
}

// CleanupPrefetchStubs merges overlapping prefetch stub rows into canonical sessions
// for the given logins (same hour-bucket rules as migration 000031).
func (s *Store) CleanupPrefetchStubs(ctx context.Context, logins []string) (merged int, err error) {
	return s.cleanupPrefetchStubs(ctx, logins, false)
}

func (s *Store) cleanupPrefetchStubs(ctx context.Context, logins []string, dryRun bool) (merged int, err error) {
	if s == nil || s.db == nil {
		return 0, fmt.Errorf("analytics store unavailable")
	}
	normalized := make([]string, 0, len(logins))
	seen := map[string]bool{}
	for _, login := range logins {
		login = normalizeLogin(login)
		if login == "" || seen[login] {
			continue
		}
		seen[login] = true
		normalized = append(normalized, login)
	}
	if len(normalized) == 0 {
		return 0, nil
	}

	rows, err := s.db.Query(ctx, `
		WITH ranked AS (
			SELECT
				s.stream_id,
				s.login,
				s.started_at,
				s.broadcaster_id,
				s.viewer_samples,
				s.chat_messages,
				s.peak_viewers,
				ROW_NUMBER() OVER (
					PARTITION BY s.login, date_trunc('hour', s.started_at)
					ORDER BY
						(CASE WHEN s.broadcaster_id = 'pending' THEN 0 ELSE 2 END)
						+ (CASE WHEN COALESCE(s.viewer_samples, 0) + COALESCE(s.chat_messages, 0) > 0 THEN 3 ELSE 0 END)
						+ (CASE WHEN COALESCE(s.peak_viewers, 0) > 0 THEN 1 ELSE 0 END) DESC,
						s.started_at ASC,
						s.stream_id ASC
				) AS rn
			FROM analytics_streams s
			WHERE s.login = ANY($1)
		),
		groups AS (
			SELECT login, date_trunc('hour', started_at) AS hour_bucket, MIN(stream_id) FILTER (WHERE rn = 1) AS canonical_id
			FROM ranked
			GROUP BY login, date_trunc('hour', started_at)
			HAVING COUNT(*) > 1
		)
		SELECT r.stream_id AS alias_id, g.canonical_id
		FROM ranked r
		JOIN groups g
		  ON r.login = g.login
		 AND date_trunc('hour', r.started_at) = g.hour_bucket
		WHERE r.stream_id <> g.canonical_id
		ORDER BY g.canonical_id, r.stream_id`, normalized)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	type mergePair struct {
		aliasID     string
		canonicalID string
	}
	var pairs []mergePair
	for rows.Next() {
		var pair mergePair
		if err := rows.Scan(&pair.aliasID, &pair.canonicalID); err != nil {
			return merged, err
		}
		pairs = append(pairs, pair)
	}
	if err := rows.Err(); err != nil {
		return merged, err
	}

	for _, pair := range pairs {
		if strings.TrimSpace(pair.aliasID) == "" || strings.TrimSpace(pair.canonicalID) == "" {
			continue
		}
		if dryRun {
			merged++
			continue
		}
		if err := s.linkSessionAlias(ctx, pair.aliasID, pair.canonicalID); err != nil {
			return merged, fmt.Errorf("merge %s -> %s: %w", pair.aliasID, pair.canonicalID, err)
		}
		merged++
	}
	return merged, nil
}
