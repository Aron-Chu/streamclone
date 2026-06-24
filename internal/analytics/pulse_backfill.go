package analytics

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"

	"streamclone/internal/analytics/heatmap"
)

const (
	pulseBackfillCooldown     = 45 * time.Second
	pulseBackfillJobTTL       = 2 * time.Hour
	pulseBackfillPollInterval = 2 * time.Second
)

var ErrPulseBackfillAtCapacity = errors.New("pulse_backfill_at_capacity")

const (
	PulseBackfillQueued            = "queued"
	PulseBackfillAlreadyAvailable  = "already_available"
	PulseBackfillResolvingVOD      = "resolving_vod"
	PulseBackfillWaitingForVOD     = "waiting_for_vod"
	PulseBackfillEnsuringEmotes    = "ensuring_emotes"
	PulseBackfillFetchingChat      = "fetching_chat"
	PulseBackfillTokenizing        = "tokenizing"
	PulseBackfillWritingRollups    = "writing_rollups"
	PulseBackfillRefreshingMoments = "refreshing_moments"
	PulseBackfillDone              = "done"
	PulseBackfillFailed            = "failed"
	PulseBackfillCancelled         = "cancelled"
)

type PulseBackfillProgress struct {
	SegmentsDone  int `json:"segmentsDone,omitempty"`
	SegmentsTotal int `json:"segmentsTotal,omitempty"`
	Percent       int `json:"percent,omitempty"`
}

type PulseBackfillRange struct {
	FromOffsetSeconds int `json:"fromOffsetSeconds"`
	ToOffsetSeconds   int `json:"toOffsetSeconds"`
}

type PulseBackfillJob struct {
	JobID     string                `json:"jobId"`
	StreamID  string                `json:"streamId"`
	Login     string                `json:"login"`
	Mode      string                `json:"mode"`
	Status    string                `json:"status"`
	Message   string                `json:"message"`
	Progress  PulseBackfillProgress `json:"progress,omitempty"`
	Range     PulseBackfillRange    `json:"range"`
	Error     string                `json:"error,omitempty"`
	CreatedAt time.Time             `json:"createdAt"`
	UpdatedAt time.Time             `json:"updatedAt"`
}

type PulseBackfillManager struct {
	sync         *SyncService
	store        *Store
	helix        *HelixClient
	rdb          *redis.Client
	heatmapCache *heatmap.Cache
	runtime      PulseRuntimeConfig

	mu             sync.Mutex
	jobs           map[string]*PulseBackfillJob
	activeByStream map[string]string
	activeByKey    map[string]string
	lastEnqueue    map[string]time.Time
	maxConcurrent  int
}

type PulseBackfillSnapshot struct {
	Active int
	Max    int
}

func NewPulseBackfillManager(
	sync *SyncService,
	store *Store,
	helix *HelixClient,
	rdb *redis.Client,
	heatmapCache *heatmap.Cache,
) *PulseBackfillManager {
	return &PulseBackfillManager{
		sync:           sync,
		store:          store,
		helix:          helix,
		rdb:            rdb,
		heatmapCache:   heatmapCache,
		runtime:        DefaultPulseRuntimeConfig(),
		jobs:           make(map[string]*PulseBackfillJob),
		activeByStream: make(map[string]string),
		activeByKey:    make(map[string]string),
		lastEnqueue:    make(map[string]time.Time),
	}
}

func (m *PulseBackfillManager) WithMaxConcurrent(n int) *PulseBackfillManager {
	if m != nil && n > 0 {
		m.maxConcurrent = n
	}
	return m
}

func (m *PulseBackfillManager) WithRuntime(cfg PulseRuntimeConfig) *PulseBackfillManager {
	if m != nil {
		m.runtime = cfg.withDefaults()
	}
	return m
}

func (m *PulseBackfillManager) Snapshot() PulseBackfillSnapshot {
	if m == nil {
		return PulseBackfillSnapshot{}
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return PulseBackfillSnapshot{
		Active: m.activeJobCountLocked(),
		Max:    m.maxConcurrent,
	}
}

func (m *PulseBackfillManager) activeJobCountLocked() int {
	n := 0
	for _, job := range m.jobs {
		if job != nil && !isPulseBackfillTerminal(job.Status) {
			n++
		}
	}
	return n
}

func (m *PulseBackfillManager) ActiveJobForStream(streamID string) *PulseBackfillJob {
	if m == nil {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	streamID = strings.TrimSpace(streamID)
	if jobID, ok := m.activeByStream[streamID]; ok {
		if job, ok := m.jobs[jobID]; ok && !isPulseBackfillTerminal(job.Status) {
			copy := *job
			return &copy
		}
	}
	for _, job := range m.jobs {
		if job != nil && job.StreamID == streamID && !isPulseBackfillTerminal(job.Status) {
			copy := *job
			return &copy
		}
	}
	return nil
}

func (m *PulseBackfillManager) GetJob(jobID string) *PulseBackfillJob {
	if m == nil {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	job, ok := m.jobs[jobID]
	if !ok {
		return nil
	}
	copy := *job
	return &copy
}

type PulseBackfillRequest struct {
	StreamID          string
	Login             string
	Mode              string
	FromOffsetSeconds int
	ToOffsetSeconds   int
	VodID             string
}

func (m *PulseBackfillManager) Enqueue(ctx context.Context, req PulseBackfillRequest) (*PulseBackfillJob, error) {
	if m == nil || m.sync == nil || m.store == nil {
		return nil, fmt.Errorf("pulse backfill unavailable")
	}
	streamID := strings.TrimSpace(req.StreamID)
	login, ok := validLogin(req.Login)
	if !ok || streamID == "" {
		return nil, fmt.Errorf("invalid channel or stream")
	}

	canonicalID, err := m.store.ResolveCanonicalStreamID(ctx, streamID)
	if err == nil && canonicalID != "" {
		streamID = canonicalID
	}

	stream, err := m.store.StreamByID(ctx, streamID)
	if err != nil {
		return nil, ErrPulseBackfillNoStream
	}
	mode := normalizePulseBackfillMode(req.Mode)

	hintVodID := strings.TrimSpace(req.VodID)
	if hintVodID != "" && strings.TrimSpace(stream.VodID) == "" {
		validatedVodID, err := validatePulseVodViaHelix(ctx, m.helix, *stream, login, hintVodID, m.runtime.withDefaults().HelixVodEnabled)
		if err != nil {
			return nil, err
		}
		if err := m.store.SetStreamVodID(ctx, streamID, validatedVodID, "extension_hint"); err == nil {
			stream.VodID = validatedVodID
		}
	}

	rollups, err := m.store.RollupsByStream(ctx, streamID)
	if err != nil {
		return nil, err
	}
	heatmapRollups, streamStart, err := consolidateHeatmapRollups(rollups, stream.StartedAt)
	if err != nil {
		return nil, err
	}
	offsets := completedChatOffsets(heatmapRollups, streamStart)

	fromOffset := req.FromOffsetSeconds
	toOffset := req.ToOffsetSeconds
	if toOffset <= 0 {
		toOffset = coverageStartOffsetSeconds(heatmapRollups, streamStart)
		if toOffset > 60 {
			toOffset -= 60
		}
	}
	if fromOffset < 0 {
		fromOffset = 0
	}
	if toOffset < fromOffset {
		toOffset = fromOffset
	}
	requestedRange := PulseBackfillRange{FromOffsetSeconds: fromOffset, ToOffsetSeconds: toOffset}
	jobKey := pulseBackfillJobKey(streamID, mode, requestedRange)

	if existing := m.activeJobForRange(streamID, mode, requestedRange); existing != nil {
		return existing, nil
	}

	m.mu.Lock()
	if m.maxConcurrent > 0 && m.activeJobCountLocked() >= m.maxConcurrent {
		m.mu.Unlock()
		return nil, ErrPulseBackfillAtCapacity
	}
	if last, ok := m.lastEnqueue[jobKey]; ok && time.Since(last) < pulseBackfillCooldown {
		for _, job := range m.jobs {
			if job.StreamID == streamID && job.Mode == mode && !isPulseBackfillTerminal(job.Status) &&
				pulseBackfillRangesOverlap(job.Range, requestedRange) {
				m.mu.Unlock()
				return job, nil
			}
		}
	}
	m.mu.Unlock()

	if rangeFullyCovered(offsets, fromOffset, toOffset) {
		job := &PulseBackfillJob{
			JobID:    newPulseBackfillJobID(),
			StreamID: streamID,
			Login:    login,
			Mode:     mode,
			Status:   PulseBackfillAlreadyAvailable,
			Message:  "Rollups already cover the requested range",
			Range:    requestedRange,
			CreatedAt: time.Now().UTC(),
			UpdatedAt: time.Now().UTC(),
		}
		m.storeJob(job)
		return job, nil
	}

	vodID := strings.TrimSpace(stream.VodID)
	if vodID != "" {
		if _, err := validatePulseVODCandidate(*stream, vodID); err != nil {
			return nil, err
		}
	}
	if vodID == "" && m.runtime.withDefaults().HelixVodEnabled && m.helix != nil && m.helix.Enabled() {
		broadcasterID := m.helix.ResolveBroadcasterID(ctx, login, stream.BroadcasterID)
		if broadcasterID != "" {
			if resolved, _ := m.helix.VideoIDByStreamID(ctx, broadcasterID, streamID); resolved != "" {
				vodID = resolved
			}
		}
	}
	if vodID == "" {
		if stream.EndedAt == nil {
			job := &PulseBackfillJob{
				JobID:     newPulseBackfillJobID(),
				StreamID:  streamID,
				Login:     login,
				Mode:      mode,
				Status:    PulseBackfillWaitingForVOD,
				Message:   "VOD chat not available yet",
				Range:     requestedRange,
				CreatedAt: time.Now().UTC(),
				UpdatedAt: time.Now().UTC(),
			}
			m.storeJob(job)
			return job, nil
		}
		job := &PulseBackfillJob{
			JobID:     newPulseBackfillJobID(),
			StreamID:  streamID,
			Login:     login,
			Mode:      mode,
			Status:    PulseBackfillFailed,
			Message:   "VOD chat replay is unavailable",
			Error:     "vod_unavailable",
			Range:     requestedRange,
			CreatedAt: time.Now().UTC(),
			UpdatedAt: time.Now().UTC(),
		}
		m.storeJob(job)
		return job, nil
	}

	job := &PulseBackfillJob{
		JobID:    newPulseBackfillJobID(),
		StreamID: streamID,
		Login:    login,
		Mode:     mode,
		Status:   PulseBackfillQueued,
		Message:  fmt.Sprintf("Backfill queued for %s–%s", formatCoverageOffset(fromOffset), formatCoverageOffset(toOffset)),
		Range:    requestedRange,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	m.storeJob(job)
	m.mu.Lock()
	m.activeByStream[streamID] = job.JobID
	m.activeByKey[jobKey] = job.JobID
	m.lastEnqueue[jobKey] = time.Now().UTC()
	m.mu.Unlock()

	go m.runJob(context.Background(), job.JobID, vodID)
	return job, nil
}

func (m *PulseBackfillManager) storeJob(job *PulseBackfillJob) {
	if m == nil || job == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.jobs[job.JobID] = job
}

func (m *PulseBackfillManager) updateJob(jobID string, mutate func(*PulseBackfillJob)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	job, ok := m.jobs[jobID]
	if !ok {
		return
	}
	mutate(job)
	job.UpdatedAt = time.Now().UTC()
}

func (m *PulseBackfillManager) finishJob(streamID, jobID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.activeByStream[streamID] == jobID {
		delete(m.activeByStream, streamID)
	}
	for key, activeJobID := range m.activeByKey {
		if activeJobID == jobID {
			delete(m.activeByKey, key)
		}
	}
}

func (m *PulseBackfillManager) runJob(ctx context.Context, jobID, vodID string) {
	job := m.GetJob(jobID)
	if job == nil {
		return
	}
	streamID := job.StreamID
	login := job.Login
	requestedFrom := job.Range.FromOffsetSeconds
	requestedTo := job.Range.ToOffsetSeconds

	defer m.finishJob(streamID, jobID)

	stream, err := m.store.StreamByID(ctx, streamID)
	if err != nil {
		m.updateJob(jobID, func(j *PulseBackfillJob) {
			j.Status = PulseBackfillFailed
			j.Message = "Stream not found"
			j.Error = err.Error()
		})
		return
	}
	beforeStart := m.coverageStartForStream(ctx, streamID, stream.StartedAt)

	m.updateJob(jobID, func(j *PulseBackfillJob) {
		j.Status = PulseBackfillResolvingVOD
		j.Message = "Resolving VOD"
	})

	done := make(chan struct{})
	go m.pollSyncProgress(jobID, streamID, done)
	defer close(done)

	syncErr := m.sync.SyncPulseMissedChat(ctx, streamID, login, vodID)
	if syncErr == nil {
		syncErr = m.verifyBackfillCoverage(ctx, streamID, stream.StartedAt, beforeStart, requestedFrom, requestedTo)
	}

	m.updateJob(jobID, func(j *PulseBackfillJob) {
		switch {
		case syncErr == nil:
			j.Status = PulseBackfillRefreshingMoments
			j.Message = "Refreshing Pulse moments"
		case syncErr == ErrPulseBackfillWaitingForVOD:
			j.Status = PulseBackfillWaitingForVOD
			j.Message = "VOD chat not available yet"
			j.Error = syncErr.Error()
		case syncErr == ErrPulseBackfillNoVOD:
			j.Status = PulseBackfillFailed
			j.Message = "VOD chat replay is unavailable"
			j.Error = syncErr.Error()
		case syncErr == ErrPulseBackfillNoData:
			j.Status = PulseBackfillFailed
			j.Message = "Twitch VOD chat does not include the missing stream start yet"
			j.Error = syncErr.Error()
		default:
			j.Status = PulseBackfillFailed
			j.Message = "Backfill failed"
			if syncErr != nil {
				j.Error = syncErr.Error()
			}
		}
	})

	if syncErr != nil {
		m.invalidatePulseBFFCache(ctx, login)
		return
	}

	m.invalidatePulseCaches(ctx, login, streamID)

	m.updateJob(jobID, func(j *PulseBackfillJob) {
		j.Status = PulseBackfillDone
		j.Message = "Missed moments loaded"
		j.Progress.Percent = 100
	})
}

func (m *PulseBackfillManager) coverageStartForStream(ctx context.Context, streamID string, streamStart time.Time) int {
	if m == nil || m.store == nil || streamStart.IsZero() {
		return 0
	}
	rollups, err := m.store.RollupsByStream(ctx, streamID)
	if err != nil || len(rollups) == 0 {
		return 0
	}
	heatmapRollups, _, err := consolidateHeatmapRollups(rollups, streamStart)
	if err != nil {
		return 0
	}
	return coverageStartOffsetSeconds(heatmapRollups, streamStart)
}

func (m *PulseBackfillManager) verifyBackfillCoverage(
	ctx context.Context,
	streamID string,
	streamStart time.Time,
	beforeStart int,
	fromOffset, toOffset int,
) error {
	if m == nil || m.store == nil || streamStart.IsZero() {
		return ErrPulseBackfillNoData
	}
	rollups, err := m.store.RollupsByStream(ctx, streamID)
	if err != nil {
		return err
	}
	heatmapRollups, _, err := consolidateHeatmapRollups(rollups, streamStart)
	if err != nil {
		return err
	}
	offsets := completedChatOffsets(heatmapRollups, streamStart)
	afterStart := coverageStartOffsetSeconds(heatmapRollups, streamStart)

	if rangeFullyCovered(offsets, fromOffset, toOffset) {
		return nil
	}
	if afterStart+coverageStartToleranceSec < beforeStart {
		return nil
	}
	return ErrPulseBackfillNoData
}

func (m *PulseBackfillManager) pollSyncProgress(jobID, streamID string, done <-chan struct{}) {
	ticker := time.NewTicker(pulseBackfillPollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-done:
			return
		case <-ticker.C:
			status, err := m.sync.GetSyncStatus(context.Background(), streamID)
			if err != nil || status == nil {
				continue
			}
			m.updateJob(jobID, func(j *PulseBackfillJob) {
				j.Status, j.Message, j.Progress = mapSyncToPulseBackfill(status)
			})
		}
	}
}

func mapSyncToPulseBackfill(st *SyncStatus) (status, message string, progress PulseBackfillProgress) {
	if st == nil {
		return PulseBackfillFetchingChat, "Working…", PulseBackfillProgress{}
	}
	message = st.Message
	switch st.Phase {
	case SyncPhaseResolvingVOD:
		return PulseBackfillResolvingVOD, message, progress
	case SyncPhaseStarting:
		return PulseBackfillEnsuringEmotes, message, progress
	case SyncPhaseFetchingComments:
		status = PulseBackfillFetchingChat
		if st.Chat != nil {
			progress.SegmentsDone = st.Chat.SegmentsDone
			progress.SegmentsTotal = st.Chat.SegmentsTotal
			if progress.SegmentsTotal > 0 {
				progress.Percent = progress.SegmentsDone * 100 / progress.SegmentsTotal
			}
			if st.Chat.IndexPhase == "tokenizing" {
				return PulseBackfillTokenizing, "Tokenizing chat and emotes", progress
			}
		}
		if message == "" {
			message = "Fetching missed chat replay"
		}
		return status, message, progress
	case SyncPhaseWritingRollups:
		return PulseBackfillWritingRollups, message, progress
	case SyncPhaseCompleted:
		return PulseBackfillRefreshingMoments, "Refreshing moments", progress
	case SyncPhaseFailed:
		return PulseBackfillFailed, message, progress
	default:
		return PulseBackfillFetchingChat, message, progress
	}
}

func (m *PulseBackfillManager) invalidatePulseBFFCache(ctx context.Context, login string) {
	InvalidatePulseBFFCache(ctx, m.rdb, login, nil)
}

func (m *PulseBackfillManager) invalidatePulseCaches(ctx context.Context, login, streamID string) {
	InvalidatePulseCaches(ctx, m.rdb, m.heatmapCache, login, streamID, nil)
}

const (
	PulseBackfillModePrefix       = "prefix"
	PulseBackfillModeMissingRange = "missing_range"
	PulseBackfillModeMomentWindow = "moment_window"
	PulseBackfillModeFullStream   = "full_stream"
)

func normalizePulseBackfillMode(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "missed", PulseBackfillModePrefix:
		return PulseBackfillModePrefix
	case PulseBackfillModeMissingRange:
		return PulseBackfillModeMissingRange
	case PulseBackfillModeMomentWindow:
		return PulseBackfillModeMomentWindow
	case PulseBackfillModeFullStream:
		return PulseBackfillModeFullStream
	default:
		return PulseBackfillModePrefix
	}
}

func pulseBackfillJobKey(streamID, mode string, r PulseBackfillRange) string {
	return strings.TrimSpace(streamID) + "|" + normalizePulseBackfillMode(mode) + "|" +
		fmt.Sprintf("%d:%d", r.FromOffsetSeconds, r.ToOffsetSeconds)
}

func pulseBackfillRangesOverlap(a, b PulseBackfillRange) bool {
	if a.ToOffsetSeconds < a.FromOffsetSeconds {
		a.ToOffsetSeconds = a.FromOffsetSeconds
	}
	if b.ToOffsetSeconds < b.FromOffsetSeconds {
		b.ToOffsetSeconds = b.FromOffsetSeconds
	}
	return a.FromOffsetSeconds <= b.ToOffsetSeconds && b.FromOffsetSeconds <= a.ToOffsetSeconds
}

func (m *PulseBackfillManager) activeJobForRange(streamID, mode string, r PulseBackfillRange) *PulseBackfillJob {
	if m == nil {
		return nil
	}
	streamID = strings.TrimSpace(streamID)
	mode = normalizePulseBackfillMode(mode)
	key := pulseBackfillJobKey(streamID, mode, r)
	m.mu.Lock()
	defer m.mu.Unlock()
	if jobID, ok := m.activeByKey[key]; ok {
		if job, ok := m.jobs[jobID]; ok && !isPulseBackfillTerminal(job.Status) {
			copy := *job
			return &copy
		}
	}
	for _, job := range m.jobs {
		if job == nil || job.StreamID != streamID || job.Mode != mode || isPulseBackfillTerminal(job.Status) {
			continue
		}
		if pulseBackfillRangesOverlap(job.Range, r) {
			copy := *job
			return &copy
		}
	}
	return nil
}

func consolidateHeatmapRollups(rollups []MinuteRollup, streamStart time.Time) ([]heatmap.MinuteRollup, time.Time, error) {
	out := make([]heatmap.MinuteRollup, len(rollups))
	for i, r := range rollups {
		out[i] = heatmap.MinuteRollup{
			MinuteTS:          r.MinuteTS,
			ViewerAvg:         r.ViewerAvg,
			ViewerMax:         r.ViewerMax,
			ViewerLatest:      r.ViewerLatest,
			ViewerSamples:     r.ViewerSamples,
			ChatCount:         r.ChatCount,
			TotalEmoteCount:   r.TotalEmoteCount,
			SevenTVEmoteCount: r.SevenTVEmoteCount,
			Emotes:            r.Emotes,
			Missing:           r.Missing,
		}
	}
	return out, streamStart, nil
}

func newPulseBackfillJobID() string {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return "pulse_backfill_" + hex.EncodeToString(b[:])
}

func isPulseBackfillTerminal(status string) bool {
	switch status {
	case PulseBackfillDone, PulseBackfillFailed, PulseBackfillCancelled, PulseBackfillAlreadyAvailable:
		return true
	default:
		return false
	}
}

func (m *PulseBackfillManager) BackfillFailedForStream(streamID string) bool {
	job := m.ActiveJobForStream(streamID)
	if job != nil {
		return false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, job := range m.jobs {
		if job.StreamID == streamID && job.Status == PulseBackfillFailed {
			if time.Since(job.UpdatedAt) < 5*time.Minute {
				return true
			}
		}
	}
	return false
}
