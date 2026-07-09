package ingestcore

import (
	"context"
	"testing"
)

type stubCandidateSource struct {
	lastTopN int
	rows     []SchedulerCandidate
}

func (s *stubCandidateSource) ListLiveCandidates(_ context.Context, topN int) ([]SchedulerCandidate, error) {
	s.lastTopN = topN
	if len(s.rows) == 0 {
		out := make([]SchedulerCandidate, topN)
		for i := range out {
			out[i] = SchedulerCandidate{
				Login:     "ch" + string(rune('a'+i%26)),
				StreamID:  "stream",
				IsLive:    true,
				HelixRank: i + 1,
				Priority:  10,
			}
		}
		return out, nil
	}
	return s.rows, nil
}

func TestTierSchedulerScanUsesHubRosterLimitWhenTieringOff(t *testing.T) {
	source := &stubCandidateSource{}
	manager := NewCollectorManager(Config{MaxActiveIRC: 250}, nil, nil)
	sched := NewTierScheduler(Config{
		TieringEnabled:    false,
		HubRosterLimit:    500,
		CandidateScanTopN: 500,
		MaxActiveIRC:      250,
		P1HotLimit:        250,
	}, manager, source, nil)

	sched.RunOnce(context.Background())
	if source.lastTopN != 500 {
		t.Fatalf("ListLiveCandidates topN = %d, want 500 when tiering off", source.lastTopN)
	}
}

func TestTierSchedulerScanUsesExplicitCandidateScanTopN(t *testing.T) {
	source := &stubCandidateSource{}
	manager := NewCollectorManager(Config{MaxActiveIRC: 250}, nil, nil)
	sched := NewTierScheduler(Config{
		TieringEnabled:    true,
		HubRosterLimit:    500,
		CandidateScanTopN: 400,
		MaxActiveIRC:      250,
		P1HotLimit:        250,
	}, manager, source, nil)

	sched.RunOnce(context.Background())
	if source.lastTopN != 400 {
		t.Fatalf("ListLiveCandidates topN = %d, want explicit INGEST_CANDIDATE_SCAN_TOP_N", source.lastTopN)
	}
}

func TestMergeDesiredChannelsPrefersP0(t *testing.T) {
	base := []DesiredChannel{
		{Login: "xqc", Tier: TierP1Hot, TrackPriority: 10, HelixRank: 1},
	}
	extra := []DesiredChannel{
		{Login: "xqc", Tier: TierP0Always, TrackPriority: 70},
	}
	merged := mergeDesiredChannels(base, extra)
	if len(merged) != 1 {
		t.Fatalf("expected 1 channel, got %d", len(merged))
	}
	if merged[0].Tier != TierP0Always {
		t.Fatalf("expected P0 tier, got %v", merged[0].Tier)
	}
}

func TestEngineOwnsIRCAdmission(t *testing.T) {
	var nilEngine *Engine
	if nilEngine.OwnsIRCAdmission() {
		t.Fatal("nil engine should not own admission")
	}
	e := &Engine{cfg: Config{CoreEnabled: true}}
	if !e.OwnsIRCAdmission() {
		t.Fatal("core enabled engine should own admission")
	}
}
