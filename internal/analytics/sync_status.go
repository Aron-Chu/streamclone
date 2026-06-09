package analytics

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	syncStatusTTL = 2 * time.Hour
	syncLockTTL   = 2 * time.Hour
)

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

type SyncStatus struct {
	StreamID        string    `json:"streamId"`
	Phase           SyncPhase `json:"phase"`
	Message         string    `json:"message,omitempty"`
	StartedAt       time.Time `json:"startedAt"`
	UpdatedAt       time.Time `json:"updatedAt"`
	CommentsFetched int       `json:"commentsFetched,omitempty"`
	RollupsWritten  int       `json:"rollupsWritten,omitempty"`
	ResultMessage   string    `json:"resultMessage,omitempty"`
	Error           string    `json:"error,omitempty"`
	ViewersOnly     bool      `json:"viewersOnly,omitempty"`
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
	return &status, nil
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

func (s *SyncService) setSyncPhase(ctx context.Context, streamID string, phase SyncPhase, message string, mutate func(*SyncStatus)) {
	status, err := s.GetSyncStatus(ctx, streamID)
	if err != nil || status == nil {
		now := time.Now().UTC()
		status = &SyncStatus{
			StreamID:  streamID,
			StartedAt: now,
			UpdatedAt: now,
		}
	}
	status.Phase = phase
	if message != "" {
		status.Message = message
	}
	if mutate != nil {
		mutate(status)
	}
	if err := s.saveSyncStatus(ctx, *status); err != nil {
		s.log.Warn("failed to persist sync status", "stream_id", streamID, "phase", phase, "err", err)
	}
}

func (s *SyncService) releaseSyncLock(ctx context.Context, streamID string) {
	if s.rdb == nil {
		return
	}
	if err := s.rdb.Del(ctx, syncLockKey(streamID)).Err(); err != nil {
		s.log.Warn("failed to release sync lock", "stream_id", streamID, "err", err)
	}
}

func (s *SyncService) TryStartSync(ctx context.Context, streamID, channelOpt string, viewersOnly bool) (bool, *SyncStatus, error) {
	streamID = strings.TrimSpace(streamID)
	if streamID == "" {
		return false, nil, errors.New("missing stream id")
	}
	if s.rdb == nil {
		return false, nil, errors.New("sync status unavailable (redis not configured)")
	}

	if existing, err := s.GetSyncStatus(ctx, streamID); err == nil && existing != nil && !existing.Phase.IsTerminal() {
		return false, existing, nil
	}

	acquired, err := s.rdb.SetNX(ctx, syncLockKey(streamID), "1", syncLockTTL).Result()
	if err != nil {
		return false, nil, fmt.Errorf("sync lock: %w", err)
	}
	if !acquired {
		if existing, getErr := s.GetSyncStatus(ctx, streamID); getErr == nil && existing != nil {
			return false, existing, nil
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

	go s.runSyncJob(streamID, channelOpt, viewersOnly)
	return true, status, nil
}

func (s *SyncService) runSyncJob(streamID, channelOpt string, viewersOnly bool) {
	ctx := context.Background()
	defer s.releaseSyncLock(ctx, streamID)

	s.setSyncPhase(ctx, streamID, SyncPhaseStarting, "Preparing sync", nil)
	msg, err := s.SyncHistoricalStream(ctx, streamID, channelOpt, viewersOnly)
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
