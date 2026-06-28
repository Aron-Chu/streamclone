package analytics

import (
	"context"
	"fmt"
	"strings"
)

// SessionIntegrityIssue describes one alias/session consistency problem.
type SessionIntegrityIssue struct {
	Kind              string `json:"kind"`
	Login             string `json:"login,omitempty"`
	StreamID          string `json:"streamId,omitempty"`
	CanonicalStreamID string `json:"canonicalStreamId,omitempty"`
	AliasStreamID     string `json:"aliasStreamId,omitempty"`
}

// SessionIntegrityReport summarizes session alias / canonical session row health.
type SessionIntegrityReport struct {
	Logins                        []string                `json:"logins,omitempty"`
	AliasMissingSession           []SessionIntegrityIssue `json:"aliasMissingSession,omitempty"`
	StreamMissingSession          []SessionIntegrityIssue `json:"streamMissingSession,omitempty"`
	ColumnCanonicalMissingSession []SessionIntegrityIssue `json:"columnCanonicalMissingSession,omitempty"`
	SessionsRepaired              int                     `json:"sessionsRepaired,omitempty"`
	DryRun                        bool                    `json:"dryRun,omitempty"`
}

// AuditSessionIntegrity returns read-only alias/session consistency issues.
func (s *Store) AuditSessionIntegrity(ctx context.Context, logins []string) (SessionIntegrityReport, error) {
	report := SessionIntegrityReport{Logins: normalizeLoginList(logins)}
	if s == nil || s.db == nil {
		return report, fmt.Errorf("analytics store unavailable")
	}
	if len(report.Logins) == 0 {
		return report, nil
	}

	aliasRows, err := s.db.Query(ctx, `
		SELECT
			a.alias_stream_id,
			a.canonical_stream_id,
			COALESCE(sa.login, sc.login, '')
		FROM analytics_stream_aliases a
		LEFT JOIN analytics_stream_sessions sess
			ON sess.canonical_stream_id = a.canonical_stream_id
		LEFT JOIN analytics_streams sa ON sa.stream_id = a.alias_stream_id
		LEFT JOIN analytics_streams sc ON sc.stream_id = a.canonical_stream_id
		WHERE sess.canonical_stream_id IS NULL
		  AND (sa.login = ANY($1) OR sc.login = ANY($1))
		ORDER BY COALESCE(sa.login, sc.login, ''), a.alias_stream_id
		LIMIT 500`, report.Logins)
	if err != nil {
		return report, err
	}
	defer aliasRows.Close()
	for aliasRows.Next() {
		var issue SessionIntegrityIssue
		if err := aliasRows.Scan(&issue.AliasStreamID, &issue.CanonicalStreamID, &issue.Login); err != nil {
			return report, err
		}
		issue.Kind = "alias_missing_session"
		report.AliasMissingSession = append(report.AliasMissingSession, issue)
	}
	if err := aliasRows.Err(); err != nil {
		return report, err
	}

	streamRows, err := s.db.Query(ctx, `
		SELECT
			st.stream_id,
			COALESCE(NULLIF(st.canonical_stream_id, ''), st.stream_id),
			st.login
		FROM analytics_streams st
		LEFT JOIN analytics_stream_sessions sess
			ON sess.canonical_stream_id = COALESCE(NULLIF(st.canonical_stream_id, ''), st.stream_id)
		WHERE st.login = ANY($1)
		  AND sess.canonical_stream_id IS NULL
		ORDER BY st.login, st.started_at ASC
		LIMIT 500`, report.Logins)
	if err != nil {
		return report, err
	}
	defer streamRows.Close()
	for streamRows.Next() {
		var issue SessionIntegrityIssue
		if err := streamRows.Scan(&issue.StreamID, &issue.CanonicalStreamID, &issue.Login); err != nil {
			return report, err
		}
		issue.Kind = "stream_missing_session"
		report.StreamMissingSession = append(report.StreamMissingSession, issue)
	}
	if err := streamRows.Err(); err != nil {
		return report, err
	}

	columnRows, err := s.db.Query(ctx, `
		SELECT
			st.stream_id,
			st.canonical_stream_id,
			st.login
		FROM analytics_streams st
		LEFT JOIN analytics_stream_sessions sess
			ON sess.canonical_stream_id = st.canonical_stream_id
		WHERE st.login = ANY($1)
		  AND COALESCE(st.canonical_stream_id, '') <> ''
		  AND st.stream_id <> st.canonical_stream_id
		  AND sess.canonical_stream_id IS NULL
		ORDER BY st.login, st.started_at ASC
		LIMIT 500`, report.Logins)
	if err != nil {
		return report, err
	}
	defer columnRows.Close()
	for columnRows.Next() {
		var issue SessionIntegrityIssue
		if err := columnRows.Scan(&issue.StreamID, &issue.CanonicalStreamID, &issue.Login); err != nil {
			return report, err
		}
		issue.Kind = "column_canonical_missing_session"
		report.ColumnCanonicalMissingSession = append(report.ColumnCanonicalMissingSession, issue)
	}
	return report, columnRows.Err()
}

// EnsureSessionForStream creates analytics_stream_sessions row for a stream id when missing.
func (s *Store) EnsureSessionForStream(ctx context.Context, streamID string) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("analytics store unavailable")
	}
	streamID = strings.TrimSpace(streamID)
	if streamID == "" {
		return nil
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err := s.ensureSessionForStreamTx(ctx, tx, streamID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// RepairMissingSessionRows ensures analytics_stream_sessions rows exist for canonical targets.
func (s *Store) RepairMissingSessionRows(ctx context.Context, logins []string, dryRun bool) (SessionIntegrityReport, error) {
	audit, err := s.AuditSessionIntegrity(ctx, logins)
	if err != nil {
		return audit, err
	}
	audit.DryRun = dryRun
	if dryRun || s == nil || s.db == nil {
		return audit, nil
	}

	targets := make([]string, 0, len(audit.AliasMissingSession)+len(audit.StreamMissingSession)+len(audit.ColumnCanonicalMissingSession))
	appendTarget := func(id string) {
		id = strings.TrimSpace(id)
		if id == "" {
			return
		}
		targets = append(targets, id)
	}
	for _, issue := range audit.AliasMissingSession {
		appendTarget(issue.CanonicalStreamID)
	}
	for _, issue := range audit.StreamMissingSession {
		appendTarget(issue.CanonicalStreamID)
		if issue.CanonicalStreamID == "" {
			appendTarget(issue.StreamID)
		}
	}
	for _, issue := range audit.ColumnCanonicalMissingSession {
		appendTarget(issue.CanonicalStreamID)
	}
	for _, targetID := range uniqueStrings(targets) {
		tx, err := s.db.Begin(ctx)
		if err != nil {
			return audit, err
		}
		if err := s.ensureSessionForStreamTx(ctx, tx, targetID); err != nil {
			_ = tx.Rollback(ctx)
			return audit, err
		}
		if err := tx.Commit(ctx); err != nil {
			return audit, err
		}
		audit.SessionsRepaired++
	}
	return audit, nil
}
