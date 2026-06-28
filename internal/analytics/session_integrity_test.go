package analytics

import (
	"testing"
)

func TestAuditSessionIntegrityDetectsMissingSessionRow(t *testing.T) {
	ctx, store := setupSessionStore(t)
	insertTestStream(t, ctx, store, "canonical", "chan", 5)
	mustExec(t, ctx, store, `DELETE FROM analytics_stream_sessions WHERE canonical_stream_id='canonical'`)

	report, err := store.AuditSessionIntegrity(ctx, []string{"chan"})
	if err != nil {
		t.Fatalf("AuditSessionIntegrity: %v", err)
	}
	if len(report.StreamMissingSession) != 1 {
		t.Fatalf("stream missing session = %#v, want one row", report.StreamMissingSession)
	}
}

func TestEnsureSessionForStreamCreatesMissingRow(t *testing.T) {
	ctx, store := setupSessionStore(t)
	insertTestStream(t, ctx, store, "gold-target", "chan", 12)
	mustExec(t, ctx, store, `DELETE FROM analytics_stream_sessions WHERE canonical_stream_id='gold-target'`)

	if err := store.EnsureSessionForStream(ctx, "gold-target"); err != nil {
		t.Fatalf("EnsureSessionForStream: %v", err)
	}
	var count int
	if err := store.db.QueryRow(ctx, `SELECT COUNT(*) FROM analytics_stream_sessions WHERE canonical_stream_id='gold-target'`).Scan(&count); err != nil {
		t.Fatalf("count session: %v", err)
	}
	if count != 1 {
		t.Fatalf("session count = %d, want 1", count)
	}
}

func TestRepairMissingSessionRowsCreatesSession(t *testing.T) {
	ctx, store := setupSessionStore(t)
	insertTestStream(t, ctx, store, "canonical", "chan", 5)
	mustExec(t, ctx, store, `DELETE FROM analytics_stream_sessions WHERE canonical_stream_id='canonical'`)

	report, err := store.RepairMissingSessionRows(ctx, []string{"chan"}, false)
	if err != nil {
		t.Fatalf("RepairMissingSessionRows: %v", err)
	}
	if report.SessionsRepaired != 1 {
		t.Fatalf("sessions repaired = %d, want 1", report.SessionsRepaired)
	}
	var sessionCount int
	if err := store.db.QueryRow(ctx, `SELECT COUNT(*) FROM analytics_stream_sessions WHERE canonical_stream_id='canonical'`).Scan(&sessionCount); err != nil {
		t.Fatalf("count sessions: %v", err)
	}
	if sessionCount != 1 {
		t.Fatalf("session count = %d, want 1", sessionCount)
	}
}

func TestLinkSessionAliasCreatesMissingCanonicalSession(t *testing.T) {
	ctx, store := setupSessionStore(t)
	mustExec(t, ctx, store, `
		INSERT INTO analytics_streams (
			stream_id, canonical_stream_id, broadcaster_id, login, started_at, title
		) VALUES ('alias-tt', 'alias-tt', 'pending', 'chan', now(), 'Syncing...')`)
	mustExec(t, ctx, store, `
		INSERT INTO analytics_streams (
			stream_id, canonical_stream_id, broadcaster_id, login, started_at, title, peak_viewers
		) VALUES ('helix-id', 'helix-id', 'bc-helix', 'chan', now(), 'Live title', 1000)`)
	mustExec(t, ctx, store, `DELETE FROM analytics_stream_sessions WHERE canonical_stream_id='helix-id'`)

	if err := store.linkSessionAlias(ctx, "alias-tt", "helix-id"); err != nil {
		t.Fatalf("linkSessionAlias: %v", err)
	}
	var sessionCount int
	if err := store.db.QueryRow(ctx, `SELECT COUNT(*) FROM analytics_stream_sessions WHERE canonical_stream_id='helix-id'`).Scan(&sessionCount); err != nil {
		t.Fatalf("count sessions: %v", err)
	}
	if sessionCount != 1 {
		t.Fatalf("session count = %d, want 1", sessionCount)
	}
}

func TestCleanupSessionStubsRepairsMissingSessions(t *testing.T) {
	ctx, store := setupSessionStore(t)
	insertTestStream(t, ctx, store, "canonical", "chan", 5)
	mustExec(t, ctx, store, `DELETE FROM analytics_stream_sessions WHERE canonical_stream_id='canonical'`)

	report, err := store.CleanupSessionStubs(ctx, []string{"chan"})
	if err != nil {
		t.Fatalf("CleanupSessionStubs: %v", err)
	}
	if report.SessionIntegrity.SessionsRepaired != 1 {
		t.Fatalf("sessions repaired = %d, want 1", report.SessionIntegrity.SessionsRepaired)
	}
}
