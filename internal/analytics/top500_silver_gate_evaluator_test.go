package analytics

import (
	"testing"
	"time"
)

func TestEvaluateSilverGateAllowsValidCandidate(t *testing.T) {
	now := time.Now().UTC()
	got := EvaluateSilverGate(validSilverGateCandidate(now), validSilverBudgetSnapshot(), SilverGateConfig{})
	if !got.AllowEnqueue || got.Decision != SilverGateAllowEnqueue {
		t.Fatalf("got %+v, want allow_enqueue", got)
	}
}

func TestEvaluateSilverGateSkips(t *testing.T) {
	now := time.Now().UTC()
	base := validSilverGateCandidate(now)
	budget := validSilverBudgetSnapshot()

	cases := []struct {
		name string
		cand SilverGateCandidate
		bud  SilverBudgetSnapshot
		want SilverGateDecisionReason
	}{
		{"not_candidate_login", func() SilverGateCandidate { c := base; c.Login = ""; return c }(), budget, SilverGateSkipNotCandidate},
		{"not_candidate_channel", func() SilverGateCandidate { c := base; c.ChannelID = ""; return c }(), budget, SilverGateSkipNotCandidate},
		{"counter_unavailable", base, func() SilverBudgetSnapshot { b := budget; b.Available = false; return b }(), SilverGateSkipCounterUnavailable},
		{"counter_stale", base, func() SilverBudgetSnapshot { b := budget; b.Stale = true; return b }(), SilverGateSkipCounterStale},
		{"metadata_stale", func() SilverGateCandidate { c := base; c.StaleAfter = now.Add(-time.Hour); return c }(), budget, SilverGateSkipMetadataStale},
		{"missing_stream_id", func() SilverGateCandidate { c := base; c.StreamID = ""; return c }(), budget, SilverGateSkipMissingStreamID},
		{"already_done", base, func() SilverBudgetSnapshot { b := budget; b.AlreadyDone = true; return b }(), SilverGateSkipAlreadyDone},
		{"duplicate_job", base, func() SilverBudgetSnapshot { b := budget; b.DuplicateQueuedOrRunning = true; return b }(), SilverGateSkipDuplicateJob},
		{"channel_cooldown", base, func() SilverBudgetSnapshot { b := budget; b.InChannelCooldown = true; return b }(), SilverGateSkipChannelCooldown},
		{"recent_failure", base, func() SilverBudgetSnapshot { b := budget; b.InFailureBackoff = true; return b }(), SilverGateSkipRecentFailure},
		{"global_backoff", base, func() SilverBudgetSnapshot { b := budget; b.GlobalTTBackoffActive = true; return b }(), SilverGateSkipGlobalBackoff},
		{"daily_budget", base, func() SilverBudgetSnapshot { b := budget; b.SilverEnqueuedToday = SilverGateGlobalMaxEnqueuePerDay; return b }(), SilverGateSkipDailyBudget},
		{"running_limit", base, func() SilverBudgetSnapshot { b := budget; b.SilverRunningNow = SilverGateGlobalMaxRunning; return b }(), SilverGateSkipRunningLimit},
		{"queue_full", base, func() SilverBudgetSnapshot { b := budget; b.SilverQueueDepth = SilverGateGlobalMaxQueueDepth; return b }(), SilverGateSkipQueueFull},
		{"disk_guard", base, func() SilverBudgetSnapshot { b := budget; b.DiskGuardActive = true; return b }(), SilverGateSkipDiskGuard},
		{"backup_guard", base, func() SilverBudgetSnapshot { b := budget; b.BackupGuardActive = true; return b }(), SilverGateSkipBackupGuard},
		{"archive_guard", base, func() SilverBudgetSnapshot { b := budget; b.ArchiveGuardActive = true; return b }(), SilverGateSkipArchiveGuard},
		{"alerting_guard", base, func() SilverBudgetSnapshot { b := budget; b.AlertingGuardActive = true; return b }(), SilverGateSkipAlertingGuard},
		{"corpus_unhealthy", base, func() SilverBudgetSnapshot { b := budget; b.CorpusUnhealthy = true; return b }(), SilverGateSkipCorpusUnhealthy},
		{"hosted_unhealthy", base, func() SilverBudgetSnapshot { b := budget; b.HostedUnhealthy = true; return b }(), SilverGateSkipHostedUnhealthy},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := EvaluateSilverGate(tc.cand, tc.bud, SilverGateConfig{})
			if got.AllowEnqueue || got.Decision != tc.want {
				t.Fatalf("got %+v, want deny %s", got, tc.want)
			}
		})
	}
}

func TestEvaluateSilverGateIsPure(t *testing.T) {
	now := time.Now().UTC()
	cand := validSilverGateCandidate(now)
	budget := validSilverBudgetSnapshot()
	cfg := SilverGateConfig{}
	first := EvaluateSilverGate(cand, budget, cfg)
	second := EvaluateSilverGate(cand, budget, cfg)
	if first != second {
		t.Fatalf("evaluator not pure: first=%+v second=%+v", first, second)
	}
}

func TestRefusingSilverEnqueueAdapterRejectsWrite(t *testing.T) {
	adapter := RefusingSilverEnqueueAdapter{WriteEnabled: false}
	inserted, err := adapter.EnqueueSilver(t.Context(), SilverEnqueueRequest{Tier: "silver", Login: "shroud", StreamID: "1"})
	if inserted || err == nil {
		t.Fatalf("inserted=%v err=%v, want false and error", inserted, err)
	}
}

func validSilverGateCandidate(now time.Time) SilverGateCandidate {
	return SilverGateCandidate{
		CandidateID: "c1",
		Login:       "shroud",
		ChannelID:   "123",
		StreamID:    "999",
		Rank:        1,
		SampledAt:   now.Add(-time.Minute),
		StaleAfter:  now.Add(time.Hour),
	}
}

func validSilverBudgetSnapshot() SilverBudgetSnapshot {
	return SilverBudgetSnapshot{Available: true}
}
