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

// SyncPulseMissedChat fetches VOD chat via GQL and patches minute rollups without
// TwitchTracker scraping or raw chat retention. Viewer samples are not required.
func (s *SyncService) SyncPulseMissedChat(ctx context.Context, streamID, login, hintVodID string) error {
	streamID = strings.TrimSpace(streamID)
	login = normalizeLogin(login)
	if streamID == "" || login == "" {
		return fmt.Errorf("missing stream or channel")
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

	pendingMinutes := 0
	for minuteOffset, comments := range commentsMap {
		if len(comments) == 0 || chatCache.has(minuteOffset) {
			continue
		}
		pendingMinutes++
	}
	if pendingMinutes == 0 {
		s.setSyncPhase(ctx, streamID, SyncPhaseFailed, "No chat replay data for the missing range", func(st *SyncStatus) {
			st.Error = ErrPulseBackfillNoData.Error()
		})
		return ErrPulseBackfillNoData
	}

	s.setSyncPhase(ctx, streamID, SyncPhaseWritingRollups, "Writing missed chat rollups", func(st *SyncStatus) {
		if st.Chat != nil {
			st.Chat.Active = false
			st.Chat.IndexPhase = "writing"
		}
	})
	if err := s.writeChatRollupsOnly(ctx, streamID, login, rollupStart, commentsMap, chatCache); err != nil {
		s.setSyncPhase(ctx, streamID, SyncPhaseFailed, "Failed writing rollups", func(st *SyncStatus) {
			st.Error = err.Error()
		})
		return err
	}

	s.setSyncPhase(ctx, streamID, SyncPhaseCompleted, "Missed moments loaded", func(st *SyncStatus) {
		if st.Chat != nil {
			st.Chat.IndexPhase = "done"
		}
	})
	return nil
}
