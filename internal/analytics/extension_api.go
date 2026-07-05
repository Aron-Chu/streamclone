package analytics

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/redis/go-redis/v9"

	"streamclone/internal/analytics/heatmap"
	pulserecap "streamclone/internal/analytics/recap"
	"streamclone/internal/emoteimage"
)

const (
	extPulseCachePrefix        = "ext:pulse:v2:"
	extPulseCacheTTL           = 12 * time.Second
	extPulseMaxRollups         = 60
	extPulseMaxFullRollups     = 480
	extPulseMinCompleted       = 5
	extPulseMaxPeaks           = 20
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
	VODLookup     bool `json:"vodLookup"`
	Backfill      bool `json:"backfill"`
	LiveTracking  bool `json:"liveTracking"`
	BFFCache      bool `json:"bffCache"`
	ActionsPaused bool `json:"actionsPaused,omitempty"`
}

type ExtensionHealthRoutes struct {
	PulseChannel  bool `json:"pulseChannel"`
	PulseCoverage bool `json:"pulseCoverage"`
	Streams       bool `json:"streams,omitempty"`
	VodHint       bool `json:"vodHint"`
	StreamHint    bool `json:"streamHint,omitempty"`
	Backfill      bool `json:"backfill"`
	Track         bool `json:"track,omitempty"`
	SyncRecent    bool `json:"syncRecent,omitempty"`
	Jobs          bool `json:"jobs,omitempty"`
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
	ID        string `json:"id"`
	Name      string `json:"name"`
	ImageURL  string `json:"imageUrl"`
	Count     int    `json:"count"`
	Provider  string `json:"provider"`
	ZeroWidth bool   `json:"zeroWidth,omitempty"`
	Animated  bool   `json:"animated,omitempty"`
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

type ExtensionPastStream struct {
	StreamID         string                  `json:"streamId"`
	VodID            string                  `json:"vodId,omitempty"`
	Title            string                  `json:"title,omitempty"`
	Category         string                  `json:"category,omitempty"`
	StartedAt        time.Time               `json:"startedAt"`
	EndedAt          *time.Time              `json:"endedAt,omitempty"`
	DurationSeconds  int                     `json:"durationSeconds,omitempty"`
	IsCurrentLive    bool                    `json:"isCurrentLive,omitempty"`
	HasRollups       bool                    `json:"hasRollups"`
	RollupPointCount int                     `json:"rollupPointCount"`
	CoverageState    string                  `json:"coverageState"`
	ThumbnailURL     string                  `json:"thumbnailUrl,omitempty"`
	Source           string                  `json:"source"`
	StoredArtifacts  *StoredArtifactsSummary `json:"storedArtifacts,omitempty"`
}

type ExtensionPastStreamsResponse struct {
	Login     string                `json:"login"`
	Items     []ExtensionPastStream `json:"items"`
	UpdatedAt int64                 `json:"updatedAt"`
}

type ExtensionGameSegment struct {
	ID              int    `json:"id,omitempty"`
	GameName        string `json:"gameName"`
	OffsetSeconds   int    `json:"offsetSeconds"`
	DurationSeconds int    `json:"durationSeconds"`
}

type ExtensionPulseResponse struct {
	Login                      string                  `json:"login"`
	IsLive                     bool                    `json:"isLive"`
	Tracking                   bool                    `json:"tracking"`
	StreamID                   string                  `json:"streamId,omitempty"`
	VodID                      *string                 `json:"vodId"`
	StartedAt                  *time.Time              `json:"startedAt,omitempty"`
	EndedAt                    *time.Time              `json:"endedAt,omitempty"`
	Title                      string                  `json:"title,omitempty"`
	Category                   string                  `json:"category,omitempty"`
	PeakViewers                int                     `json:"peakViewers,omitempty"`
	DurationSeconds            int                     `json:"durationSeconds,omitempty"`
	PeakEmotePerMin            int                     `json:"peakEmotePerMin,omitempty"`
	CurrentOffsetSeconds       int                     `json:"currentOffsetSeconds"`
	CoverageStartOffsetSeconds int                     `json:"coverageStartOffsetSeconds"`
	ViewerStartOffsetSeconds   int                     `json:"viewerStartOffsetSeconds,omitempty"`
	Coverage                   ExtensionCoverage       `json:"coverage"`
	TopEmotes                  []ExtensionEmote        `json:"topEmotes,omitempty"`
	Rollups                    []ExtensionRollup       `json:"rollups"`
	FullRollups                []ExtensionRollup       `json:"fullRollups,omitempty"`
	Lanes                      ExtensionLanes          `json:"lanes"`
	Peaks                      []ExtensionPeak         `json:"peaks"`
	Recap                      any                     `json:"recap"`
	EmoteSync                  EmoteSyncSnapshot       `json:"emoteSync"`
	HelixEnabled               bool                    `json:"helixEnabled,omitempty"`
	Games                      []ExtensionGameSegment  `json:"games,omitempty"`
	StoredArtifacts            *StoredArtifactsSummary `json:"storedArtifacts,omitempty"`
	Top500Eligible             bool                    `json:"top500Eligible"`
}

func (h *Handler) WithRedis(rdb *redis.Client) *Handler {
	h.rdb = rdb
	return h
}

func (h *Handler) ExtensionRoutes(r chi.Router) {
	r.Route("/v1/extension", func(r chi.Router) {
		r.Get("/health", h.extensionHealth)
		r.Group(func(r chi.Router) {
			if h.pulseHosted.Hosted {
				r.Use(h.pulseHostedAuthMiddleware)
			}
			if h.pulseHosted.Hosted {
				r.Post("/auth/device", h.extensionAuthDevice)
			}
			r.Get("/me", h.extensionMe)
			r.Get("/pulse/channels/{login}", h.extensionPulseChannel)
			r.Get("/pulse/vods/{vodId}", h.extensionPulseVod)
			r.Get("/pulse/channels/{login}/streams", h.extensionPulseChannelStreams)
			r.Get("/pulse/channels/{login}/coverage", h.extensionPulseChannelCoverage)
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
			PulseChannel:  true,
			PulseCoverage: true,
			Streams:       true,
			VodHint:       true,
			StreamHint:    true,
			Backfill:      true,
			Track:         liveTracking,
			Jobs:          true,
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
			payload := emptyExtensionPulse(login, false, h.extensionTop500Eligible(ctx, login))
			sanitizeExtensionPulseForNonTop500(&payload)
			writeJSON(w, http.StatusOK, payload)
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

func (h *Handler) extensionPulseChannelStreams(w http.ResponseWriter, r *http.Request) {
	login, ok := validLogin(chi.URLParam(r, "login"))
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_channel"})
		return
	}
	if h.store == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "store_unavailable"})
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 || limit > 20 {
		limit = 5
	}
	streams, err := h.store.StreamsByLogin(r.Context(), login, maxInt(limit+4, 8))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	items := h.buildExtensionPastStreams(r.Context(), streams, limit)
	writeJSON(w, http.StatusOK, ExtensionPastStreamsResponse{
		Login:     login,
		Items:     items,
		UpdatedAt: time.Now().UnixMilli(),
	})
}

func (h *Handler) buildExtensionPastStreams(ctx context.Context, streams []StreamRecord, limit int) []ExtensionPastStream {
	if limit <= 0 {
		limit = 5
	}
	items := make([]ExtensionPastStream, 0, minInt(limit, len(streams)))
	for _, stream := range streams {
		if len(items) >= limit {
			break
		}
		if stream.EndedAt == nil && len(items) > 0 {
			continue
		}
		item := h.buildExtensionPastStream(ctx, stream)
		items = append(items, item)
	}
	return items
}

func (h *Handler) buildExtensionPastStream(ctx context.Context, stream StreamRecord) ExtensionPastStream {
	rollupPointCount := 0
	if h.store != nil {
		if rollups, err := h.store.RollupsByStream(ctx, stream.StreamID); err == nil {
			for _, rollup := range rollups {
				if rollup.Missing {
					continue
				}
				if rollup.ChatCount > 0 || rollup.TotalEmoteCount > 0 || rollup.SevenTVEmoteCount > 0 || rollup.ViewerSamples > 0 {
					rollupPointCount++
				}
			}
		}
	}
	coverageState := "no_rollups"
	if rollupPointCount > 0 {
		coverageState = "synced"
	} else if stream.EndedAt == nil && strings.TrimSpace(stream.VodID) == "" {
		coverageState = "waiting_for_vod"
	} else if strings.TrimSpace(stream.VodID) == "" {
		coverageState = "vod_unavailable"
	}
	durationSeconds := 0
	if stream.EndedAt != nil && !stream.StartedAt.IsZero() && stream.EndedAt.After(stream.StartedAt) {
		durationSeconds = int(stream.EndedAt.Sub(stream.StartedAt).Seconds())
	}
	return ExtensionPastStream{
		StreamID:         stream.StreamID,
		VodID:            strings.TrimSpace(stream.VodID),
		Title:            stream.Title,
		Category:         stream.Category,
		StartedAt:        stream.StartedAt,
		EndedAt:          stream.EndedAt,
		DurationSeconds:  durationSeconds,
		IsCurrentLive:    stream.EndedAt == nil,
		HasRollups:       rollupPointCount > 0,
		RollupPointCount: rollupPointCount,
		CoverageState:    coverageState,
		ThumbnailURL:     stream.ThumbnailURL,
		Source:           "db_streams",
		StoredArtifacts:  ptrStoredArtifacts(h.storedArtifactsForStream(ctx, stream.StreamID)),
	}
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

func (h *Handler) extensionTop500Eligible(ctx context.Context, login string) bool {
	if h.store == nil {
		return !h.pulseHosted.Hosted
	}
	ok, err := h.store.IsTop500RosterMember(ctx, login)
	if err != nil {
		return !h.pulseHosted.Hosted
	}
	if !ok && !h.pulseHosted.Hosted {
		return true
	}
	return ok
}

func emptyExtensionPulse(login string, tracking bool, top500Eligible bool) ExtensionPulseResponse {
	return ExtensionPulseResponse{
		Login:                      login,
		Tracking:                   tracking,
		Top500Eligible:             top500Eligible,
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

type extensionLiveReconcileStore interface {
	LatestStreamByLogin(context.Context, string) (*StreamRecord, error)
	CloseStream(context.Context, string, time.Time) error
	UpsertLiveStream(context.Context, LiveStream, UserProfile, time.Time) error
}

// reconcileExtensionLiveStream refreshes analytics when Twitch is live but the DB
// still has the previous ended session (common right after a new broadcast starts).
// Helix reconcile runs for all channels (tracked or not) so stale open rows are closed
// before the extension BFF serves rollups as current live Pulse.
func (h *Handler) reconcileExtensionLiveStream(
	ctx context.Context,
	login string,
	stream *StreamRecord,
	tracking bool,
) (*StreamRecord, bool, error) {
	_ = tracking // collector admission is orthogonal; Helix truth applies to every login.
	var store extensionLiveReconcileStore
	if h.store != nil {
		store = h.store
	}
	return h.reconcileExtensionLiveStreamWithStore(ctx, login, stream, store)
}

func (h *Handler) reconcileExtensionLiveStreamWithStore(
	ctx context.Context,
	login string,
	stream *StreamRecord,
	store extensionLiveReconcileStore,
) (*StreamRecord, bool, error) {
	if stream == nil {
		return stream, false, nil
	}
	isLive := stream.EndedAt == nil
	if h.helix == nil || !h.helix.Enabled() || !h.pulseRuntimeConfig().HelixLiveEnabled {
		return stream, isLive, nil
	}
	liveMap, err := h.helix.StreamsByLogin(ctx, []string{login})
	if err != nil {
		return stream, isLive, nil
	}
	liveStream, onTwitch := liveMap[login]
	if !onTwitch {
		if isLive && store != nil && stream.StreamID != "" {
			endedAt := time.Now().UTC()
			closedID := stream.StreamID
			if err := store.CloseStream(ctx, stream.StreamID, endedAt); err != nil {
				return stream, true, nil
			}
			h.invalidatePulseCaches(ctx, login, closedID)
			if refreshed, err := store.LatestStreamByLogin(ctx, login); err == nil && refreshed != nil {
				return refreshed, refreshed.EndedAt == nil, nil
			}
			closed := *stream
			closed.EndedAt = &endedAt
			return &closed, false, nil
		}
		return stream, false, nil
	}
	if isLive && liveStream.ID == stream.StreamID {
		return stream, true, nil
	}
	priorStreamID := stream.StreamID
	profiles, _ := h.helix.UsersByLogin(ctx, []string{login})
	now := time.Now().UTC()
	if err := store.UpsertLiveStream(ctx, liveStream, profiles[login], now); err != nil {
		return stream, true, nil
	}
	if priorStreamID != "" {
		h.invalidatePulseCaches(ctx, login, priorStreamID)
	}
	if liveStream.ID != "" && liveStream.ID != priorStreamID {
		h.invalidatePulseCaches(ctx, login, liveStream.ID)
	}
	refreshed, err := store.LatestStreamByLogin(ctx, login)
	if err != nil || refreshed == nil {
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
	rollups = filterTimelineRollups(rollups)
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
	viewerStart := viewerStartOffsetSeconds(heatmapRollups, streamStart)

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
	coverage = enrichCoverageChatSources(coverage, rollups)
	coverage = enrichExtensionCoverage(coverage, coverageStart, vodIDStr, isLive)
	stored := h.storedArtifactsForStream(ctx, stream.StreamID)
	coverage = enrichCoverageWithStoredArtifacts(coverage, stored, vodIDStr, isLive)
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

	var games []ExtensionGameSegment
	if stream.StreamID != "" {
		if segments, err := h.resolveStreamGameSegments(ctx, stream.StreamID); err == nil {
			games = convertGameSegmentsForExtension(segments)
		}
	}

	durationSeconds := currentOffset
	if durationSeconds <= 0 && stream.EndedAt != nil && !streamStart.IsZero() {
		durationSeconds = int(stream.EndedAt.Sub(streamStart).Seconds())
	}
	if durationSeconds <= 0 && isLive && !streamStart.IsZero() {
		durationSeconds = int(time.Since(streamStart).Seconds())
	}
	category := strings.TrimSpace(stream.Category)
	if category == "" && !isLive && vodIDStr != "" && h.helix != nil && h.helix.Enabled() {
		if helixCategory, err := h.helix.VideoGameName(ctx, vodIDStr); err == nil {
			category = strings.TrimSpace(helixCategory)
		}
	}
	games = resolveExtensionGames(games, durationSeconds, category)
	games = extendLiveGameSegments(games, durationSeconds, isLive)

	var endedAtPtr *time.Time
	if stream.EndedAt != nil {
		t := stream.EndedAt.UTC()
		endedAtPtr = &t
	}

	top500Eligible := h.extensionTop500Eligible(ctx, login)

	payload := ExtensionPulseResponse{
		Login:                      login,
		IsLive:                     isLive,
		Tracking:                   tracking,
		Top500Eligible:             top500Eligible,
		StreamID:                   stream.StreamID,
		VodID:                      vodPtr,
		StartedAt:                  startedAtPtr,
		EndedAt:                    endedAtPtr,
		Title:                      strings.TrimSpace(stream.Title),
		Category:                   category,
		PeakViewers:                stream.PeakViewers,
		DurationSeconds:            durationSeconds,
		PeakEmotePerMin:            peakEmotePerMinFromHeatmapRollups(heatmapRollups),
		CurrentOffsetSeconds:       currentOffset,
		CoverageStartOffsetSeconds: coverageStart,
		ViewerStartOffsetSeconds:   viewerStart,
		Coverage:                   coverage,
		TopEmotes:                  streamTopEmotes,
		Rollups:                    extRollups,
		FullRollups:                fullRollups,
		Lanes:                      lanes,
		Peaks:                      peaks,
		Recap:                      recap,
		EmoteSync:                  h.extensionEmoteSync(ctx, login, tracking),
		HelixEnabled:               h.helix != nil && h.helix.Enabled(),
		Games:                      games,
		StoredArtifacts:            &stored,
	}
	h.rewriteExtensionPulseEmoteURLs(ctx, &payload)
	sanitizeExtensionPulseForCollectorTruth(&payload)
	sanitizeExtensionPulseForNonTop500(&payload)
	return payload, nil
}

// sanitizeExtensionPulseForNonTop500 strips Pulse product surfaces for channels outside
// the hosted top-500 roster so the extension can show a single unsupported state.
func sanitizeExtensionPulseForNonTop500(payload *ExtensionPulseResponse) {
	if payload == nil || payload.Top500Eligible {
		return
	}
	payload.Tracking = false
	payload.Rollups = []ExtensionRollup{}
	payload.FullRollups = []ExtensionRollup{}
	payload.Peaks = []ExtensionPeak{}
	payload.Lanes = ExtensionLanes{
		Composite: []int{},
		Chat:      []int{},
		SevenTV:   []int{},
	}
	payload.Recap = nil
	payload.Coverage = ExtensionCoverage{
		State:   CoverageStatePartialTracking,
		Message: "StreamPulse live chat is limited to the top-500 roster on hosted.",
		CopyKey: "top500_required",
	}
}

// sanitizeExtensionPulseForCollectorTruth strips live chat artifacts when the IRC
// collector is not active, even if historical rollups exist on a stale or prior stream.
func sanitizeExtensionPulseForCollectorTruth(payload *ExtensionPulseResponse) {
	if payload == nil || payload.Tracking {
		return
	}
	if !payload.IsLive {
		return
	}
	payload.Rollups = []ExtensionRollup{}
	payload.FullRollups = []ExtensionRollup{}
	payload.Peaks = []ExtensionPeak{}
	payload.Lanes = ExtensionLanes{
		Composite: []int{},
		Chat:      []int{},
		SevenTV:   []int{},
	}
	payload.TopEmotes = nil
	payload.PeakEmotePerMin = 0
	payload.CoverageStartOffsetSeconds = 0
	payload.Coverage = ExtensionCoverage{
		State:       CoverageStatePartialTracking,
		Message:     "Live chat is not being collected for this channel",
		CanBackfill: false,
		CopyKey:     "not_tracking",
	}
}

func peakEmotePerMinFromHeatmapRollups(rollups []heatmap.MinuteRollup) int {
	peak := 0
	for _, r := range rollups {
		if r.Missing {
			continue
		}
		total := r.TotalEmoteCount
		if total <= 0 {
			total = r.SevenTVEmoteCount
		}
		if total > peak {
			peak = total
		}
	}
	return peak
}

// resolveExtensionGames synthesizes a single full-stream segment when segments
// are missing but category is known (Helix /videos game_name or stream row).
func resolveExtensionGames(
	segments []ExtensionGameSegment,
	durationSeconds int,
	category string,
) []ExtensionGameSegment {
	if len(segments) > 0 || durationSeconds <= 0 {
		return segments
	}
	gameName := strings.TrimSpace(category)
	if gameName == "" {
		return segments
	}
	return []ExtensionGameSegment{{
		GameName:        gameName,
		OffsetSeconds:   0,
		DurationSeconds: durationSeconds,
	}}
}

// extendLiveGameSegments keeps the open last segment aligned with live stream duration.
func extendLiveGameSegments(segments []ExtensionGameSegment, durationSeconds int, isLive bool) []ExtensionGameSegment {
	if !isLive || durationSeconds <= 0 || len(segments) == 0 {
		return segments
	}
	out := make([]ExtensionGameSegment, len(segments))
	copy(out, segments)
	last := len(out) - 1
	seg := out[last]
	remaining := durationSeconds - seg.OffsetSeconds
	if remaining <= 0 {
		return out
	}
	if seg.DurationSeconds < remaining {
		seg.DurationSeconds = remaining
		out[last] = seg
	}
	return out
}

func convertGameSegmentsForExtension(segments []GameSegment) []ExtensionGameSegment {
	if len(segments) == 0 {
		return nil
	}
	out := make([]ExtensionGameSegment, 0, len(segments))
	for _, seg := range segments {
		name := strings.TrimSpace(seg.GameName)
		if name == "" {
			continue
		}
		out = append(out, ExtensionGameSegment{
			ID:              seg.ID,
			GameName:        name,
			OffsetSeconds:   seg.OffsetSeconds,
			DurationSeconds: seg.DurationSeconds,
		})
	}
	if len(out) == 0 {
		return nil
	}
	return out
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

// viewerStartOffsetSeconds is the earliest rollup minute with Helix viewer samples.
func viewerStartOffsetSeconds(rollups []heatmap.MinuteRollup, streamStart time.Time) int {
	if streamStart.IsZero() || len(rollups) == 0 {
		return 0
	}
	earliest := -1
	base := streamStart.UTC().Truncate(time.Minute)
	for _, r := range rollups {
		if r.Missing {
			continue
		}
		if r.ViewerSamples <= 0 && r.ViewerLatest <= 0 && r.ViewerAvg <= 0 && r.ViewerMax <= 0 {
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
	heatmap.ReasonSevenTVSpike:     "Emote spike",
	heatmap.ReasonTwitchEmoteSpike: "Emote spike",
	heatmap.ReasonFFZSpike:         "Emote spike",
	heatmap.ReasonViewerSpike:      "Viewer spike",
	heatmap.ReasonGameChange:       "Game change",
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
			ID:        emote.ID,
			Name:      emote.Name,
			ImageURL:  emote.ImageURL,
			Count:     emote.Count,
			Provider:  emote.Provider,
			ZeroWidth: emote.ZeroWidth,
			Animated:  emote.Animated,
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
	if payload == nil || h == nil {
		return
	}
	base := h.hostedEmoteCDNBase()
	var lookup map[string]string
	var metadata map[string]EmoteMetadata
	if h.store != nil {
		localIDs := collectExtensionProviderLocalIDs(payload)
		if len(localIDs) > 0 {
			lookup, _ = h.store.LookupProviderEmoteIDs(ctx, localIDs)
			metadata, _ = h.store.LookupEmoteMetadata(ctx, localIDs)
		}
	}
	if lookup == nil {
		lookup = map[string]string{}
	}
	if metadata == nil {
		metadata = map[string]EmoteMetadata{}
	}
	if base == "" && len(lookup) == 0 && len(metadata) == 0 {
		return
	}
	payload.TopEmotes = decorateExtensionEmotes(payload.TopEmotes, lookup, metadata, base)
	for i := range payload.Rollups {
		payload.Rollups[i].TopEmotes = decorateExtensionEmotes(payload.Rollups[i].TopEmotes, lookup, metadata, base)
	}
	for i := range payload.FullRollups {
		payload.FullRollups[i].TopEmotes = decorateExtensionEmotes(payload.FullRollups[i].TopEmotes, lookup, metadata, base)
	}
	for i := range payload.Peaks {
		payload.Peaks[i].TopEmotes = decorateExtensionEmotes(payload.Peaks[i].TopEmotes, lookup, metadata, base)
	}
	if recap, ok := payload.Recap.(pulserecap.StreamRecap); ok {
		payload.Recap = decorateStreamRecapEmotes(recap, lookup, metadata, base)
	}
}

func decorateStreamRecapEmotes(
	recap pulserecap.StreamRecap,
	lookup map[string]string,
	metadata map[string]EmoteMetadata,
	base string,
) pulserecap.StreamRecap {
	recap.TopEmotes = decorateRecapEmoteSlice(recap.TopEmotes, lookup, metadata, base)
	for i := range recap.TopMoments {
		recap.TopMoments[i].TopEmotes = decorateRecapEmoteSlice(recap.TopMoments[i].TopEmotes, lookup, metadata, base)
	}
	for i := range recap.ClipCandidates {
		recap.ClipCandidates[i].TopEmotes = decorateRecapEmoteSlice(recap.ClipCandidates[i].TopEmotes, lookup, metadata, base)
	}
	return recap
}

func decorateRecapEmoteSlice(
	emotes []pulserecap.Emote,
	lookup map[string]string,
	metadata map[string]EmoteMetadata,
	base string,
) []pulserecap.Emote {
	if len(emotes) == 0 {
		return emotes
	}
	out := make([]pulserecap.Emote, len(emotes))
	copy(out, emotes)
	for i := range out {
		id := strings.TrimSpace(out[i].ID)
		if emoteimage.IsLocalEmoteID(id) {
			if resolved, ok := lookup[id]; ok && strings.TrimSpace(resolved) != "" {
				out[i].ID = resolved
			}
		}
		if strings.TrimSpace(out[i].ImageURL) != "" {
			out[i].ImageURL = emoteimage.AbsolutizeHostedCDN(base, out[i].ImageURL)
		}
	}
	return out
}

func collectExtensionProviderLocalIDs(payload *ExtensionPulseResponse) []string {
	if payload == nil {
		return nil
	}
	seen := map[string]struct{}{}
	var out []string
	add := func(emotes []ExtensionEmote) {
		for _, emote := range emotes {
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
	if recap, ok := payload.Recap.(pulserecap.StreamRecap); ok {
		addRecapEmoteIDs(recap.TopEmotes, seen, &out)
		for _, moment := range recap.TopMoments {
			addRecapEmoteIDs(moment.TopEmotes, seen, &out)
		}
		for _, moment := range recap.ClipCandidates {
			addRecapEmoteIDs(moment.TopEmotes, seen, &out)
		}
	}
	return out
}

func addRecapEmoteIDs(emotes []pulserecap.Emote, seen map[string]struct{}, out *[]string) {
	for _, emote := range emotes {
		id := strings.TrimSpace(emote.ID)
		if !emoteimage.IsLocalEmoteID(id) {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		*out = append(*out, id)
	}
}

func rewriteExtensionEmoteURLs(emotes []ExtensionEmote, lookup map[string]string) []ExtensionEmote {
	return rewriteHostedExtensionEmoteURLs(emotes, lookup, "")
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
		if reason == heatmap.ReasonViewerSpike {
			continue
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
