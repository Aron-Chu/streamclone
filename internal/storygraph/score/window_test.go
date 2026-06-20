package score

import (
	"testing"
	"time"
)

func TestComputeWindowScoreTodayPrefersFreshBreaking(t *testing.T) {
	now := time.Date(2026, 6, 17, 12, 0, 0, 0, time.UTC)
	since := time.Date(2026, 6, 17, 0, 0, 0, 0, time.UTC)
	trend := 80.0

	freshBan := ComputeWindowScore(WindowScoreInput{
		Window: "today", Since: since, Now: now,
		EvidenceCount: 3, SourceCount: 2, WeightedSum: 2.1,
		LatestAt: now.Add(-30 * time.Minute), Category: "bans",
		HasReddit: true, Trend: &trend,
	})
	staleClip := ComputeWindowScore(WindowScoreInput{
		Window: "today", Since: since, Now: now,
		EvidenceCount: 1, SourceCount: 1, WeightedSum: 0.9,
		LatestAt: now.Add(-10 * time.Hour), Category: "funny",
		OnlyTwitch: true,
	})
	if freshBan.RankScore <= staleClip.RankScore {
		t.Fatalf("today breaking corroborated story should outrank stale twitch-only clip: fresh=%v stale=%v", freshBan.RankScore, staleClip.RankScore)
	}
}

func TestComputeWindowScore7dPrefersSustainedImpact(t *testing.T) {
	now := time.Date(2026, 6, 17, 12, 0, 0, 0, time.UTC)
	since := now.Add(-7 * 24 * time.Hour)

	sustained := ComputeWindowScore(WindowScoreInput{
		Window: "7d", Since: since, Now: now,
		EvidenceCount: 8, SourceCount: 4, WeightedSum: 5.5,
		LatestAt: now.Add(-36 * time.Hour), Category: "drama",
		HasReddit: true, HasStreamerBans: true,
	})
	spike := ComputeWindowScore(WindowScoreInput{
		Window: "7d", Since: since, Now: now,
		EvidenceCount: 2, SourceCount: 1, WeightedSum: 1.4,
		LatestAt: now.Add(-20 * time.Minute), Category: "funny",
		OnlyTwitch: true,
	})
	if sustained.RankScore <= spike.RankScore {
		t.Fatalf("7d should favor sustained multi-source impact: sustained=%v spike=%v", sustained.RankScore, spike.RankScore)
	}
}

func TestComputeWindowScoreWindowsDiverge(t *testing.T) {
	now := time.Date(2026, 6, 17, 15, 0, 0, 0, time.UTC)
	latest := now.Add(-2 * time.Hour)
	todaySince := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	today := ComputeWindowScore(WindowScoreInput{
		Window: "today", Since: todaySince, Now: now,
		EvidenceCount: 4, SourceCount: 2, WeightedSum: 2.8,
		LatestAt: latest, Category: "drama", HasReddit: true,
	})
	hday := ComputeWindowScore(WindowScoreInput{
		Window: "24h", Since: now.Add(-24 * time.Hour), Now: now,
		EvidenceCount: 4, SourceCount: 2, WeightedSum: 2.8,
		LatestAt: latest, Category: "drama", HasReddit: true,
	})
	base := WindowScoreInput{
		Window: "7d", Since: now.Add(-7 * 24 * time.Hour), Now: now,
		EvidenceCount: 4, SourceCount: 2, WeightedSum: 2.8,
		LatestAt: latest, Category: "drama", HasReddit: true,
	}
	week := ComputeWindowScore(base)
	if today.RankScore == hday.RankScore || hday.RankScore == week.RankScore {
		t.Fatalf("windows should produce different rank scores: today=%v 24h=%v 7d=%v", today.RankScore, hday.RankScore, week.RankScore)
	}
}

func TestComputeWindowScoreRewardsSourceDiversity(t *testing.T) {
	now := time.Date(2026, 6, 17, 18, 0, 0, 0, time.UTC)
	base := WindowScoreInput{
		Window: "24h", Since: now.Add(-24 * time.Hour), Now: now,
		EvidenceCount: 3, WeightedSum: 2.1,
		LatestAt: now.Add(-2 * time.Hour),
	}

	single := ComputeWindowScore(base)
	diverse := base
	diverse.SourceCount = 3
	got := ComputeWindowScore(diverse)

	if got.CredibilityScore <= single.CredibilityScore {
		t.Fatalf("source diversity should increase credibility: single=%.1f diverse=%.1f", single.CredibilityScore, got.CredibilityScore)
	}
	if got.RankScore <= single.RankScore {
		t.Fatalf("source diversity should improve rank: single=%.1f diverse=%.1f", single.RankScore, got.RankScore)
	}
}
