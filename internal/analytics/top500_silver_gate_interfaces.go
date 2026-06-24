package analytics

import (
	"context"
	"errors"
)

var ErrSilverGateWriteDisabled = errors.New("top500 silver gate write path disabled")

// SilverCandidateReader loads Top500 metadata candidates for gate evaluation.
type SilverCandidateReader interface {
	ListCandidates(ctx context.Context, limit int) ([]SilverGateCandidate, error)
}

// SilverBudgetCounterReader loads budget and guard counters for gate evaluation.
type SilverBudgetCounterReader interface {
	ReadSnapshot(ctx context.Context, login string) (SilverBudgetSnapshot, error)
}

// SilverEnqueueAdapter inserts silver-tier backfill jobs when write-enabled.
type SilverEnqueueAdapter interface {
	EnqueueSilver(ctx context.Context, req SilverEnqueueRequest) (inserted bool, err error)
}

// NoopSilverCandidateReader is a stub reader for LOAD-003a scaffold.
type NoopSilverCandidateReader struct{}

func (NoopSilverCandidateReader) ListCandidates(context.Context, int) ([]SilverGateCandidate, error) {
	return nil, nil
}

// NoopSilverBudgetCounterReader is a stub counter reader for LOAD-003a scaffold.
type NoopSilverBudgetCounterReader struct{}

func (NoopSilverBudgetCounterReader) ReadSnapshot(context.Context, string) (SilverBudgetSnapshot, error) {
	return SilverBudgetSnapshot{Available: false}, nil
}

// RefusingSilverEnqueueAdapter rejects enqueue unless write-enabled is true.
type RefusingSilverEnqueueAdapter struct {
	WriteEnabled bool
}

func (a RefusingSilverEnqueueAdapter) EnqueueSilver(_ context.Context, _ SilverEnqueueRequest) (bool, error) {
	if !a.WriteEnabled {
		return false, ErrSilverGateWriteDisabled
	}
	return false, ErrSilverGateWriteDisabled
}
