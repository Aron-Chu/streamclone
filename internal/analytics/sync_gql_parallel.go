package analytics

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	gqlVideoCommentsSHA256           = "b70a3591ff0f4e0313d126c6a1502d79a1c02baebb288227c582044aa76adf6a"
	vodCommentsMaxCount              = 200000
	vodCommentsProgressEvery         = 200
	vodCommentsParallelMaxFail       = 3
	vodGQLSegmentLargeVOD            = 300 // 5-minute segments for long VODs
	vodGQLSegmentDenseVOD            = 120 // 2-minute segments for very high comment volume
	vodGQLHotSegmentPageThreshold    = 50  // split hot segments after this many pages
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

func (c *gqlRateCoordinator) Throttle(retryAfter time.Duration, attempt int) {
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
	streamID         string
	login            string
	videoID          string
	vodDurationSec   int
	chatAlignSec     int
	fetchMode        string
	concurrency      int
	commentsMap      map[int][]string
	deduper          gqlCommentDeduper
	shardedComments  gqlCommentsMap
	commentsCount    *atomic.Int64
	segmentsMu       sync.Mutex
	segments         *[]gqlSegmentProgress
	coord            *gqlRateCoordinator
	pages            *atomic.Int64
	rollupStartFn    func() time.Time
	onSegmentDone    func(seg gqlSegmentProgress)
	report           func(force bool)
	saveParallel     func(force bool)
	hotPageThreshold int
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

func (st *vodCommentsFetchState) finishSegment(seg *gqlSegmentProgress, offset int) {
	seg.Done = true
	seg.OffsetSec = offset
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

const vodCommentsCheckpointMinGap = 2 * time.Second

func effectiveGQLSegmentSeconds(configured, denseSec, vodDurationSec int, estimatedComments int64) int {
	if configured <= 0 {
		configured = 600
	}
	if denseSec <= 0 {
		denseSec = vodGQLSegmentDenseVOD
	}
	if vodDurationSec >= vodGQLLargeVODDurationSec {
		return minInt(configured, vodGQLSegmentLargeVOD)
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
	}
	return configured
}

func minInt(a, b int) int {
	if a < b {
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
	return false
}

func (s *SyncService) estimatedStreamComments(ctx context.Context, streamID string) int64 {
	rec, err := s.store.StreamByID(ctx, streamID)
	if err != nil || rec == nil {
		return 0
	}
	return rec.ChatMessages
}

func (s *SyncService) fetchVODComments(ctx context.Context, streamID, login, videoID string, commentsMap map[int][]string, vodDurationSec, chatAlignSec int, rollupStartFn func() time.Time, chatCache *chatRollupCache) error {
	estimatedComments := s.estimatedStreamComments(ctx, streamID)
	fetchMode := "serial"
	concurrency := 1
	segmentsTotal := 1
	segmentSec := effectiveGQLSegmentSeconds(s.vodGQLSegmentSeconds, s.vodGQLDenseSegmentSeconds, vodDurationSec, estimatedComments)
	if s.vodGQLConcurrency > 1 {
		if vodDurationSec <= 0 && s.helix != nil && s.helix.Enabled() {
			if d, err := s.helix.VideoDurationSeconds(ctx, videoID); err == nil && d > 0 {
				vodDurationSec = d
				segmentSec = effectiveGQLSegmentSeconds(s.vodGQLSegmentSeconds, s.vodGQLDenseSegmentSeconds, vodDurationSec, estimatedComments)
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
		cp.VodDurationSec = vodDurationSec
		cp.SegmentsTotal = segmentsTotal
		cp.Message = fmt.Sprintf("Loading VOD %s chat via Twitch GQL (%s)", videoID, fetchMode)
	}, true)
	defer s.updateChatProgress(ctx, streamID, func(cp *SyncChatProgress) {
		cp.Active = false
	}, true)

	if s.vodGQLConcurrency <= 1 {
		return s.fetchVODCommentsSerial(ctx, streamID, videoID, commentsMap, vodDurationSec, chatAlignSec)
	}
	if vodDurationSec <= 0 {
		vodDurationSec = 86400
	}
	return s.fetchVODCommentsParallel(ctx, streamID, login, videoID, commentsMap, vodDurationSec, chatAlignSec, rollupStartFn, chatCache)
}

func (s *SyncService) fetchVODCommentsSerial(ctx context.Context, streamID, videoID string, commentsMap map[int][]string, vodDurationSec, chatAlignSec int) error {
	useCursor := false
	cursorFailed := false
	nextCursor := ""
	contentOffsetSeconds := 0
	commentsCount := 0
	lastOffset := 0
	gqlPages := 0
	seenCommentIDs := make(map[string]struct{})

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
			cp.SegmentsTotal = 1
			cp.SegmentsDone = 0
			cp.CommentsFetched = count
			cp.TimelineSec = timeline
			cp.VodDurationSec = vodDurationSec
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
	}

	for {
		reqBody := buildVideoCommentsGQLRequest(videoID, gqlVideoCommentsSHA256, useCursor, contentOffsetSeconds, nextCursor)
		gqlResp, err := s.postGQLVideoComments(ctx, reqBody, nil)
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

func (s *SyncService) fetchVODCommentsParallel(ctx context.Context, streamID, login, videoID string, commentsMap map[int][]string, vodDurationSec, chatAlignSec int, rollupStartFn func() time.Time, chatCache *chatRollupCache) error {
	estimatedComments := s.estimatedStreamComments(ctx, streamID)
	segmentSec := effectiveGQLSegmentSeconds(s.vodGQLSegmentSeconds, s.vodGQLDenseSegmentSeconds, vodDurationSec, estimatedComments)
	segments := buildGQLSegments(vodDurationSec, segmentSec)
	var commentsCount atomic.Int64
	hotThreshold := s.vodGQLHotSegmentPageThreshold
	if hotThreshold <= 0 {
		hotThreshold = vodGQLHotSegmentPageThreshold
	}

	if cp, err := s.store.GetSyncCheckpoint(ctx, streamID, videoID); err != nil {
		s.log.Warn("failed to load sync checkpoint", "stream_id", streamID, "video_id", videoID, "err", err)
	} else if cp != nil && cp.FetchMode == "parallel" && strings.TrimSpace(cp.SegmentsJSON) != "" {
		var saved gqlParallelCheckpoint
		if jsonErr := json.Unmarshal([]byte(cp.SegmentsJSON), &saved); jsonErr == nil && len(saved.Segments) > 0 {
			segments = saved.Segments
			commentsCount.Store(int64(saved.CommentsFetched))
			s.log.Info("resuming parallel VOD comment fetch",
				"stream_id", streamID,
				"video_id", videoID,
				"segments", len(segments),
				"comments_fetched", saved.CommentsFetched,
			)
		}
	}

	coord := newGQLRateCoordinator(s.vodGQLConcurrencyMin, s.vodGQLConcurrencyMax, s.vodGQLConcurrency)
	var gqlPageCount atomic.Int64

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
	if s.vodGQLIncrementalDB {
		onSegmentDone = func(seg gqlSegmentProgress) {
			if err := s.patchChatRollupsForSegment(ctx, streamID, login, rollupStartFn, commentsMap, seg, chatAlignSec, chatCache); err != nil {
				s.log.Warn("incremental chat rollup patch failed", "stream_id", streamID, "segment_start", seg.StartSec, "err", err)
			}
		}
	}
	state := &vodCommentsFetchState{
		streamID:         streamID,
		login:            login,
		videoID:          videoID,
		vodDurationSec:   vodDurationSec,
		chatAlignSec:     chatAlignSec,
		fetchMode:        "parallel",
		concurrency:      coord.ActiveConcurrency(),
		commentsMap:      commentsMap,
		commentsCount:    &commentsCount,
		segments:         &segments,
		coord:            coord,
		pages:            &gqlPageCount,
		rollupStartFn:    rollupStartFn,
		onSegmentDone:    onSegmentDone,
		hotPageThreshold: hotThreshold,
	}
	state.report = func(force bool) {
		count := int(commentsCount.Load())
		if !force && (count == 0 || count%vodCommentsProgressEvery != 0) {
			return
		}
		state.segmentsMu.Lock()
		segs := append([]gqlSegmentProgress(nil), (*state.segments)...)
		state.segmentsMu.Unlock()
		segmentsDone, timelineSec := chatProgressFromSegments(segs)
		pageCount := int(gqlPageCount.Load())
		throttled := coord.Paused()
		workers := coord.ActiveConcurrency()
		s.updateChatProgress(ctx, streamID, func(cp *SyncChatProgress) {
			cp.Active = true
			cp.IndexPhase = "fetching"
			cp.VodID = videoID
			cp.FetchMode = "parallel"
			cp.Concurrency = workers
			cp.SegmentsTotal = len(segs)
			cp.SegmentsDone = segmentsDone
			cp.CommentsFetched = count
			cp.TimelineSec = timelineSec
			cp.VodDurationSec = vodDurationSec
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
		state.segmentsMu.Lock()
		segs := append([]gqlSegmentProgress(nil), (*state.segments)...)
		state.segmentsMu.Unlock()
		saveParallelCheckpoint(segs)
	}

	segCh := make(chan int)
	var wg sync.WaitGroup
	var integrityFails atomic.Int32
	started := time.Now()

	maxWorkers := s.vodGQLConcurrencyMax
	if maxWorkers <= 0 {
		maxWorkers = s.vodGQLConcurrency
	}
	for w := 0; w < maxWorkers; w++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for segIdx := range segCh {
				if int(commentsCount.Load()) >= vodCommentsMaxCount {
					continue
				}
				for coord.ActiveConcurrency() <= workerID {
					select {
					case <-ctx.Done():
						return
					case <-time.After(100 * time.Millisecond):
					}
				}
				var processSegment func(idx int)
				processSegment = func(idx int) {
					tailIdx, fetchErr := s.fetchGQLSegment(ctx, videoID, &segments[idx], state, coord, &integrityFails, &gqlPageCount)
					if fetchErr != nil {
						s.log.Warn("segment fetch failed", "stream_id", streamID, "segment", idx, "err", fetchErr)
					}
					if tailIdx >= 0 {
						processSegment(tailIdx)
					}
				}
				processSegment(segIdx)
			}
		}(w)
	}

	for i := range segments {
		if segments[i].Done {
			continue
		}
		if int(commentsCount.Load()) >= vodCommentsMaxCount {
			break
		}
		segCh <- i
	}
	close(segCh)
	wg.Wait()

	var serialRetries int
	for i := range segments {
		if segments[i].Done {
			continue
		}
		serialRetries++
		s.log.Warn("retrying VOD chat segment serially",
			"stream_id", streamID,
			"video_id", videoID,
			"segment", i,
			"start_sec", segments[i].StartSec,
			"end_sec", segments[i].EndSec,
		)
		s.updateChatProgress(ctx, streamID, func(cp *SyncChatProgress) {
			cp.Message = fmt.Sprintf("VOD %s · serial retry segment %d/%d", videoID, i+1, len(segments))
		}, true)
		if err := s.fetchGQLSegmentSerial(ctx, videoID, &segments[i], state, coord, &gqlPageCount); err != nil {
			s.log.Warn("segment serial retry failed",
				"stream_id", streamID,
				"segment", i,
				"err", err,
			)
		}
	}

	incomplete := 0
	for i := range segments {
		if !segments[i].Done {
			incomplete++
		}
	}
	if incomplete > 0 {
		return fmt.Errorf("parallel gql fetch incomplete: %d/%d segments after serial retry (integrity_fails=%d, serial_retries=%d)",
			incomplete, len(segments), integrityFails.Load(), serialRetries)
	}

	state.shardedComments.mergeInto(commentsMap)

	state.report(true)
	if err := s.store.DeleteSyncCheckpoint(ctx, streamID, videoID); err != nil {
		s.log.Warn("failed to clear sync checkpoint", "stream_id", streamID, "err", err)
	}
	elapsed := time.Since(started).Seconds()
	pageCount := gqlPageCount.Load()
	var pagesPerSec float64
	if elapsed > 0 {
		pagesPerSec = float64(pageCount) / elapsed
	}
	s.log.Info("finished VOD comments paging",
		"total_comments", commentsCount.Load(),
		"mode", "parallel",
		"workers", coord.ActiveConcurrency(),
		"segments", len(segments),
		"pages", pageCount,
		"pages_per_sec", pagesPerSec,
	)
	return nil
}

func (s *SyncService) fetchGQLSegment(
	ctx context.Context,
	videoID string,
	seg *gqlSegmentProgress,
	state *vodCommentsFetchState,
	coord *gqlRateCoordinator,
	integrityFails *atomic.Int32,
	pages *atomic.Int64,
) (tailIdx int, err error) {
	offset := seg.OffsetSec
	if offset < seg.StartSec {
		offset = seg.StartSec
	}
	useCursor := false
	cursorFailed := false
	nextCursor := ""
	pageCount := 0

	for {
		if int(state.commentsCount.Load()) >= vodCommentsMaxCount {
			state.finishSegment(seg, offset)
			return -1, nil
		}
		if integrityFails.Load() >= vodCommentsParallelMaxFail {
			return -1, fmt.Errorf("integrity threshold reached")
		}
		if offset > seg.EndSec {
			state.finishSegment(seg, offset)
			return -1, nil
		}

		if err := coord.Wait(ctx); err != nil {
			return -1, err
		}

		reqBody := buildVideoCommentsGQLRequest(videoID, gqlVideoCommentsSHA256, useCursor, offset, nextCursor)
		gqlResp, err := s.postGQLVideoComments(ctx, reqBody, coord)
		pages.Add(1)
		pageCount++
		if err != nil {
			seg.OffsetSec = offset
			state.saveParallel(true)
			return -1, err
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
			return -1, fmt.Errorf("gql video comments integrity error")
		}
		if len(gqlResp.Errors) > 0 {
			seg.OffsetSec = offset
			state.saveParallel(true)
			return -1, fmt.Errorf("gql video comments error: %s", gqlResp.Errors[0].Message)
		}
		if gqlResp.Data.Video == nil || gqlResp.Data.Video.Comments == nil {
			state.finishSegment(seg, offset)
			return -1, nil
		}

		edges := gqlResp.Data.Video.Comments.Edges
		if len(edges) == 0 {
			state.finishSegment(seg, offset)
			return -1, nil
		}

		lastOffset := offset
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
			state.finishSegment(seg, offset)
			return -1, nil
		}

		if state.hotPageThreshold > 0 &&
			pageCount >= state.hotPageThreshold &&
			lastOffset < seg.EndSec-60 {
			splitAt := lastOffset + 1
			tail := gqlSegmentProgress{
				StartSec:  splitAt,
				EndSec:    seg.EndSec,
				OffsetSec: splitAt,
			}
			seg.EndSec = splitAt - 1
			state.segmentsMu.Lock()
			*state.segments = append(*state.segments, tail)
			tailIdx = len(*state.segments) - 1
			state.segmentsMu.Unlock()
			state.finishSegment(seg, lastOffset)
			return tailIdx, nil
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
		gqlResp, err := s.postGQLVideoComments(ctx, reqBody, coord)
		pages.Add(1)
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
