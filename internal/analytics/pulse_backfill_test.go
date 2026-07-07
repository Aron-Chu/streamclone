package analytics

import (
	"testing"
	"time"
)

func TestPulseBackfillDedupeByStreamRangeAndMode(t *testing.T) {
	base := PulseBackfillRange{FromOffsetSeconds: 0, ToOffsetSeconds: 300}

	prefixKey := pulseBackfillJobKey("stream-1", "missed", "", base)
	if got, want := prefixKey, pulseBackfillJobKey("stream-1", PulseBackfillModePrefix, "", base); got != want {
		t.Fatalf("legacy missed mode should alias prefix: got %q want %q", got, want)
	}

	differentRangeKey := pulseBackfillJobKey("stream-1", PulseBackfillModePrefix, "", PulseBackfillRange{
		FromOffsetSeconds: 600,
		ToOffsetSeconds:   900,
	})
	if differentRangeKey == prefixKey {
		t.Fatalf("different range should produce a distinct key")
	}

	differentModeKey := pulseBackfillJobKey("stream-1", PulseBackfillModeMomentWindow, "", base)
	if differentModeKey == prefixKey {
		t.Fatalf("different mode should produce a distinct key")
	}
}

func TestPulseBackfillOverlappingPrefixReturnsExistingJob(t *testing.T) {
	m := &PulseBackfillManager{
		jobs:           map[string]*PulseBackfillJob{},
		activeByStream: map[string]string{},
		activeByKey:    map[string]string{},
		lastEnqueue:    map[string]time.Time{},
	}
	job := &PulseBackfillJob{
		JobID:    "job-1",
		StreamID: "stream-1",
		Login:    "xqc",
		Mode:     PulseBackfillModePrefix,
		Status:   PulseBackfillQueued,
		Range: PulseBackfillRange{
			FromOffsetSeconds: 0,
			ToOffsetSeconds:   300,
		},
	}
	m.jobs[job.JobID] = job
	m.activeByKey[pulseBackfillJobKey(job.StreamID, job.Mode, "", job.Range)] = job.JobID

	existing := m.activeJobForRange("stream-1", "missed", PulseBackfillRange{
		FromOffsetSeconds: 240,
		ToOffsetSeconds:   600,
	})
	if existing == nil {
		t.Fatalf("expected overlapping prefix request to reuse active job")
	}
	if existing.JobID != job.JobID {
		t.Fatalf("expected job %q, got %q", job.JobID, existing.JobID)
	}
}

func TestPulseBackfillDifferentRangeAtCapacityReturns429(t *testing.T) {
	m := &PulseBackfillManager{
		jobs:           map[string]*PulseBackfillJob{},
		activeByStream: map[string]string{},
		activeByKey:    map[string]string{},
		lastEnqueue:    map[string]time.Time{},
		maxConcurrent:  1,
	}
	active := &PulseBackfillJob{
		JobID:    "job-1",
		StreamID: "stream-1",
		Login:    "xqc",
		Mode:     PulseBackfillModePrefix,
		Status:   PulseBackfillQueued,
		Range: PulseBackfillRange{
			FromOffsetSeconds: 0,
			ToOffsetSeconds:   300,
		},
	}
	m.jobs[active.JobID] = active
	m.activeByKey[pulseBackfillJobKey(active.StreamID, active.Mode, "", active.Range)] = active.JobID

	if existing := m.activeJobForRange("stream-1", PulseBackfillModePrefix, PulseBackfillRange{
		FromOffsetSeconds: 600,
		ToOffsetSeconds:   900,
	}); existing != nil {
		t.Fatalf("non-overlapping range should not reuse active job")
	}
	if got := m.activeJobCountLocked(); got != m.maxConcurrent {
		t.Fatalf("test setup expected active count at capacity, got %d", got)
	}
}
