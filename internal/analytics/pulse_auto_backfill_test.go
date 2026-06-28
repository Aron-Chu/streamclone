package analytics

import (
	"context"
	"errors"
	"testing"
	"time"
)

type fakePulseBackfillScheduler struct {
	active   map[string]*PulseBackfillJob
	failed   map[string]bool
	err      error
	enqueued []PulseBackfillRequest
}

func (f *fakePulseBackfillScheduler) ActiveJobForStream(streamID string) *PulseBackfillJob {
	if f == nil {
		return nil
	}
	if job := f.active[streamID]; job != nil {
		copy := *job
		return &copy
	}
	return nil
}

func (f *fakePulseBackfillScheduler) BackfillFailedForStream(streamID string) bool {
	return f != nil && f.failed[streamID]
}

func (f *fakePulseBackfillScheduler) Enqueue(_ context.Context, req PulseBackfillRequest) (*PulseBackfillJob, error) {
	if f.err != nil {
		return nil, f.err
	}
	f.enqueued = append(f.enqueued, req)
	return &PulseBackfillJob{
		JobID:    "job-test",
		StreamID: req.StreamID,
		Login:    req.Login,
		Mode:     normalizePulseBackfillMode(req.Mode),
		Status:   PulseBackfillQueued,
		Range: PulseBackfillRange{
			FromOffsetSeconds: req.FromOffsetSeconds,
			ToOffsetSeconds:   req.ToOffsetSeconds,
		},
	}, nil
}

func TestPulseAutoBackfillRangePrefersPrefix(t *testing.T) {
	got, ok := pulseAutoBackfillRange(ExtensionCoverage{
		CanBackfill: true,
		MissingRanges: []ExtensionCoverageRange{
			{FromOffsetSeconds: 600, ToOffsetSeconds: 1800},
			{FromOffsetSeconds: 0, ToOffsetSeconds: 300},
		},
	})
	if !ok {
		t.Fatal("expected auto-backfill range")
	}
	if got.FromOffsetSeconds != 0 || got.ToOffsetSeconds != 300 {
		t.Fatalf("range = %+v, want prefix 0-300", got)
	}
}

func TestPulseAutoBackfillRangeChoosesLongestInternalGap(t *testing.T) {
	got, ok := pulseAutoBackfillRange(ExtensionCoverage{
		CanBackfill: true,
		MissingRanges: []ExtensionCoverageRange{
			{FromOffsetSeconds: 600, ToOffsetSeconds: 900},
			{FromOffsetSeconds: 1200, ToOffsetSeconds: 2400},
		},
	})
	if !ok {
		t.Fatal("expected auto-backfill range")
	}
	if got.FromOffsetSeconds != 1200 || got.ToOffsetSeconds != 2400 {
		t.Fatalf("range = %+v, want longest internal gap", got)
	}
}

func TestPulseAutoBackfillRunOnceEnqueuesVODReadyGap(t *testing.T) {
	ctx, store := setupSessionStore(t)
	startedAt := time.Date(2026, 6, 22, 19, 0, 0, 0, time.UTC)
	insertTestStream(t, ctx, store, "stream-auto", "xqc", 10)
	mustExec(t, ctx, store, `
		UPDATE analytics_streams
		SET started_at=$2, ended_at=$3, vod_id='123456'
		WHERE stream_id=$1`, "stream-auto", startedAt, startedAt.Add(2*time.Hour))
	mustExec(t, ctx, store, `
		INSERT INTO analytics_minute_rollups (stream_id, minute_ts, chat_count)
		VALUES ('stream-auto', $1, 5)`, startedAt.Add(30*time.Minute))

	scheduler := &fakePulseBackfillScheduler{}
	enqueuer := &PulseAutoBackfillEnqueuer{
		store:       store,
		scheduler:   scheduler,
		runtime:     DefaultPulseRuntimeConfig(),
		opts:        PulseAutoBackfillOptions{MaxPerRun: 1, ScanLimit: 10, Cooldown: time.Hour, Since: 72 * time.Hour},
		lastAttempt: map[string]time.Time{},
	}
	report, err := enqueuer.RunOnce(ctx)
	if err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if report.Enqueued != 1 || len(scheduler.enqueued) != 1 {
		t.Fatalf("report=%+v enqueued=%d, want one enqueue", report, len(scheduler.enqueued))
	}
	req := scheduler.enqueued[0]
	if req.Mode != PulseBackfillModePrefix || req.FromOffsetSeconds != 0 {
		t.Fatalf("request = %+v, want prefix from 0", req)
	}

	report, err = enqueuer.RunOnce(ctx)
	if err != nil {
		t.Fatalf("second RunOnce: %v", err)
	}
	if report.SkippedCooldown != 1 {
		t.Fatalf("second report = %+v, want cooldown skip", report)
	}
}

func TestPulseAutoBackfillRunOnceSkipsActiveAndCapacity(t *testing.T) {
	ctx, store := setupSessionStore(t)
	startedAt := time.Date(2026, 6, 22, 19, 0, 0, 0, time.UTC)
	insertTestStream(t, ctx, store, "stream-active-gap", "xqc", 10)
	mustExec(t, ctx, store, `
		UPDATE analytics_streams
		SET started_at=$2, ended_at=$3, vod_id='123456'
		WHERE stream_id=$1`, "stream-active-gap", startedAt, startedAt.Add(2*time.Hour))
	mustExec(t, ctx, store, `
		INSERT INTO analytics_minute_rollups (stream_id, minute_ts, chat_count)
		VALUES ('stream-active-gap', $1, 5)`, startedAt.Add(30*time.Minute))

	activeScheduler := &fakePulseBackfillScheduler{
		active: map[string]*PulseBackfillJob{
			"stream-active-gap": {JobID: "job-active", Status: PulseBackfillQueued},
		},
	}
	enqueuer := &PulseAutoBackfillEnqueuer{
		store:       store,
		scheduler:   activeScheduler,
		runtime:     DefaultPulseRuntimeConfig(),
		opts:        PulseAutoBackfillOptions{MaxPerRun: 1, ScanLimit: 10, Cooldown: time.Hour, Since: 72 * time.Hour},
		lastAttempt: map[string]time.Time{},
	}
	report, err := enqueuer.RunOnce(ctx)
	if err != nil {
		t.Fatalf("RunOnce active: %v", err)
	}
	if report.SkippedActive != 1 || len(activeScheduler.enqueued) != 0 {
		t.Fatalf("active report=%+v enqueued=%d, want active skip", report, len(activeScheduler.enqueued))
	}

	capacityScheduler := &fakePulseBackfillScheduler{err: ErrPulseBackfillAtCapacity}
	enqueuer.scheduler = capacityScheduler
	report, err = enqueuer.RunOnce(ctx)
	if err != nil {
		t.Fatalf("RunOnce capacity: %v", err)
	}
	if report.SkippedCapacity != 1 {
		t.Fatalf("capacity report=%+v, want capacity skip", report)
	}
}

func TestPulseAutoBackfillRunOnceSkipsNoVODAndUnexpectedErrors(t *testing.T) {
	ctx, store := setupSessionStore(t)
	startedAt := time.Date(2026, 6, 22, 19, 0, 0, 0, time.UTC)
	insertTestStream(t, ctx, store, "stream-no-vod", "xqc", 10)
	mustExec(t, ctx, store, `
		UPDATE analytics_streams
		SET started_at=$2, ended_at=$3, vod_id=''
		WHERE stream_id=$1`, "stream-no-vod", startedAt, startedAt.Add(2*time.Hour))
	mustExec(t, ctx, store, `
		INSERT INTO analytics_minute_rollups (stream_id, minute_ts, chat_count)
		VALUES ('stream-no-vod', $1, 5)`, startedAt.Add(30*time.Minute))

	scheduler := &fakePulseBackfillScheduler{err: errors.New("unexpected")}
	enqueuer := &PulseAutoBackfillEnqueuer{
		store:       store,
		scheduler:   scheduler,
		runtime:     DefaultPulseRuntimeConfig(),
		opts:        PulseAutoBackfillOptions{MaxPerRun: 1, ScanLimit: 10, Cooldown: time.Hour, Since: 72 * time.Hour},
		lastAttempt: map[string]time.Time{},
	}
	report, err := enqueuer.RunOnce(ctx)
	if err != nil {
		t.Fatalf("RunOnce no vod: %v", err)
	}
	if report.Scanned != 0 || len(scheduler.enqueued) != 0 {
		t.Fatalf("report=%+v enqueued=%d, want no VOD row filtered out", report, len(scheduler.enqueued))
	}
}
