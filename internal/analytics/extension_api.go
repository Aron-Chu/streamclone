package analytics

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/redis/go-redis/v9"

	"streamclone/internal/analytics/heatmap"
	"streamclone/internal/emoteimage"
)

const (
	extPulseCachePrefix        = "ext:pulse:v2:"
	extPulseCacheTTL           = 12 * time.Second
	extPulseMaxRollups         = 60
	extPulseMaxFullRollups     = 480
	extPulseMinCompleted       = 5
	extPulseMaxPeaks           = 10
	extPulsePeakScoreCutoffPct = 0.25
)

type ExtensionHealthResponse struct {
	OK           bool                        `json:"ok"`
	Version      string                      `json:"version"`
	BuildSHA     string                      `json:"buildSha,omitempty"`
	Time         int64                       `json:"time"`
	HostedMode   bool                        `json:"hostedMode"`
	HelixEnabled bool                        `json:"helixEnabled"`
	Degraded     ExtensionHealthDegraded     `json:"degraded"`
	Routes       ExtensionHealthRoutes       `json:"routes"`
	Capabilities ExtensionHealthCapabilities `json:"capabilities"`
}

type ExtensionHealthDegraded struct {
	VODLookup    bool `json:"vodLookup"`
	Backfill     bool `json:"backfill"`
	LiveTracking bool `json:"liveTracking"`
	BFFCache     bool `json:"bffCache"`
	ActionsPaused bool `json:"actionsPaused,omitempty"`
}

type ExtensionHealthRoutes struct {
	PulseChannel bool `json:"pulseChannel"`
	VodHint      bool `json:"vodHint"`
	Backfill     bool `json:"backfill"`
}

type ExtensionHealthCapabilities struct {
	LiveTracking          bool `json:"liveTracking"`
	VODLookup             bool `json:"vodLookup"`
	MissedMomentsBackfill bool `json:"missedMomentsBackfill"`
	ProtectedTracking     bool `json:"protectedTracking"`
	Mutations             bool `json:"mutations,omitempty"`
	Backfill              bool `json:"backfill,omitempty"`
}

type ExtensionEmote struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	ImageURL string `json:"imageUrl"`
	Count    int    `json:"count"`
	Provider string `json:"provider"`
}

type ExtensionRollup struct {
	OffsetSeconds     int              `json:"offsetSeconds"`
	ChatCount         int              `json:"chatCount"`
	SevenTvEmoteCount int              `json:"sevenTvEmoteCount"`
	TotalEmoteCount   int              `json:"totalEmoteCount"`
	ViewerCount       int              `json:"viewerCount,omitempty"`
	KeywordCount      int              `json:"keywordCount,omitempty"`
	TopEmotes         []ExtensionEmote `json:"topEmotes,omitempty"`
}

type ExtensionLanes struct {
	Composite []int `json:"composite"`
	Chat      []int `json:"chat"`
	SevenTV   []int `json:"seventv"`
	Viewers   []int `json:"viewers,omitempty"`
	Keywords  []int `json:"keywords,omitempty"`
}

type ExtensionPeak struct {
	OffsetSeconds  int              `json:"offsetSeconds"`
	Score          int              `json:"score"`
	Reasons        []string         `json:"reasons"`
	ReasonLabel    string           `json:"reasonLabel"`
	DominantSignal string           `json:"dominantSignal"`
	ChatCount      int              `json:"chatCount"`
	EmoteCount     int              `json:"emoteCount"`
	TopEmotes      []ExtensionEmote `json:"topEmotes,omitempty"`
}

type ExtensionPulseResponse struct {
	Login                      string            `json:"login"`
	IsLive                     bool              `json:"isLive"`
	Tracking                   bool              `json:"tracking"`
	StreamID                   string            `json:"streamId,omitempty"`
	VodID                      *string           `json:"vodId"`
	StartedAt                  *time.Time        `json:"startedAt,omitempty"`
	CurrentOffsetSeconds       int               `json:"currentOffsetSeconds"`
	CoverageStartOffsetSeconds int               `json:"coverageStartOffsetSeconds"`
	Coverage                   ExtensionCoverage `json:"coverage"`
	TopEmotes                  []ExtensionEmote  `json:"topEmotes,omitempty"`
	Rollups                    []ExtensionRollup `json:"rollups"`
	FullRollups                []ExtensionRollup `json:"fullRollups,omitempty"`
	Lanes                      ExtensionLanes    `json:"lanes"`
	Peaks                      []ExtensionPeak   `json:"peaks"`
	Recap                      any               `json:"recap"`
	EmoteSync                  EmoteSyncSnapshot `json:"emoteSync"`
	HelixEnabled               bool              `json:"helixEnabled,omitempty"`
}

func (h *Handler) WithRedis(rdb *redis.Client) *Handler {
	h.rdb = rdb
	return h
}

func (h *Handler) ExtensionRoutes(r chi.Router) {
	r.Route("/v1/extension", func(r chi.Router) {
		r.Get("/health", h.extensionHealth)
		if h.pulseHosted.Hosted {
			r.With(h.pulseBetaKeyMiddleware, h.pulsePrincipalMiddleware).Post("/auth/device", h.extensionAuthDevice)
		}
		r.Group(func(r chi.Router) {
			if h.pulseHosted.Hosted {
				r.Use(h.pulseHostedAuthMiddleware)
			}
			r.Get("/me", h.extensionMe)
			r.Get("/pulse/channels/{login}", h.extensionPulseChannel)
			r.Post("/pulse/channels/{login}/vod-hint", h.extensionPulseVodHint)
			r.Post("/pulse/channels/{login}/vod-retry", h.extensionPulseVodRetry)
			r.Post("/pulse/channels/{login}/backfill", h.extensionPulseBackfill)
			r.Get("/pulse/backfill/{jobId}", h.extensionPulseBackfillStatus)
		})
	})
}

func extensionVersion() string {
	if v := strings.TrimSpace(os.Getenv("STREAMCLONE_VERSION")); v != "" {
		return v
	}
	return "dev"
}

func extensionBuildSHA() string {
	for _, key := range []string{"STREAMCLONE_BUILD_SHA", "BUILD_SHA", "GIT_SHA"} {
		if v := strings.TrimSpace(os.Getenv(key)); v != "" {
			return v
		}
	}
	return ""
}

func (h *Handler) extensionHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, h.extensionHealthPayload())
}

func (h *Handler) extensionHealthPayload() ExtensionHealthResponse {
	runtime := h.pulseRuntimeConfig()
	helixConfigured := h.helix != nil && h.helix.Enabled()
	vodLookup := helixConfigured && runtime.HelixVodEnabled
	liveTracking := h.collector != nil && runtime.HelixLiveEnabled
	backfill := runtime.BackfillEnabled && runtime.GQLCommentsEnabled && h.pulseBackfill != nil
	mutationsEnabled := !runtime.ReadOnlyMode
	return ExtensionHealthResponse{
		OK:           true,
		Version:      extensionVersion(),
		BuildSHA:     extensionBuildSHA(),
		Time:         time.Now().UnixMilli(),
		HostedMode:   h.pulseHosted.Hosted,
		HelixEnabled: helixConfigured,
		Degraded: ExtensionHealthDegraded{
			VODLookup:     !vodLookup,
			Backfill:      !backfill,
			LiveTracking:  !liveTracking,
			BFFCache:      !runtime.BFFCacheEnabled,
			ActionsPaused: runtime.ReadOnlyMode,
		},
		Routes: ExtensionHealthRoutes{
			PulseChannel: true,
			VodHint:      true,
			Backfill:     true,
		},
		Capabilities: ExtensionHealthCapabilities{
			LiveTracking:          liveTracking,
			VODLookup:             vodLookup,
			MissedMomentsBackfill: backfill,
			ProtectedTracking:     h.pulseHosted.Hosted || runtime.ProtectedGoLiveEnabled,
			Mutations:             mutationsEnabled,
			Backfill:              backfill,
		},
	}
}

func (h *Handler) extensionPulseChannel(w http.ResponseWriter, r *http.Request) {
	login, ok := validLogin(chi.URLParam(r, "login"))
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_channel"})
		return
	}

	ctx := r.Context()
	if h.collector != nil {
		if principal, ok := pulsePrincipalFromContext(ctx); ok {
			h.collector.TouchForPrincipal(login, principal.ID)
		}
	}
	window := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("window")))
	cacheKey := extPulseCachePrefix + login
	cacheEnabled := h.pulseRuntimeConfig().BFFCacheEnabled
	if window != "full" && cacheEnabled && h.rdb != nil {
		if cached, err := h.rdb.Get(ctx, cacheKey).Bytes(); err == nil && len(cached) > 0 {
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("X-Cache", "HIT")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(cached)
			return
		}
	}

	payload, err := h.buildExtensionPulse(ctx, login, window == "full")
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeJSON(w, http.StatusOK, emptyExtensionPulse(login, h.isLoginTracked(login)))
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	body, _ := json.Marshal(payload)
	if window != "full" && cacheEnabled && h.rdb != nil {
		_ = h.rdb.Set(ctx, cacheKey, body, extPulseCacheTTL).Err()
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Cache", "MISS")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}

type extensionVodHintRequest struct {
	StreamID string `json:"streamId"`
	VodID    string `json:"vodId"`
}

func (h *Handler) extensionPulseVodHint(w http.ResponseWriter, r *http.Request) {
	if !h.requirePulseWrite(w) {
		return
	}
	login, ok := validLogin(chi.URLParam(r, "login"))
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_channel"})
		return
	}
	var req extensionVodHintRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_body"})
		return
	}
	streamID := strings.TrimSpace(req.StreamID)
	vodID := strings.TrimSpace(req.VodID)
	if streamID == "" || vodID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing_ids"})
		return
	}
	ctx := r.Context()
	if h.store == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "store_unavailable"})
		return
	}
	stream, err := h.store.StreamByID(ctx, streamID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "stream_not_found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if normalizeLogin(stream.Login) != login {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "stream_login_mismatch"})
		return
	}
	vodID, err = validatePulseVodViaHelix(ctx, h.helix, *stream, login, vodID, h.pulseRuntimeConfig().HelixVodEnabled)
	if err != nil {
		status, code := pulseVodValidationHTTPError(err)
		writeJSON(w, status, map[string]string{"error": code})
		return
	}
	if err := h.store.SetStreamVodID(ctx, streamID, vodID, "extension_hint"); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	h.invalidatePulseCaches(ctx, login, streamID)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "vodId": vodID, "streamId": streamID})
}

func (h *Handler) isLoginTracked(login string) bool {
	if h.collector == nil {
		return false
	}
	return h.collector.IsTracking(login)
}

func emptyExtensionPulse(login string, tracking bool) ExtensionPulseResponse {
	return ExtensionPulseResponse{
		Login:                      login,
		Tracking:                   tracking,
		VodID:                      nil,
		CoverageStartOffsetSeconds: 0,
		Coverage: ExtensionCoverage{
			State:   CoverageStatePartialTracking,
			Message: "No stream data yet",
		},
		Rollups:   []ExtensionRollup{},
		EmoteSync: defaultExtensionEmoteSync(tracking),
		Lanes: ExtensionLanes{
			Composite: []int{},
			Chat:      []int{},
			SevenTV:   []int{},
		},
		Peaks: []ExtensionPeak{},
		Recap: nil,
	}
}

// reconcileExtensionLiveStream refreshes analytics when Twitch is live but the DB
// still has the previous ended session (common right after a new broadcast starts).
func (h *Handler) reconcileExtensionLiveStream(
	ctx context.Context,
	login string,
	stream *StreamRecord,
	tracking bool,
) (*StreamRecord, bool, error) {
	if stream == nil {
		return stream, false, nil
	}
	isLive := stream.EndedAt == nil
	if isLive || !tracking || h.helix == nil || !h.helix.Enabled() || !h.pulseRuntimeConfig().HelixLiveEnabled {
		return stream, isLive, nil
	}
	liveMap, err := h.helix.StreamsByLogin(ctx, []string{login})
	if err != nil {
		return stream, false, nil
	}
	liveStream, onTwitch := liveMap[login]
	if !onTwitch {
		return stream, false, nil
	}
	profiles, _ := h.helix.UsersByLogin(ctx, []string{login})
	now := time.Now().UTC()
	if err := h.store.UpsertLiveStream(ctx, liveStream, profiles[login], now); err != nil {
		return stream, true, nil
	}
	refreshed, err := h.store.LatestStreamByLogin(ctx, login)
	if err != nil {
		return stream, true, nil
	}
	return refreshed, refreshed.EndedAt == nil, nil
}

func (h *Handler) buildExtensionPulse(ctx context.Context, login string, fullWindow bool) (ExtensionPulseResponse, error) {
	stream, err := h.store.LatestStreamByLogin(ctx, login)
	if err != nil {
		return ExtensionPulseResponse{}, err
	}

	tracking := h.isLoginTracked(login)
	stream, isLive, err := h.reconcileExtensionLiveStream(ctx, login, stream, tracking)
	if err != nil {
		return ExtensionPulseResponse{}, err
	}
	state := "historical"
	if isLive {
		state = "live"
	}

	rollups, err := h.store.RollupsByStream(ctx, stream.StreamID)
	if err != nil {
		return ExtensionPulseResponse{}, err
	}
	for i := range rollups {
		rollups[i] = normalizeRollup(rollups[i], 200)
	}

	heatmapRollups, startedAt, err := h.consolidateForHeatmap(ctx, stream.StreamID)
	if err != nil {
		return ExtensionPulseResponse{}, err
	}

	cfg, cfgErr := heatmap.LoadScoringConfig()
	if cfgErr != nil {
		cfg = heatmap.DefaultScoringConfig()
	}
	// Decimated ComputeHeatmapDetail.Points is shorter than rollups; extension payloads
	// slice both series by index — use 1:1 aligned points so rollups are not dropped.
	alignedPoints := heatmap.AlignedDetailPoints(heatmapRollups, cfg)

	vodID := strings.TrimSpace(stream.VodID)
	if vodID == "" && h.helix != nil && h.helix.Enabled() {
		broadcasterID := NormalizeBroadcasterID(stream.BroadcasterID)
		if broadcasterID == "" && stream.Login != "" {
			broadcasterID = h.helix.ResolveBroadcasterID(ctx, stream.Login, "")
		}
		if broadcasterID != "" {
			if resolved, _ := h.helix.VideoIDByStreamID(ctx, broadcasterID, stream.StreamID); resolved != "" {
				vodID = resolved
				if h.store != nil {
					_ = h.store.SetStreamVodID(ctx, stream.StreamID, resolved, "helix")
				}
			}
		}
	}

	var vodPtr *string
	if vodID != "" {
		vodPtr = &vodID
	}

	streamStart := stream.StartedAt
	if streamStart.IsZero() && !startedAt.IsZero() {
		streamStart = startedAt
	}

	currentOffset := 0
	if isLive && !streamStart.IsZero() {
		currentOffset = int(math.Max(0, time.Since(streamStart).Seconds()))
	} else if !isLive && stream.EndedAt != nil && !streamStart.IsZero() {
		currentOffset = int(stream.EndedAt.Sub(streamStart).Seconds())
	}

	// Sparkline + lanes: last extPulseMaxRollups minutes only.
	windowRollups, windowPoints := trimExtensionRecentWindow(heatmapRollups, alignedPoints, isLive)
	extRollups, lanes := buildExtensionRollupsAndLanes(windowRollups, windowPoints, streamStart)
	fullMax := extPulseMaxFullRollups
	if fullWindow {
		fullMax = len(heatmapRollups) + 1
	}
	fullWindowRollups, fullWindowPoints := trimExtensionFullWindow(heatmapRollups, alignedPoints, fullMax, isLive)
	fullRollups, _ := buildExtensionRollupsAndLanes(fullWindowRollups, fullWindowPoints, streamStart)
	// Peaks ("Most Reacted So Far"): score across the full tracked stream history.
	peaks := buildExtensionPeaks(heatmapRollups, alignedPoints, isLive, state, streamStart)
	coverageStart := coverageStartOffsetSeconds(heatmapRollups, streamStart)

	vodIDStr := ""
	if vodPtr != nil {
		vodIDStr = *vodPtr
	}
	backfillRunning := false
	backfillFailed := false
	if h.pulseBackfill != nil && stream.StreamID != "" {
		if active := h.pulseBackfill.ActiveJobForStream(stream.StreamID); active != nil {
			backfillRunning = !isPulseBackfillTerminal(active.Status)
		}
		backfillFailed = h.pulseBackfill.BackfillFailedForStream(stream.StreamID)
	}
	coverage := computePulseCoverage(heatmapRollups, streamStart, currentOffset, isLive, vodIDStr, backfillRunning, backfillFailed)
	coverage = enrichExtensionCoverage(coverage, coverageStart, vodIDStr, isLive)
	var recap any
	if !isLive {
		built, err := h.buildPulseStreamRecap(ctx, stream.StreamID)
		if err != nil {
			return ExtensionPulseResponse{}, err
		}
		recap = built
	}

	var startedAtPtr *time.Time
	if !streamStart.IsZero() {
		t := streamStart.UTC()
		startedAtPtr = &t
	}

	_ = rollups // used indirectly via heatmap pipeline

	streamTopEmotes := convertTopEmotesToExtension(
		TopEmotesFromRollups(storeRollupsFromHeatmap(heatmapRollups), 8),
	)

	payload := ExtensionPulseResponse{
		Login:                      login,
		IsLive:                     isLive,
		Tracking:                   tracking,
		StreamID:                   stream.StreamID,
		VodID:                      vodPtr,
		StartedAt:                  startedAtPtr,
		CurrentOffsetSeconds:       currentOffset,
		CoverageStartOffsetSeconds: coverageStart,
		Coverage:                   coverage,
		TopEmotes:                  streamTopEmotes,
		Rollups:                    extRollups,
		FullRollups:                fullRollups,
		Lanes:                      lanes,
		Peaks:                      peaks,
		Recap:                      recap,
		EmoteSync:                  h.extensionEmoteSync(ctx, login, tracking),
		HelixEnabled:               h.helix != nil && h.helix.Enabled(),
	}
	h.rewriteExtensionPulseEmoteURLs(ctx, &payload)
	return payload, nil
}

func (h *Handler) extensionEmoteSync(ctx context.Context, login string, tracking bool) EmoteSyncSnapshot {
	if h.collector != nil {
		snap := h.collector.EmoteSyncSnapshot(ctx, login)
		if tracking && (snap.State == EmoteSyncStale || snap.State == EmoteSyncAggregateOnly) {
			h.collector.kickoffLiveEmoteEnsure(login)
		}
		return snap
	}
	return defaultExtensionEmoteSync(tracking)
}

func defaultExtensionEmoteSync(tracking bool) EmoteSyncSnapshot {
	state := EmoteSyncAggregateOnly
	if !tracking {
		state = EmoteSyncUnavailable
	}
	return emoteSyncSnapshotForState(state, false, "", nil)
}

// coverageStartOffsetSeconds is the earliest completed rollup offset relative to
// stream start. Zero means tracking began at (or before) stream start; >0 means
// late tracking and the UI must not imply pre-tracking coverage.
func coverageStartOffsetSeconds(rollups []heatmap.MinuteRollup, streamStart time.Time) int {
	if streamStart.IsZero() || len(rollups) == 0 {
		return 0
	}
	earliest := -1
	base := streamStart.UTC().Truncate(time.Minute)
	for _, r := range rollups {
		if r.Missing {
			continue
		}
		if r.ChatCount == 0 && r.TotalEmoteCount == 0 && r.SevenTVEmoteCount == 0 && r.ViewerSamples == 0 {
			continue
		}
		offset := int(r.MinuteTS.Sub(base).Seconds())
		if offset < 0 {
			offset = 0
		}
		if earliest < 0 || offset < earliest {
			earliest = offset
		}
	}
	if earliest < 0 {
		return 0
	}
	return earliest
}

func trimExtensionRecentWindow(
	rollups []heatmap.MinuteRollup,
	points []heatmap.ReplayHeatmapDetailPoint,
	isLive bool,
) ([]heatmap.MinuteRollup, []heatmap.ReplayHeatmapDetailPoint) {
	if len(rollups) == 0 {
		return rollups, points
	}
	start := 0
	if len(rollups) > extPulseMaxRollups {
		start = len(rollups) - extPulseMaxRollups
	}
	rollups = rollups[start:]
	points = points[start:]
	if isLive && len(rollups) > 1 {
		rollups = rollups[:len(rollups)-1]
		if len(points) > 0 {
			points = points[:len(points)-1]
		}
	}
	return rollups, points
}

func trimExtensionFullWindow(
	rollups []heatmap.MinuteRollup,
	points []heatmap.ReplayHeatmapDetailPoint,
	max int,
	isLive bool,
) ([]heatmap.MinuteRollup, []heatmap.ReplayHeatmapDetailPoint) {
	if len(rollups) == 0 {
		return rollups, points
	}
	if len(rollups) > max {
		start := len(rollups) - max
		rollups = rollups[start:]
		points = points[start:]
	}
	if isLive && len(rollups) > 1 {
		rollups = rollups[:len(rollups)-1]
		if len(points) > 0 {
			points = points[:len(points)-1]
		}
	}
	return rollups, points
}

func buildExtensionRollupsAndLanes(
	rollups []heatmap.MinuteRollup,
	points []heatmap.ReplayHeatmapDetailPoint,
	streamStart time.Time,
) ([]ExtensionRollup, ExtensionLanes) {
	out := make([]ExtensionRollup, 0, len(rollups))
	composite := make([]int, 0, len(points))
	chatLane := make([]int, 0, len(points))
	seventvLane := make([]int, 0, len(points))
	viewersLane := make([]int, 0, len(points))
	keywordsLane := make([]int, 0, len(points))
	hasViewers := false
	hasKeywords := false

	chatRaw := make([]float64, len(points))
	seventvRaw := make([]float64, len(points))
	viewerRaw := make([]float64, len(points))
	keywordRaw := make([]float64, len(points))

	for i, pt := range points {
		r := rollups[i]
		offset := 0
		if !streamStart.IsZero() {
			offset = int(r.MinuteTS.Sub(streamStart.UTC().Truncate(time.Minute)).Seconds())
			if offset < 0 {
				offset = pt.OffsetSeconds
			}
		} else {
			offset = pt.OffsetSeconds
		}

		viewerCount := r.ViewerLatest
		if viewerCount == 0 {
			viewerCount = r.ViewerMax
		}
		if viewerCount == 0 {
			viewerCount = r.ViewerAvg
		}

		topEmotes := convertExtensionEmotes(pt.TopEmotes)

		out = append(out, ExtensionRollup{
			OffsetSeconds:     offset,
			ChatCount:         r.ChatCount,
			SevenTvEmoteCount: r.SevenTVEmoteCount,
			TotalEmoteCount:   rollupEmoteCount(r),
			ViewerCount:       viewerCount,
			KeywordCount:      peakKeywordCount(r.Emotes),
			TopEmotes:         topEmotes,
		})

		composite = append(composite, clampScore(pt.Score))
		chatRaw[i] = componentNorm(pt.Components, "chatRate")
		seventvRaw[i] = seventvLaneValue(r, pt.Components)
		if r.ViewerSamples > 0 || viewerCount > 0 {
			hasViewers = true
			viewerRaw[i] = componentNorm(pt.Components, "viewerMomentum")
		}
		kw := componentNorm(pt.Components, "topEmoteDominance")
		if kw > 0 {
			hasKeywords = true
			keywordRaw[i] = kw
		}
	}

	chatLane = normalizeLaneSeries(chatRaw)
	seventvLane = normalizeLaneSeries(seventvRaw)
	if hasViewers {
		viewersLane = normalizeLaneSeries(viewerRaw)
	}
	if hasKeywords {
		keywordsLane = normalizeLaneSeries(keywordRaw)
	}

	lanes := ExtensionLanes{
		Composite: composite,
		Chat:      chatLane,
		SevenTV:   seventvLane,
	}
	if hasViewers {
		lanes.Viewers = viewersLane
	}
	if hasKeywords {
		lanes.Keywords = keywordsLane
	}
	return out, lanes
}

func componentNorm(components map[string]heatmap.SignalComponent, key string) float64 {
	c, ok := components[key]
	if !ok {
		return 0
	}
	return math.Max(0, c.WeightedScore)
}

func seventvLaneValue(r heatmap.MinuteRollup, components map[string]heatmap.SignalComponent) float64 {
	if r.SevenTVEmoteCount > 0 {
		return float64(r.SevenTVEmoteCount)
	}
	return componentNorm(components, "providerSpike")
}

func peakKeywordCount(emotes map[string]int) int {
	if len(emotes) == 0 {
		return 0
	}
	max := 0
	for _, count := range emotes {
		if count > max {
			max = count
		}
	}
	return max
}

func normalizeLaneSeries(values []float64) []int {
	if len(values) == 0 {
		return []int{}
	}
	max := 0.0
	for _, v := range values {
		if v > max {
			max = v
		}
	}
	out := make([]int, len(values))
	if max <= 0 {
		return out
	}
	for i, v := range values {
		out[i] = clampScore(int(math.Round(v / max * 100)))
	}
	return out
}

func clampScore(v int) int {
	if v < 0 {
		return 0
	}
	if v > 100 {
		return 100
	}
	return v
}

var extensionReasonLabels = map[string]string{
	heatmap.ReasonChatSpike:        "Chat spike",
	"emote_spike":                  "Emote spike",
	heatmap.ReasonSevenTVSpike:     "7TV emote spike",
	heatmap.ReasonTwitchEmoteSpike: "Twitch emote spike",
	heatmap.ReasonFFZSpike:         "FFZ emote spike",
	heatmap.ReasonViewerSpike:      "Viewer spike",
	heatmap.ReasonManual:           "Moment",
}

func extensionReasonLabel(reason string) string {
	if label, ok := extensionReasonLabels[reason]; ok {
		return label
	}
	if reason == "" {
		return extensionReasonLabels[heatmap.ReasonManual]
	}
	return strings.ReplaceAll(reason, "_", " ")
}

func storeRollupsFromHeatmap(in []heatmap.MinuteRollup) []MinuteRollup {
	out := make([]MinuteRollup, len(in))
	for i, r := range in {
		out[i] = MinuteRollup{
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
	return out
}

func convertTopEmotesToExtension(in []TopEmote) []ExtensionEmote {
	if len(in) == 0 {
		return nil
	}
	out := make([]ExtensionEmote, 0, len(in))
	for _, emote := range in {
		if emote.Name == "" {
			continue
		}
		out = append(out, ExtensionEmote{
			ID:       emote.ID,
			Name:     emote.Name,
			ImageURL: emote.ImageURL,
			Count:    emote.Count,
			Provider: emote.Provider,
		})
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func convertExtensionEmotes(in []heatmap.HeatmapEmote) []ExtensionEmote {
	if len(in) == 0 {
		return nil
	}
	out := make([]ExtensionEmote, 0, len(in))
	for _, emote := range in {
		if emote.Name == "" {
			continue
		}
		out = append(out, ExtensionEmote{
			ID:       emote.ID,
			Name:     emote.Name,
			ImageURL: emote.ImageURL,
			Count:    emote.Count,
			Provider: emote.Provider,
		})
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func (h *Handler) rewriteExtensionPulseEmoteURLs(ctx context.Context, payload *ExtensionPulseResponse) {
	if payload == nil || h == nil || h.store == nil {
		return
	}
	localIDs := collectExtensionSevenTVLocalIDs(payload)
	if len(localIDs) == 0 {
		return
	}
	lookup, err := h.store.LookupSevenTVProviderEmoteIDs(ctx, localIDs)
	if err != nil || len(lookup) == 0 {
		return
	}
	payload.TopEmotes = rewriteExtensionEmoteURLs(payload.TopEmotes, lookup)
	for i := range payload.Rollups {
		payload.Rollups[i].TopEmotes = rewriteExtensionEmoteURLs(payload.Rollups[i].TopEmotes, lookup)
	}
	for i := range payload.FullRollups {
		payload.FullRollups[i].TopEmotes = rewriteExtensionEmoteURLs(payload.FullRollups[i].TopEmotes, lookup)
	}
	for i := range payload.Peaks {
		payload.Peaks[i].TopEmotes = rewriteExtensionEmoteURLs(payload.Peaks[i].TopEmotes, lookup)
	}
}

func collectExtensionSevenTVLocalIDs(payload *ExtensionPulseResponse) []string {
	if payload == nil {
		return nil
	}
	seen := map[string]struct{}{}
	var out []string
	add := func(emotes []ExtensionEmote) {
		for _, emote := range emotes {
			provider := strings.ToLower(strings.TrimSpace(emote.Provider))
			if provider != "seventv" && provider != "7tv" {
				continue
			}
			id := strings.TrimSpace(emote.ID)
			if !emoteimage.IsLocalEmoteID(id) {
				continue
			}
			if _, ok := seen[id]; ok {
				continue
			}
			seen[id] = struct{}{}
			out = append(out, id)
		}
	}
	add(payload.TopEmotes)
	for i := range payload.Rollups {
		add(payload.Rollups[i].TopEmotes)
	}
	for i := range payload.FullRollups {
		add(payload.FullRollups[i].TopEmotes)
	}
	for i := range payload.Peaks {
		add(payload.Peaks[i].TopEmotes)
	}
	return out
}

func rewriteExtensionEmoteURLs(emotes []ExtensionEmote, lookup map[string]string) []ExtensionEmote {
	if len(emotes) == 0 || len(lookup) == 0 {
		return emotes
	}
	out := make([]ExtensionEmote, len(emotes))
	copy(out, emotes)
	for i := range out {
		providerID, ok := lookup[strings.TrimSpace(out[i].ID)]
		if !ok {
			continue
		}
		out[i].ImageURL = emoteimage.ExtensionBrowserURL(out[i].Provider, out[i].ID, providerID)
	}
	return out
}

// rollupAtOffset returns the minute rollup aligned with a heatmap point offset.
func rollupAtOffset(
	rollups []heatmap.MinuteRollup,
	points []heatmap.ReplayHeatmapDetailPoint,
	offsetSeconds int,
) (heatmap.MinuteRollup, bool) {
	for i, pt := range points {
		if pt.OffsetSeconds == offsetSeconds && i < len(rollups) {
			return rollups[i], true
		}
	}
	return heatmap.MinuteRollup{}, false
}

func rollupEmoteCount(r heatmap.MinuteRollup) int {
	if r.TotalEmoteCount > 0 {
		return r.TotalEmoteCount
	}
	total := 0
	for _, count := range r.Emotes {
		if count > 0 {
			total += count
		}
	}
	return total
}

func buildExtensionPeaks(
	rollups []heatmap.MinuteRollup,
	points []heatmap.ReplayHeatmapDetailPoint,
	isLive bool,
	_ string,
	_ time.Time,
) []ExtensionPeak {
	if len(points) == 0 {
		return []ExtensionPeak{}
	}

	completedRollups := rollups
	completedPoints := points
	if isLive && len(rollups) > 0 {
		completedRollups = rollups[:len(rollups)-1]
		if len(points) > 0 {
			completedPoints = points[:len(points)-1]
		}
	}

	dataCount := 0
	for _, r := range completedRollups {
		if !r.Missing && (r.ChatCount > 0 || r.TotalEmoteCount > 0 || r.ViewerSamples > 0) {
			dataCount++
		}
	}
	if dataCount < extPulseMinCompleted {
		return []ExtensionPeak{}
	}

	type scored struct {
		point heatmap.ReplayHeatmapDetailPoint
		score int
		index int
	}
	scoredPoints := make([]scored, 0, len(completedPoints))
	for i, pt := range completedPoints {
		if pt.Score <= 0 {
			continue
		}
		scoredPoints = append(scoredPoints, scored{point: pt, score: pt.Score, index: i})
	}
	if len(scoredPoints) == 0 {
		return []ExtensionPeak{}
	}

	maxScore := 0
	for _, sp := range scoredPoints {
		if sp.score > maxScore {
			maxScore = sp.score
		}
	}
	cutoff := int(math.Max(1, math.Round(float64(maxScore)*extPulsePeakScoreCutoffPct)))

	filtered := make([]scored, 0, len(scoredPoints))
	for _, sp := range scoredPoints {
		if sp.score >= cutoff {
			filtered = append(filtered, sp)
		}
	}
	sort.Slice(filtered, func(i, j int) bool {
		if filtered[i].score != filtered[j].score {
			return filtered[i].score > filtered[j].score
		}
		return filtered[i].point.OffsetSeconds < filtered[j].point.OffsetSeconds
	})
	if len(filtered) > extPulseMaxPeaks {
		filtered = filtered[:extPulseMaxPeaks]
	}

	out := make([]ExtensionPeak, 0, len(filtered))
	for _, sp := range filtered {
		pt := sp.point
		reason := pt.Reason
		if reason == "" {
			reason = heatmap.ReasonManual
		}
		chatCount := 0
		emoteCount := 0
		if sp.index >= 0 && sp.index < len(completedRollups) {
			rollup := completedRollups[sp.index]
			chatCount = rollup.ChatCount
			emoteCount = rollupEmoteCount(rollup)
		} else if rollup, ok := rollupAtOffset(completedRollups, completedPoints, pt.OffsetSeconds); ok {
			chatCount = rollup.ChatCount
			emoteCount = rollupEmoteCount(rollup)
		}
		out = append(out, ExtensionPeak{
			OffsetSeconds:  pt.OffsetSeconds,
			Score:          pt.Score,
			Reasons:        []string{reason},
			ReasonLabel:    extensionReasonLabel(reason),
			DominantSignal: dominantSignalFromReason(reason),
			ChatCount:      chatCount,
			EmoteCount:     emoteCount,
			TopEmotes:      convertExtensionEmotes(pt.TopEmotes),
		})
	}
	return out
}

func dominantSignalFromReason(reason string) string {
	switch reason {
	case heatmap.ReasonChatSpike:
		return "chat"
	case heatmap.ReasonSevenTVSpike:
		return "seventv"
	case heatmap.ReasonTwitchEmoteSpike:
		return "twitch"
	case heatmap.ReasonFFZSpike:
		return "ffz"
	case heatmap.ReasonViewerSpike:
		return "viewers"
	default:
		return "composite"
	}
}
