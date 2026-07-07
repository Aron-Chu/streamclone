package analytics

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/redis/go-redis/v9"
)

const (
	extCoverageCachePrefix = "ext:coverage:v1:"
	extCoverageCacheTTL    = 15 * time.Second

	// extensionMetadataFreshnessMax bounds top500_metadata_only to recent live snapshots.
	extensionMetadataFreshnessMax = 15 * time.Minute

	CoverageTierActiveLiveCoverage   = "active_live_coverage"
	CoverageTierRosterMetadataOnly   = "roster_metadata_only"
	CoverageTierTop500MetadataOnly   = "top500_metadata_only" // Deprecated API alias; see coverageTierForAPI
	CoverageTierHistoricalEnriched   = "historical_enriched"
	CoverageTierOnDemandAvailable    = "on_demand_available"
	CoverageTierBudgetLimited        = "budget_limited"
	CoverageTierUnknownOrUnsupported = "unknown_or_unsupported"

	reasonHostedCapFull           = "hosted_cap_full"
	reasonNoStreamRecord          = "no_stream_record"
	reasonMetadataWithoutChat     = "metadata_without_active_chat"
	reasonTop500MetadataAvailable = "top500_metadata_available"
	reasonMetadataStale           = "metadata_stale"
	reasonMetadataOffline         = "metadata_offline_not_live"
	reasonHistoricalAvailable     = "historical_analytics_available"
	reasonActiveCollector         = "active_collector_attached"
)

// ExtensionCoverageTierResponse is the read-only Top 500 coverage contract for the extension.
type ExtensionCoverageTierResponse struct {
	Login            string                        `json:"login"`
	ChannelID        *string                       `json:"channelId"`
	DisplayName      *string                       `json:"displayName"`
	CoverageTier     string                        `json:"coverageTier"`
	HostedCap        ExtensionHostedCapStatus      `json:"hostedCap"`
	LiveMetadata     ExtensionCoverageLiveMetadata `json:"liveMetadata"`
	DataAvailability ExtensionDataAvailability     `json:"dataAvailability"`
	Actions          ExtensionCoverageActions      `json:"actions"`
	ReasonCodes      []string                      `json:"reasonCodes"`
}

type ExtensionHostedCapStatus struct {
	ActiveLimit     int  `json:"activeLimit"`
	ActiveCount     *int `json:"activeCount"`
	ActiveAvailable bool `json:"activeAvailable"`
	BackfillLimit   *int `json:"backfillLimit"`
	BackfillActive  *int `json:"backfillActive"`
}

type ExtensionCoverageLiveMetadata struct {
	Available        bool     `json:"available"`
	Source           string   `json:"source"`
	IsLive           *bool    `json:"isLive"`
	StreamID         *string  `json:"streamId"`
	Title            *string  `json:"title"`
	Category         *string  `json:"category"`
	StartedAt        *string  `json:"startedAt"`
	ViewerCount      *int     `json:"viewerCount"`
	Language         *string  `json:"language"`
	Tags             []string `json:"tags"`
	SnapshotTime     *string  `json:"snapshotTime"`
	FreshnessSeconds *int     `json:"freshnessSeconds"`
}

type ExtensionDataAvailability struct {
	Rollups          bool `json:"rollups"`
	Peaks            bool `json:"peaks"`
	Moments          bool `json:"moments"`
	Heatmap          bool `json:"heatmap"`
	VodBackfill      bool `json:"vodBackfill"`
	SevenTvSet       bool `json:"sevenTvSet"`
	HistoricalSilver bool `json:"historicalSilver"`
	HistoricalGold   bool `json:"historicalGold"`
}

type ExtensionCoverageActions struct {
	CanStartTracking         bool `json:"canStartTracking"`
	CanLoadChatAnalytics     bool `json:"canLoadChatAnalytics"`
	CanBackfillMissedMoments bool `json:"canBackfillMissedMoments"`
	CanOpenStreamPulse       bool `json:"canOpenStreamPulse"`
}

type extensionCoverageInputs struct {
	login            string
	stream           *StreamRecord
	isLive           bool
	tracking         bool
	rollups          []MinuteRollup
	top500Current    *Top500Current
	historicalChat   bool
	historicalGold   bool
	emoteSync        EmoteSyncSnapshot
	hostedCap        ExtensionHostedCapStatus
	hasChatRollups   bool
	hasViewerRollups bool
}

func (h *Handler) extensionPulseChannelCoverage(w http.ResponseWriter, r *http.Request) {
	login, ok := validLogin(chi.URLParam(r, "login"))
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_channel"})
		return
	}

	ctx := r.Context()
	cacheKey := extCoverageCachePrefix + login
	cacheEnabled := h.pulseRuntimeConfig().BFFCacheEnabled
	if cacheEnabled && h.rdb != nil {
		if cached, err := h.rdb.Get(ctx, cacheKey).Bytes(); err == nil && len(cached) > 0 {
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("X-Cache", "HIT")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(cached)
			return
		}
	}

	payload, err := h.buildExtensionCoverageTier(ctx, login)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	body, _ := json.Marshal(payload)
	if cacheEnabled && h.rdb != nil {
		_ = h.rdb.Set(ctx, cacheKey, body, extCoverageCacheTTL).Err()
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Cache", "MISS")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}

func (h *Handler) buildExtensionCoverageTier(ctx context.Context, login string) (ExtensionCoverageTierResponse, error) {
	inputs := extensionCoverageInputs{
		login:     login,
		emoteSync: defaultExtensionEmoteSync(false),
	}

	inputs.tracking = h.isLoginTracked(login)
	inputs.hostedCap = h.extensionHostedCapStatus()

	if h.collector != nil {
		snap := h.collector.EmoteSyncSnapshot(ctx, login)
		inputs.emoteSync = snap
	}

	if h.store != nil {
		top500Current, err := h.store.GetTop500CurrentByLogin(ctx, login)
		if err != nil {
			return ExtensionCoverageTierResponse{}, err
		}
		inputs.top500Current = top500Current

		stream, err := h.store.LatestStreamByLogin(ctx, login)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return ExtensionCoverageTierResponse{}, err
		}
		if stream != nil {
			stream, isLive, err := h.reconcileExtensionLiveStream(ctx, login, stream, inputs.tracking)
			if err != nil {
				return ExtensionCoverageTierResponse{}, err
			}
			inputs.stream = stream
			inputs.isLive = isLive
			rollups, err := h.store.RollupsByStream(ctx, stream.StreamID)
			if err != nil {
				return ExtensionCoverageTierResponse{}, err
			}
			inputs.rollups = rollups
			inputs.hasChatRollups, inputs.hasViewerRollups = rollupAvailabilityFlags(rollups)

			past, err := h.store.StreamsByLogin(ctx, login, 8)
			if err != nil {
				return ExtensionCoverageTierResponse{}, err
			}
			for _, s := range past {
				if s.StreamID == stream.StreamID {
					continue
				}
				if s.ChatMessages > 0 {
					inputs.historicalChat = true
				}
				if s.ChatMessages > 0 && strings.TrimSpace(s.VodID) != "" {
					inputs.historicalGold = true
				}
			}
			if !inputs.historicalChat && !inputs.isLive && inputs.hasChatRollups {
				inputs.historicalChat = true
			}
			if !inputs.historicalGold && strings.TrimSpace(stream.VodID) != "" && inputs.hasChatRollups {
				inputs.historicalGold = true
			}
		}
	}

	return assembleExtensionCoverageResponse(inputs), nil
}

func (h *Handler) extensionHostedCapStatus() ExtensionHostedCapStatus {
	activeLimit := 0
	var activeCount *int
	activeAvailable := true

	if h.pulseHosted.MaxActiveChannels > 0 {
		activeLimit = h.pulseHosted.MaxActiveChannels
	}
	if h.collector != nil {
		snap := h.collector.TrackingSnapshot()
		if snap.Max > 0 {
			activeLimit = snap.Max
		}
		n := snap.Active
		activeCount = &n
		activeAvailable = activeLimit <= 0 || n < activeLimit
	} else if activeLimit > 0 {
		zero := 0
		activeCount = &zero
		activeAvailable = true
	}

	var backfillLimit, backfillActive *int
	if h.pulseBackfill != nil {
		snap := h.pulseBackfill.Snapshot()
		if snap.Max > 0 {
			backfillLimit = intPtr(snap.Max)
		}
		if snap.Active >= 0 {
			backfillActive = intPtr(snap.Active)
		}
	}
	if backfillLimit == nil {
		if max := envIntDefault("PULSE_MAX_BACKFILLS", 0); max > 0 {
			backfillLimit = intPtr(max)
		}
	}

	return ExtensionHostedCapStatus{
		ActiveLimit:     activeLimit,
		ActiveCount:     activeCount,
		ActiveAvailable: activeAvailable,
		BackfillLimit:   backfillLimit,
		BackfillActive:  backfillActive,
	}
}

func assembleExtensionCoverageResponse(in extensionCoverageInputs) ExtensionCoverageTierResponse {
	tier, reasons := mapExtensionCoverageTier(in)
	availability := buildExtensionDataAvailability(in)
	actions := buildExtensionCoverageActions(in, tier)
	liveMetadata := buildExtensionLiveMetadata(in)

	var channelID, displayName *string
	if in.stream != nil {
		if id := strings.TrimSpace(in.stream.BroadcasterID); id != "" {
			channelID = &id
		}
		if name := strings.TrimSpace(in.stream.DisplayName); name != "" {
			displayName = &name
		} else if login := strings.TrimSpace(in.stream.Login); login != "" {
			displayName = &login
		}
	}
	if in.top500Current != nil {
		if channelID == nil {
			if id := strings.TrimSpace(in.top500Current.ChannelID); id != "" {
				channelID = &id
			}
		}
		if displayName == nil {
			if name := strings.TrimSpace(in.top500Current.DisplayName); name != "" {
				displayName = &name
			} else if login := strings.TrimSpace(in.top500Current.Login); login != "" {
				displayName = &login
			}
		}
	}

	return ExtensionCoverageTierResponse{
		Login:            in.login,
		ChannelID:        channelID,
		DisplayName:      displayName,
		CoverageTier:     coverageTierForAPI(tier),
		HostedCap:        in.hostedCap,
		LiveMetadata:     liveMetadata,
		DataAvailability: availability,
		Actions:          actions,
		ReasonCodes:      reasons,
	}
}

func mapExtensionCoverageTier(in extensionCoverageInputs) (string, []string) {
	reasons := make([]string, 0, 4)

	if in.tracking {
		reasons = append(reasons, reasonActiveCollector)
		return CoverageTierActiveLiveCoverage, reasons
	}

	capFull := !in.hostedCap.ActiveAvailable && in.hostedCap.ActiveLimit > 0
	if hasFreshLiveMetadataSnapshot(in) {
		reasons = append(reasons, reasonMetadataWithoutChat)
		if hasTop500Metadata(in) {
			reasons = append(reasons, reasonTop500MetadataAvailable)
		}
		if capFull {
			reasons = append(reasons, reasonHostedCapFull)
		}
		return CoverageTierRosterMetadataOnly, reasons
	}

	if in.historicalChat || in.historicalGold {
		reasons = append(reasons, reasonHistoricalAvailable)
		if capFull {
			reasons = append(reasons, reasonHostedCapFull)
		}
		return CoverageTierHistoricalEnriched, reasons
	}

	if capFull {
		reasons = append(reasons, reasonHostedCapFull)
		return CoverageTierBudgetLimited, reasons
	}

	if in.stream != nil || in.top500Current != nil {
		if hasLiveMetadataSnapshot(in) {
			if !coverageMetadataLive(in) {
				reasons = append(reasons, reasonMetadataOffline)
			} else if !metadataSnapshotFresh(in) {
				reasons = append(reasons, reasonMetadataStale)
			}
		}
		return CoverageTierOnDemandAvailable, reasons
	}

	reasons = append(reasons, reasonNoStreamRecord)
	return CoverageTierUnknownOrUnsupported, reasons
}

func buildExtensionCoverageActions(in extensionCoverageInputs, tier string) ExtensionCoverageActions {
	capAvailable := in.hostedCap.ActiveAvailable || in.hostedCap.ActiveLimit <= 0
	canStart := !in.tracking && capAvailable
	canLoad := !in.tracking && capAvailable && (isRosterMetadataCoverageTier(tier) ||
		tier == CoverageTierOnDemandAvailable ||
		tier == CoverageTierHistoricalEnriched)

	canBackfill := in.tracking && in.hasChatRollups && in.stream != nil &&
		strings.TrimSpace(in.stream.VodID) != "" && tier == CoverageTierActiveLiveCoverage

	if tier == CoverageTierBudgetLimited {
		canStart = false
		canLoad = false
	}

	return ExtensionCoverageActions{
		CanStartTracking:         canStart,
		CanLoadChatAnalytics:     canLoad,
		CanBackfillMissedMoments: canBackfill,
		CanOpenStreamPulse:       true,
	}
}

func buildExtensionDataAvailability(in extensionCoverageInputs) ExtensionDataAvailability {
	hasRollups := in.hasChatRollups
	hasHeatmap := hasRollups
	hasPeaks := hasRollups
	hasMoments := hasRollups
	sevenTv := in.emoteSync.State == EmoteSyncReady || in.emoteSync.State == EmoteSyncAggregateOnly ||
		in.emoteSync.State == EmoteSyncStale || in.emoteSync.State == EmoteSyncSyncing

	vodBackfill := in.stream != nil && strings.TrimSpace(in.stream.VodID) != "" && in.tracking

	return ExtensionDataAvailability{
		Rollups:          hasRollups,
		Peaks:            hasPeaks,
		Moments:          hasMoments,
		Heatmap:          hasHeatmap,
		VodBackfill:      vodBackfill,
		SevenTvSet:       sevenTv,
		HistoricalSilver: in.historicalChat,
		HistoricalGold:   in.historicalGold,
	}
}

func buildExtensionLiveMetadata(in extensionCoverageInputs) ExtensionCoverageLiveMetadata {
	out := ExtensionCoverageLiveMetadata{
		Available: false,
		Source:    "none",
		Tags:      []string{},
	}
	if in.stream == nil && in.top500Current == nil {
		return out
	}

	out.Available = hasLiveMetadataSnapshot(in)
	if !out.Available {
		return out
	}
	if !in.tracking && in.top500Current != nil && metadataSourceForTop500Current(in.top500Current) != "none" {
		current := in.top500Current
		out.Source = metadataSourceForTop500Current(current)
		isLive := current.IsLive
		out.IsLive = &isLive
		if sid := strings.TrimSpace(ptrStringValue(current.StreamID)); sid != "" {
			out.StreamID = &sid
		}
		if title := strings.TrimSpace(current.Title); title != "" {
			out.Title = &title
		}
		if cat := strings.TrimSpace(current.CategoryName); cat != "" {
			out.Category = &cat
		}
		if current.StartedAt != nil && !current.StartedAt.IsZero() {
			started := current.StartedAt.UTC().Format(time.RFC3339)
			out.StartedAt = &started
		}
		if current.ViewerCount != nil {
			vc := *current.ViewerCount
			out.ViewerCount = &vc
		}
		if lang := strings.TrimSpace(current.Language); lang != "" {
			out.Language = &lang
		}
		if len(current.Tags) > 0 {
			out.Tags = append([]string(nil), current.Tags...)
		}
		if !current.SampledAt.IsZero() {
			snap := current.SampledAt.UTC().Format(time.RFC3339)
			out.SnapshotTime = &snap
			out.FreshnessSeconds = current.FreshnessSeconds(time.Now().UTC())
		}
		return out
	}

	stream := in.stream

	out.Source = metadataSourceForStream(stream, in.tracking)
	isLive := in.isLive
	out.IsLive = &isLive
	if sid := strings.TrimSpace(stream.StreamID); sid != "" {
		out.StreamID = &sid
	}
	if title := strings.TrimSpace(stream.Title); title != "" {
		out.Title = &title
	}
	if cat := strings.TrimSpace(stream.Category); cat != "" {
		out.Category = &cat
	}
	if !stream.StartedAt.IsZero() {
		started := stream.StartedAt.UTC().Format(time.RFC3339)
		out.StartedAt = &started
	}
	if stream.CurrentViewers > 0 {
		vc := stream.CurrentViewers
		out.ViewerCount = &vc
	}
	if lang := strings.TrimSpace(stream.Language); lang != "" {
		out.Language = &lang
	}
	if len(stream.Tags) > 0 {
		out.Tags = append([]string(nil), stream.Tags...)
	}
	snapshotAt := stream.LastSeenAt
	if snapshotAt.IsZero() && !stream.StartedAt.IsZero() {
		snapshotAt = stream.StartedAt
	}
	if !snapshotAt.IsZero() {
		snap := snapshotAt.UTC().Format(time.RFC3339)
		out.SnapshotTime = &snap
		age := int(time.Since(snapshotAt).Seconds())
		if age < 0 {
			age = 0
		}
		out.FreshnessSeconds = &age
	}
	return out
}

func hasLiveMetadataSnapshot(in extensionCoverageInputs) bool {
	if hasTop500Metadata(in) {
		return true
	}
	if in.stream == nil {
		return false
	}
	stream := in.stream
	if strings.TrimSpace(stream.Title) != "" ||
		stream.CurrentViewers > 0 ||
		strings.TrimSpace(stream.Category) != "" ||
		!stream.StartedAt.IsZero() {
		return true
	}
	return in.hasViewerRollups
}

func metadataSnapshotAgeSeconds(stream *StreamRecord) (int, bool) {
	if stream == nil {
		return 0, false
	}
	snapshotAt := stream.LastSeenAt
	if snapshotAt.IsZero() && !stream.StartedAt.IsZero() {
		snapshotAt = stream.StartedAt
	}
	if snapshotAt.IsZero() {
		return 0, false
	}
	age := int(time.Since(snapshotAt).Seconds())
	if age < 0 {
		age = 0
	}
	return age, true
}

func metadataSnapshotFresh(in extensionCoverageInputs) bool {
	if hasTop500Metadata(in) {
		if !in.top500Current.StaleAfter.IsZero() {
			return time.Now().UTC().Before(in.top500Current.StaleAfter)
		}
		if in.top500Current.SampledAt.IsZero() {
			return false
		}
		return time.Since(in.top500Current.SampledAt) <= extensionMetadataFreshnessMax
	}
	age, ok := metadataSnapshotAgeSeconds(in.stream)
	return ok && age <= int(extensionMetadataFreshnessMax.Seconds())
}

func coverageMetadataLive(in extensionCoverageInputs) bool {
	if hasTop500Metadata(in) {
		return in.top500Current.IsLive
	}
	return in.isLive
}

func hasFreshLiveMetadataSnapshot(in extensionCoverageInputs) bool {
	if in.tracking {
		return false
	}
	if !hasLiveMetadataSnapshot(in) || !coverageMetadataLive(in) || !metadataSnapshotFresh(in) {
		return false
	}
	if hasTop500Metadata(in) {
		return true
	}
	if in.stream == nil {
		return false
	}
	source := metadataSourceForStream(in.stream, in.tracking)
	return source != "none" || in.stream.CurrentViewers > 0 || strings.TrimSpace(in.stream.Title) != ""
}

func hasTop500Metadata(in extensionCoverageInputs) bool {
	return !in.tracking && in.top500Current != nil && metadataSourceForTop500Current(in.top500Current) != "none"
}

func metadataSourceForTop500Current(current *Top500Current) string {
	if current == nil {
		return "none"
	}
	source := strings.TrimSpace(current.CoverageSource)
	if source == "" {
		return Top500CoverageSourceMetadata
	}
	return source
}

func ptrStringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func metadataSourceForStream(stream *StreamRecord, tracking bool) string {
	if tracking {
		return "collector"
	}
	switch normalizeViewerSource(stream.ViewerSource) {
	case ViewerSourceLive:
		return "helix"
	case ViewerSourceTT, ViewerSourceMerged:
		return "tier0"
	default:
		if !stream.LastSeenAt.IsZero() {
			return "cache"
		}
		return "none"
	}
}

func rollupAvailabilityFlags(rollups []MinuteRollup) (hasChat bool, hasViewer bool) {
	for _, r := range rollups {
		if r.Missing {
			continue
		}
		if r.ChatCount > 0 || r.TotalEmoteCount > 0 || r.SevenTVEmoteCount > 0 {
			hasChat = true
		}
		if r.ViewerAvg > 0 || r.ViewerSamples > 0 {
			hasViewer = true
		}
		if hasChat && hasViewer {
			return
		}
	}
	return
}

func intPtr(v int) *int {
	return &v
}

func isRosterMetadataCoverageTier(tier string) bool {
	return tier == CoverageTierRosterMetadataOnly || tier == CoverageTierTop500MetadataOnly
}

// coverageTierForAPI emits legacy top500_metadata_only while internal logic uses roster_metadata_only.
func coverageTierForAPI(tier string) string {
	if tier == CoverageTierRosterMetadataOnly {
		return CoverageTierTop500MetadataOnly
	}
	return tier
}

func InvalidateExtensionCoverageCache(ctx context.Context, rdb *redis.Client, login string) {
	if rdb == nil {
		return
	}
	login = normalizeLogin(login)
	if login == "" {
		return
	}
	_ = rdb.Del(ctx, extCoverageCachePrefix+login).Err()
}
