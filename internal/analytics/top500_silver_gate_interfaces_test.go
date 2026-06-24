package analytics

import (
	"context"
	"errors"
	"testing"
)

func TestNoopSilverCandidateReaderReturnsEmpty(t *testing.T) {
	var reader NoopSilverCandidateReader
	out, err := reader.ListCandidates(context.Background(), 5)
	if err != nil {
		t.Fatalf("ListCandidates err = %v", err)
	}
	if len(out) != 0 {
		t.Fatalf("len(candidates) = %d, want 0", len(out))
	}
}

func TestNoopSilverBudgetCounterReaderFailClosed(t *testing.T) {
	var reader NoopSilverBudgetCounterReader
	snap, err := reader.ReadSnapshot(context.Background(), "shroud")
	if err != nil {
		t.Fatalf("ReadSnapshot err = %v", err)
	}
	if snap.Available {
		t.Fatal("noop budget reader must fail closed (Available=false)")
	}
}

func TestRefusingSilverEnqueueAdapterNeverWritesEvenWhenWriteEnabledTrue(t *testing.T) {
	adapter := RefusingSilverEnqueueAdapter{WriteEnabled: true}
	inserted, err := adapter.EnqueueSilver(context.Background(), SilverEnqueueRequest{
		Tier: "silver", Login: "shroud", StreamID: "1",
	})
	if inserted {
		t.Fatal("adapter must never insert in LOAD-003a scaffold")
	}
	if !errors.Is(err, ErrSilverGateWriteDisabled) {
		t.Fatalf("err = %v, want ErrSilverGateWriteDisabled", err)
	}
}

func TestSilverGateInterfacesAreMockable(t *testing.T) {
	var _ SilverCandidateReader = (*mockSilverCandidateReader)(nil)
	var _ SilverBudgetCounterReader = (*mockSilverBudgetCounterReader)(nil)
	var _ SilverEnqueueAdapter = (*mockSilverEnqueueAdapter)(nil)
}

type mockSilverCandidateReader struct {
	candidates []SilverGateCandidate
}

func (m *mockSilverCandidateReader) ListCandidates(_ context.Context, limit int) ([]SilverGateCandidate, error) {
	if limit <= 0 || limit >= len(m.candidates) {
		return append([]SilverGateCandidate(nil), m.candidates...), nil
	}
	return append([]SilverGateCandidate(nil), m.candidates[:limit]...), nil
}

type mockSilverBudgetCounterReader struct {
	snap SilverBudgetSnapshot
}

func (m *mockSilverBudgetCounterReader) ReadSnapshot(context.Context, string) (SilverBudgetSnapshot, error) {
	return m.snap, nil
}

type mockSilverEnqueueAdapter struct {
	calls int
}

func (m *mockSilverEnqueueAdapter) EnqueueSilver(context.Context, SilverEnqueueRequest) (bool, error) {
	m.calls++
	return false, ErrSilverGateWriteDisabled
}
