package store

import (
	"context"
	"testing"
	"time"
)

func TestRisingScoreComposition(t *testing.T) {
	ctx, st := setupStatsStoreTest(t)
	runID := "11111111-1111-1111-1111-111111111111"
	now := time.Now().UTC().Truncate(time.Minute)
	samples := []DirectorySample{
		{TwitchLogin: "alpha", DisplayName: "Alpha", Viewers: 1000, Rank: 1, IsLive: true, SampleRunID: runID, SampledAt: now},
		{TwitchLogin: "beta", DisplayName: "Beta", Viewers: 500, Rank: 2, IsLive: true, SampleRunID: runID, SampledAt: now},
	}
	if err := st.InsertDirectorySamples(ctx, samples); err != nil {
		t.Fatalf("InsertDirectorySamples: %v", err)
	}
	rows := []RisingRow{{
		TwitchLogin: "alpha", Window: "today", ViewersNow: 1000, ViewersPrev: 500,
		ViewerDeltaPct: 100, RankNow: 1, RankPrev: 3, RankDelta: 2, RisingScore: 80, ComputedAt: now,
	}}
	if err := st.UpsertRisingRows(ctx, rows); err != nil {
		t.Fatalf("UpsertRisingRows: %v", err)
	}
	got, err := st.RisingCandidates(ctx, "today", "", 5)
	if err != nil {
		t.Fatalf("RisingCandidates: %v", err)
	}
	if len(got) != 1 || got[0].Login != "alpha" {
		t.Fatalf("RisingCandidates = %+v", got)
	}
	series, err := st.ViewerSeriesForLogin(ctx, "alpha", now.Add(-24*time.Hour), 10)
	if err != nil {
		t.Fatalf("ViewerSeriesForLogin: %v", err)
	}
	if len(series) != 1 || series[0].Viewers != 1000 {
		t.Fatalf("ViewerSeriesForLogin = %+v", series)
	}
	n, err := st.DeleteExpiredDirectorySamples(ctx, 30)
	if err != nil {
		t.Fatalf("DeleteExpiredDirectorySamples: %v", err)
	}
	if n != 0 {
		t.Fatalf("expected 0 deleted fresh rows, got %d", n)
	}
}

func setupStatsStoreTest(t *testing.T) (context.Context, *Store) {
	t.Helper()
	return setupPreviewStoreTest(t)
}
