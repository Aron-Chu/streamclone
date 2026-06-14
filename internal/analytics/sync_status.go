package analytics

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	syncStatusTTL               = 2 * time.Hour
	syncLockTTL                 = 2 * time.Hour
	syncStatusStaleAfter        = 90 * time.Second
	syncChatProgressFlushMinGap = time.Second
	syncOrphanFailMessage       = "Sync interrupted (service restarted). Click sync to retry."
	releaseSyncLockScript       = `if redis.call("get", KEYS[1]) == ARGV[1] then return redis.call("del", KEYS[1]) else return 0 end`
)

type syncStatusCache struct {
	mu        sync.Mutex
	pending   map[string]SyncStatus
	lastFlush map[string]time.Time
}

func (c *syncStatusCache) get(streamID string) (*SyncStatus, bool) {
	if c == nil {
		return nil, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	st, ok := c.pending[streamID]
	if !ok {
		return nil, false
	}
	copy := st
	return &copy, true
}

func (c *syncStatusCache) put(status SyncStatus) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.pending == nil {
		c.pending = make(map[string]SyncStatus)
		c.lastFlush = make(map[string]time.Time)
	}
	c.pending[status.StreamID] = status
}

func (c *syncStatusCache) shouldFlush(streamID string, force bool) bool {
	if c == nil {
		return force
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if force {
		return true
	}
	last, ok := c.lastFlush[streamID]
	return !ok || time.Since(last) >= syncChatProgressFlushMinGap
}

func (c *syncStatusCache) markFlushed(streamID string) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.lastFlush == nil {
		c.lastFlush = make(map[string]time.Time)
	}
	c.lastFlush[streamID] = time.Now()
}

type SyncPhase string

const (
	SyncPhaseStarting         SyncPhase = "starting"
	SyncPhaseScrapingTracker  SyncPhase = "scraping_tracker"
	SyncPhaseParsingTracker   SyncPhase = "parsing_tracker"
	SyncPhaseResolvingVOD     SyncPhase = "resolving_vod"
	SyncPhaseFetchingComments SyncPhase = "fetching_comments"
	SyncPhaseWritingRollups   SyncPhase = "writing_rollups"
	SyncPhaseCompleted        SyncPhase = "completed"
	SyncPhaseFailed           SyncPhase = "failed"
)

func (p SyncPhase) IsTerminal() bool {
	return p == SyncPhaseCompleted || p == SyncPhaseFailed
}

type SyncPhaseTiming struct {
	TrackerScrapeMS int64 `json:"trackerScrapeMs,omitempty"`
	VodResolveMS    int64 `json:"vodResolveMs,omitempty"`
	GQLFetchMS      int64 `json:"gqlFetchMs,omitempty"`
	TokenizeMS      int64 `json:"tokenizeMs,omitempty"`
	RollupWriteMS   int64 `json:"rollupWriteMs,omitempty"`
}

// SyncChatProgress tracks VOD GQL comment paging (runs in parallel with tracker scrape).
type SyncChatProgress struct {
	Active            bool   `json:"active,omitempty"`
	VodID             string `json:"vodId,omitempty"`
	FetchMode         string `json:"fetchMode,omitempty"` // parallel | serial
	Concurrency       int    `json:"concurrency,omitempty"`
	SegmentsTotal     int    `json:"segmentsTotal,omitempty"`
	SegmentsDone      int    `json:"segmentsDone,omitempty"`
	CommentsFetched   int    `json:"commentsFetched,omitempty"`
	TimelineSec       int    `json:"timelineSec,omitempty"`
	VodDurationSec    int    `json:"vodDurationSec,omitempty"`
	StreamDurationSec int    `json:"streamDurationSec,omitempty"`
	RollupsExpected   int    `json:"rollupsExpected,omitempty"`
	IndexPhase        string `json:"indexPhase,omitempty"` // fetching | tokenizing | writing | done
	GQLPages          int    `json:"gqlPages,omitempty"`
	Throttled         bool   `json:"throttled,omitempty"`
	Message           string `json:"message,omitempty"`
}

// SyncTrackerProgress tracks TwitchTracker browser scrape (parallel with chat on first sync).
type SyncTrackerProgress struct {
	Active  bool   `json:"active,omitempty"`
	URL     string `json:"url,omitempty"`
	Message string `json:"message,omitempty"`
}

type SyncStatus struct {
	StreamID        string               `json:"streamId"`
	Phase           SyncPhase            `json:"phase"`
	Message         string               `json:"message,omitempty"`
	StartedAt       time.Time            `json:"startedAt"`
	UpdatedAt       time.Time            `json:"updatedAt"`
	CommentsFetched int                  `json:"commentsFetched,omitempty"`
	RollupsWritten  int                  `json:"rollupsWritten,omitempty"`
	ResultMessage   string               `json:"resultMessage,omitempty"`
	Error           string               `json:"error,omitempty"`
	ViewersOnly     bool                 `json:"viewersOnly,omitempty"`
	ViewerStatus    string               `json:"viewerStatus,omitempty"`
	Stale           bool                 `json:"stale,omitempty"`
	Timing          *SyncPhaseTiming     `json:"timing,omitempty"`
	Chat            *SyncChatProgress    `json:"chat,omitempty"`
	Tracker         *SyncTrackerProgress `json:"tracker,omitempty"`
}

func syncStatusIsStale(status *SyncStatus) bool {
	if status == nil || status.Phase.IsTerminal() {
		return false
	}
	if status.UpdatedAt.IsZero() {
		return false
	}
	return time.Since(status.UpdatedAt) > syncStatusStaleAfter
}

func syncStatusShouldReportStale(status *SyncStatus, lockOwnedByInstance bool) bool {
	return syncStatusIsStale(status) && !lockOwnedByInstance
}

func syncLockOwnedBy(lockValue, ownerID string) bool {
	return strings.TrimSpace(lockValue) != "" && lockValue == ownerID
}

func (s *SyncService) syncLockHeld(ctx context.Context, streamID string) bool {
	if s.rdb == nil {
		return false
	}
	n, err := s.rdb.Exists(ctx, syncLockKey(streamID)).Result()
	return err == nil && n > 0
}

func (s *SyncService) syncLockOwnedByThisInstance(ctx context.Context, streamID string) bool {
	if s.rdb == nil {
		return false
	}
	raw, err := s.rdb.Get(ctx, syncLockKey(streamID)).Result()
	if err != nil {
		return false
	}
	return syncLockOwnedBy(raw, s.syncOwnerID)
}

func (s *SyncService) enrichSyncStatus(ctx context.Context, status *SyncStatus) *SyncStatus {
	if status == nil {
		return nil
	}
	stale := syncStatusShouldReportStale(status, s.syncLockOwnedByThisInstance(ctx, status.StreamID))
	status.Stale = stale
	return status
}

func (s *SyncService) markSyncOrphaned(ctx context.Context, streamID string, existing *SyncStatus) {
	if existing == nil {
		return
	}
	existing.Phase = SyncPhaseFailed
	existing.Error = syncOrphanFailMessage
	existing.Message = "Sync interrupted"
	existing.Stale = true
	if existing.Chat != nil {
		existing.Chat.Active = false
		existing.Chat.IndexPhase = "done"
	}
	if existing.Tracker != nil {
		existing.Tracker.Active = false
		existing.Tracker.Message = ""
	}
	if err := s.saveSyncStatus(ctx, *existing); err != nil {
		s.log.Warn("failed to mark orphaned sync status", "stream_id", streamID, "err", err)
	}
}

type StartSyncResponse struct {
	Accepted bool        `json:"accepted"`
	Status   *SyncStatus `json:"status,omitempty"`
}

func syncStatusKey(streamID string) string {
	return "analytics:sync:" + streamID
}

func syncLockKey(streamID string) string {
	return "analytics:sync-lock:" + streamID
}

func newSyncOwnerID() string {
	return fmt.Sprintf("%d:%d", os.Getpid(), time.Now().UnixNano())
}

func (s *SyncService) GetSyncStatus(ctx context.Context, streamID string) (*SyncStatus, error) {
	if s.rdb == nil {
		return nil, redis.Nil
	}
	raw, err := s.rdb.Get(ctx, syncStatusKey(streamID)).Result()
	if err != nil {
		return nil, err
	}
	var status SyncStatus
	if err := json.Unmarshal([]byte(raw), &status); err != nil {
		return nil, fmt.Errorf("decode sync status: %w", err)
	}
	return s.enrichSyncStatus(ctx, &status), nil
}

func (s *SyncService) saveSyncStatus(ctx context.Context, status SyncStatus) error {
	if s.rdb == nil {
		return nil
	}
	status.UpdatedAt = time.Now().UTC()
	raw, err := json.Marshal(status)
	if err != nil {
		return err
	}
	return s.rdb.Set(ctx, syncStatusKey(status.StreamID), raw, syncStatusTTL).Err()
}

func (s *SyncService) loadOrInitSyncStatus(ctx context.Context, streamID string) *SyncStatus {
	if cached, ok := s.syncStatusCache.get(streamID); ok {
		return cached
	}
	status, err := s.GetSyncStatus(ctx, streamID)
	if err != nil || status == nil {
		now := time.Now().UTC()
		return &SyncStatus{
			StreamID:  streamID,
			StartedAt: now,
			UpdatedAt: now,
		}
	}
	return status
}

func (s *SyncService) persistSyncStatus(ctx context.Context, status SyncStatus, force bool) error {
	s.syncStatusCache.put(status)
	if !s.syncStatusCache.shouldFlush(status.StreamID, force) {
		return nil
	}
	if err := s.saveSyncStatus(ctx, status); err != nil {
		return err
	}
	s.syncStatusCache.markFlushed(status.StreamID)
	return nil
}

func (s *SyncService) setSyncPhase(ctx context.Context, streamID string, phase SyncPhase, message string, mutate func(*SyncStatus)) {
	status := s.loadOrInitSyncStatus(ctx, streamID)
	status.Phase = phase
	if message != "" {
		status.Message = message
	}
	if mutate != nil {
		mutate(status)
	}
	if err := s.persistSyncStatus(ctx, *status, true); err != nil {
		s.log.Warn("failed to persist sync status", "stream_id", streamID, "phase", phase, "err", err)
	}
}

func (s *SyncService) updateChatProgress(ctx context.Context, streamID string, mutate func(*SyncChatProgress), forceFlush ...bool) {
	force := len(forceFlush) > 0 && forceFlush[0]
	status := s.loadOrInitSyncStatus(ctx, streamID)
	if status.Chat == nil {
		status.Chat = &SyncChatProgress{}
	}
	prevPhase := status.Phase
	if mutate != nil {
		wasActive := status.Chat.Active
		mutate(status.Chat)
		if wasActive && !status.Chat.Active {
			force = true
		}
	}
	if status.Chat.CommentsFetched > 0 {
		status.CommentsFetched = status.Chat.CommentsFetched
	}
	if status.Chat.Active {
		status.Phase = SyncPhaseFetchingComments
		if strings.TrimSpace(status.Chat.Message) != "" {
			status.Message = status.Chat.Message
		} else if status.Message == "" {
			status.Message = "Fetching VOD chat via Twitch GQL"
		}
	}
	if status.Phase != prevPhase {
		force = true
	}
	if err := s.persistSyncStatus(ctx, *status, force); err != nil {
		s.log.Warn("failed to persist chat sync progress", "stream_id", streamID, "err", err)
	}
}

func (s *SyncService) updateTrackerProgress(ctx context.Context, streamID string, mutate func(*SyncTrackerProgress)) {
	status := s.loadOrInitSyncStatus(ctx, streamID)
	if status.Tracker == nil {
		status.Tracker = &SyncTrackerProgress{}
	}
	if mutate != nil {
		mutate(status.Tracker)
	}
	if !status.Tracker.Active {
		if status.Tracker.Message == "Parsing meta#ecs viewer chart" || status.Tracker.Message == "Browser scrape for viewer chart (meta#ecs)" {
			status.Tracker.Message = ""
		}
	}
	if err := s.saveSyncStatus(ctx, *status); err != nil {
		s.log.Warn("failed to persist tracker sync progress", "stream_id", streamID, "err", err)
	}
}

func (s *SyncService) releaseSyncLock(ctx context.Context, streamID string) {
	if s.rdb == nil {
		return
	}
	ownerID := strings.TrimSpace(s.syncOwnerID)
	if ownerID == "" {
		if err := s.rdb.Del(ctx, syncLockKey(streamID)).Err(); err != nil {
			s.log.Warn("failed to release sync lock", "stream_id", streamID, "err", err)
		}
		return
	}
	if err := s.rdb.Eval(ctx, releaseSyncLockScript, []string{syncLockKey(streamID)}, ownerID).Err(); err != nil {
		s.log.Warn("failed to release sync lock", "stream_id", streamID, "err", err)
	}
}

func (s *SyncService) clearSyncLock(ctx context.Context, streamID string) {
	if s.rdb == nil {
		return
	}
	if err := s.rdb.Del(ctx, syncLockKey(streamID)).Err(); err != nil {
		s.log.Warn("failed to release sync lock", "stream_id", streamID, "err", err)
	}
}

func (s *SyncService) TryStartSync(ctx context.Context, streamID, channelOpt string, viewersOnly bool, forceChat bool, hintVodID string) (bool, *SyncStatus, error) {
	streamID = strings.TrimSpace(streamID)
	if streamID == "" {
		return false, nil, errors.New("missing stream id")
	}
	if s.rdb == nil {
		return false, nil, errors.New("sync status unavailable (redis not configured)")
	}

	if existing, err := s.GetSyncStatus(ctx, streamID); err == nil && existing != nil && !existing.Phase.IsTerminal() {
		if syncStatusShouldReportStale(existing, s.syncLockOwnedByThisInstance(ctx, streamID)) {
			s.markSyncOrphaned(ctx, streamID, existing)
			s.clearSyncLock(ctx, streamID)
		} else {
			return false, s.enrichSyncStatus(ctx, existing), nil
		}
	}

	acquired, err := s.rdb.SetNX(ctx, syncLockKey(streamID), s.syncOwnerID, syncLockTTL).Result()
	if err != nil {
		return false, nil, fmt.Errorf("sync lock: %w", err)
	}
	if !acquired {
		if existing, getErr := s.GetSyncStatus(ctx, streamID); getErr == nil && existing != nil && !existing.Phase.IsTerminal() {
			return false, s.enrichSyncStatus(ctx, existing), nil
		}
		return false, nil, errors.New("sync already in progress")
	}

	now := time.Now().UTC()
	status := &SyncStatus{
		StreamID:    streamID,
		Phase:       SyncPhaseStarting,
		Message:     "Starting historical sync",
		StartedAt:   now,
		UpdatedAt:   now,
		ViewersOnly: viewersOnly,
	}
	if err := s.saveSyncStatus(ctx, *status); err != nil {
		s.releaseSyncLock(ctx, streamID)
		return false, nil, err
	}

	go s.runSyncJob(streamID, channelOpt, viewersOnly, forceChat, strings.TrimSpace(hintVodID))
	return true, status, nil
}

func (s *SyncService) runSyncJob(streamID, channelOpt string, viewersOnly, forceChat bool, hintVodID string) {
	ctx := context.Background()
	defer s.releaseSyncLock(ctx, streamID)

	s.setSyncPhase(ctx, streamID, SyncPhaseStarting, "Preparing sync", nil)
	msg, err := s.SyncHistoricalStream(ctx, streamID, channelOpt, viewersOnly, forceChat, hintVodID)
	if err != nil {
		s.setSyncPhase(ctx, streamID, SyncPhaseFailed, "Sync failed", func(st *SyncStatus) {
			st.Error = err.Error()
		})
		return
	}
	s.setSyncPhase(ctx, streamID, SyncPhaseCompleted, "Sync completed", func(st *SyncStatus) {
		st.ResultMessage = msg
	})
}
