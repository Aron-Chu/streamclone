package analytics

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

var (
	errGoldSegmentSkip       = errors.New("gold segment already done")
	errGoldSegmentRetryLater = errors.New("gold segment lease saturated")
	errGoldSegmentNotClaimed = errors.New("gold segment not claimed")
)

type goldBackfillJobIDContextKey struct{}

// WithGoldBackfillJobID attaches the active backfill_jobs.id for durable Gold segment rows.
func WithGoldBackfillJobID(ctx context.Context, jobID int64) context.Context {
	if ctx == nil || jobID <= 0 {
		return ctx
	}
	return context.WithValue(ctx, goldBackfillJobIDContextKey{}, jobID)
}

func goldBackfillJobIDFromContext(ctx context.Context) *int64 {
	if ctx == nil {
		return nil
	}
	v, ok := ctx.Value(goldBackfillJobIDContextKey{}).(int64)
	if !ok || v <= 0 {
		return nil
	}
	id := v
	return &id
}

type goldVODFetchLedger struct {
	svc           *SyncService
	ctx           context.Context
	vodID         string
	streamID      string
	login         string
	owner         string
	jobID         *int64
	maxAttempts   int
	leaseTTL      time.Duration
	maxPerVOD     int
	strategy      string
	mu            sync.Mutex
	activeClaimID map[string]int64
	lastHeartbeat map[string]time.Time
}

const goldSegmentHeartbeatMinInterval = 20 * time.Second

func (s *SyncService) goldVODFetchLedger(ctx context.Context, streamID, login, vodID string) *goldVODFetchLedger {
	if s == nil || !s.goldVODSegmentsEnabled || s.store == nil {
		return nil
	}
	owner := strings.TrimSpace(s.goldVODSegmentOwner)
	if owner == "" {
		owner = defaultGoldVODSegmentOwner()
	}
	return &goldVODFetchLedger{
		svc:           s,
		ctx:           ctx,
		vodID:         strings.TrimSpace(vodID),
		streamID:      strings.TrimSpace(streamID),
		login:         normalizeLogin(login),
		owner:         owner,
		jobID:         goldBackfillJobIDFromContext(ctx),
		maxAttempts:   s.goldRetryMax,
		leaseTTL:      s.goldLeaseTTL,
		maxPerVOD:     s.goldMaxSegmentsPerVOD,
		strategy:      GoldVODSegmentStrategyV1,
		activeClaimID: make(map[string]int64),
		lastHeartbeat: make(map[string]time.Time),
	}
}

func defaultGoldVODSegmentOwner() string {
	if host, err := os.Hostname(); err == nil {
		host = strings.TrimSpace(host)
		if host != "" {
			return "gold-" + host
		}
	}
	return "gold-worker-local"
}

func (l *goldVODFetchLedger) upsertPlansForSegments(segs []gqlSegmentProgress) error {
	if l == nil || l.svc == nil || l.svc.store == nil || len(segs) == 0 {
		return nil
	}
	plans := make([]GoldVODSegmentPlan, 0, len(segs))
	for _, seg := range segs {
		if seg.Done || seg.EndSec <= seg.StartSec {
			continue
		}
		plans = append(plans, GoldVODSegmentPlan{
			SegmentKey:         GoldVODSegmentKey(l.vodID, seg.StartSec, seg.EndSec, l.strategy),
			VODID:              l.vodID,
			StreamID:           l.streamID,
			Login:              l.login,
			StrategyVersion:    l.strategy,
			StartOffsetSeconds: seg.StartSec,
			EndOffsetSeconds:   seg.EndSec,
		})
	}
	if len(plans) == 0 {
		return nil
	}
	_, err := l.svc.store.UpsertGoldVODSegmentPlans(l.ctx, plans, l.jobID, l.maxAttempts)
	return err
}

func (l *goldVODFetchLedger) segmentKey(seg gqlSegmentProgress) string {
	return GoldVODSegmentKey(l.vodID, seg.StartSec, seg.EndSec, l.strategy)
}

func (l *goldVODFetchLedger) beginSegment(seg *gqlSegmentProgress) error {
	if l == nil || seg == nil {
		return nil
	}
	if seg.EndSec <= seg.StartSec {
		return nil
	}
	key := l.segmentKey(*seg)
	status, found, err := l.svc.store.GoldVODSegmentStatusByKey(l.ctx, key)
	if err != nil {
		return err
	}
	if found {
		switch strings.ToLower(status) {
		case "done", "skipped":
			seg.Done = true
			if seg.OffsetSec < seg.StartSec {
				seg.OffsetSec = seg.StartSec
			}
			return errGoldSegmentSkip
		case "dead_letter":
			return fmt.Errorf("gold segment dead_letter: %s", key)
		}
	}
	if _, err := l.svc.store.UpsertGoldVODSegmentPlans(l.ctx, []GoldVODSegmentPlan{{
		SegmentKey:         key,
		VODID:              l.vodID,
		StreamID:           l.streamID,
		Login:              l.login,
		StrategyVersion:    l.strategy,
		StartOffsetSeconds: seg.StartSec,
		EndOffsetSeconds:   seg.EndSec,
	}}, l.jobID, l.maxAttempts); err != nil {
		return err
	}
	claim, err := l.svc.store.ClaimGoldVODSegmentByKey(l.ctx, key, l.owner, l.leaseTTL, l.maxPerVOD)
	if err != nil {
		return err
	}
	if claim == nil {
		return errGoldSegmentRetryLater
	}
	l.mu.Lock()
	l.activeClaimID[key] = claim.ID
	l.mu.Unlock()
	return nil
}

func (l *goldVODFetchLedger) heartbeatSegment(seg gqlSegmentProgress) {
	if l == nil || l.svc == nil || l.svc.store == nil {
		return
	}
	key := l.segmentKey(seg)
	l.mu.Lock()
	claimID := l.activeClaimID[key]
	if claimID <= 0 {
		l.mu.Unlock()
		return
	}
	if last, ok := l.lastHeartbeat[key]; ok && time.Since(last) < goldSegmentHeartbeatMinInterval {
		l.mu.Unlock()
		return
	}
	l.lastHeartbeat[key] = time.Now()
	l.mu.Unlock()
	if _, err := l.svc.store.HeartbeatGoldVODSegment(l.ctx, claimID, l.owner, l.leaseTTL); err != nil {
		l.svc.log.Warn("gold vod segment heartbeat failed",
			"stream_id", l.streamID,
			"vod_id", l.vodID,
			"segment_key", key,
			"err", err,
		)
	}
}

func (l *goldVODFetchLedger) completeSegment(seg gqlSegmentProgress, commentsFetched int) {
	if l == nil || l.svc == nil || l.svc.store == nil {
		return
	}
	key := l.segmentKey(seg)
	l.mu.Lock()
	claimID := l.activeClaimID[key]
	delete(l.activeClaimID, key)
	l.mu.Unlock()
	if claimID <= 0 {
		return
	}
	cursor := strconv.Itoa(seg.OffsetSec)
	if _, err := l.svc.store.CompleteGoldVODSegment(l.ctx, claimID, l.owner, commentsFetched, cursor); err != nil {
		l.svc.log.Warn("gold vod segment complete failed",
			"stream_id", l.streamID,
			"vod_id", l.vodID,
			"segment_key", key,
			"err", err,
		)
	}
}

func (l *goldVODFetchLedger) failSegment(seg gqlSegmentProgress, cause error) {
	if l == nil || l.svc == nil || l.svc.store == nil || cause == nil {
		return
	}
	if errors.Is(cause, errGoldSegmentSkip) || errors.Is(cause, errGoldSegmentRetryLater) {
		return
	}
	key := l.segmentKey(seg)
	l.mu.Lock()
	claimID := l.activeClaimID[key]
	delete(l.activeClaimID, key)
	l.mu.Unlock()
	if claimID <= 0 {
		return
	}
	retryAfter := 60 * time.Second
	if isRetriableGoldFailure(cause.Error()) || strings.Contains(strings.ToLower(cause.Error()), "429") || strings.Contains(strings.ToLower(cause.Error()), "throttl") {
		retryAfter = 2 * time.Minute
	}
	if _, err := l.svc.store.FailGoldVODSegment(l.ctx, claimID, l.owner, sanitizeGoldSegmentError(cause), retryAfter); err != nil {
		l.svc.log.Warn("gold vod segment fail failed",
			"stream_id", l.streamID,
			"vod_id", l.vodID,
			"segment_key", key,
			"err", err,
		)
	}
}

func (l *goldVODFetchLedger) onHotSplit(beforeSplit gqlSegmentProgress, splitAt int, tail gqlSegmentProgress) error {
	if l == nil {
		return nil
	}
	key := l.segmentKey(beforeSplit)
	l.mu.Lock()
	parentClaimID := l.activeClaimID[key]
	delete(l.activeClaimID, key)
	l.mu.Unlock()
	shrunk := beforeSplit
	shrunk.EndSec = splitAt - 1
	if err := l.upsertPlansForSegments([]gqlSegmentProgress{shrunk, tail}); err != nil {
		if parentClaimID > 0 {
			l.mu.Lock()
			l.activeClaimID[key] = parentClaimID
			l.mu.Unlock()
		}
		return fmt.Errorf("gold vod hot-split child upsert: %w", err)
	}
	if parentClaimID <= 0 {
		return nil
	}
	skipped, err := l.svc.store.SkipGoldVODSegment(l.ctx, parentClaimID, l.owner, "hot_split rescheduled")
	if err != nil {
		return fmt.Errorf("gold vod hot-split parent skip: %w", err)
	}
	if !skipped {
		return fmt.Errorf("gold vod hot-split parent skip: claim %d not skipped", parentClaimID)
	}
	return nil
}

func sanitizeGoldSegmentError(err error) string {
	if err == nil {
		return ""
	}
	msg := strings.TrimSpace(err.Error())
	if msg == "" {
		return "segment fetch failed"
	}
	if len(msg) > 500 {
		msg = msg[:500]
	}
	return msg
}
