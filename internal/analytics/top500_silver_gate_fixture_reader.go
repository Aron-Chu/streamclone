package analytics

import (
	"context"
	"time"
)

const (
	fixtureSilverAllowLogin       = "shroud"
	fixtureSilverStaleLogin       = "stale_meta"
	fixtureSilverNoStreamLogin    = "no_stream"
	fixtureSilverDuplicateLogin   = "dup_skip"
	fixtureSilverBudgetLogin      = "budget_skip"
)

// FixtureSilverCandidateReader returns deterministic local-only candidates for LOAD-003c dry-run.
type FixtureSilverCandidateReader struct{}

func NewFixtureSilverCandidateReader() *FixtureSilverCandidateReader {
	return &FixtureSilverCandidateReader{}
}

func (r *FixtureSilverCandidateReader) ListCandidates(_ context.Context, limit int) ([]SilverGateCandidate, error) {
	if r == nil {
		return nil, nil
	}
	now := time.Now().UTC()
	all := []SilverGateCandidate{
		fixtureAllowCandidate(now),
		fixtureStaleCandidate(now),
		fixtureMissingStreamCandidate(now),
		fixtureDuplicateSkipCandidate(now),
		fixtureBudgetSkipCandidate(now),
	}
	if limit <= 0 || limit >= len(all) {
		return append([]SilverGateCandidate(nil), all...), nil
	}
	return append([]SilverGateCandidate(nil), all[:limit]...), nil
}

func fixtureAllowCandidate(now time.Time) SilverGateCandidate {
	return SilverGateCandidate{
		CandidateID:     "fixture-allow",
		Login:           fixtureSilverAllowLogin,
		ChannelID:       "141981764",
		StreamID:        "fixture-stream-live",
		Rank:            1,
		ViewerCount:     12500,
		IsLive:          true,
		SampledAt:       now,
		StaleAfter:      now.Add(15 * time.Minute),
		CandidateSource: "top500_fixture",
		PriorityScore:   100,
	}
}

func fixtureStaleCandidate(now time.Time) SilverGateCandidate {
	return SilverGateCandidate{
		CandidateID:     "fixture-stale",
		Login:           fixtureSilverStaleLogin,
		ChannelID:       "200001",
		StreamID:        "fixture-stream-stale",
		Rank:            2,
		SampledAt:       now.Add(-2 * time.Hour),
		StaleAfter:      now.Add(-time.Hour),
		CandidateSource: "top500_fixture",
	}
}

func fixtureMissingStreamCandidate(now time.Time) SilverGateCandidate {
	return SilverGateCandidate{
		CandidateID:     "fixture-no-stream",
		Login:           fixtureSilverNoStreamLogin,
		ChannelID:       "200002",
		StreamID:        "",
		Rank:            3,
		SampledAt:       now,
		StaleAfter:      now.Add(15 * time.Minute),
		CandidateSource: "top500_fixture",
	}
}

func fixtureDuplicateSkipCandidate(now time.Time) SilverGateCandidate {
	return SilverGateCandidate{
		CandidateID:     "fixture-dup",
		Login:           fixtureSilverDuplicateLogin,
		ChannelID:       "200003",
		StreamID:        "fixture-stream-dup",
		Rank:            4,
		SampledAt:       now,
		StaleAfter:      now.Add(15 * time.Minute),
		CandidateSource: "top500_fixture",
	}
}

func fixtureBudgetSkipCandidate(now time.Time) SilverGateCandidate {
	return SilverGateCandidate{
		CandidateID:     "fixture-budget",
		Login:           fixtureSilverBudgetLogin,
		ChannelID:       "200004",
		StreamID:        "fixture-stream-budget",
		Rank:            5,
		SampledAt:       now,
		StaleAfter:      now.Add(15 * time.Minute),
		CandidateSource: "top500_fixture",
	}
}

// FixtureSilverBudgetCounterReader returns per-login budget snapshots for fixture dry-run.
type FixtureSilverBudgetCounterReader struct{}

func NewFixtureSilverBudgetCounterReader() *FixtureSilverBudgetCounterReader {
	return &FixtureSilverBudgetCounterReader{}
}

func (r *FixtureSilverBudgetCounterReader) ReadSnapshot(_ context.Context, login string) (SilverBudgetSnapshot, error) {
	if r == nil {
		return SilverBudgetSnapshot{Available: false}, nil
	}
	login = normalizeLogin(login)
	switch login {
	case fixtureSilverDuplicateLogin:
		return SilverBudgetSnapshot{Available: true, DuplicateQueuedOrRunning: true}, nil
	case fixtureSilverBudgetLogin:
		return SilverBudgetSnapshot{
			Available:           true,
			SilverEnqueuedToday: SilverGateGlobalMaxEnqueuePerDay,
		}, nil
	default:
		return healthySilverBudgetSnapshot(), nil
	}
}

// HealthySilverBudgetCounterReader is an in-memory healthy snapshot for local read-only dry-run.
type HealthySilverBudgetCounterReader struct{}

func (HealthySilverBudgetCounterReader) ReadSnapshot(context.Context, string) (SilverBudgetSnapshot, error) {
	return healthySilverBudgetSnapshot(), nil
}

func healthySilverBudgetSnapshot() SilverBudgetSnapshot {
	return SilverBudgetSnapshot{
		Available:           true,
		SilverEnqueuedToday: 0,
		SilverRunningNow:    0,
		SilverQueueDepth:    0,
	}
}
