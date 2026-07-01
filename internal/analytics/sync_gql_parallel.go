package analytics

import (
	"container/heap"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"streamclone/internal/analytics/chatreplay"
	"streamclone/internal/chat/enrich"
	"streamclone/internal/emoteimage"
	"streamclone/internal/metrics"
)

const (
	gqlVideoCommentsSHA256           = "b70a3591ff0f4e0313d126c6a1502d79a1c02baebb288227c582044aa76adf6a"
	vodCommentsMaxCount              = 200000
	vodCommentsProgressEvery         = 200
	vodCommentsParallelMaxFail       = 3
	vodGQLSegmentLargeVOD            = 300 // 5-minute segments for long VODs
	vodGQLSegmentDenseVOD            = 120 // 2-minute segments for very high comment volume
	vodGQLHotSegmentPageThreshold    = 10  // split hot segments after this many pages
	vodGQLHotSlowAdvanceSecDefault   = 30
	vodGQLHotSlowAdvancePagesDefault = 5
	vodGQLHotCommentsPerPageDefault  = 80
	gqlPriorityEdgeSecondsDefault    = 600 // first/last 10 minutes
	gqlMomentWindowBufferSec         = 120
	gqlMaxMomentWindows              = 5
	gqlMomentSpikeMultiplier         = 1.5

	gqlSegPriorityMoment             = 0
	gqlSegPriorityGame               = 1
	gqlSegPriorityEdge               = 2
	gqlSegPriorityBackground         = 3
	vodGQLLargeVODDurationSec        = 4 * 3600
	vodGQLDenseCommentsThreshold     = 50_000
	vodGQLVeryDenseCommentsThreshold = 250_000
	vodGQLCommentsPerHourDense       = 12_000
	vodGQLCommentsPerHourVeryDense   = 30_000
)

type gqlRateCoordinator struct {
	mu             sync.Mutex
	pauseUntil     time.Time
	minConcurrency int
	maxConcurrency int
	activeLimit    atomic.Int32
	successStreak  atomic.Int32
}

func newGQLRateCoordinator(minConcurrency, maxConcurrency, initial int) *gqlRateCoordinator {
	if minConcurrency <= 0 {
		minConcurrency = 1
	}
	if maxConcurrency < minConcurrency {
		maxConcurrency = minConcurrency
	}
	if initial < minConcurrency {
		initial = minConcurrency
	}
	if initial > maxConcurrency {
		initial = maxConcurrency
	}
	coord := &gqlRateCoordinator{
		minConcurrency: minConcurrency,
		maxConcurrency: maxConcurrency,
	}
	coord.activeLimit.Store(int32(initial))
	return coord
}

func (c *gqlRateCoordinator) ActiveConcurrency() int {
	if c == nil {
		return 1
	}
	limit := int(c.activeLimit.Load())
	if limit <= 0 {
		return 1
	}
	return limit
}

func (c *gqlRateCoordinator) RecordRateLimit() {
	if c == nil {
		return
	}
	c.successStreak.Store(0)
	for {
		cur := c.activeLimit.Load()
		if cur <= int32(c.minConcurrency) {
			return
		}
		if c.activeLimit.CompareAndSwap(cur, cur-1) {
			return
		}
	}
}

func (c *gqlRateCoordinator) RecordSuccess() {
	if c == nil {
		return
	}
	streak := c.successStreak.Add(1)
	if streak%50 != 0 {
		return
	}
	for {
		cur := c.activeLimit.Load()
		if cur >= int32(c.maxConcurrency) {
			return
		}
		if c.activeLimit.CompareAndSwap(cur, cur+1) {
			return
		}
	}
}

func (c *gqlRateCoordinator) Wait(ctx context.Context) error {
	for {
		c.mu.Lock()
		until := c.pauseUntil
		c.mu.Unlock()
		if until.IsZero() || time.Now().After(until) {
			return nil
		}
		delay := time.Until(until)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
		}
	}
}

func (c *gqlRateCoordinator) Throttle(_ int, retryAfter time.Duration, attempt int) {
	delay := gqlBackoffDelay(attempt, retryAfter)
	c.mu.Lock()
	newUntil := time.Now().Add(delay)
	if newUntil.After(c.pauseUntil) {
		c.pauseUntil = newUntil
	}
	c.mu.Unlock()
}

func (c *gqlRateCoordinator) Paused() bool {
	if c == nil {
		return false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return !c.pauseUntil.IsZero() && time.Now().Before(c.pauseUntil)
}

type gqlSegmentProgress struct {
	StartSec  int  `json:"startSec"`
	EndSec    int  `json:"endSec"`
	OffsetSec int  `json:"offsetSec"`
	Done      bool `json:"done"`
}

type gqlParallelCheckpoint struct {
	Segments        []gqlSegmentProgress `json:"segments"`
	CommentsFetched int                  `json:"commentsFetched"`
}

type gqlTimeRange struct {
	StartSec int
	EndSec   int
}

// gqlFetchScheduleHints drives priority enqueue order for parallel VOD GQL workers.
type gqlFetchScheduleHints struct {
	MomentWindows   []gqlTimeRange
	GameRanges      []gqlTimeRange
	VodDurationSec  int
	EdgePrioritySec int
}

type gqlPageSample struct {
	offsetAdvance int
	commentCount  int
}

type gqlRequestStats struct {
	throttle429   atomic.Int64
	throttle503   atomic.Int64
	backoffMillis atomic.Int64
}

func (s *gqlRequestStats) recordThrottle(status int, delay time.Duration) {
	if s == nil {
		return
	}
	switch status {
	case 429:
		s.throttle429.Add(1)
	case 503:
		s.throttle503.Add(1)
	}
	if delay > 0 {
		s.backoffMillis.Add(delay.Milliseconds())
	}
}

func (s *gqlRequestStats) snapshot() (count429, count503 int64, backoffSeconds float64) {
	if s == nil {
		return 0, 0, 0
	}
	return s.throttle429.Load(), s.throttle503.Load(), float64(s.backoffMillis.Load()) / 1000
}

type gqlSegmentWorkItem struct {
	idx      int
	priority int
	order    int
}

type gqlWorkHeap []gqlSegmentWorkItem

func (h gqlWorkHeap) Len() int { return len(h) }

func (h gqlWorkHeap) Less(i, j int) bool {
	if h[i].priority != h[j].priority {
		return h[i].priority < h[j].priority
	}
	return h[i].order < h[j].order
}

func (h gqlWorkHeap) Swap(i, j int) { h[i], h[j] = h[j], h[i] }

func (h *gqlWorkHeap) Push(x any) {
	*h = append(*h, x.(gqlSegmentWorkItem))
}

func (h *gqlWorkHeap) Pop() any {
	old := *h
	n := len(old)
	item := old[n-1]
	*h = old[:n-1]
	return item
}

// gqlSegmentWorkQueue is a mutex-protected priority heap shared by parallel GQL workers.
// Hot splits push tail segments back; idle workers acquire the next highest-priority item.
type gqlSegmentWorkQueue struct {
	mu       sync.Mutex
	cond     *sync.Cond
	heap     gqlWorkHeap
	orderSeq int
	inFlight int
}

func newGQLSegmentWorkQueue() *gqlSegmentWorkQueue {
	q := &gqlSegmentWorkQueue{}
	q.cond = sync.NewCond(&q.mu)
	heap.Init(&q.heap)
	return q
}

func (q *gqlSegmentWorkQueue) push(idx, priority int) {
	q.mu.Lock()
	heap.Push(&q.heap, gqlSegmentWorkItem{idx: idx, priority: priority, order: q.orderSeq})
	q.orderSeq++
	q.mu.Unlock()
	q.cond.Signal()
}

func (q *gqlSegmentWorkQueue) acquire(ctx context.Context) (int, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	for {
		if q.heap.Len() > 0 {
			item := heap.Pop(&q.heap).(gqlSegmentWorkItem)
			q.inFlight++
			return item.idx, true
		}
		if q.inFlight == 0 {
			return -1, false
		}
		q.mu.Unlock()
		select {
		case <-ctx.Done():
			q.mu.Lock()
			return -1, false
		case <-time.After(50 * time.Millisecond):
		}
		q.mu.Lock()
	}
}

func (q *gqlSegmentWorkQueue) release() {
	q.mu.Lock()
	q.inFlight--
	if q.inFlight == 0 {
		q.cond.Broadcast()
	}
	q.mu.Unlock()
}

func segmentOverlapsRange(seg gqlSegmentProgress, r gqlTimeRange) bool {
	return seg.StartSec < r.EndSec && seg.EndSec > r.StartSec
}

func segmentSchedulePriority(seg gqlSegmentProgress, hints gqlFetchScheduleHints) int {
	for _, r := range hints.MomentWindows {
		if segmentOverlapsRange(seg, r) {
			return gqlSegPriorityMoment
		}
	}
	for _, r := range hints.GameRanges {
		if segmentOverlapsRange(seg, r) {
			return gqlSegPriorityGame
		}
	}
	edge := hints.EdgePrioritySec
	if edge <= 0 {
		edge = gqlPriorityEdgeSecondsDefault
	}
	if seg.StartSec < edge {
		return gqlSegPriorityEdge
	}
	if hints.VodDurationSec > 0 && seg.EndSec > hints.VodDurationSec-edge {
		return gqlSegPriorityEdge
	}
	return gqlSegPriorityBackground
}

func buildGQLScheduleHints(vodDurationSec, edgeSec int, momentWindows []gqlTimeRange, gameSegments []GameSegment) gqlFetchScheduleHints {
	hints := gqlFetchScheduleHints{
		MomentWindows:   momentWindows,
		VodDurationSec:  vodDurationSec,
		EdgePrioritySec: edgeSec,
	}
	if hints.EdgePrioritySec <= 0 {
		hints.EdgePrioritySec = gqlPriorityEdgeSecondsDefault
	}
	for _, g := range gameSegments {
		if g.DurationSeconds <= 0 {
			continue
		}
		hints.GameRanges = append(hints.GameRanges, gqlTimeRange{
			StartSec: g.OffsetSeconds,
			EndSec:   g.OffsetSeconds + g.DurationSeconds,
		})
	}
	return hints
}

func medianPositiveInt(values []int) int {
	if len(values) == 0 {
		return 0
	}
	sorted := append([]int(nil), values...)
	sort.Ints(sorted)
	return sorted[len(sorted)/2]
}

func mergeOverlappingGQLRanges(ranges []gqlTimeRange) []gqlTimeRange {
	if len(ranges) <= 1 {
		return ranges
	}
	sort.Slice(ranges, func(i, j int) bool {
		if ranges[i].StartSec != ranges[j].StartSec {
			return ranges[i].StartSec < ranges[j].StartSec
		}
		return ranges[i].EndSec < ranges[j].EndSec
	})
	merged := []gqlTimeRange{ranges[0]}
	for _, r := range ranges[1:] {
		last := &merged[len(merged)-1]
		if r.StartSec <= last.EndSec {
			if r.EndSec > last.EndSec {
				last.EndSec = r.EndSec
			}
			continue
		}
		merged = append(merged, r)
	}
	return merged
}

// buildGQLMomentWindowsFromViewerPoints returns high-priority fetch windows around
// top viewer spikes (± buffer). Used during sync when chat rollups are not ready yet.
func buildGQLMomentWindowsFromViewerPoints(points []parsedViewerPoint) []gqlTimeRange {
	if len(points) < 3 {
		return nil
	}
	values := make([]int, 0, len(points))
	for _, p := range points {
		if p.Viewers > 0 {
			values = append(values, p.Viewers)
		}
	}
	if len(values) < 3 {
		return nil
	}
	baseline := medianPositiveInt(values)
	if baseline <= 0 {
		return nil
	}
	threshold := int(float64(baseline) * gqlMomentSpikeMultiplier)

	type spike struct {
		offset  int
		viewers int
	}
	spikes := make([]spike, 0, len(points))
	for _, p := range points {
		if p.Viewers >= threshold {
			spikes = append(spikes, spike{offset: p.OffsetSeconds, viewers: p.Viewers})
		}
	}
	if len(spikes) == 0 {
		return nil
	}
	sort.Slice(spikes, func(i, j int) bool {
		if spikes[i].viewers != spikes[j].viewers {
			return spikes[i].viewers > spikes[j].viewers
		}
		return spikes[i].offset < spikes[j].offset
	})
	if len(spikes) > gqlMaxMomentWindows {
		spikes = spikes[:gqlMaxMomentWindows]
	}

	windows := make([]gqlTimeRange, 0, len(spikes))
	for _, sp := range spikes {
		start := sp.offset - gqlMomentWindowBufferSec
		if start < 0 {
			start = 0
		}
		windows = append(windows, gqlTimeRange{
			StartSec: start,
			EndSec:   sp.offset + gqlMomentWindowBufferSec,
		})
	}
	return mergeOverlappingGQLRanges(windows)
}

func hotSegmentSplitReason(pageCount, pageThreshold int, recent []gqlPageSample, slowAdvanceSec, slowAdvancePages, commentsPerPage int) string {
	if pageThreshold > 0 && pageCount >= pageThreshold {
		return "page_threshold"
	}
	if slowAdvancePages <= 0 || len(recent) < slowAdvancePages {
		return ""
	}
	window := recent[len(recent)-slowAdvancePages:]
	totalAdvance := 0
	totalComments := 0
	for _, sample := range window {
		totalAdvance += sample.offsetAdvance
		totalComments += sample.commentCount
	}
	if slowAdvanceSec > 0 && totalAdvance/slowAdvancePages < slowAdvanceSec {
		return "slow_advance"
	}
	if commentsPerPage > 0 && totalComments/slowAdvancePages >= commentsPerPage {
		return "comments_per_page"
	}
	return ""
}

func impliedAvgSegmentSecFromSegments(vodDurationSec, effectiveSegmentSec, segmentCount int) float64 {
	if vodDurationSec > 0 && segmentCount > 0 {
		return float64(vodDurationSec) / float64(segmentCount)
	}
	return float64(effectiveSegmentSec)
}

func vodChatAlignSeconds(streamStart, vodCreated time.Time) int {
	if streamStart.IsZero() || vodCreated.IsZero() {
		return 0
	}
	streamStart = streamStart.UTC().Truncate(time.Minute)
	vodCreated = vodCreated.UTC().Truncate(time.Minute)
	delta := int(vodCreated.Sub(streamStart).Seconds())
	if delta < 0 {
		return 0
	}
	return delta
}

type vodCommentsFetchState struct {
	streamID            string
	login               string
	videoID             string
	vodDurationSec      int
	chatAlignSec        int
	fetchMode           string
	concurrency         int
	commentsMap         map[int][]string
	deduper             gqlCommentDeduper
	shardedComments     gqlCommentsMap
	commentsCount       *atomic.Int64
	segmentsMu          sync.Mutex
	segments            *[]*gqlSegmentProgress
	commentsMapMu       sync.Mutex
	coord               *gqlRateCoordinator
	pages               *atomic.Int64
	rollupStartFn       func() time.Time
	rollupCache         *chatRollupCache
	onSegmentDone       func(seg gqlSegmentProgress)
	report              func(force bool)
	saveParallel        func(force bool)
	flushRollups        func(force bool)
	hotPageThreshold    int
	hotSlowAdvanceSec   int
	hotSlowAdvancePages int
	hotCommentsPerPage  int
	initialSegmentCount int
	effectiveSegmentSec int
	estimatedComments   int64
	scheduleHints       gqlFetchScheduleHints
	workQueue           *gqlSegmentWorkQueue
	requestStats        *gqlRequestStats
	rollupFlushSegments int
	rollupFlushInterval time.Duration

	sink          chatreplay.Sink
	sanitizeCfg   chatreplay.SanitizeConfig
	enricher      *enrich.Enricher
	replayEnabled bool

	cleanupPhaseMu      sync.Mutex
	cleanupPhase        string
	commentsSaved       atomic.Int64
	hotSplitsTotal      atomic.Int64
	hotSplitsByReasonMu sync.Mutex
	hotSplitsByReason   map[string]int
	lastHotSplitReason  string
	autoClosedTotal     atomic.Int64
	workerPagesMu       sync.Mutex
	workerPages         map[string]int
	pendingRollupMu     sync.Mutex
	pendingRollupSegs   []gqlSegmentProgress
	lastPendingRollupAt time.Time

	goldLedger *goldVODFetchLedger
}

func stateSnapshotSegments(segments []*gqlSegmentProgress) []gqlSegmentProgress {
	if len(segments) == 0 {
		return nil
	}
	out := make([]gqlSegmentProgress, 0, len(segments))
	for _, seg := range segments {
		if seg == nil {
			continue
		}
		out = append(out, *seg)
	}
	return out
}

func gqlSegmentPointers(segs []gqlSegmentProgress) []*gqlSegmentProgress {
	ptrs := make([]*gqlSegmentProgress, 0, len(segs)*4+16)
	for i := range segs {
		seg := segs[i]
		ptrs = append(ptrs, &seg)
	}
	return ptrs
}

func (st *vodCommentsFetchState) segmentAt(idx int) (*gqlSegmentProgress, bool) {
	if st == nil || st.segments == nil {
		return nil, false
	}
	st.segmentsMu.Lock()
	defer st.segmentsMu.Unlock()
	if idx < 0 || idx >= len(*st.segments) || (*st.segments)[idx] == nil {
		return nil, false
	}
	return (*st.segments)[idx], true
}

func (st *vodCommentsFetchState) appendSegment(seg gqlSegmentProgress) int {
	st.segmentsMu.Lock()
	defer st.segmentsMu.Unlock()
	segCopy := seg
	*st.segments = append(*st.segments, &segCopy)
	return len(*st.segments) - 1
}

func (st *vodCommentsFetchState) snapshotSegments() []gqlSegmentProgress {
	if st == nil || st.segments == nil {
		return nil
	}
	st.segmentsMu.Lock()
	defer st.segmentsMu.Unlock()
	out := make([]gqlSegmentProgress, 0, len(*st.segments))
	for _, seg := range *st.segments {
		if seg == nil {
			continue
		}
		out = append(out, *seg)
	}
	return out
}

func (st *vodCommentsFetchState) recordHotSplit(reason string) {
	if st == nil || strings.TrimSpace(reason) == "" {
		return
	}
	reason = strings.TrimSpace(reason)
	st.hotSplitsTotal.Add(1)
	st.hotSplitsByReasonMu.Lock()
	if st.hotSplitsByReason == nil {
		st.hotSplitsByReason = make(map[string]int)
	}
	st.hotSplitsByReason[reason]++
	st.lastHotSplitReason = reason
	st.hotSplitsByReasonMu.Unlock()
}

func (st *vodCommentsFetchState) snapshotHotSplitsByReason() map[string]int {
	if st == nil {
		return nil
	}
	st.hotSplitsByReasonMu.Lock()
	defer st.hotSplitsByReasonMu.Unlock()
	if len(st.hotSplitsByReason) == 0 {
		return nil
	}
	out := make(map[string]int, len(st.hotSplitsByReason))
	for reason, count := range st.hotSplitsByReason {
		out[reason] = count
	}
	return out
}

func (st *vodCommentsFetchState) latestHotSplitReason() string {
	if st == nil {
		return ""
	}
	st.hotSplitsByReasonMu.Lock()
	defer st.hotSplitsByReasonMu.Unlock()
	return st.lastHotSplitReason
}

func (st *vodCommentsFetchState) recordAutoClose(_ string) {
	if st == nil {
		return
	}
	st.autoClosedTotal.Add(1)
}

func (st *vodCommentsFetchState) recordWorkerPage(worker string) {
	if st == nil || strings.TrimSpace(worker) == "" {
		return
	}
	st.workerPagesMu.Lock()
	if st.workerPages == nil {
		st.workerPages = make(map[string]int)
	}
	st.workerPages[worker]++
	st.workerPagesMu.Unlock()
}

func (st *vodCommentsFetchState) snapshotWorkerPages() map[string]int {
	if st == nil {
		return nil
	}
	st.workerPagesMu.Lock()
	defer st.workerPagesMu.Unlock()
	if len(st.workerPages) == 0 {
		return nil
	}
	out := make(map[string]int, len(st.workerPages))
	for worker, count := range st.workerPages {
		out[worker] = count
	}
	return out
}

func (st *vodCommentsFetchState) queueRollupSegment(seg gqlSegmentProgress) {
	if st == nil {
		return
	}
	st.pendingRollupMu.Lock()
	if len(st.pendingRollupSegs) == 0 {
		st.lastPendingRollupAt = time.Now()
	}
	st.pendingRollupSegs = append(st.pendingRollupSegs, seg)
	st.pendingRollupMu.Unlock()
}

func (st *vodCommentsFetchState) drainPendingRollupSegments(force bool) []gqlSegmentProgress {
	if st == nil {
		return nil
	}
	st.pendingRollupMu.Lock()
	defer st.pendingRollupMu.Unlock()
	if len(st.pendingRollupSegs) == 0 {
		return nil
	}
	if !force {
		if st.rollupFlushSegments > 0 && len(st.pendingRollupSegs) >= st.rollupFlushSegments {
			// flush now
		} else if st.rollupFlushInterval > 0 && time.Since(st.lastPendingRollupAt) >= st.rollupFlushInterval {
			// flush now
		} else {
			return nil
		}
	}
	out := append([]gqlSegmentProgress(nil), st.pendingRollupSegs...)
	st.pendingRollupSegs = nil
	st.lastPendingRollupAt = time.Now()
	return out
}

func (st *vodCommentsFetchState) enqueueIncompleteSegments() int {
	if st == nil || st.workQueue == nil {
		return 0
	}
	var items []gqlSegmentWorkItem
	st.segmentsMu.Lock()
	segments := append([]*gqlSegmentProgress(nil), (*st.segments)...)
	st.segmentsMu.Unlock()
	for i, seg := range segments {
		if seg == nil || seg.Done {
			continue
		}
		if tryAutoCloseSegment(st, seg) {
			continue
		}
		items = append(items, gqlSegmentWorkItem{
			idx:      i,
			priority: segmentSchedulePriority(*seg, st.scheduleHints),
		})
	}
	for _, item := range items {
		st.workQueue.push(item.idx, item.priority)
	}
	return len(items)
}

func (st *vodCommentsFetchState) incompleteSegmentCount() int {
	if st == nil || st.segments == nil {
		return 0
	}
	st.segmentsMu.Lock()
	defer st.segmentsMu.Unlock()
	incomplete := 0
	for _, seg := range *st.segments {
		if seg != nil && !seg.Done {
			incomplete++
		}
	}
	return incomplete
}

func segmentAlignedMinuteBounds(seg gqlSegmentProgress, chatAlignSec int) (startMinute, endMinute int) {
	startMinute = (seg.StartSec + chatAlignSec) / 60
	if seg.EndSec <= seg.StartSec {
		return startMinute, startMinute
	}
	endMinute = (seg.EndSec - 1 + chatAlignSec) / 60
	if endMinute < startMinute {
		endMinute = startMinute
	}
	return startMinute, endMinute
}

func tryAutoCloseSegment(state *vodCommentsFetchState, seg *gqlSegmentProgress) bool {
	if state == nil || seg == nil || seg.Done {
		return false
	}
	offset := seg.OffsetSec
	if offset < seg.StartSec {
		offset = seg.StartSec
	}
	autoClose := false
	reason := ""
	switch {
	case seg.EndSec <= seg.StartSec:
		autoClose = true
		reason = "empty_segment"
	case state.vodDurationSec > 0 && seg.EndSec >= state.vodDurationSec:
		autoClose = offset > seg.EndSec
		reason = "tail_past_end"
	default:
		autoClose = offset >= seg.EndSec
		reason = "segment_boundary"
	}
	if !autoClose {
		return false
	}
	metrics.AnalyticsVODGQLSegments.WithLabelValues("auto_closed").Inc()
	metrics.AnalyticsVODGQLAutoCloseTotal.WithLabelValues(reason).Inc()
	state.recordAutoClose(reason)
	state.finishSegment(seg, maxInt(offset, seg.EndSec))
	return true
}

func (st *vodCommentsFetchState) finishSegment(seg *gqlSegmentProgress, offset int) {
	wasDone := seg.Done
	if offset < seg.StartSec {
		offset = seg.StartSec
	}
	if seg.EndSec > seg.StartSec && offset > seg.EndSec {
		offset = seg.EndSec
	}
	if st != nil && st.vodDurationSec > 0 && offset > st.vodDurationSec {
		offset = st.vodDurationSec
	}
	seg.Done = true
	seg.OffsetSec = offset
	if !wasDone {
		metrics.AnalyticsVODGQLSegments.WithLabelValues("completed").Inc()
	}
	if st.goldLedger != nil && !wasDone {
		st.goldLedger.completeSegment(*seg, 0)
	}
	if st.commentsMap != nil || st.onSegmentDone != nil {
		st.commentsMapMu.Lock()
		defer st.commentsMapMu.Unlock()
	}
	if st.commentsMap != nil {
		startMinute, endMinute := segmentAlignedMinuteBounds(*seg, st.chatAlignSec)
		st.shardedComments.extractMinuteRangeInto(st.commentsMap, startMinute, endMinute)
	}
	if st.onSegmentDone != nil {
		st.onSegmentDone(*seg)
	}
	if st.saveParallel != nil {
		st.saveParallel(true)
	}
}

func chatProgressFromSegments(segs []gqlSegmentProgress) (done, timelineSec int) {
	for _, seg := range segs {
		if seg.Done {
			done++
		}
		if seg.OffsetSec > timelineSec {
			timelineSec = seg.OffsetSec
		}
	}
	return done, timelineSec
}

func (st *vodCommentsFetchState) publishProgress(force bool) {
	if st.report == nil {
		return
	}
	st.report(force)
}

func (s *SyncService) flushPendingRollupSegments(ctx context.Context, state *vodCommentsFetchState, force bool) {
	if s == nil || state == nil || !s.vodGQLIncrementalDB {
		return
	}
	segs := state.drainPendingRollupSegments(force)
	if len(segs) == 0 {
		return
	}
	if err := s.patchChatRollupsForSegments(ctx, state.streamID, state.login, state.rollupStartFn, state.commentsMap, segs, state.chatAlignSec, state.rollupCache); err != nil {
		s.log.Warn("incremental chat rollup flush failed",
			"stream_id", state.streamID,
			"segments", len(segs),
			"err", err,
		)
	}
}

const vodCommentsCheckpointMinGap = 2 * time.Second

func effectiveGQLSegmentSeconds(configured, denseSec, quietSec, vodDurationSec int, estimatedComments int64) int {
	if configured <= 0 {
		configured = 600
	}
	if denseSec <= 0 {
		denseSec = vodGQLSegmentDenseVOD
	}
	if quietSec <= 0 {
		quietSec = 900
	}
	if estimatedComments >= int64(vodCommentsMaxCount*9/10) {
		return minInt(configured, denseSec)
	}
	if estimatedComments >= vodGQLVeryDenseCommentsThreshold {
		return minInt(configured, denseSec)
	}
	if estimatedComments >= vodGQLDenseCommentsThreshold {
		return minInt(configured, denseSec)
	}
	if vodDurationSec > 0 && estimatedComments > 0 {
		perHour := estimatedComments * 3600 / int64(vodDurationSec)
		if perHour >= vodGQLCommentsPerHourVeryDense {
			return minInt(configured, denseSec)
		}
		if perHour >= vodGQLCommentsPerHourDense {
			return minInt(configured, denseSec)
		}
		return quietSec
	}
	if vodDurationSec >= vodGQLLargeVODDurationSec {
		return quietSec
	}
	return configured
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func (st *vodCommentsFetchState) mergeEdge(edge GQLCommentEdge, segmentStart, segmentEnd int) (pastSegment bool) {
	offset := edge.Node.ContentOffsetSeconds
	if st.vodDurationSec > 0 && segmentEnd < st.vodDurationSec && offset >= segmentEnd {
		return true
	}
	if offset > segmentEnd {
		return true
	}
	if offset < segmentStart {
		return false
	}
	commentID := strings.TrimSpace(edge.Node.ID)
	if commentID != "" && st.deduper.markSeen(commentID) {
		return false
	}
	text := gqlCommentText(edge.Node.Message)
	if strings.TrimSpace(text) == "" {
		return false
	}
	adjOffset := edge.Node.ContentOffsetSeconds + st.chatAlignSec
	if adjOffset < 0 {
		return false
	}
	minOffset := adjOffset / 60
	st.shardedComments.append(minOffset, text)
	st.commentsCount.Add(1)
	if st.replayEnabled {
		st.addReplayMessage(edge, text, adjOffset, minOffset)
	}
	return false
}

// addReplayMessage builds and buffers a sanitized VOD chat message for the
// chat-replay sink. It is invoked from the shared per-edge path (mergeEdge),
// so it is reached by both the parallel workers and the parallel→serial
// segment fallback, all sharing the same gqlCommentDeduper.
func (st *vodCommentsFetchState) addReplayMessage(edge GQLCommentEdge, text string, adjOffset, minOffset int) {
	if !st.replayEnabled || st.sink == nil {
		return
	}
	msg, ok := buildReplayMessage(st.streamID, st.login, edge, text, adjOffset, minOffset, st.sanitizeCfg, st.enricher, st.rollupStartFn)
	if !ok {
		return
	}
	st.sink.Add(msg)
}

// buildReplayMessage maps a GQL comment edge into a sanitized, privacy-
// preserving VODChatMessage. The raw Twitch user id is converted to an HMAC
// digest inside BuildMessage and is never stored. Emote fragments are derived
// from the same enricher tokenization the rollup path uses so stored fragments
// reference local emote-service ids, not raw provider ids. MinuteTS is aligned
// to the rollup minute bucket.
func buildReplayMessage(streamID, login string, edge GQLCommentEdge, text string, adjOffset, minOffset int, cfg chatreplay.SanitizeConfig, enricher *enrich.Enricher, rollupStartFn func() time.Time) (chatreplay.VODChatMessage, bool) {
	commentID := strings.TrimSpace(edge.Node.ID)
	if commentID == "" {
		return chatreplay.VODChatMessage{}, false
	}
	raw := chatreplay.RawComment{
		StreamID:      streamID,
		MessageID:     commentID,
		Text:          text,
		OffsetSeconds: adjOffset,
		EmoteFrags:    replayEmoteFrags(enricher, login, text),
	}
	if edge.Node.Commenter != nil {
		raw.DisplayName = edge.Node.Commenter.DisplayName
		if raw.DisplayName == "" {
			raw.DisplayName = edge.Node.Commenter.Login
		}
		raw.SenderUserID = edge.Node.Commenter.ID
	}
	msg, ok := chatreplay.BuildMessage(raw, cfg)
	if !ok {
		return chatreplay.VODChatMessage{}, false
	}
	if rollupStartFn != nil {
		if rollupStart := rollupStartFn(); !rollupStart.IsZero() {
			msg.MinuteTS = rollupStart.Add(time.Duration(minOffset) * time.Minute)
		}
	}
	return msg, true
}

// replayEmoteFrags tokenizes message text into local emote fragments using the
// shared enricher dictionary (loaded via preloadChannelEmotes before sync), so
// replay rows carry the same local emote-service ids as the rollup path. The
// enricher Trie is read-only here and safe for concurrent use by workers.
func replayEmoteFrags(enricher *enrich.Enricher, login, text string) []chatreplay.EmoteFrag {
	if enricher == nil || login == "" || strings.TrimSpace(text) == "" {
		return nil
	}
	frags := enricher.Tokenize(login, text, nil)
	var out []chatreplay.EmoteFrag
	for _, f := range frags {
		if f.T != "emote" {
			continue
		}
		url := f.U
		if url == "" && f.ID != "" {
			url = emoteimage.URL(f.Provider, f.ID, "1x")
		}
		out = append(out, chatreplay.EmoteFrag{
			Name:     f.C,
			ID:       f.ID,
			Provider: f.Provider,
			ImageURL: url,
		})
	}
	return out
}

func (s *SyncService) estimatedStreamComments(ctx context.Context, streamID string) int64 {
	rec, err := s.store.StreamByID(ctx, streamID)
	if err != nil || rec == nil {
		return 0
	}
	return rec.ChatMessages
}

func (s *SyncService) gqlScheduleHintsForStream(ctx context.Context, streamID string, vodDurationSec int, scrapedGames []scrapedGame, viewerPoints []parsedViewerPoint) gqlFetchScheduleHints {
	momentWindows := buildGQLMomentWindowsFromViewerPoints(viewerPoints)
	if len(momentWindows) == 0 && streamID != "" {
		if rollups, err := s.store.RollupsByStream(ctx, streamID); err == nil {
			momentWindows = buildGQLMomentWindowsFromViewerPoints(rollupsToViewerPoints(rollups))
		}
	}
	var gameSegs []GameSegment
	if len(scrapedGames) > 0 {
		gameSegs = buildGameSegments(scrapedGames, vodDurationSec)
	} else if stored, err := s.store.GetGameSegments(ctx, streamID); err == nil && len(stored) > 0 {
		gameSegs = stored
	}
	return buildGQLScheduleHints(vodDurationSec, s.vodGQLPriorityEdgeSeconds, momentWindows, gameSegs)
}

func (s *SyncService) fetchVODComments(ctx context.Context, streamID, login, videoID string, commentsMap map[int][]string, vodDurationSec, chatAlignSec int, rollupStartFn func() time.Time, chatCache *chatRollupCache, scheduleHints gqlFetchScheduleHints) error {
	estimatedComments := s.estimatedStreamComments(ctx, streamID)
	fetchMode := "serial"
	concurrency := 1
	segmentsTotal := 1
	segmentSec := effectiveGQLSegmentSeconds(s.vodGQLSegmentSeconds, s.vodGQLDenseSegmentSeconds, s.vodGQLQuietSegmentSeconds, vodDurationSec, estimatedComments)
	if s.vodGQLConcurrency > 1 {
		if vodDurationSec <= 0 && s.helix != nil && s.helix.Enabled() {
			if d, err := s.helix.VideoDurationSeconds(ctx, videoID); err == nil && d > 0 {
				vodDurationSec = d
				segmentSec = effectiveGQLSegmentSeconds(s.vodGQLSegmentSeconds, s.vodGQLDenseSegmentSeconds, s.vodGQLQuietSegmentSeconds, vodDurationSec, estimatedComments)
			}
		}
		if vodDurationSec > 0 {
			fetchMode = "parallel"
			concurrency = s.vodGQLConcurrency
			segmentsTotal = len(buildGQLSegments(vodDurationSec, segmentSec))
		}
	}
	s.updateChatProgress(ctx, streamID, func(cp *SyncChatProgress) {
		cp.Active = true
		cp.IndexPhase = "fetching"
		cp.VodID = videoID
		cp.FetchMode = fetchMode
		cp.Concurrency = concurrency
		cp.EffectiveSegmentSec = segmentSec
		cp.InitialSegments = segmentsTotal
		cp.HotSplits = 0
		cp.HotSegmentSplitReason = ""
		cp.AutoClosedSegments = 0
		cp.VodDurationSec = vodDurationSec
		cp.SegmentsTotal = segmentsTotal
		cp.SummaryRefreshDeferred = s.vodGQLIncrementalDB && s.vodGQLDeferSummaryRefresh
		cp.Message = fmt.Sprintf("Loading VOD %s chat via Twitch GQL (%s)", videoID, fetchMode)
	}, true)
	defer s.updateChatProgress(ctx, streamID, func(cp *SyncChatProgress) {
		cp.Active = false
	}, true)

	if s.vodGQLConcurrency <= 1 {
		return s.fetchVODCommentsSerial(ctx, streamID, login, videoID, commentsMap, vodDurationSec, chatAlignSec, rollupStartFn)
	}
	if vodDurationSec <= 0 {
		vodDurationSec = 86400
	}
	return s.fetchVODCommentsParallel(ctx, streamID, login, videoID, commentsMap, vodDurationSec, chatAlignSec, rollupStartFn, chatCache, scheduleHints)
}

func (s *SyncService) fetchVODCommentsSerial(ctx context.Context, streamID, login, videoID string, commentsMap map[int][]string, vodDurationSec, chatAlignSec int, rollupStartFn func() time.Time) error {
	useCursor := false
	cursorFailed := false
	nextCursor := ""
	contentOffsetSeconds := 0
	commentsCount := 0
	commentsSaved := 0
	lastOffset := 0
	gqlPages := 0
	seenCommentIDs := make(map[string]struct{})

	estimatedComments := s.estimatedStreamComments(ctx, streamID)
	segmentSec := effectiveGQLSegmentSeconds(s.vodGQLSegmentSeconds, s.vodGQLDenseSegmentSeconds, s.vodGQLQuietSegmentSeconds, vodDurationSec, estimatedComments)

	if cp, err := s.store.GetSyncCheckpoint(ctx, streamID, videoID); err != nil {
		s.log.Warn("failed to load sync checkpoint", "stream_id", streamID, "video_id", videoID, "err", err)
	} else if cp != nil && cp.FetchMode != "parallel" {
		s.log.Info("resuming VOD comment fetch from checkpoint",
			"stream_id", streamID,
			"video_id", videoID,
			"offset", cp.OffsetSeconds,
			"comments_fetched", cp.CommentsFetched,
		)
		commentsCount = cp.CommentsFetched
		lastOffset = cp.OffsetSeconds
		contentOffsetSeconds = cp.OffsetSeconds
		if strings.TrimSpace(cp.Cursor) != "" {
			useCursor = true
			nextCursor = cp.Cursor
		}
	}

	reportProgress := func(force bool) {
		if !force && (commentsCount == 0 || commentsCount%vodCommentsProgressEvery != 0) {
			return
		}
		count := commentsCount
		timeline := lastOffset
		pages := gqlPages
		s.updateChatProgress(ctx, streamID, func(cp *SyncChatProgress) {
			cp.Active = true
			cp.IndexPhase = "fetching"
			cp.VodID = videoID
			cp.FetchMode = "serial"
			cp.Concurrency = 1
			cp.EffectiveSegmentSec = segmentSec
			cp.InitialSegments = 1
			cp.HotSplits = 0
			cp.HotSegmentSplitReason = ""
			cp.AutoClosedSegments = 0
			cp.SegmentsTotal = 1
			cp.SegmentsDone = 0
			cp.CommentsFetched = count
			cp.CommentsSaved = commentsSaved
			cp.TimelineSec = timeline
			cp.VodDurationSec = vodDurationSec
			cp.SummaryRefreshDeferred = false
			cp.GQLPages = pages
			cp.Throttled = false
			cp.Message = fmt.Sprintf("VOD %s · serial · %s", videoID, formatChatTimelineMessage(timeline, vodDurationSec, count, pages, 0, 1))
		})
	}

	saveCheckpoint := func() {
		cp := SyncCheckpoint{
			StreamID:        streamID,
			VideoID:         videoID,
			Cursor:          nextCursor,
			OffsetSeconds:   contentOffsetSeconds,
			CommentsFetched: commentsCount,
			FetchMode:       "serial",
		}
		if !useCursor {
			cp.Cursor = ""
			cp.OffsetSeconds = contentOffsetSeconds
		} else {
			cp.OffsetSeconds = lastOffset
		}
		if err := s.store.UpsertSyncCheckpoint(ctx, cp); err != nil {
			s.log.Warn("failed to save sync checkpoint", "stream_id", streamID, "err", err)
		}
		if s.chatReplayEnabled && s.chatReplaySink != nil {
			inserted, err := s.chatReplaySink.Flush(ctx)
			if err != nil {
				s.log.Warn("chat replay flush failed at checkpoint", "stream_id", streamID, "err", err)
			} else {
				commentsSaved += inserted
			}
		}
	}

	for {
		reqBody := buildVideoCommentsGQLRequest(videoID, gqlVideoCommentsSHA256, useCursor, contentOffsetSeconds, nextCursor)
		gqlResp, err := s.postGQLVideoComments(ctx, reqBody, nil, nil)
		gqlPages++
		if err != nil {
			saveCheckpoint()
			return err
		}
		if isGQLIntegrityError(gqlResp) {
			if useCursor {
				s.log.Warn("GQL cursor pagination failed integrity check; falling back to offset",
					"stream_id", streamID,
					"offset", lastOffset+1,
				)
				useCursor = false
				cursorFailed = true
				nextCursor = ""
				contentOffsetSeconds = lastOffset + 1
				continue
			}
			saveCheckpoint()
			return fmt.Errorf("gql video comments integrity error")
		}
		if len(gqlResp.Errors) > 0 {
			saveCheckpoint()
			return fmt.Errorf("gql video comments error: %s", gqlResp.Errors[0].Message)
		}
		if gqlResp.Data.Video == nil || gqlResp.Data.Video.Comments == nil {
			break
		}

		commentsNode := gqlResp.Data.Video.Comments
		edges := commentsNode.Edges
		if len(edges) == 0 {
			break
		}

		for _, edge := range edges {
			if commentID := strings.TrimSpace(edge.Node.ID); commentID != "" {
				if _, seen := seenCommentIDs[commentID]; seen {
					continue
				}
				seenCommentIDs[commentID] = struct{}{}
			}
			adjOffset := edge.Node.ContentOffsetSeconds + chatAlignSec
			if adjOffset < 0 {
				continue
			}
			minOffset := adjOffset / 60
			text := gqlCommentText(edge.Node.Message)
			if strings.TrimSpace(text) == "" {
				continue
			}
			commentsMap[minOffset] = append(commentsMap[minOffset], text)
			commentsCount++
			lastOffset = edge.Node.ContentOffsetSeconds
			if s.chatReplayEnabled && s.chatReplaySink != nil {
				if msg, ok := buildReplayMessage(streamID, login, edge, text, adjOffset, minOffset, s.chatReplayCfg, s.enricher, rollupStartFn); ok {
					s.chatReplaySink.Add(msg)
				}
			}
		}

		reportProgress(false)

		if commentsCount >= vodCommentsMaxCount {
			s.log.Warn("safety threshold of 200,000 comments reached; truncating comments paging")
			break
		}
		if !commentsNode.PageInfo.HasNextPage {
			break
		}

		lastEdge := edges[len(edges)-1]
		if !cursorFailed && strings.TrimSpace(lastEdge.Cursor) != "" {
			useCursor = true
			nextCursor = lastEdge.Cursor
		} else {
			useCursor = false
			nextCursor = ""
			contentOffsetSeconds = lastOffset + 1
		}
		saveCheckpoint()

		if s.vodGQLPageDelay > 0 {
			time.Sleep(s.vodGQLPageDelay)
		}
	}

	reportProgress(true)
	if s.chatReplayEnabled && s.chatReplaySink != nil {
		inserted, err := s.chatReplaySink.Flush(ctx)
		if err != nil {
			s.log.Warn("chat replay final flush failed", "stream_id", streamID, "mode", "serial", "err", err)
		} else {
			commentsSaved += inserted
		}
	}
	reportProgress(true)
	if err := s.store.DeleteSyncCheckpoint(ctx, streamID, videoID); err != nil {
		s.log.Warn("failed to clear sync checkpoint", "stream_id", streamID, "err", err)
	}
	s.log.Info("finished VOD comments paging", "total_comments", commentsCount, "mode", "serial", "cursor_mode", !cursorFailed)
	return nil
}

func buildGQLSegments(durationSec, segmentSec int) []gqlSegmentProgress {
	if segmentSec <= 0 {
		segmentSec = 600
	}
	if durationSec <= 0 {
		durationSec = segmentSec
	}
	var segments []gqlSegmentProgress
	for start := 0; start < durationSec; start += segmentSec {
		end := start + segmentSec
		if end > durationSec {
			end = durationSec
		}
		segments = append(segments, gqlSegmentProgress{
			StartSec:  start,
			EndSec:    end,
			OffsetSec: start,
		})
	}
	if len(segments) == 0 {
		segments = append(segments, gqlSegmentProgress{StartSec: 0, EndSec: durationSec, OffsetSec: 0})
	}
	return segments
}

func formatVodClock(sec int) string {
	if sec < 0 {
		sec = 0
	}
	h := sec / 3600
	m := (sec % 3600) / 60
	s := sec % 60
	if h > 0 {
		return fmt.Sprintf("%dh %02dm", h, m)
	}
	if m > 0 {
		return fmt.Sprintf("%dm %02ds", m, s)
	}
	return fmt.Sprintf("%ds", s)
}

func formatChatTimelineMessage(timelineSec, vodDurationSec, comments, gqlPages, segmentsDone, segmentsTotal int) string {
	parts := []string{fmt.Sprintf("%s comments", formatIntComma(comments))}
	if gqlPages > 0 {
		parts = append(parts, fmt.Sprintf("%s GQL pages", formatIntComma(gqlPages)))
	}
	if segmentsTotal > 1 {
		parts = append([]string{fmt.Sprintf("segment %d/%d", segmentsDone, segmentsTotal)}, parts...)
	}
	if vodDurationSec > 0 {
		parts = append(parts, fmt.Sprintf("timeline %s / %s", formatVodClock(timelineSec), formatVodClock(vodDurationSec)))
	} else if timelineSec > 0 {
		parts = append(parts, fmt.Sprintf("at %s", formatVodClock(timelineSec)))
	}
	return strings.Join(parts, " · ")
}

func formatIntComma(n int) string {
	return fmt.Sprintf("%d", n)
}

func (s *SyncService) fetchVODCommentsParallel(ctx context.Context, streamID, login, videoID string, commentsMap map[int][]string, vodDurationSec, chatAlignSec int, rollupStartFn func() time.Time, chatCache *chatRollupCache, scheduleHints gqlFetchScheduleHints) error {
	estimatedComments := s.estimatedStreamComments(ctx, streamID)
	segmentSec := effectiveGQLSegmentSeconds(s.vodGQLSegmentSeconds, s.vodGQLDenseSegmentSeconds, s.vodGQLQuietSegmentSeconds, vodDurationSec, estimatedComments)
	segments := gqlSegmentPointers(buildGQLSegments(vodDurationSec, segmentSec))
	var commentsCount atomic.Int64
	hotThreshold := s.vodGQLHotSegmentPageThreshold
	if hotThreshold <= 0 {
		hotThreshold = vodGQLHotSegmentPageThreshold
	}
	slowAdvanceSec := s.vodGQLHotSlowAdvanceSec
	if slowAdvanceSec <= 0 {
		slowAdvanceSec = vodGQLHotSlowAdvanceSecDefault
	}
	slowAdvancePages := s.vodGQLHotSlowAdvancePages
	if slowAdvancePages <= 0 {
		slowAdvancePages = vodGQLHotSlowAdvancePagesDefault
	}
	hotCommentsPerPage := s.vodGQLHotCommentsPerPage
	if hotCommentsPerPage <= 0 {
		hotCommentsPerPage = vodGQLHotCommentsPerPageDefault
	}
	if scheduleHints.VodDurationSec <= 0 {
		scheduleHints.VodDurationSec = vodDurationSec
	}
	if scheduleHints.EdgePrioritySec <= 0 {
		scheduleHints.EdgePrioritySec = s.vodGQLPriorityEdgeSeconds
	}
	if scheduleHints.EdgePrioritySec <= 0 {
		scheduleHints.EdgePrioritySec = gqlPriorityEdgeSecondsDefault
	}

	if cp, err := s.store.GetSyncCheckpoint(ctx, streamID, videoID); err != nil {
		s.log.Warn("failed to load sync checkpoint", "stream_id", streamID, "video_id", videoID, "err", err)
	} else if cp != nil && cp.FetchMode == "parallel" && strings.TrimSpace(cp.SegmentsJSON) != "" {
		var saved gqlParallelCheckpoint
		if jsonErr := json.Unmarshal([]byte(cp.SegmentsJSON), &saved); jsonErr == nil && len(saved.Segments) > 0 {
			segments = gqlSegmentPointers(saved.Segments)
			commentsCount.Store(int64(saved.CommentsFetched))
			s.log.Info("resuming parallel VOD comment fetch",
				"stream_id", streamID,
				"video_id", videoID,
				"segments", len(saved.Segments),
				"comments_fetched", saved.CommentsFetched,
			)
		}
	}

	goldLedger := s.goldVODFetchLedger(ctx, streamID, login, videoID)
	if goldLedger != nil {
		if err := goldLedger.upsertPlansForSegments(stateSnapshotSegments(segments)); err != nil {
			s.log.Warn("gold vod segment plan upsert failed",
				"stream_id", streamID,
				"video_id", videoID,
				"err", err,
			)
		}
	}

	coord := newGQLRateCoordinator(s.vodGQLConcurrencyMin, s.vodGQLConcurrencyMax, s.vodGQLConcurrency)
	var gqlPageCount atomic.Int64
	requestStats := &gqlRequestStats{}
	initialSegments := len(segments)
	s.log.Info("starting parallel VOD comments fetch",
		"stream_id", streamID,
		"video_id", videoID,
		"initial_segments", initialSegments,
		"effective_segment_sec", segmentSec,
		"estimated_comments", estimatedComments,
	)

	saveParallelCheckpoint := func(segs []gqlSegmentProgress) {
		payload, err := json.Marshal(gqlParallelCheckpoint{
			Segments:        segs,
			CommentsFetched: int(commentsCount.Load()),
		})
		if err != nil {
			s.log.Warn("failed to encode parallel checkpoint", "stream_id", streamID, "err", err)
			return
		}
		cp := SyncCheckpoint{
			StreamID:        streamID,
			VideoID:         videoID,
			CommentsFetched: int(commentsCount.Load()),
			SegmentsJSON:    string(payload),
			FetchMode:       "parallel",
		}
		if err := s.store.UpsertSyncCheckpoint(ctx, cp); err != nil {
			s.log.Warn("failed to save parallel sync checkpoint", "stream_id", streamID, "err", err)
		}
	}

	var onSegmentDone func(gqlSegmentProgress)
	workQueue := newGQLSegmentWorkQueue()
	state := &vodCommentsFetchState{
		streamID:            streamID,
		login:               login,
		videoID:             videoID,
		vodDurationSec:      vodDurationSec,
		chatAlignSec:        chatAlignSec,
		fetchMode:           "parallel",
		concurrency:         coord.ActiveConcurrency(),
		commentsMap:         commentsMap,
		commentsCount:       &commentsCount,
		segments:            &segments,
		coord:               coord,
		pages:               &gqlPageCount,
		rollupStartFn:       rollupStartFn,
		rollupCache:         chatCache,
		initialSegmentCount: initialSegments,
		effectiveSegmentSec: segmentSec,
		estimatedComments:   estimatedComments,
		hotPageThreshold:    hotThreshold,
		hotSlowAdvanceSec:   slowAdvanceSec,
		hotSlowAdvancePages: slowAdvancePages,
		hotCommentsPerPage:  hotCommentsPerPage,
		scheduleHints:       scheduleHints,
		workQueue:           workQueue,
		requestStats:        requestStats,
		rollupFlushSegments: s.vodGQLRollupFlushSegments,
		rollupFlushInterval: s.vodGQLRollupFlushInterval,
		sink:                s.chatReplaySink,
		sanitizeCfg:         s.chatReplayCfg,
		enricher:            s.enricher,
		replayEnabled:       s.chatReplayEnabled,
		goldLedger:          goldLedger,
	}
	if s.vodGQLIncrementalDB || s.chatReplayEnabled {
		onSegmentDone = func(seg gqlSegmentProgress) {
			if s.chatReplayEnabled && s.chatReplaySink != nil {
				startMinute, endMinute := segmentAlignedMinuteBounds(seg, chatAlignSec)
				inserted, err := s.chatReplaySink.FlushSegment(ctx, startMinute, endMinute)
				if err != nil {
					s.log.Warn("chat replay segment flush failed", "stream_id", streamID, "segment_start", seg.StartSec, "err", err)
				} else {
					state.commentsSaved.Add(int64(inserted))
				}
			}
			if s.vodGQLIncrementalDB {
				state.queueRollupSegment(seg)
				s.flushPendingRollupSegments(ctx, state, false)
			}
		}
		state.onSegmentDone = onSegmentDone
	}
	state.report = func(force bool) {
		count := int(commentsCount.Load())
		if !force && (count == 0 || count%vodCommentsProgressEvery != 0) {
			return
		}
		segs := state.snapshotSegments()
		segmentsDone, timelineSec := chatProgressFromSegments(segs)
		pageCount := int(gqlPageCount.Load())
		throttled := coord.Paused()
		workers := coord.ActiveConcurrency()
		state.cleanupPhaseMu.Lock()
		cleanupPhase := state.cleanupPhase
		state.cleanupPhaseMu.Unlock()
		s.updateChatProgress(ctx, streamID, func(cp *SyncChatProgress) {
			cp.Active = true
			cp.IndexPhase = "fetching"
			cp.VodID = videoID
			cp.FetchMode = "parallel"
			cp.Concurrency = workers
			cp.EffectiveSegmentSec = state.effectiveSegmentSec
			cp.InitialSegments = state.initialSegmentCount
			cp.SegmentsTotal = len(segs)
			cp.SegmentsDone = segmentsDone
			cp.SegmentsIncomplete = state.incompleteSegmentCount()
			cp.HotSplits = int(state.hotSplitsTotal.Load())
			cp.HotSegmentSplitReason = state.latestHotSplitReason()
			cp.AutoClosedSegments = int(state.autoClosedTotal.Load())
			cp.CleanupPhase = cleanupPhase
			cp.CommentsFetched = count
			cp.CommentsSaved = int(state.commentsSaved.Load())
			cp.TimelineSec = timelineSec
			cp.VodDurationSec = vodDurationSec
			cp.SummaryRefreshDeferred = s.vodGQLIncrementalDB && s.vodGQLDeferSummaryRefresh
			cp.GQLPages = pageCount
			cp.Throttled = throttled
			cp.Message = fmt.Sprintf(
				"VOD %s · parallel · %d workers · %s",
				videoID,
				workers,
				formatChatTimelineMessage(timelineSec, vodDurationSec, count, pageCount, segmentsDone, len(segs)),
			)
		}, false)
	}
	var checkpointMu sync.Mutex
	var lastCheckpointSave time.Time
	state.saveParallel = func(force bool) {
		checkpointMu.Lock()
		if !force && time.Since(lastCheckpointSave) < vodCommentsCheckpointMinGap {
			checkpointMu.Unlock()
			return
		}
		lastCheckpointSave = time.Now()
		checkpointMu.Unlock()
		saveParallelCheckpoint(state.snapshotSegments())
	}

	for i, seg := range segments {
		if seg == nil || seg.Done {
			continue
		}
		if int(commentsCount.Load()) >= vodCommentsMaxCount {
			break
		}
		workQueue.push(i, segmentSchedulePriority(*seg, scheduleHints))
	}

	var integrityFails atomic.Int32
	started := time.Now()

	state.cleanupPhaseMu.Lock()
	state.cleanupPhase = "initial"
	state.cleanupPhaseMu.Unlock()

	maxWorkers := s.vodGQLConcurrencyMax
	if maxWorkers <= 0 {
		maxWorkers = s.vodGQLConcurrency
	}
	s.runGQLSegmentWorkerPass(ctx, streamID, videoID, state, coord, &integrityFails, &gqlPageCount, maxWorkers, "initial")

	parallelRetries := state.enqueueIncompleteSegments()
	if parallelRetries > 0 && int(commentsCount.Load()) < vodCommentsMaxCount {
		metrics.AnalyticsVODGQLSegments.WithLabelValues("retry").Add(float64(parallelRetries))
		s.log.Info("retrying unfinished VOD chat segments in parallel",
			"stream_id", streamID,
			"video_id", videoID,
			"segments", parallelRetries,
			"workers", coord.ActiveConcurrency(),
		)
		state.cleanupPhaseMu.Lock()
		state.cleanupPhase = "parallel_cleanup"
		state.cleanupPhaseMu.Unlock()
		s.updateChatProgress(ctx, streamID, func(cp *SyncChatProgress) {
			cp.CleanupPhase = "parallel_cleanup"
			cp.SegmentsIncomplete = parallelRetries
			cp.Message = fmt.Sprintf("VOD %s · parallel cleanup · %d unfinished segments", videoID, parallelRetries)
		}, true)
		s.runGQLSegmentWorkerPass(ctx, streamID, videoID, state, coord, &integrityFails, &gqlPageCount, maxWorkers, "parallel cleanup")
	}

	var serialRetries int
	for i, seg := range segments {
		if seg == nil || seg.Done {
			continue
		}
		if tryAutoCloseSegment(state, seg) {
			continue
		}
		serialRetries++
		metrics.AnalyticsVODGQLSegments.WithLabelValues("retry").Inc()
		s.log.Warn("retrying VOD chat segment serially",
			"stream_id", streamID,
			"video_id", videoID,
			"segment", i,
			"start_sec", seg.StartSec,
			"end_sec", seg.EndSec,
		)
		state.cleanupPhaseMu.Lock()
		state.cleanupPhase = "serial_retry"
		state.cleanupPhaseMu.Unlock()
		s.updateChatProgress(ctx, streamID, func(cp *SyncChatProgress) {
			cp.CleanupPhase = "serial_retry"
			cp.SegmentsIncomplete = state.incompleteSegmentCount()
			cp.Message = fmt.Sprintf("VOD %s · serial retry segment %d/%d", videoID, i+1, len(segments))
		}, true)
		if err := s.fetchGQLSegmentSerial(ctx, videoID, seg, state, coord, &gqlPageCount, "serial"); err != nil {
			s.log.Warn("segment serial retry failed",
				"stream_id", streamID,
				"segment", i,
				"err", err,
			)
		}
	}

	incomplete := state.incompleteSegmentCount()
	if incomplete > 0 {
		return fmt.Errorf("parallel gql fetch incomplete: %d/%d segments after retry (integrity_fails=%d, parallel_retries=%d, serial_retries=%d)",
			incomplete, len(state.snapshotSegments()), integrityFails.Load(), parallelRetries, serialRetries)
	}

	state.commentsMapMu.Lock()
	state.shardedComments.mergeInto(commentsMap)
	state.commentsMapMu.Unlock()
	state.report(true)
	s.flushPendingRollupSegments(ctx, state, true)
	if s.vodGQLIncrementalDB && s.vodGQLDeferSummaryRefresh {
		s.updateChatProgress(ctx, streamID, func(cp *SyncChatProgress) {
			cp.Active = true
			cp.IndexPhase = "finalizing"
			cp.SummaryRefreshDeferred = true
			cp.Message = "Finalizing chat index"
		}, true)
		metrics.AnalyticsVODGQLFinalizingJobs.Inc()
		defer metrics.AnalyticsVODGQLFinalizingJobs.Dec()
		if err := s.store.RefreshStreamSummaryWithMode(ctx, streamID, "deferred_finalize"); err != nil {
			return fmt.Errorf("refresh stream summary after parallel gql sync: %w", err)
		}
	}

	if s.chatReplayEnabled && s.chatReplaySink != nil {
		inserted, err := s.chatReplaySink.Flush(ctx)
		if err != nil {
			s.log.Warn("chat replay final flush failed", "stream_id", streamID, "mode", "parallel", "err", err)
		} else {
			state.commentsSaved.Add(int64(inserted))
			s.updateChatProgress(ctx, streamID, func(cp *SyncChatProgress) {
				cp.CommentsSaved = int(state.commentsSaved.Load())
			}, true)
		}
	}

	if err := s.store.DeleteSyncCheckpoint(ctx, streamID, videoID); err != nil {
		s.log.Warn("failed to clear sync checkpoint", "stream_id", streamID, "err", err)
	}
	elapsed := time.Since(started).Seconds()
	pageCount := gqlPageCount.Load()
	var pagesPerSec float64
	if elapsed > 0 {
		pagesPerSec = float64(pageCount) / elapsed
	}
	finalSegments := state.snapshotSegments()
	impliedAvgSegmentSec := impliedAvgSegmentSecFromSegments(vodDurationSec, state.effectiveSegmentSec, len(finalSegments))
	throttle429, throttle503, backoffSeconds := requestStats.snapshot()
	s.log.Info("finished VOD comments paging",
		"total_comments", commentsCount.Load(),
		"mode", "parallel",
		"workers", coord.ActiveConcurrency(),
		"segments", len(finalSegments),
		"initial_segments", state.initialSegmentCount,
		"hot_splits", state.hotSplitsTotal.Load(),
		"hot_splits_by_reason", state.snapshotHotSplitsByReason(),
		"implied_avg_segment_sec", impliedAvgSegmentSec,
		"pages", pageCount,
		"pages_per_sec", pagesPerSec,
		"gql_429", throttle429,
		"gql_503", throttle503,
		"gql_backoff_sec", backoffSeconds,
		"worker_pages", state.snapshotWorkerPages(),
	)
	return nil
}

func (s *SyncService) runGQLSegmentWorkerPass(
	ctx context.Context,
	streamID string,
	videoID string,
	state *vodCommentsFetchState,
	coord *gqlRateCoordinator,
	integrityFails *atomic.Int32,
	pages *atomic.Int64,
	maxWorkers int,
	phase string,
) {
	if maxWorkers <= 0 {
		maxWorkers = 1
	}
	var wg sync.WaitGroup
	for w := 0; w < maxWorkers; w++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for {
				segIdx, ok := state.workQueue.acquire(ctx)
				if !ok {
					return
				}
				if int(state.commentsCount.Load()) >= vodCommentsMaxCount {
					state.workQueue.release()
					continue
				}
				for coord.ActiveConcurrency() <= workerID {
					select {
					case <-ctx.Done():
						state.workQueue.release()
						return
					case <-time.After(100 * time.Millisecond):
					}
				}
				seg, ok := state.segmentAt(segIdx)
				if !ok {
					state.workQueue.release()
					continue
				}
				workerLabel := fmt.Sprintf("%d", workerID)
				if err := s.fetchGQLSegment(ctx, videoID, seg, state, coord, integrityFails, pages, workerLabel); err != nil {
					if errors.Is(err, errGoldSegmentRetryLater) {
						state.workQueue.push(segIdx, segmentSchedulePriority(*seg, state.scheduleHints))
						select {
						case <-ctx.Done():
							state.workQueue.release()
							return
						case <-time.After(200 * time.Millisecond):
						}
						state.workQueue.release()
						continue
					}
					if errors.Is(err, errGoldSegmentSkip) {
						state.workQueue.release()
						continue
					}
					s.log.Warn("segment fetch failed",
						"stream_id", streamID,
						"segment", segIdx,
						"worker", workerID,
						"phase", phase,
						"err", err,
					)
				}
				state.workQueue.release()
			}
		}(w)
	}
	wg.Wait()
}

func (s *SyncService) fetchGQLSegment(
	ctx context.Context,
	videoID string,
	seg *gqlSegmentProgress,
	state *vodCommentsFetchState,
	coord *gqlRateCoordinator,
	integrityFails *atomic.Int32,
	pages *atomic.Int64,
	workerLabel string,
) error {
	offset := seg.OffsetSec
	if offset < seg.StartSec {
		offset = seg.StartSec
	}
	if state.goldLedger != nil {
		if err := state.goldLedger.beginSegment(seg); err != nil {
			if errors.Is(err, errGoldSegmentSkip) {
				return errGoldSegmentSkip
			}
			return err
		}
	}
	var fetchErr error
	defer func() {
		if fetchErr != nil && state.goldLedger != nil {
			state.goldLedger.failSegment(*seg, fetchErr)
		}
	}()
	useCursor := false
	cursorFailed := false
	nextCursor := ""
	pageCount := 0
	var pageSamples []gqlPageSample

	for {
		if int(state.commentsCount.Load()) >= vodCommentsMaxCount {
			state.finishSegment(seg, offset)
			return nil
		}
		if integrityFails.Load() >= vodCommentsParallelMaxFail {
			fetchErr = fmt.Errorf("integrity threshold reached")
			return fetchErr
		}
		if offset > seg.EndSec {
			state.finishSegment(seg, offset)
			return nil
		}

		if err := coord.Wait(ctx); err != nil {
			fetchErr = err
			return fetchErr
		}

		pageStartOffset := offset
		reqBody := buildVideoCommentsGQLRequest(videoID, gqlVideoCommentsSHA256, useCursor, offset, nextCursor)
		gqlResp, err := s.postGQLVideoComments(ctx, reqBody, coord, state.requestStats)
		pages.Add(1)
		metrics.AnalyticsVODGQLWorkerPagesTotal.WithLabelValues(workerLabel).Inc()
		state.recordWorkerPage(workerLabel)
		pageCount++
		if err != nil {
			seg.OffsetSec = offset
			state.saveParallel(true)
			fetchErr = err
			return fetchErr
		}
		if isGQLIntegrityError(gqlResp) {
			if useCursor {
				useCursor = false
				cursorFailed = true
				nextCursor = ""
				offset = seg.OffsetSec + 1
				if offset < seg.StartSec {
					offset = seg.StartSec
				}
				continue
			}
			integrityFails.Add(1)
			seg.OffsetSec = offset
			state.saveParallel(true)
			fetchErr = fmt.Errorf("gql video comments integrity error")
			return fetchErr
		}
		if len(gqlResp.Errors) > 0 {
			seg.OffsetSec = offset
			state.saveParallel(true)
			fetchErr = fmt.Errorf("gql video comments error: %s", gqlResp.Errors[0].Message)
			return fetchErr
		}
		if gqlResp.Data.Video == nil || gqlResp.Data.Video.Comments == nil {
			state.finishSegment(seg, offset)
			return nil
		}

		edges := gqlResp.Data.Video.Comments.Edges
		if len(edges) == 0 {
			state.finishSegment(seg, offset)
			return nil
		}

		lastOffset := offset
		pastSegment := false
		pageComments := 0
		for _, edge := range edges {
			if state.mergeEdge(edge, seg.StartSec, seg.EndSec) {
				pastSegment = true
				break
			}
			lastOffset = edge.Node.ContentOffsetSeconds
			pageComments++
		}
		offsetAdvance := 0
		if lastOffset > pageStartOffset {
			offsetAdvance = lastOffset - pageStartOffset
		}
		pageSamples = append(pageSamples, gqlPageSample{
			offsetAdvance: offsetAdvance,
			commentCount:  pageComments,
		})
		if len(pageSamples) > state.hotSlowAdvancePages*2 {
			pageSamples = pageSamples[len(pageSamples)-state.hotSlowAdvancePages*2:]
		}
		state.report(false)
		seg.OffsetSec = lastOffset

		if pastSegment || !gqlResp.Data.Video.Comments.PageInfo.HasNextPage {
			state.finishSegment(seg, lastOffset)
			return nil
		}

		reason := hotSegmentSplitReason(
			pageCount,
			state.hotPageThreshold,
			pageSamples,
			state.hotSlowAdvanceSec,
			state.hotSlowAdvancePages,
			state.hotCommentsPerPage,
		)
		if reason != "" && lastOffset < seg.EndSec-60 {
			splitAt := lastOffset + 1
			beforeSplit := *seg
			tail := gqlSegmentProgress{
				StartSec:  splitAt,
				EndSec:    beforeSplit.EndSec,
				OffsetSec: splitAt,
			}
			if state.goldLedger != nil {
				state.goldLedger.onHotSplit(beforeSplit, splitAt, tail)
			}
			seg.EndSec = splitAt - 1
			if state.goldLedger != nil {
				if err := state.goldLedger.beginSegment(seg); err != nil && !errors.Is(err, errGoldSegmentSkip) {
					s.log.Warn("gold segment hot-split claim failed",
						"stream_id", state.streamID,
						"vod_id", videoID,
						"start_sec", seg.StartSec,
						"end_sec", seg.EndSec,
						"err", err,
					)
				}
			}
			state.recordHotSplit(reason)
			metrics.AnalyticsVODGQLHotSplitsTotal.WithLabelValues(reason).Inc()
			tailIdx := state.appendSegment(tail)
			if state.workQueue != nil {
				state.workQueue.push(tailIdx, segmentSchedulePriority(tail, state.scheduleHints))
			}
			state.finishSegment(seg, lastOffset)
			return nil
		}

		lastEdge := edges[len(edges)-1]
		if !cursorFailed && strings.TrimSpace(lastEdge.Cursor) != "" {
			useCursor = true
			nextCursor = lastEdge.Cursor
		} else {
			useCursor = false
			nextCursor = ""
			offset = lastOffset + 1
		}
		seg.OffsetSec = offset
		state.saveParallel(false)

		if s.vodGQLPageDelay > 0 {
			time.Sleep(s.vodGQLPageDelay)
		}
	}
}

func (s *SyncService) fetchGQLSegmentSerial(
	ctx context.Context,
	videoID string,
	seg *gqlSegmentProgress,
	state *vodCommentsFetchState,
	coord *gqlRateCoordinator,
	pages *atomic.Int64,
	workerLabel string,
) error {
	offset := seg.OffsetSec
	if offset < seg.StartSec {
		offset = seg.StartSec
	}
	useCursor := false
	cursorFailed := false
	nextCursor := ""
	lastOffset := offset

	for {
		if int(state.commentsCount.Load()) >= vodCommentsMaxCount {
			state.finishSegment(seg, offset)
			return nil
		}
		if offset > seg.EndSec {
			state.finishSegment(seg, offset)
			return nil
		}

		if err := coord.Wait(ctx); err != nil {
			return err
		}

		reqBody := buildVideoCommentsGQLRequest(videoID, gqlVideoCommentsSHA256, useCursor, offset, nextCursor)
		gqlResp, err := s.postGQLVideoComments(ctx, reqBody, coord, state.requestStats)
		pages.Add(1)
		metrics.AnalyticsVODGQLWorkerPagesTotal.WithLabelValues(workerLabel).Inc()
		state.recordWorkerPage(workerLabel)
		if err != nil {
			seg.OffsetSec = offset
			state.saveParallel(true)
			return err
		}
		if isGQLIntegrityError(gqlResp) {
			if useCursor {
				s.log.Warn("segment serial cursor failed integrity; falling back to offset",
					"video_id", videoID,
					"segment_start", seg.StartSec,
					"offset", lastOffset+1,
				)
				useCursor = false
				cursorFailed = true
				nextCursor = ""
				offset = lastOffset + 1
				if offset < seg.StartSec {
					offset = seg.StartSec
				}
				continue
			}
			seg.OffsetSec = offset
			state.saveParallel(true)
			return fmt.Errorf("gql video comments integrity error")
		}
		if len(gqlResp.Errors) > 0 {
			seg.OffsetSec = offset
			state.saveParallel(true)
			return fmt.Errorf("gql video comments error: %s", gqlResp.Errors[0].Message)
		}
		if gqlResp.Data.Video == nil || gqlResp.Data.Video.Comments == nil {
			state.finishSegment(seg, offset)
			return nil
		}

		edges := gqlResp.Data.Video.Comments.Edges
		if len(edges) == 0 {
			state.finishSegment(seg, offset)
			return nil
		}

		pastSegment := false
		for _, edge := range edges {
			if state.mergeEdge(edge, seg.StartSec, seg.EndSec) {
				pastSegment = true
				break
			}
			lastOffset = edge.Node.ContentOffsetSeconds
		}
		state.report(false)
		seg.OffsetSec = lastOffset

		if pastSegment || !gqlResp.Data.Video.Comments.PageInfo.HasNextPage {
			state.finishSegment(seg, lastOffset)
			return nil
		}

		lastEdge := edges[len(edges)-1]
		if !cursorFailed && strings.TrimSpace(lastEdge.Cursor) != "" {
			useCursor = true
			nextCursor = lastEdge.Cursor
		} else {
			useCursor = false
			nextCursor = ""
			offset = lastOffset + 1
		}
		seg.OffsetSec = offset
		state.saveParallel(false)

		if s.vodGQLPageDelay > 0 {
			time.Sleep(s.vodGQLPageDelay)
		}
	}
}
