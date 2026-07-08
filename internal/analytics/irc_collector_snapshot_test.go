package analytics

import (
	"context"
	"testing"
	"time"

	"streamclone/internal/analytics/ingestcore"
)

type stubIngestCandidateSource struct {
	rows []ingestcore.SchedulerCandidate
}

func (s stubIngestCandidateSource) ListLiveCandidates(_ context.Context, _ int) ([]ingestcore.SchedulerCandidate, error) {
	return s.rows, nil
}

type noopIngestIRC struct{}

func (noopIngestIRC) Join(context.Context, string) {}
func (noopIngestIRC) Part(context.Context, string) {}

func TestHandlerIRCCollectorSnapshotPrefersIngestCore(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	engine := ingestcore.NewEngine(ingestcore.EngineDeps{
		Config: ingestcore.Config{
			CoreEnabled:     true,
			MaxActiveIRC:    250,
			FlushInterval:   time.Hour,
			OpenMinuteFlush: time.Hour,
		},
		IRC: noopIngestIRC{},
		Source: stubIngestCandidateSource{
			rows: []ingestcore.SchedulerCandidate{
				{Login: "xqc", StreamID: "1", IsLive: true, HelixRank: 1, Priority: 10},
				{Login: "ludwig", StreamID: "2", IsLive: true, HelixRank: 2, Priority: 10},
			},
		},
	})
	engine.Start(ctx, 10*time.Millisecond)
	t.Cleanup(func() { engine.Stop(ctx) })
	time.Sleep(25 * time.Millisecond)

	legacy := NewCollector(&fakeStore{}, fakeProvider{}, &fakeJoiner{}, nil, nilLogger(), 250, time.Hour, time.Hour, 200)
	h := &Handler{ingestEngine: engine, collector: legacy}

	active, max := h.ircCollectorSnapshot()
	if active != 2 {
		t.Fatalf("active = %d, want 2 from ingest-core", active)
	}
	if max != 250 {
		t.Fatalf("max = %d, want 250", max)
	}
	if !h.isIRCActiveLogin("xqc") {
		t.Fatal("expected xqc active on ingest-core")
	}
	if h.isIRCActiveLogin("notlive") {
		t.Fatal("unexpected inactive login marked active")
	}
}

func TestCorpusCriticalFromStaleLegacyCollector(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	engine := ingestcore.NewEngine(ingestcore.EngineDeps{
		Config: ingestcore.Config{
			CoreEnabled:     true,
			MaxActiveIRC:    250,
			FlushInterval:   time.Hour,
			OpenMinuteFlush: time.Hour,
		},
		IRC: noopIngestIRC{},
		Source: stubIngestCandidateSource{
			rows: []ingestcore.SchedulerCandidate{
				{Login: "xqc", StreamID: "1", IsLive: true, HelixRank: 1, Priority: 10},
			},
		},
	})
	engine.Start(ctx, 10*time.Millisecond)
	t.Cleanup(func() { engine.Stop(ctx) })
	time.Sleep(25 * time.Millisecond)

	h := &Handler{ingestEngine: engine}
	pipeline := HubCorpusPipeline{
		State: CorpusStatusCritical,
		Roster: HubTrackerSummary{
			CollectorTracking: 0,
		},
	}
	if !corpusCriticalFromStaleLegacyCollector(h, pipeline) {
		t.Fatal("expected stale legacy critical detection when ingest-core is operational")
	}
	pipeline.CollectorActive = 3
	if corpusCriticalFromStaleLegacyCollector(h, pipeline) {
		t.Fatal("expected false once corpus pipeline reports active collectors")
	}
}

func TestCorpusPipelineStateHealthyWithIngestCoreTracking(t *testing.T) {
	cfg := CorpusRuntimeConfig{
		MetadataEnabled:      true,
		MetadataWriteEnabled: true,
		LiveAdmissionEnabled: true,
	}
	report := Top100ReadinessReport{
		CollectorActive: 90,
		CollectorMax:    250,
		Summary: Top100ReadinessSummary{
			LiveRows:              90,
			CollectorTrackingRows: 90,
			ExpectedCollectorRows: 90,
		},
	}
	if got := corpusPipelineStateFromReadiness(cfg, report); got != CorpusStatusHealthy {
		t.Fatalf("state = %q, want healthy", got)
	}
}
