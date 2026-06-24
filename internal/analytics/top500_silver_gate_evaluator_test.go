package analytics

import (
	"errors"
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
	if !errors.Is(err, ErrSilverGateWriteDisabled) {
		t.Fatalf("err = %v, want ErrSilverGateWriteDisabled", err)
	}
}

func TestEvaluateSilverGateOrderingEarlierFailureWins(t *testing.T) {
	now := time.Now().UTC()
	base := validSilverGateCandidate(now)
	budget := validSilverBudgetSnapshot()
	budget.SilverEnqueuedToday = SilverGateGlobalMaxEnqueuePerDay

	cases := []struct {
		name string
		cand SilverGateCandidate
		bud  SilverBudgetSnapshot
		want SilverGateDecisionReason
	}{
		{
			name: "shape_before_daily_budget",
			cand: func() SilverGateCandidate { c := base; c.Login = ""; return c }(),
			bud:  budget,
			want: SilverGateSkipNotCandidate,
		},
		{
			name: "counter_unavailable_before_daily_budget",
			cand: base,
			bud: func() SilverBudgetSnapshot {
				b := budget
				b.Available = false
				return b
			}(),
			want: SilverGateSkipCounterUnavailable,
		},
		{
			name: "metadata_stale_before_missing_stream_id",
			cand: func() SilverGateCandidate {
				c := base
				c.StaleAfter = now.Add(-time.Hour)
				c.StreamID = ""
				return c
			}(),
			bud:  budget,
			want: SilverGateSkipMetadataStale,
		},
		{
			name: "already_done_before_duplicate",
			cand: base,
			bud: func() SilverBudgetSnapshot {
				b := budget
				b.AlreadyDone = true
				b.DuplicateQueuedOrRunning = true
				return b
			}(),
			want: SilverGateSkipAlreadyDone,
		},
		{
			name: "duplicate_before_daily_budget",
			cand: base,
			bud: func() SilverBudgetSnapshot {
				b := validSilverBudgetSnapshot()
				b.DuplicateQueuedOrRunning = true
				b.SilverEnqueuedToday = SilverGateGlobalMaxEnqueuePerDay
				return b
			}(),
			want: SilverGateSkipDuplicateJob,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := EvaluateSilverGate(tc.cand, tc.bud, SilverGateConfig{})
			if got.Decision != tc.want {
				t.Fatalf("decision = %q, want %q", got.Decision, tc.want)
			}
		})
	}
}

func TestEvaluateSilverGateEdgeCases(t *testing.T) {
	now := time.Now().UTC()

	t.Run("zero_sampled_at_is_stale", func(t *testing.T) {
		c := validSilverGateCandidate(now)
		c.SampledAt = time.Time{}
		got := EvaluateSilverGate(c, validSilverBudgetSnapshot(), SilverGateConfig{})
		if got.Decision != SilverGateSkipMetadataStale {
			t.Fatalf("decision = %q, want skip_metadata_stale", got.Decision)
		}
	})

	t.Run("empty_candidate_login_and_channel", func(t *testing.T) {
		got := EvaluateSilverGate(SilverGateCandidate{}, validSilverBudgetSnapshot(), SilverGateConfig{})
		if got.Decision != SilverGateSkipNotCandidate {
			t.Fatalf("decision = %q, want skip_not_candidate", got.Decision)
		}
	})

	t.Run("all_guards_healthy_allows", func(t *testing.T) {
		got := EvaluateSilverGate(validSilverGateCandidate(now), validSilverBudgetSnapshot(), SilverGateConfig{})
		if !got.AllowEnqueue {
			t.Fatalf("got %+v, want allow", got)
		}
	})
}

func TestEvaluateSilverGateBudgetBoundaries(t *testing.T) {
	now := time.Now().UTC()
	base := validSilverGateCandidate(now)

	t.Run("daily_budget_under_limit_allows", func(t *testing.T) {
		b := validSilverBudgetSnapshot()
		b.SilverEnqueuedToday = SilverGateGlobalMaxEnqueuePerDay - 1
		got := EvaluateSilverGate(base, b, SilverGateConfig{})
		if !got.AllowEnqueue {
			t.Fatalf("got %+v, want allow under daily budget", got)
		}
	})

	t.Run("daily_budget_at_limit_denies", func(t *testing.T) {
		b := validSilverBudgetSnapshot()
		b.SilverEnqueuedToday = SilverGateGlobalMaxEnqueuePerDay
		got := EvaluateSilverGate(base, b, SilverGateConfig{})
		if got.Decision != SilverGateSkipDailyBudget {
			t.Fatalf("got %+v, want skip_daily_budget", got)
		}
	})

	t.Run("running_under_limit_allows", func(t *testing.T) {
		b := validSilverBudgetSnapshot()
		b.SilverRunningNow = SilverGateGlobalMaxRunning - 1
		got := EvaluateSilverGate(base, b, SilverGateConfig{})
		if !got.AllowEnqueue {
			t.Fatalf("got %+v, want allow under running limit", got)
		}
	})

	t.Run("running_at_limit_denies", func(t *testing.T) {
		b := validSilverBudgetSnapshot()
		b.SilverRunningNow = SilverGateGlobalMaxRunning
		got := EvaluateSilverGate(base, b, SilverGateConfig{})
		if got.Decision != SilverGateSkipRunningLimit {
			t.Fatalf("got %+v, want skip_running_limit", got)
		}
	})

	t.Run("queue_depth_under_limit_allows", func(t *testing.T) {
		b := validSilverBudgetSnapshot()
		b.SilverQueueDepth = SilverGateGlobalMaxQueueDepth - 1
		got := EvaluateSilverGate(base, b, SilverGateConfig{})
		if !got.AllowEnqueue {
			t.Fatalf("got %+v, want allow under queue depth", got)
		}
	})

	t.Run("queue_depth_at_limit_denies", func(t *testing.T) {
		b := validSilverBudgetSnapshot()
		b.SilverQueueDepth = SilverGateGlobalMaxQueueDepth
		got := EvaluateSilverGate(base, b, SilverGateConfig{})
		if got.Decision != SilverGateSkipQueueFull {
			t.Fatalf("got %+v, want skip_queue_full", got)
		}
	})
}

func TestEvaluateSilverGateCoversAllDecisionReasons(t *testing.T) {
	all := []SilverGateDecisionReason{
		SilverGateAllowEnqueue,
		SilverGateSkipNotCandidate,
		SilverGateSkipMetadataStale,
		SilverGateSkipMissingStreamID,
		SilverGateSkipAlreadyDone,
		SilverGateSkipDuplicateJob,
		SilverGateSkipChannelCooldown,
		SilverGateSkipRecentFailure,
		SilverGateSkipGlobalBackoff,
		SilverGateSkipDailyBudget,
		SilverGateSkipRunningLimit,
		SilverGateSkipQueueFull,
		SilverGateSkipDiskGuard,
		SilverGateSkipBackupGuard,
		SilverGateSkipArchiveGuard,
		SilverGateSkipAlertingGuard,
		SilverGateSkipCorpusUnhealthy,
		SilverGateSkipHostedUnhealthy,
		SilverGateSkipCounterUnavailable,
		SilverGateSkipCounterStale,
	}
	seen := map[SilverGateDecisionReason]bool{}
	for _, reason := range all {
		seen[reason] = true
	}
	if len(seen) != len(all) {
		t.Fatal("duplicate decision reason constants")
	}
	// exercised by TestEvaluateSilverGateSkips + allow test
	if len(all) != 20 {
		t.Fatalf("decision reason count = %d, want 20", len(all))
	}
}

func TestRecordSilverGateDecisionAllowPath(t *testing.T) {
	RecordSilverGateDecision(SilverGateResult{
		Decision:     SilverGateAllowEnqueue,
		AllowEnqueue: true,
	}, SilverGateLaneTop500Selective, "evaluate")
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
