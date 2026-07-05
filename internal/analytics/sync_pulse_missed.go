package analytics

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

var (
	ErrPulseBackfillWaitingForVOD = errors.New("waiting_for_vod")
	ErrPulseBackfillNoVOD         = errors.New("vod_unavailable")
	ErrPulseBackfillNoStream      = errors.New("stream_not_found")
	ErrPulseBackfillNoData        = errors.New("no_chat_data_in_range")
)

// PulseMissedChatOptions bounds manual VOD import to a stream offset window.
type PulseMissedChatOptions struct {
	FromOffsetSeconds int
	ToOffsetSeconds   int
}

// SyncPulseMissedChat fetches VOD chat via GQL and patches minute rollups without
// TwitchTracker scraping or raw chat retention. Viewer samples are not required.
func (s *SyncService) SyncPulseMissedChat(ctx context.Context, streamID, login, hintVodID string, opts ...PulseMissedChatOptions) error {
	streamID = strings.TrimSpace(streamID)
	login = normalizeLogin(login)
	if streamID == "" || login == "" {
		return fmt.Errorf("missing stream or channel")
	}
	var missedOpts PulseMissedChatOptions
	if len(opts) > 0 {
		missedOpts = opts[0]
	}

	canonicalID, err := s.store.ResolveCanonicalStreamID(ctx, streamID)
	if err == nil && canonicalID != "" {
		streamID = canonicalID
	}

	stream, err := s.store.StreamByID(ctx, streamID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrPulseBackfillNoStream
		}
		return err
	}
	if login == "" {
		login = normalizeLogin(stream.Login)
	}

	broadcasterID := ""
	if s.helix != nil {
		broadcasterID = s.helix.ResolveBroadcasterID(ctx, login, stream.BroadcasterID)
	} else {
		broadcasterID = NormalizeBroadcasterID(stream.BroadcasterID)
	}

	s.setSyncPhase(ctx, streamID, SyncPhaseResolvingVOD, "Resolving VOD for missed chat replay", func(st *SyncStatus) {
		st.Channel = login
	})

	storedVodID := strings.TrimSpace(stream.VodID)
	vodID := strings.TrimSpace(hintVodID)
	if vodID != "" {
		if _, err := validatePulseVODCandidate(*stream, vodID); err != nil {
			return err
		}
		if vodID != storedVodID {
			validatedVodID, err := validatePulseVodViaHelix(ctx, s.helix, *stream, login, vodID, true)
			if err != nil {
				return err
			}
			vodID = validatedVodID
		}
	}
	if vodID == "" {
		vodID = storedVodID
		if vodID != "" {
			if _, err := validatePulseVODCandidate(*stream, vodID); err != nil {
				return err
			}
		}
	}
	if vodID == "" && broadcasterID != "" && s.helix != nil && s.helix.Enabled() {
		if resolved, _ := s.helix.VideoIDByStreamID(ctx, broadcasterID, streamID); resolved != "" {
			vodID = resolved
		}
	}
	if vodID == "" {
		if stream.EndedAt == nil {
			return ErrPulseBackfillWaitingForVOD
		}
		return ErrPulseBackfillNoVOD
	}
	if err := s.store.SetStreamVodID(ctx, streamID, vodID, "pulse_backfill"); err != nil {
		s.log.Warn("failed to persist vod_id for pulse backfill", "stream_id", streamID, "err", err)
	}

	s.setSyncPhase(ctx, streamID, SyncPhaseStarting, "Ensuring channel emotes", nil)
	if err := NewEmoteEnsureClient(s.emoteURL, s.log).RequireReadyForGold(ctx, login, broadcasterID, s.enricher); err != nil {
		return fmt.Errorf("emote ensure: %w", err)
	}

	commentsMap := make(map[int][]string)
	chatCache := newChatRollupCache()
	rollupStart := stream.StartedAt.UTC().Truncate(time.Minute)
	if rollupStart.IsZero() {
		return fmt.Errorf("stream start time missing")
	}
	rollupStartFn := func() time.Time { return rollupStart }

	resolveChatAlignSec := func(vod string) int {
		streamStart := stream.StartedAt
		vodCreated := time.Time{}
		if broadcasterID != "" && s.helix != nil && s.helix.Enabled() {
			if meta, err := s.helix.VideoByStreamID(ctx, broadcasterID, streamID); err == nil && !meta.CreatedAt.IsZero() {
				vodCreated = meta.CreatedAt
			}
		}
		if vodCreated.IsZero() {
			if createdAt, err := s.helix.VideoCreatedAt(ctx, vod); err == nil {
				vodCreated = createdAt
			}
		}
		return vodChatAlignSeconds(streamStart, vodCreated)
	}

	s.setSyncPhase(ctx, streamID, SyncPhaseFetchingComments, "Fetching missed chat replay", func(st *SyncStatus) {
		st.Chat = &SyncChatProgress{Active: true, VodID: vodID, FetchMode: "parallel"}
	})
	vodDur := s.vodDurationSeconds(ctx, vodID)
	chatAlignSec := resolveChatAlignSec(vodID)
	scheduleHints := s.gqlScheduleHintsForStream(ctx, streamID, vodDur, nil, nil)
	if err := s.fetchVODComments(ctx, streamID, login, vodID, commentsMap, vodDur, chatAlignSec, rollupStartFn, chatCache, scheduleHints); err != nil {
		s.setSyncPhase(ctx, streamID, SyncPhaseFailed, "VOD chat fetch failed", func(st *SyncStatus) {
			st.Error = err.Error()
		})
		return err
	}
	filterCommentsMapByOffsetRange(commentsMap, missedOpts.FromOffsetSeconds, missedOpts.ToOffsetSeconds)

	finalize := summarizeMissedChatFinalize(rollupStart, commentsMap, chatCache)
	diagnostics := s.missedChatDiagnostics(ctx, login, streamID, vodID, finalize, "")
	if finalize.PendingMinutes == 0 {
		if finalize.CommentsMatched > 0 && finalize.RollupsMatched > 0 {
			if err := s.store.RefreshStreamSummaryWithMode(ctx, streamID, "pulse_backfill_cached_finalize"); err != nil {
				s.setSyncPhase(ctx, streamID, SyncPhaseFailed, "Summary refresh failed", func(st *SyncStatus) {
					st.Error = err.Error()
					if st.Chat != nil {
						st.Chat.Diagnostics = diagnostics
					}
				})
				return err
			}
			s.log.Info("missed chat replay already finalized by incremental rollup flush",
				"stream_id", streamID,
				"vod_id", vodID,
				"comments_matched", finalize.CommentsMatched,
				"rollups_matched", finalize.RollupsMatched,
				"query_start", diagnostics.QueryStart,
				"query_end", diagnostics.QueryEnd,
			)
			s.setSyncPhase(ctx, streamID, SyncPhaseCompleted, "Missed moments loaded", func(st *SyncStatus) {
				st.RollupsWritten = finalize.RollupsMatched
				if st.Chat != nil {
					st.Chat.Active = false
					st.Chat.IndexPhase = "done"
					st.Chat.RollupsExpected = finalize.RollupMinutes
					st.Chat.SummaryRefreshDeferred = false
					st.Chat.Message = "Minute rollups already written"
					st.Chat.Diagnostics = diagnostics
				}
			})
			return nil
		}

		diagnostics = s.missedChatDiagnostics(ctx, login, streamID, vodID, finalize, ErrPulseBackfillNoData.Error())
		s.log.Warn("no chat replay data in requested range",
			"login", login,
			"stream_id", streamID,
			"vod_id", vodID,
			"query_start", diagnostics.QueryStart,
			"query_end", diagnostics.QueryEnd,
			"offset_start", diagnostics.OffsetStart,
			"offset_end", diagnostics.OffsetEnd,
			"comments_matched", diagnostics.CommentsMatched,
			"comments_total_for_stream", diagnostics.CommentsTotalForStream,
			"rollups_matched", diagnostics.RollupsMatched,
			"rollups_total_for_stream", diagnostics.RollupsTotalForStream,
		)
		s.setSyncPhase(ctx, streamID, SyncPhaseFailed, "No chat replay data for the missing range", func(st *SyncStatus) {
			st.Error = ErrPulseBackfillNoData.Error()
			if st.Chat != nil {
				st.Chat.Active = false
				st.Chat.IndexPhase = "done"
				st.Chat.SummaryRefreshDeferred = false
				st.Chat.Message = "No chat replay data for the requested range"
				st.Chat.Diagnostics = diagnostics
			}
		})
		return ErrPulseBackfillNoData
	}

	s.setSyncPhase(ctx, streamID, SyncPhaseWritingRollups, "Writing missed chat rollups", func(st *SyncStatus) {
		if st.Chat != nil {
			st.Chat.Active = false
			st.Chat.IndexPhase = "writing"
			st.Chat.RollupsExpected = finalize.RollupMinutes
			st.Chat.Diagnostics = diagnostics
		}
	})
	if err := s.writeManualImportChatRollups(ctx, streamID, login, rollupStart, commentsMap, chatCache); err != nil {
		s.setSyncPhase(ctx, streamID, SyncPhaseFailed, "Failed writing rollups", func(st *SyncStatus) {
			st.Error = err.Error()
		})
		return err
	}

	s.setSyncPhase(ctx, streamID, SyncPhaseCompleted, "Missed moments loaded", func(st *SyncStatus) {
		st.RollupsWritten = finalize.PendingMinutes
		if st.Chat != nil {
			st.Chat.Active = false
			st.Chat.IndexPhase = "done"
			st.Chat.SummaryRefreshDeferred = false
			st.Chat.RollupsExpected = finalize.RollupMinutes
			st.Chat.Diagnostics = diagnostics
		}
	})
	return nil
}

type missedChatFinalizeSummary struct {
	CommentsMatched int
	RollupMinutes   int
	RollupsMatched  int
	PendingMinutes  int
	OffsetStart     int
	OffsetEnd       int
	QueryStart      time.Time
	QueryEnd        time.Time
}

func summarizeMissedChatFinalize(rollupStart time.Time, commentsMap map[int][]string, cache *chatRollupCache) missedChatFinalizeSummary {
	summary := missedChatFinalizeSummary{OffsetStart: -1, OffsetEnd: -1}
	firstMinute := 0
	lastMinute := 0
	for minuteOffset, comments := range commentsMap {
		if len(comments) == 0 {
			continue
		}
		summary.CommentsMatched += len(comments)
		summary.RollupMinutes++
		if summary.OffsetStart < 0 || minuteOffset < firstMinute {
			firstMinute = minuteOffset
			summary.OffsetStart = minuteOffset * 60
		}
		if summary.OffsetEnd < 0 || minuteOffset > lastMinute {
			lastMinute = minuteOffset
			summary.OffsetEnd = minuteOffset*60 + 59
		}
		if cache.has(minuteOffset) {
			summary.RollupsMatched++
			continue
		}
		summary.PendingMinutes++
	}
	if !rollupStart.IsZero() && summary.OffsetStart >= 0 {
		summary.QueryStart = rollupStart.Add(time.Duration(firstMinute) * time.Minute)
		summary.QueryEnd = rollupStart.Add(time.Duration(lastMinute+1) * time.Minute)
	}
	return summary
}

func (s *SyncService) missedChatDiagnostics(ctx context.Context, login, streamID, vodID string, summary missedChatFinalizeSummary, reason string) *SyncChatDiagnostics {
	diagnostics := &SyncChatDiagnostics{
		Login:           login,
		StreamID:        streamID,
		VodID:           vodID,
		OffsetStart:     summary.OffsetStart,
		OffsetEnd:       summary.OffsetEnd,
		CommentsMatched: summary.CommentsMatched,
		RollupsMatched:  summary.RollupsMatched,
		RangeMode:       "offset",
		Reason:          reason,
	}
	if !summary.QueryStart.IsZero() {
		diagnostics.QueryStart = summary.QueryStart.UTC().Format(time.RFC3339)
	}
	if !summary.QueryEnd.IsZero() {
		diagnostics.QueryEnd = summary.QueryEnd.UTC().Format(time.RFC3339)
	}
	if s == nil || s.store == nil || s.store.db == nil || strings.TrimSpace(streamID) == "" {
		return diagnostics
	}
	if err := s.store.db.QueryRow(ctx, `SELECT COUNT(*) FROM analytics_vod_chat_messages WHERE stream_id=$1`, streamID).Scan(&diagnostics.CommentsTotalForStream); err != nil {
		s.log.Warn("failed to count stream chat diagnostics", "stream_id", streamID, "err", err)
	}
	if err := s.store.db.QueryRow(ctx, `SELECT COUNT(*) FROM analytics_minute_rollups WHERE stream_id=$1 AND (chat_count > 0 OR seventv_emote_count > 0 OR total_emote_count > 0)`, streamID).Scan(&diagnostics.RollupsTotalForStream); err != nil {
		s.log.Warn("failed to count stream rollup diagnostics", "stream_id", streamID, "err", err)
	}
	return diagnostics
}

func filterCommentsMapByOffsetRange(commentsMap map[int][]string, fromOffsetSec, toOffsetSec int) {
	if commentsMap == nil || (fromOffsetSec <= 0 && toOffsetSec <= 0) {
		return
	}
	for minute := range commentsMap {
		minuteStart := minute * 60
		minuteEnd := minuteStart + 59
		if toOffsetSec > 0 && minuteStart > toOffsetSec {
			delete(commentsMap, minute)
			continue
		}
		if fromOffsetSec > 0 && minuteEnd < fromOffsetSec {
			delete(commentsMap, minute)
		}
	}
}
