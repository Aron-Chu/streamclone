package analytics

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

const (
	defaultPulseAutoBackfillInterval = 15 * time.Minute
	defaultPulseAutoBackfillCooldown = 30 * time.Minute
	defaultPulseAutoBackfillSince    = 48 * time.Hour
)

// PulseAutoBackfillOptions bounds automatic VOD chat gap repair.
type PulseAutoBackfillOptions struct {
	Interval  time.Duration
	Cooldown  time.Duration
	Since     time.Duration
	MaxPerRun int
	ScanLimit int
}

// PulseAutoBackfillReport summarizes one scan pass.
type PulseAutoBackfillReport struct {
	Scanned         int
	Eligible        int
	Enqueued        int
	SkippedActive   int
	SkippedCooldown int
	SkippedCapacity int
	SkippedNoGap    int
	SkippedError    int
}

type pulseAutoBackfillLog interface {
	Info(string, ...any)
	Warn(string, ...any)
}

type pulseBackfillScheduler interface {
	ActiveJobForStream(streamID string) *PulseBackfillJob
	BackfillFailedForStream(streamID string) bool
	Enqueue(ctx context.Context, req PulseBackfillRequest) (*PulseBackfillJob, error)
}

// PulseAutoBackfillEnqueuer scans ended, VOD-linked streams for chat coverage
// gaps and reuses PulseBackfillManager's dedupe/capacity gates to repair them.
type PulseAutoBackfillEnqueuer struct {
	store     *Store
	scheduler pulseBackfillScheduler
	runtime   PulseRuntimeConfig
	opts      PulseAutoBackfillOptions

	mu          sync.Mutex
	lastAttempt map[string]time.Time
}

func NewPulseAutoBackfillEnqueuer(store *Store, scheduler *PulseBackfillManager, runtime PulseRuntimeConfig, opts PulseAutoBackfillOptions) *PulseAutoBackfillEnqueuer {
	return &PulseAutoBackfillEnqueuer{
		store:       store,
		scheduler:   scheduler,
		runtime:     runtime.withDefaults(),
		opts:        normalizePulseAutoBackfillOptions(opts),
		lastAttempt: make(map[string]time.Time),
	}
}

func normalizePulseAutoBackfillOptions(opts PulseAutoBackfillOptions) PulseAutoBackfillOptions {
	if opts.Interval <= 0 {
		opts.Interval = defaultPulseAutoBackfillInterval
	}
	if opts.Cooldown <= 0 {
		opts.Cooldown = defaultPulseAutoBackfillCooldown
	}
	if opts.Since <= 0 {
		opts.Since = defaultPulseAutoBackfillSince
	}
	if opts.MaxPerRun <= 0 {
		opts.MaxPerRun = 1
	}
	if opts.ScanLimit <= 0 {
		opts.ScanLimit = opts.MaxPerRun * 20
	}
	if opts.ScanLimit < opts.MaxPerRun {
		opts.ScanLimit = opts.MaxPerRun
	}
	return opts
}

func (e *PulseAutoBackfillEnqueuer) RunOnce(ctx context.Context) (PulseAutoBackfillReport, error) {
	var report PulseAutoBackfillReport
	if e == nil || e.store == nil || e.store.db == nil || e.scheduler == nil {
		return report, nil
	}
	runtime := e.runtime.withDefaults()
	if !runtime.BackfillEnabled || !runtime.GQLCommentsEnabled || runtime.ReadOnlyMode {
		return report, nil
	}
	opts := normalizePulseAutoBackfillOptions(e.opts)
	since := time.Now().UTC().Add(-opts.Since)
	rows, err := e.store.db.Query(ctx, `
		SELECT stream_id, COALESCE(login,''), started_at, ended_at, COALESCE(vod_id,'')
		FROM analytics_streams
		WHERE ended_at IS NOT NULL
		  AND started_at >= $1
		  AND COALESCE(vod_id,'') <> ''
		ORDER BY ended_at DESC
		LIMIT $2`, since, opts.ScanLimit)
	if err != nil {
		return report, err
	}
	defer rows.Close()

	now := time.Now().UTC()
	for rows.Next() {
		if report.Enqueued >= opts.MaxPerRun {
			break
		}
		var streamID, login, vodID string
		var startedAt time.Time
		var endedAt time.Time
		if err := rows.Scan(&streamID, &login, &startedAt, &endedAt, &vodID); err != nil {
			return report, err
		}
		report.Scanned++
		if endedAt.IsZero() || strings.TrimSpace(vodID) == "" {
			report.SkippedNoGap++
			continue
		}
		if e.scheduler.ActiveJobForStream(streamID) != nil {
			report.SkippedActive++
			continue
		}
		rollups, err := e.store.RollupsByStream(ctx, streamID)
		if err != nil {
			report.SkippedError++
			continue
		}
		heatmapRollups, _, err := consolidateHeatmapRollups(rollups, startedAt)
		if err != nil {
			report.SkippedError++
			continue
		}
		currentOffset := int(endedAt.Sub(startedAt).Seconds())
		if currentOffset < 0 {
			currentOffset = 0
		}
		coverage := computePulseCoverage(
			heatmapRollups,
			startedAt,
			currentOffset,
			false,
			vodID,
			false,
			e.scheduler.BackfillFailedForStream(streamID),
		)
		r, ok := pulseAutoBackfillRange(coverage)
		if !ok {
			report.SkippedNoGap++
			continue
		}
		report.Eligible++
		backfillRange := PulseBackfillRange{
			FromOffsetSeconds: r.FromOffsetSeconds,
			ToOffsetSeconds:   r.ToOffsetSeconds,
		}
		key := pulseBackfillJobKey(streamID, PulseBackfillModeMissingRange, "", backfillRange)
		if e.inCooldown(key, now, opts.Cooldown) {
			report.SkippedCooldown++
			continue
		}
		e.markAttempt(key, now, opts.Cooldown)
		mode := PulseBackfillModeMissingRange
		if r.FromOffsetSeconds == 0 {
			mode = PulseBackfillModePrefix
		}
		job, err := e.scheduler.Enqueue(ctx, PulseBackfillRequest{
			StreamID:          streamID,
			Login:             login,
			Mode:              mode,
			FromOffsetSeconds: r.FromOffsetSeconds,
			ToOffsetSeconds:   r.ToOffsetSeconds,
			VodID:             vodID,
		})
		if errors.Is(err, ErrPulseBackfillAtCapacity) {
			report.SkippedCapacity++
			break
		}
		if err != nil {
			report.SkippedError++
			continue
		}
		if job != nil && job.Status != PulseBackfillAlreadyAvailable {
			report.Enqueued++
		}
	}
	return report, rows.Err()
}

func (e *PulseAutoBackfillEnqueuer) inCooldown(key string, now time.Time, cooldown time.Duration) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	last, ok := e.lastAttempt[key]
	return ok && now.Sub(last) < cooldown
}

func (e *PulseAutoBackfillEnqueuer) markAttempt(key string, now time.Time, cooldown time.Duration) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if cooldown <= 0 {
		cooldown = defaultPulseAutoBackfillCooldown
	}
	for existingKey, last := range e.lastAttempt {
		if now.Sub(last) > 2*cooldown {
			delete(e.lastAttempt, existingKey)
		}
	}
	e.lastAttempt[key] = now
}

func pulseAutoBackfillRange(c ExtensionCoverage) (ExtensionCoverageRange, bool) {
	if !c.CanBackfill || len(c.MissingRanges) == 0 {
		return ExtensionCoverageRange{}, false
	}
	best := c.MissingRanges[0]
	for _, r := range c.MissingRanges {
		if r.FromOffsetSeconds == 0 {
			return r, true
		}
		if r.ToOffsetSeconds-r.FromOffsetSeconds > best.ToOffsetSeconds-best.FromOffsetSeconds {
			best = r
		}
	}
	if best.ToOffsetSeconds < best.FromOffsetSeconds {
		best.ToOffsetSeconds = best.FromOffsetSeconds
	}
	return best, true
}

// StartPulseAutoBackfillEnqueuer starts conservative automatic Pulse chat gap repair.
func StartPulseAutoBackfillEnqueuer(ctx context.Context, enqueuer *PulseAutoBackfillEnqueuer, log pulseAutoBackfillLog) {
	if enqueuer == nil {
		return
	}
	opts := normalizePulseAutoBackfillOptions(enqueuer.opts)
	go func() {
		run := func(trigger string) {
			report, err := enqueuer.RunOnce(ctx)
			if err != nil {
				if log != nil {
					log.Warn("pulse auto-backfill scan failed", "trigger", trigger, "err", err)
				}
				return
			}
			if log != nil && (report.Enqueued > 0 || report.SkippedCapacity > 0 || report.SkippedError > 0) {
				log.Info("pulse auto-backfill scan completed",
					"trigger", trigger,
					"scanned", report.Scanned,
					"eligible", report.Eligible,
					"enqueued", report.Enqueued,
					"capacity_skipped", report.SkippedCapacity,
					"errors", report.SkippedError,
				)
			}
		}
		run("startup")
		ticker := time.NewTicker(jitterPulseAutoBackfillInterval(opts.Interval))
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				run("interval")
				ticker.Reset(jitterPulseAutoBackfillInterval(opts.Interval))
			}
		}
	}()
}

func jitterPulseAutoBackfillInterval(interval time.Duration) time.Duration {
	if interval <= 0 {
		interval = defaultPulseAutoBackfillInterval
	}
	// Deterministic jitter is enough to avoid exact alignment across services.
	jitter := time.Duration(time.Now().UnixNano() % int64(interval/5+time.Second))
	return interval + jitter
}

func (r PulseAutoBackfillReport) String() string {
	return fmt.Sprintf("scanned=%d eligible=%d enqueued=%d active=%d cooldown=%d capacity=%d no_gap=%d errors=%d",
		r.Scanned, r.Eligible, r.Enqueued, r.SkippedActive, r.SkippedCooldown, r.SkippedCapacity, r.SkippedNoGap, r.SkippedError)
}
