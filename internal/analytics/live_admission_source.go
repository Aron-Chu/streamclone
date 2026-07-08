package analytics

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"streamclone/internal/config"
)

const (
	PulseLiveAdmissionSourceHelixTopLive    = "helix_top_live"
	PulseLiveAdmissionSourceRoster          = "roster"
	PulseLiveAdmissionSourceRosterThenHelix = "roster_then_helix"
)

// LiveAdmissionSource returns live IRC admission candidates ordered by priority
// (viewer rank for helix_top_live, roster metadata for legacy roster mode).
type LiveAdmissionSource interface {
	ListLiveCandidates(ctx context.Context, topN int) ([]Top500Current, error)
}

type rosterTopLiveAdmissionStore interface {
	ListTop500LiveForPriorityWatch(ctx context.Context, topN, limit int) ([]Top500Current, error)
}

type RosterTopLiveAdmissionSource struct {
	store rosterTopLiveAdmissionStore
}

func (s *RosterTopLiveAdmissionSource) ListLiveCandidates(ctx context.Context, topN int) ([]Top500Current, error) {
	if s == nil || s.store == nil {
		return nil, nil
	}
	if topN <= 0 {
		topN = DefaultTop500MetadataTopN
	}
	return s.store.ListTop500LiveForPriorityWatch(ctx, topN, topN)
}

type HelixTopLiveAdmissionSource struct {
	helix *HelixClient
	log   *slog.Logger
}

func (s *HelixTopLiveAdmissionSource) ListLiveCandidates(ctx context.Context, topN int) ([]Top500Current, error) {
	if s == nil || s.helix == nil || !s.helix.Enabled() {
		return nil, ErrHelixDisabled
	}
	if topN <= 0 {
		topN = DefaultTop500MetadataTopN
	}
	streams, err := s.helix.TopLiveStreams(ctx, topN)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	staleAfter := now.Add(DefaultTop500MetadataStaleAfter)
	out := make([]Top500Current, 0, len(streams))
	for i, stream := range streams {
		login := normalizeLogin(stream.Login)
		if login == "" {
			continue
		}
		streamID := strings.TrimSpace(stream.ID)
		if streamID == "" {
			continue
		}
		viewerCount := stream.ViewerCount
		startedAt := stream.StartedAt
		var startedPtr *time.Time
		if !startedAt.IsZero() {
			started := startedAt
			startedPtr = &started
		}
		out = append(out, Top500Current{
			ChannelID:      strings.TrimSpace(stream.BroadcasterID),
			Login:          login,
			DisplayName:    firstNonEmpty(stream.DisplayName, login),
			Rank:           i + 1,
			CoverageSource: Top500CoverageSourceHelix,
			IsLive:         true,
			StreamID:       &streamID,
			Title:          stream.Title,
			CategoryName:   stream.GameName,
			StartedAt:      startedPtr,
			ViewerCount:    &viewerCount,
			Language:       stream.Language,
			Tags:           append([]string(nil), stream.Tags...),
			SampledAt:      now,
			StaleAfter:     staleAfter,
			LastSuccessAt:  &now,
		})
	}
	return out, nil
}

// BlendedRosterFirstAdmissionSource admits metadata-roster live first, then Helix top-live fill.
type BlendedRosterFirstAdmissionSource struct {
	roster     *RosterTopLiveAdmissionSource
	helix      *HelixTopLiveAdmissionSource
	log        *slog.Logger
	rosterTopN int
}

func (s *BlendedRosterFirstAdmissionSource) ListLiveCandidates(ctx context.Context, topN int) ([]Top500Current, error) {
	if s == nil {
		return nil, nil
	}
	if topN <= 0 {
		topN = DefaultTop500MetadataTopN
	}
	rosterLimit := s.rosterTopN
	if rosterLimit <= 0 {
		rosterLimit = DefaultTop500MetadataTopN
	}
	var rosterLive []Top500Current
	if s.roster != nil {
		live, err := s.roster.ListLiveCandidates(ctx, rosterLimit)
		if err != nil {
			return nil, err
		}
		rosterLive = live
	}
	var helixLive []Top500Current
	if s.helix != nil {
		live, err := s.helix.ListLiveCandidates(ctx, topN)
		if err != nil {
			if s.log != nil {
				s.log.Warn("blended admission helix list failed; roster live only", "err", err)
			}
		} else {
			helixLive = live
		}
	}
	return mergeLiveAdmissionRosterFirst(rosterLive, helixLive, topN), nil
}

func mergeLiveAdmissionRosterFirst(rosterLive, helixLive []Top500Current, topN int) []Top500Current {
	if topN <= 0 {
		topN = DefaultTop500MetadataTopN
	}
	seen := make(map[string]struct{})
	out := make([]Top500Current, 0, topN+len(rosterLive))
	for _, row := range rosterLive {
		if !row.IsLive {
			continue
		}
		login := normalizeLogin(row.Login)
		if login == "" {
			continue
		}
		if _, ok := seen[login]; ok {
			continue
		}
		seen[login] = struct{}{}
		row.Login = login
		if strings.TrimSpace(row.CoverageSource) == "" {
			row.CoverageSource = Top500CoverageSourceMetadata
		}
		out = append(out, row)
	}
	if len(out) >= topN {
		return out
	}
	for _, row := range helixLive {
		if len(out) >= topN {
			break
		}
		if !row.IsLive {
			continue
		}
		login := normalizeLogin(row.Login)
		if login == "" {
			continue
		}
		if _, ok := seen[login]; ok {
			continue
		}
		seen[login] = struct{}{}
		row.Login = login
		row.CoverageSource = Top500CoverageSourceHelix
		out = append(out, row)
	}
	return out
}

func rosterCorpusTopN(cfg config.Config) int {
	topN := cfg.Top500MetadataTopN
	if cfg.CorpusTargetTopN > 0 {
		topN = cfg.CorpusTargetTopN
	}
	if topN <= 0 {
		topN = DefaultTop500MetadataTopN
	}
	return topN
}

// NewReadinessLiveAdmissionSource returns DB-backed roster metadata for readiness
// and public hub reports. Status endpoints must not call Helix directly; IRC
// admission uses NewLiveAdmissionSource instead.
func NewReadinessLiveAdmissionSource(store rosterTopLiveAdmissionStore) LiveAdmissionSource {
	if store == nil {
		return nil
	}
	return &RosterTopLiveAdmissionSource{store: store}
}

// NewLiveAdmissionSource selects helix top-live or roster metadata based on config.
// When helix_top_live is configured but Helix credentials are absent, falls back to roster.
func NewLiveAdmissionSource(cfg config.Config, store rosterTopLiveAdmissionStore, helix *HelixClient, log *slog.Logger) LiveAdmissionSource {
	if log == nil {
		log = slog.Default()
	}
	source := normalizeLiveAdmissionSource(cfg.PulseLiveAdmissionSource)
	if source == PulseLiveAdmissionSourceRoster {
		if store == nil {
			return nil
		}
		return &RosterTopLiveAdmissionSource{store: store}
	}
	if source == PulseLiveAdmissionSourceRosterThenHelix {
		if store == nil {
			return nil
		}
		blended := &BlendedRosterFirstAdmissionSource{
			roster:     &RosterTopLiveAdmissionSource{store: store},
			log:        log,
			rosterTopN: rosterCorpusTopN(cfg),
		}
		if helix != nil && helix.Enabled() {
			blended.helix = &HelixTopLiveAdmissionSource{helix: helix, log: log}
		} else if log != nil {
			log.Warn("blended admission helix unavailable; roster live only",
				"configured_source", source,
			)
		}
		return blended
	}
	if helix != nil && helix.Enabled() {
		return &HelixTopLiveAdmissionSource{helix: helix, log: log}
	}
	log.Warn("helix top-live admission unavailable; falling back to roster metadata",
		"configured_source", source,
	)
	if store == nil {
		return &HelixTopLiveAdmissionSource{helix: helix, log: log}
	}
	return &RosterTopLiveAdmissionSource{store: store}
}

func normalizeLiveAdmissionSource(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case PulseLiveAdmissionSourceRoster:
		return PulseLiveAdmissionSourceRoster
	case PulseLiveAdmissionSourceRosterThenHelix:
		return PulseLiveAdmissionSourceRosterThenHelix
	default:
		return PulseLiveAdmissionSourceHelixTopLive
	}
}

// TrackPriorityForAdmissionCoverageSource maps roster vs Helix fill rows for ingest reconcile.
func TrackPriorityForAdmissionCoverageSource(source string) int {
	switch strings.ToLower(strings.TrimSpace(source)) {
	case Top500CoverageSourceMetadata:
		return TrackPriorityCorpusRosterLive
	case Top500CoverageSourceHelix:
		return TrackPriorityHelixTopLiveFill
	default:
		return TrackPriorityTopRoster
	}
}

// Deprecated: use PulseLiveAdmissionSourceHelixTopLive.
const PulseTop500AdmissionSourceHelixTopLive = PulseLiveAdmissionSourceHelixTopLive

// Deprecated: use PulseLiveAdmissionSourceRoster.
const PulseTop500AdmissionSourceRoster = PulseLiveAdmissionSourceRoster

// Deprecated: use normalizeLiveAdmissionSource.
func normalizePulseTop500AdmissionSource(raw string) string {
	return normalizeLiveAdmissionSource(raw)
}

// Deprecated alias retained for one release.
func normalizePulseLiveAdmissionSource(raw string) string {
	return normalizeLiveAdmissionSource(raw)
}
