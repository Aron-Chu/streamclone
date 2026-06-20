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
	OrphanAliasesMerged []string `json:"orphanAliasesMerged"`
	StubSessionsMerged  int      `json:"stubSessionsMerged"`
	Errors              []string `json:"errors,omitempty"`
}

type orphanedSessionAlias struct {
	AliasStreamID     string
	CanonicalStreamID string
	Login             string
}

// CleanupSessionStubs removes orphaned alias rows and merges overlapping prefetch
// stubs for the given logins using canonical session rules.
func (s *Store) CleanupSessionStubs(ctx context.Context, logins []string) (SessionCleanupReport, error) {
	report := SessionCleanupReport{Logins: normalizeLoginList(logins)}
	if s == nil || s.db == nil {
		return report, fmt.Errorf("analytics store unavailable")
	}
	if len(report.Logins) == 0 {
		return report, nil
	}

	orphans, err := s.listOrphanedSessionAliases(ctx, report.Logins)
	if err != nil {
		return report, err
	}
	for _, row := range orphans {
		if err := s.purgeOrphanedAliasStream(ctx, row.AliasStreamID, row.CanonicalStreamID); err != nil {
			report.Errors = append(report.Errors, fmt.Sprintf(
				"merge orphan alias %s -> %s (%s): %v",
				row.AliasStreamID, row.CanonicalStreamID, row.Login, err,
			))
			continue
		}
		report.OrphanAliasesMerged = append(report.OrphanAliasesMerged, row.AliasStreamID)
	}

	merged, err := s.CleanupPrefetchStubs(ctx, report.Logins)
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
		if err := s.linkSessionAlias(ctx, pair.aliasID, pair.canonicalID); err != nil {
			return merged, fmt.Errorf("merge %s -> %s: %w", pair.aliasID, pair.canonicalID, err)
		}
		merged++
	}
	return merged, nil
}
