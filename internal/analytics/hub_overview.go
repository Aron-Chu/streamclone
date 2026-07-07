package analytics

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
)

// The public hub endpoint powers the StreamPulse portal landing + analytics hub.
// It is an UNAUTHENTICATED, hosted-safe aggregate over the live tracking pool.
// Everything it returns is derived from public corpus counts and bounded
// per-minute activity — never raw rollups, emote maps, principals, or storage
// URIs (guarded by TestPublicHubResponseOmitsSensitiveKeys).
const (
	publicHubCacheKeyPrefix = "sp:public:hub"
	publicHubCacheTTL       = 30 * time.Second
	publicHubLongCacheTTL   = 5 * time.Minute

	// Per-login profile-image cache. Many channels enter the tracking pool via
	// metadata/top-500 paths that never persisted a profile_image_url, so the
	// hub backfills missing avatars from Helix and caches them here to avoid
	// re-fetching on every 30s rebuild.
	hubProfileCachePrefix = "sp:hub:profimg:"
	hubProfileCacheTTL    = 12 * time.Hour

	hubActivityWindowMinutes      = 30 // default minutes of aggregated activity returned
	hubActivityMaxPoints          = 240
	hubPoolCap                    = 96 // channels we join rollups for (bounded cost; aligns with IRC pool expand)
	hubLiveCap                    = 96 // live channel rows returned (portal table expand)
	hubMoversCap                  = 12 // portal Top movers rail; pairs with hubEmotesCap / economy panel height
	hubEmotesCap                  = 12
	hubActivityBucketTopEmotesCap = 10 // per-bucket inspector + chart tooltip (tooltip slices to 3)
	hubMomentsCap                 = 12
	hubMomentLookback             = 45 * time.Minute
	hubTrackerTopN                = MaxTop500MetadataTopN // top-N roster rows examined for the tracker summary
)

type publicHubOptions struct {
	ActivityWindowMinutes int
}

type PublicHubResponse struct {
	GeneratedAt            time.Time            `json:"generatedAt"`
	PoolSize               int                  `json:"poolSize"`
	Corpus                 HubCorpus            `json:"corpus"`
	Coverage               HubCoverage          `json:"coverage"`
	CorpusPipeline         HubCorpusPipeline    `json:"corpusPipeline"`
	Activity               HubActivity          `json:"activity"`
	EmoteIntel             HubEmoteIntel        `json:"emoteIntel"`
	TopEmotes              []HubEmote           `json:"topEmotes"`
	TopMovers              []HubMover           `json:"topMovers"`
	LiveChannels           []HubLiveChannel     `json:"liveChannels"`
	Moments                []HubMoment          `json:"moments"`
	LivePulseMoments       []HubLivePulseMoment `json:"livePulseMoments,omitempty"`
	LivePulseMomentsStatus string               `json:"livePulseMomentsStatus,omitempty"`
	LivePulseMomentsReason string               `json:"livePulseMomentsReason,omitempty"`
	FeaturedSession        HubFeaturedSession   `json:"featuredSession,omitempty"`
}

// HubCorpusPipeline is a hosted-safe, aggregate-only view of the Top-500 roster
// metadata tracker and the Silver/Gold VOD backfill corpus. It deliberately
// carries ONLY counts — never per-channel readiness rows, admission attempts,
// admission messages, logins, stream IDs, job errors, or scraper/GQL internals
// (guarded by TestPublicHubResponseOmitsSensitiveKeys).
type HubCorpusPipeline struct {
	GeneratedAt               time.Time         `json:"generatedAt"`
	State                     string            `json:"state"`
	TopN                      int               `json:"topN"`
	LiveAdmissionEnabled      bool              `json:"liveAdmissionEnabled"`
	LiveAdmissionTopN         int               `json:"liveAdmissionTopN"`
	MaxActiveIRCChannels      int               `json:"maxActiveIrcChannels"`
	CollectorActive           int               `json:"collectorActive"`
	CollectorMax              int               `json:"collectorMax"`
	RuntimeConfigFingerprint  string            `json:"runtimeConfigFingerprint,omitempty"`
	MetadataSampledAgoSeconds *int              `json:"metadataSampledAgoSeconds,omitempty"`
	Roster                    HubTrackerSummary `json:"roster"`
	Silver                    HubTierCounts     `json:"silver"`
	Gold                      HubTierCounts     `json:"gold"`
}

// HubTrackerSummary mirrors the aggregate state counts from the Top-500 roster
// readiness report (no per-channel rows).
type HubTrackerSummary struct {
	Live                     int `json:"live"`
	CollectorTracking        int `json:"collectorTracking"`
	ExpectedCollectorRows    int `json:"expectedCollectorRows"`
	LiveCollectorDeficitRows int `json:"liveCollectorDeficitRows"`
	MetadataOnly             int `json:"metadataOnly"`
	MetadataStale            int `json:"metadataStale"`
	AdmissionFeatureDisabled int `json:"admissionFeatureDisabled"`
	AdmissionDisabled        int `json:"admissionDisabled"`
	CapacityBlocked          int `json:"capacityBlocked"`
	Warming                  int `json:"warming"`
	Collecting               int `json:"collecting"`
	ViewerOnly               int `json:"viewerOnly"`
	ZeroChatAfterAge         int `json:"zeroChatAfterAge"`
}

// HubTierCounts holds aggregate backfill job counts for a single corpus tier.
type HubTierCounts struct {
	Queued              int  `json:"queued"`
	Running             int  `json:"running"`
	Done                int  `json:"done"`
	Skipped             int  `json:"skipped"`
	Failed              int  `json:"failed"`
	Total               int  `json:"total"`
	Eligible            int  `json:"eligible"`
	OldestQueuedSeconds *int `json:"oldestQueuedSeconds,omitempty"`
}

type HubCorpus struct {
	StreamsTracked        int64 `json:"streamsTracked"`
	MomentsDetected       int64 `json:"momentsDetected"`
	ChatMessagesProcessed int64 `json:"chatMessagesProcessed"`
	EmotesIndexed         int64 `json:"emotesIndexed"`
	VodsAnalyzed          int64 `json:"vodsAnalyzed"`
}

type HubCoverage struct {
	LiveChannels   int    `json:"liveChannels"`
	TrackingMax    int    `json:"trackingMax"`
	BackfillActive int    `json:"backfillActive"`
	BackfillMax    int    `json:"backfillMax"`
	SyncActive     int    `json:"syncActive"`
	EmotesIndexed  int64  `json:"emotesIndexed"`
	DatabaseOK     bool   `json:"databaseOk"`
	State          string `json:"state"`
}

type HubBucketEmote struct {
	Name     string `json:"name"`
	Provider string `json:"provider,omitempty"`
	ImageURL string `json:"imageUrl,omitempty"`
	Count    int    `json:"count"`
}

type HubActivityPoint struct {
	T               int64 `json:"t"`
	Chat            int   `json:"chat"`
	Emotes          int   `json:"emotes"`
	SevenTV         int   `json:"seventv"`
	Twitch          int   `json:"twitch,omitempty"`
	BTTV            int   `json:"bttv,omitempty"`
	FFZ             int   `json:"ffz,omitempty"`
	Viewers         int   `json:"viewers"`
	HasChatRollup   bool  `json:"hasChatRollup,omitempty"`
	HasViewerRollup bool  `json:"hasViewerRollup,omitempty"`
	// BucketComplete is false when the bucket period has not yet ended (open/in-progress).
	BucketComplete bool `json:"bucketComplete,omitempty"`
	// TopEmotes are sanitized name+provider+count+public imageUrl — never raw emote map keys.
	TopEmotes []HubBucketEmote `json:"topEmotes,omitempty"`
}

type HubActivity struct {
	Points            []HubActivityPoint `json:"points"`
	WindowMinutes     int                `json:"windowMinutes"`
	ChannelCount      int                `json:"channelCount"`
	PeakViewersAt     int64              `json:"peakViewersAt,omitempty"`
	LivePoolViewerSum int                `json:"livePoolViewerSum,omitempty"`
}

type HubEmoteIntel struct {
	EmotesPerMin    float64            `json:"emotesPerMin"`
	TopEmoteShare   float64            `json:"topEmoteSharePct"`
	UniqueEmotes    int                `json:"uniqueEmotes"`
	BiggestPeak     int                `json:"biggestPeakPerMin"`
	SevenTVSharePct float64            `json:"seventvSharePct"`
	ProviderShares  []HubProviderShare `json:"providerShares,omitempty"`
}

type HubProviderShare struct {
	Provider string  `json:"provider"`
	Count    int     `json:"count"`
	SharePct float64 `json:"sharePct"`
}

type HubEmote struct {
	Name      string  `json:"name"`
	Provider  string  `json:"provider,omitempty"`
	ImageURL  string  `json:"imageUrl,omitempty"`
	Count     int     `json:"count"`
	SharePct  float64 `json:"sharePct"`
	ZeroWidth bool    `json:"zeroWidth,omitempty"`
	Animated  bool    `json:"animated,omitempty"`
}

type HubMover struct {
	Login           string  `json:"login"`
	DisplayName     string  `json:"displayName,omitempty"`
	Category        string  `json:"category,omitempty"`
	ProfileImageURL string  `json:"profileImageUrl,omitempty"`
	Viewers         int     `json:"viewers"`
	EmotesPerMin    float64 `json:"emotesPerMin"`
	SevenTVPerMin   float64 `json:"seventvPerMin"`
	ChatPerMin      float64 `json:"chatPerMin"`
	TrendPct        float64 `json:"trendPct"`
}

type HubLiveChannel struct {
	Login             string   `json:"login"`
	DisplayName       string   `json:"displayName,omitempty"`
	Category          string   `json:"category,omitempty"`
	ProfileImageURL   string   `json:"profileImageUrl,omitempty"`
	Viewers           int      `json:"viewers"`
	ChatPerMin        float64  `json:"chatPerMin"`
	EmotesPerMin      float64  `json:"emotesPerMin"`
	SevenTVPerMin     float64  `json:"seventvPerMin"`
	CoverageState     string   `json:"coverageState"`
	TrendPct          float64  `json:"trendPct"`
	StreamingTogether bool     `json:"streamingTogether,omitempty"`
	HostLogin         string   `json:"hostLogin,omitempty"`
	TogetherWith      []string `json:"togetherWith,omitempty"`
}

type HubMoment struct {
	Kind        string  `json:"kind"`
	Login       string  `json:"login,omitempty"`
	DisplayName string  `json:"displayName,omitempty"`
	Label       string  `json:"label"`
	Detail      string  `json:"detail,omitempty"`
	Magnitude   float64 `json:"magnitude,omitempty"`
	At          int64   `json:"at"`
}

func (h *Handler) getPublicHub(w http.ResponseWriter, r *http.Request) {
	opts := publicHubOptionsFromRequest(r)
	payload, fromCache, err := h.loadPublicHub(r.Context(), false, opts)
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "hub_unavailable"})
		return
	}
	if fromCache {
		w.Header().Set("X-Cache", "HIT")
	} else {
		w.Header().Set("X-Cache", "MISS")
	}
	w.Header().Set("Cache-Control", "public, max-age=15, s-maxage=30, stale-while-revalidate=60")
	writeJSON(w, http.StatusOK, payload)
}

func (h *Handler) loadPublicHub(ctx context.Context, forceRefresh bool, opts publicHubOptions) (PublicHubResponse, bool, error) {
	opts = normalizePublicHubOptions(opts)
	runtimeFP := h.publicHubRuntimeFingerprint()
	cacheKey := publicHubCacheKey(opts, runtimeFP)
	if !forceRefresh && h.rdb != nil {
		if cached, err := h.rdb.Get(ctx, cacheKey).Bytes(); err == nil && len(cached) > 0 {
			var payload PublicHubResponse
			if json.Unmarshal(cached, &payload) == nil && publicHubRuntimeFingerprintMatches(payload.CorpusPipeline, runtimeFP) {
				return payload, true, nil
			}
		}
	}
	v, err, _ := h.hubGroup.Do(cacheKey, func() (any, error) {
		if !forceRefresh && h.rdb != nil {
			if cached, err := h.rdb.Get(ctx, cacheKey).Bytes(); err == nil && len(cached) > 0 {
				var payload PublicHubResponse
				if json.Unmarshal(cached, &payload) == nil && publicHubRuntimeFingerprintMatches(payload.CorpusPipeline, runtimeFP) {
					return payload, nil
				}
			}
		}
		payload := h.buildPublicHub(ctx, opts)
		if h.rdb != nil {
			body, _ := json.Marshal(payload)
			_ = h.rdb.Set(ctx, cacheKey, body, publicHubCacheTTLForOptions(opts)).Err()
		}
		return payload, nil
	})
	if err != nil {
		return PublicHubResponse{}, false, err
	}
	return v.(PublicHubResponse), false, nil
}

func publicHubOptionsFromRequest(r *http.Request) publicHubOptions {
	if r == nil {
		return normalizePublicHubOptions(publicHubOptions{})
	}
	return normalizePublicHubOptions(publicHubOptions{
		ActivityWindowMinutes: parseHubActivityWindow(r.URL.Query().Get("activityWindow")),
	})
}

func parseHubActivityWindow(value string) int {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "all", "1y", "year", "1year":
		return 365 * 24 * 60
	case "3m", "3mo", "3month", "3months":
		return 90 * 24 * 60
	case "1m", "1mo", "1month":
		return 30 * 24 * 60
	case "7d", "7day", "7days":
		return 7 * 24 * 60
	case "24h", "1d", "day":
		return 24 * 60
	case "30m", "30min", "recent", "":
		return hubActivityWindowMinutes
	default:
		if n, err := strconv.Atoi(value); err == nil {
			return n
		}
		return hubActivityWindowMinutes
	}
}

func normalizePublicHubOptions(opts publicHubOptions) publicHubOptions {
	if opts.ActivityWindowMinutes <= 0 {
		opts.ActivityWindowMinutes = hubActivityWindowMinutes
	}
	maxWindow := 365 * 24 * 60
	if opts.ActivityWindowMinutes > maxWindow {
		opts.ActivityWindowMinutes = maxWindow
	}
	return opts
}

func publicHubCacheKey(opts publicHubOptions, runtimeFingerprint string) string {
	base := publicHubCacheKeyPrefix + ":activity:" + strconv.Itoa(normalizePublicHubOptions(opts).ActivityWindowMinutes)
	fp := strings.TrimSpace(runtimeFingerprint)
	if fp == "" {
		return base
	}
	return base + ":cfg:" + fp
}

func corpusRuntimeHubCacheFingerprint(cfg CorpusRuntimeConfig, collectorMax int) string {
	admission := 0
	if cfg.LiveAdmissionEnabled {
		admission = 1
	}
	return fmt.Sprintf("n%d:a%d:t%d:irc%d:col%d", cfg.TargetTopN, admission, cfg.LiveAdmissionTopN, cfg.MaxActiveIRCChannels, collectorMax)
}

func (h *Handler) publicHubRuntimeFingerprint() string {
	cfg := h.corpusRuntimeConfig()
	collectorMax := 0
	if h.collector != nil {
		collectorMax = h.collector.TrackingSnapshot().Max
	}
	return corpusRuntimeHubCacheFingerprint(cfg, collectorMax)
}

func publicHubRuntimeFingerprintMatches(pipeline HubCorpusPipeline, expected string) bool {
	expected = strings.TrimSpace(expected)
	if expected == "" {
		return true
	}
	if strings.TrimSpace(pipeline.RuntimeConfigFingerprint) == expected {
		return true
	}
	// Legacy cache entries without fingerprint: rebuild when static fields drift.
	if pipeline.RuntimeConfigFingerprint != "" {
		return false
	}
	rebuilt := corpusRuntimeHubCacheFingerprint(CorpusRuntimeConfig{
		TargetTopN:           pipeline.TopN,
		LiveAdmissionEnabled: pipeline.LiveAdmissionEnabled,
		LiveAdmissionTopN:    pipeline.LiveAdmissionTopN,
		MaxActiveIRCChannels: pipeline.MaxActiveIRCChannels,
	}, pipeline.CollectorMax)
	return rebuilt == expected
}

func publicHubCacheTTLForOptions(opts publicHubOptions) time.Duration {
	opts = normalizePublicHubOptions(opts)
	if opts.ActivityWindowMinutes > hubActivityWindowMinutes {
		return publicHubLongCacheTTL
	}
	return publicHubCacheTTL
}

func hubActivityBucketMinutes(windowMinutes int) int {
	if windowMinutes <= hubActivityMaxPoints {
		return 1
	}
	bucket := int(math.Ceil(float64(windowMinutes) / float64(hubActivityMaxPoints)))
	if bucket < 1 {
		return 1
	}
	return bucket
}

// channelWindow holds the bounded per-channel metrics derived from the recent
// rollup window. It never leaves this file — only sanitized DTOs are exposed.
type channelWindow struct {
	record        *StreamRecord
	chatPerMin    float64
	seventvPerMin float64
	emotesPerMin  float64
	viewers       int
	coverageState string
	trendPct      float64
}

func rollupsSince(rollups []MinuteRollup, since time.Time) []MinuteRollup {
	if len(rollups) == 0 || since.IsZero() {
		return rollups
	}
	out := make([]MinuteRollup, 0, len(rollups))
	for _, ru := range rollups {
		if !ru.MinuteTS.Before(since) {
			out = append(out, ru)
		}
	}
	return out
}

func (h *Handler) hubRecentRollupWindow(ctx context.Context, rec *StreamRecord, since time.Time, limit int) (*StreamRecord, []MinuteRollup, []MinuteRollup) {
	if h == nil || h.store == nil || rec == nil {
		return rec, nil, nil
	}
	loaded, err := h.store.RecentRollupsByStreamID(ctx, rec.StreamID, limit)
	recent := rollupsSince(loaded, since)
	if err == nil && len(recent) > 0 {
		return rec, loaded, recent
	}
	fallbackRec, fallbackRollups, fallbackErr := h.store.LatestLiveStreamWithRecentRollupsByLogin(ctx, rec.Login, since, limit)
	if fallbackErr == nil && fallbackRec != nil && fallbackRec.StreamID != rec.StreamID && len(fallbackRollups) > 0 {
		return fallbackRec, fallbackRollups, fallbackRollups
	}
	return rec, loaded, recent
}

// buildHubPoolWindows joins the bounded live IRC pool with recent minute rollups
// for hub live-channel and pulse-moment builders.
func (h *Handler) buildHubPoolWindows(ctx context.Context) []channelWindow {
	if h == nil {
		return nil
	}
	var snapshot TrackingSnapshot
	if h.collector != nil {
		snapshot = h.collector.TrackingSnapshot()
	}
	logins := make([]string, 0, len(snapshot.TrackedChannels)+hubPoolCap)
	seenLogins := make(map[string]bool, len(snapshot.TrackedChannels)+hubPoolCap)
	for _, login := range snapshot.TrackedChannels {
		login = normalizeLogin(login)
		if login == "" || seenLogins[login] {
			continue
		}
		seenLogins[login] = true
		logins = append(logins, login)
	}
	if h.store != nil {
		if roster, err := h.store.ListTop500LiveForPriorityWatch(ctx, hubTrackerTopN, hubPoolCap); err == nil {
			for _, row := range roster {
				login := normalizeLogin(row.Login)
				if login == "" || seenLogins[login] {
					continue
				}
				seenLogins[login] = true
				logins = append(logins, login)
				if len(logins) >= hubPoolCap {
					break
				}
			}
		}
	}
	var streams map[string]*StreamRecord
	if h.store != nil && len(logins) > 0 {
		if loaded, err := h.store.LatestStreamsByLogins(ctx, logins); err == nil {
			streams = loaded
		}
	}
	live := make([]*StreamRecord, 0, len(streams))
	for _, rec := range streams {
		if rec == nil || rec.EndedAt != nil {
			continue
		}
		live = append(live, rec)
	}
	sort.SliceStable(live, func(i, j int) bool {
		if live[i].CurrentViewers != live[j].CurrentViewers {
			return live[i].CurrentViewers > live[j].CurrentViewers
		}
		return live[i].Login < live[j].Login
	})
	if len(live) > hubPoolCap {
		live = live[:hubPoolCap]
	}
	recentSince := time.Now().UTC().Add(-time.Duration(hubActivityWindowMinutes+5) * time.Minute)
	windows := make([]channelWindow, 0, len(live))
	for _, rec := range live {
		rec, _, rollups := h.hubRecentRollupWindow(ctx, rec, recentSince, hubActivityWindowMinutes+5)
		if len(rollups) == 0 {
			windows = append(windows, channelWindow{record: rec, viewers: rec.CurrentViewers, coverageState: "stats_only"})
			continue
		}
		windows = append(windows, summarizeChannelWindow(rec, filterPublicHubLiveRollups(rollups)))
	}
	return windows
}

func (h *Handler) hubLivePulseMomentsInBucket(ctx context.Context, start, end time.Time) []HubLivePulseMoment {
	windows := h.buildHubPoolWindows(ctx)
	if len(windows) == 0 {
		return nil
	}
	liveChannels := h.buildHubLiveChannels(ctx, windows)
	liveAll := h.buildHubLivePulseMoments(ctx, liveChannels)
	return hubPulseMomentsInTimeRange(liveAll, start.UnixMilli(), end.UnixMilli())
}

func (h *Handler) buildPublicHub(ctx context.Context, opts publicHubOptions) PublicHubResponse {
	now := time.Now().UTC()
	opts = normalizePublicHubOptions(opts)
	activityWindow := opts.ActivityWindowMinutes
	resp := PublicHubResponse{
		GeneratedAt:  now,
		TopEmotes:    []HubEmote{},
		TopMovers:    []HubMover{},
		LiveChannels: []HubLiveChannel{},
		Moments:      []HubMoment{},
		Activity:     HubActivity{Points: []HubActivityPoint{}, WindowMinutes: activityWindow},
	}

	// 1. Corpus counts (best-effort; zeros if DB unavailable).
	dbOK := true
	if h.store != nil {
		if stats, statsErr := h.store.PublicAggregateStats(ctx); statsErr == nil {
			resp.Corpus = HubCorpus{
				StreamsTracked:        stats.StreamsTracked,
				MomentsDetected:       stats.MomentsDetected,
				ChatMessagesProcessed: stats.ChatMessagesProcessed,
				EmotesIndexed:         stats.EmotesIndexed,
				VodsAnalyzed:          stats.VodsAnalyzed,
			}
		}
		if pingErr := h.store.Ping(ctx); pingErr != nil {
			dbOK = false
		}
	}

	// 2. Coverage / collector health.
	var snapshot TrackingSnapshot
	if h.collector != nil {
		snapshot = h.collector.TrackingSnapshot()
	}
	resp.Coverage = HubCoverage{
		LiveChannels:  snapshot.Active,
		TrackingMax:   snapshot.Max,
		EmotesIndexed: resp.Corpus.EmotesIndexed,
		DatabaseOK:    dbOK,
		State:         "operational",
	}
	if h.pulseBackfill != nil {
		bf := h.pulseBackfill.Snapshot()
		resp.Coverage.BackfillActive = bf.Active
		resp.Coverage.BackfillMax = bf.Max
	}
	if h.syncService != nil {
		resp.Coverage.SyncActive = len(h.syncService.ListActiveSyncs(ctx))
	}
	if !dbOK {
		resp.Coverage.State = "degraded"
	}

	// 2b. Corpus pipeline: Top-500 roster tracker summary + Silver/Gold tier
	// counts (aggregate, hosted-safe). Degrades to zeros when the underlying
	// tables are absent (e.g. local dev) or the store is unavailable.
	resp.CorpusPipeline = h.buildHubCorpusPipeline(ctx, now)
	if resp.CorpusPipeline.Roster.Live > resp.Coverage.LiveChannels {
		resp.Coverage.LiveChannels = resp.CorpusPipeline.Roster.Live
	}
	if resp.CorpusPipeline.State == CorpusStatusCritical {
		resp.Coverage.State = CorpusStatusCritical
	} else if resp.CorpusPipeline.State == CorpusStatusDegraded && resp.Coverage.State == "operational" {
		resp.Coverage.State = CorpusStatusDegraded
	}

	// 3. Join the live pool with the latest stream records. The collector
	// snapshot is in-memory and can be empty immediately after an analytics
	// restart, so seed the public chart pool from the persistent Top-500 live
	// roster as well. The roster is still bounded by hubPoolCap before we join
	// rollups, so long-range chart queries stay cheap.
	logins := make([]string, 0, len(snapshot.TrackedChannels)+hubPoolCap)
	seenLogins := make(map[string]bool, len(snapshot.TrackedChannels)+hubPoolCap)
	for _, login := range snapshot.TrackedChannels {
		login = normalizeLogin(login)
		if login == "" || seenLogins[login] {
			continue
		}
		seenLogins[login] = true
		logins = append(logins, login)
	}
	if h.store != nil {
		if roster, err := h.store.ListTop500LiveForPriorityWatch(ctx, hubTrackerTopN, hubPoolCap); err == nil {
			for _, row := range roster {
				login := normalizeLogin(row.Login)
				if login == "" || seenLogins[login] {
					continue
				}
				seenLogins[login] = true
				logins = append(logins, login)
				if len(logins) >= hubPoolCap {
					break
				}
			}
		}
	}
	var streams map[string]*StreamRecord
	if h.store != nil && len(logins) > 0 {
		if loaded, err := h.store.LatestStreamsByLogins(ctx, logins); err == nil {
			streams = loaded
		}
	}

	live := make([]*StreamRecord, 0, len(streams))
	for _, rec := range streams {
		if rec == nil || rec.EndedAt != nil {
			continue
		}
		live = append(live, rec)
	}
	sort.SliceStable(live, func(i, j int) bool {
		if live[i].CurrentViewers != live[j].CurrentViewers {
			return live[i].CurrentViewers > live[j].CurrentViewers
		}
		return live[i].Login < live[j].Login
	})
	resp.PoolSize = len(live)
	resp.Activity.ChannelCount = len(live)
	if len(live) > hubPoolCap {
		live = live[:hubPoolCap]
	}

	// 4. Aggregate pool-scoped rollups for live-channel windows and emote KPIs.
	// The public activity chart series is built from corpus-wide rollups below.
	poolActivity := map[int64]*HubActivityPoint{}
	emoteTotals := map[string]int{}
	minutePeak := 0
	var totalEmotes, totalSeventv int
	windows := make([]channelWindow, 0, len(live))
	activityBucketMinutes := hubActivityBucketMinutes(activityWindow)
	activitySince := now.Add(-time.Duration(activityWindow) * time.Minute)
	recentSince := now.Add(-time.Duration(hubActivityWindowMinutes+5) * time.Minute)
	longActivityWindow := activityWindow > hubActivityWindowMinutes

	for i, rec := range live {
		rec, loadedRollups, rollups := h.hubRecentRollupWindow(ctx, rec, recentSince, hubActivityWindowMinutes+5)
		if rec != nil {
			live[i] = rec
		}
		if len(rollups) == 0 {
			windows = append(windows, channelWindow{record: rec, viewers: rec.CurrentViewers, coverageState: "stats_only"})
		} else {
			win := summarizeChannelWindow(rec, filterPublicHubLiveRollups(rollups))
			windows = append(windows, win)
		}

		liveRollups := filterPublicHubLiveRollups(rollups)
		perMinuteEmote := map[int64]int{}
		for _, ru := range liveRollups {
			emoteCount := hubRollupEmoteCount(ru)
			bucket := ru.MinuteTS.UTC().Truncate(time.Minute).UnixMilli()
			perMinuteEmote[bucket] += emoteCount
			totalEmotes += emoteCount
			totalSeventv += ru.SevenTVEmoteCount
			for key, count := range ru.Emotes {
				emoteTotals[key] += count
			}
		}
		for _, v := range perMinuteEmote {
			if v > minutePeak {
				minutePeak = v
			}
		}
		activityRollups := rollupsSince(loadedRollups, activitySince)
		if !longActivityWindow && h.store != nil {
			if bucketed, err := h.store.RecentRollupBucketsByStreamID(ctx, rec.StreamID, activitySince, activityBucketMinutes, hubActivityMaxPoints); err == nil {
				activityRollups = bucketed
			}
		}
		for _, ru := range activityRollups {
			bucket := ru.MinuteTS.UTC().Truncate(time.Minute).UnixMilli()
			pt := poolActivity[bucket]
			if pt == nil {
				pt = &HubActivityPoint{T: bucket}
				poolActivity[bucket] = pt
			}
			chat, emotes, sevenTV := hubLiveActivityCounts(ru)
			pt.Chat += chat
			pt.Emotes += emotes
			pt.SevenTV += sevenTV
			if chat > 0 || emotes > 0 {
				pt.HasChatRollup = true
				tw, bt, fz := hubRollupProviderCounts(ru)
				pt.Twitch += tw
				pt.BTTV += bt
				pt.FFZ += fz
			}
			if hubLiveViewerRollup(ru) {
				if pickViewer(ru) > 0 {
					pt.HasViewerRollup = true
				}
				pt.Viewers += pickViewer(ru)
			}
		}
	}

	recentPoolActivity := cloneHubActivityPoints(poolActivity)
	activity := map[int64]*HubActivityPoint{}
	if h.store != nil {
		if buckets, err := h.store.AggregateRollupBucketsSince(ctx, activitySince, activityBucketMinutes, hubActivityMaxPoints); err == nil && len(buckets) > 0 {
			providerByBucket, _ := h.store.AggregateRollupProviderBucketsSince(ctx, activitySince, activityBucketMinutes, hubActivityMaxPoints)
			activity = hubActivityPointsFromRollupBuckets(buckets, providerByBucket)
		}
	}
	if len(activity) == 0 {
		activity = cloneHubActivityPoints(poolActivity)
	}
	if longActivityWindow {
		overlayRecentPoolHubActivity(activity, recentPoolActivity, now.Add(-time.Duration(hubActivityWindowMinutes)*time.Minute), activityWindow)
	}
	var top500Buckets []Top500ViewerBucket
	currentHelixSum := 0
	if h.store != nil {
		if buckets, err := h.store.AggregateTop500ViewerBucketsSince(ctx, activitySince, activityBucketMinutes, hubActivityMaxPoints); err == nil && len(buckets) > 0 {
			top500Buckets = buckets
		}
		if sum, err := h.store.SumTop500CurrentLiveViewers(ctx, resp.CorpusPipeline.TopN); err == nil {
			currentHelixSum = sum
		}
	}
	finalizeHubActivityViewers(activity, top500Buckets, currentHelixSum, now, activityWindow)

	// 5. Activity series (sorted, trimmed to the window).
	points := make([]HubActivityPoint, 0, len(activity))
	for _, pt := range activity {
		points = append(points, *pt)
	}
	sort.Slice(points, func(i, j int) bool { return points[i].T < points[j].T })
	if len(points) > hubActivityMaxPoints {
		points = points[len(points)-hubActivityMaxPoints:]
	}
	markHubActivityBucketComplete(points, now, activityBucketMinutes)
	if h.store != nil && len(points) > 0 {
		if byBucket, err := h.store.AggregateRollupTopEmotesBucketsSince(
			ctx, activitySince, activityBucketMinutes, hubActivityMaxPoints, hubActivityBucketTopEmotesCap,
		); err == nil {
			attachHubActivityBucketTopEmotes(ctx, h, points, byBucket)
		}
	}
	resp.Activity.Points = points

	// 6. Top emotes (sanitized, hosted CDN URLs when hosted).
	topEmotes := TopEmotesFromRollups([]MinuteRollup{{Emotes: emoteTotals}}, hubEmotesCap)
	topEmotes = h.rewritePortalTopEmotes(ctx, topEmotes)
	var emoteSum int
	for _, e := range topEmotes {
		emoteSum += e.Count
	}
	allEmoteSum := 0
	for _, c := range emoteTotals {
		allEmoteSum += c
	}
	for _, e := range topEmotes {
		share := 0.0
		if allEmoteSum > 0 {
			share = round2(float64(e.Count) / float64(allEmoteSum) * 100)
		}
		resp.TopEmotes = append(resp.TopEmotes, HubEmote{
			Name:      e.Name,
			Provider:  e.Provider,
			ImageURL:  e.ImageURL,
			Count:     e.Count,
			SharePct:  share,
			ZeroWidth: e.ZeroWidth,
			Animated:  e.Animated,
		})
	}

	// 7. Emote intelligence KPIs.
	minutesObserved := math.Max(1, float64(len(points)))
	topShare := 0.0
	if allEmoteSum > 0 && len(resp.TopEmotes) > 0 {
		topShare = resp.TopEmotes[0].SharePct
	}
	seventvShare := 0.0
	if totalEmotes > 0 {
		seventvShare = round2(float64(totalSeventv) / float64(totalEmotes) * 100)
	}
	resp.EmoteIntel = HubEmoteIntel{
		EmotesPerMin:    round2(float64(totalEmotes) / minutesObserved),
		TopEmoteShare:   topShare,
		UniqueEmotes:    len(emoteTotals),
		BiggestPeak:     minutePeak,
		SevenTVSharePct: seventvShare,
		ProviderShares:  hubProviderShares(emoteTotals, allEmoteSum),
	}

	// 8. Live channel rows + movers.
	resp.LiveChannels = h.buildHubLiveChannels(ctx, windows)
	resp.Activity.LivePoolViewerSum = hubLivePoolViewerSum(resp.LiveChannels)
	if _, peakAt := hubActivityPeakViewers(resp.Activity.Points); peakAt > 0 {
		resp.Activity.PeakViewersAt = peakAt
	}
	// Backfill avatars that the stored stream record never captured so the
	// portal's top-streamers rail and movers always render profile pictures.
	h.enrichHubProfileImages(ctx, resp.LiveChannels)

	movers := make([]channelWindow, len(windows))
	copy(movers, windows)
	sort.SliceStable(movers, func(i, j int) bool {
		if movers[i].emotesPerMin != movers[j].emotesPerMin {
			return movers[i].emotesPerMin > movers[j].emotesPerMin
		}
		if movers[i].seventvPerMin != movers[j].seventvPerMin {
			return movers[i].seventvPerMin > movers[j].seventvPerMin
		}
		return movers[i].chatPerMin > movers[j].chatPerMin
	})
	for _, win := range movers {
		if win.emotesPerMin <= 0 && win.chatPerMin <= 0 {
			continue
		}
		profileURL := ""
		if win.record != nil {
			profileURL = win.record.ProfileImageURL
		}
		resp.TopMovers = append(resp.TopMovers, HubMover{
			Login:           win.record.Login,
			DisplayName:     displayNameOrLogin(win.record),
			Category:        win.record.Category,
			ProfileImageURL: profileURL,
			Viewers:         win.viewers,
			EmotesPerMin:    win.emotesPerMin,
			SevenTVPerMin:   win.seventvPerMin,
			ChatPerMin:      win.chatPerMin,
			TrendPct:        win.trendPct,
		})
		if len(resp.TopMovers) >= hubMoversCap {
			break
		}
	}
	h.enrichHubMoverProfileImages(ctx, resp.TopMovers, resp.LiveChannels)

	// 9. Lightweight moments feed.
	resp.Moments = h.buildHubMoments(live, windows, now)

	// 10. Network-wide live pulse moments + featured session preview.
	resp.LivePulseMoments = h.buildHubLivePulseMoments(ctx, resp.LiveChannels)
	resp.LivePulseMomentsStatus, resp.LivePulseMomentsReason = hubLivePulseMomentsMeta(resp.LiveChannels, resp.LivePulseMoments)
	resp.FeaturedSession = h.buildHubFeaturedSession(ctx, resp.LiveChannels)

	return resp
}

// buildHubLiveChannels assembles the live-channel rail + directory rows. It is
// sourced primarily from the PERSISTENT Top-500 live roster (viewer-ordered) so
// the portal stays populated across analytics restarts even when the in-memory
// collector pool is sparse, then overlays rich per-minute stats (chat/7TV
// velocity, coverage state, avatar) for any channel the collector is actively
// tracking. Channels tracked outside the roster (e.g. portal/extension picks)
// are appended afterward. Falls back entirely to the collector windows when the
// roster is empty/unavailable (local dev without the roster tables).
func (h *Handler) buildHubLiveChannels(ctx context.Context, windows []channelWindow) []HubLiveChannel {
	rich := make(map[string]channelWindow, len(windows))
	for _, win := range windows {
		if win.record != nil {
			rich[normalizeLogin(win.record.Login)] = win
		}
	}

	var roster []Top500Current
	if h.store != nil {
		roster, _ = h.store.ListTop500LiveForPriorityWatch(ctx, hubTrackerTopN, hubLiveCap)
	}
	rosterByLogin := make(map[string]Top500Current, len(roster))
	for _, row := range roster {
		if login := normalizeLogin(row.Login); login != "" {
			rosterByLogin[login] = row
		}
	}

	out := make([]HubLiveChannel, 0, hubLiveCap)
	seen := make(map[string]bool, hubLiveCap)
	for _, row := range roster {
		login := normalizeLogin(row.Login)
		if login == "" || seen[login] {
			continue
		}
		seen[login] = true
		together, hostLogin, togetherWith := resolveStreamTogetherHubFields(login, row.Title, row.Tags, rosterByLogin)
		ch := HubLiveChannel{
			Login:             login,
			DisplayName:       strings.TrimSpace(row.DisplayName),
			Category:          categoryForTogetherStream(login, row.CategoryName, row.Title, row.Tags, rosterByLogin),
			CoverageState:     "stats_only",
			StreamingTogether: together,
			HostLogin:         hostLogin,
			TogetherWith:      togetherWith,
		}
		if row.ViewerCount != nil {
			ch.Viewers = *row.ViewerCount
		}
		if win, ok := rich[login]; ok {
			ch.ChatPerMin = win.chatPerMin
			ch.EmotesPerMin = win.emotesPerMin
			ch.SevenTVPerMin = win.seventvPerMin
			ch.TrendPct = win.trendPct
			if win.coverageState != "" {
				ch.CoverageState = win.coverageState
			}
			if win.viewers > 0 {
				ch.Viewers = win.viewers
			}
			if win.record != nil && win.record.ProfileImageURL != "" {
				ch.ProfileImageURL = win.record.ProfileImageURL
			}
			if ch.DisplayName == "" {
				ch.DisplayName = displayNameOrLogin(win.record)
			}
		}
		if ch.DisplayName == "" {
			ch.DisplayName = login
		}
		out = append(out, ch)
		if len(out) >= hubLiveCap {
			break
		}
	}

	if len(out) < hubLiveCap {
		extra := make([]channelWindow, 0, len(windows))
		for _, win := range windows {
			if win.record == nil || seen[normalizeLogin(win.record.Login)] {
				continue
			}
			extra = append(extra, win)
		}
		sort.SliceStable(extra, func(i, j int) bool { return extra[i].viewers > extra[j].viewers })
		for _, win := range extra {
			rec := win.record
			out = append(out, HubLiveChannel{
				Login:           normalizeLogin(rec.Login),
				DisplayName:     displayNameOrLogin(rec),
				Category:        rec.Category,
				ProfileImageURL: rec.ProfileImageURL,
				Viewers:         win.viewers,
				ChatPerMin:      win.chatPerMin,
				EmotesPerMin:    win.emotesPerMin,
				SevenTVPerMin:   win.seventvPerMin,
				CoverageState:   win.coverageState,
				TrendPct:        win.trendPct,
			})
			if len(out) >= hubLiveCap {
				break
			}
		}
	}
	return out
}

// enrichHubProfileImages fills missing avatars on the live-channel rows. It
// first consults a per-login Redis cache, then falls back to a single bounded
// Helix /users batch (<=100 logins) for the remainder, caching what it learns.
// It is best-effort: a disabled Helix client or any lookup error simply leaves
// the affected rows without an avatar (the UI shows an initial placeholder).
func (h *Handler) enrichHubProfileImages(ctx context.Context, channels []HubLiveChannel) {
	if len(channels) == 0 {
		return
	}
	missing := make([]string, 0, len(channels))
	for i := range channels {
		if channels[i].ProfileImageURL != "" {
			continue
		}
		login := normalizeLogin(channels[i].Login)
		if login == "" {
			continue
		}
		missing = append(missing, login)
	}
	resolved := h.resolveHubProfileURLs(ctx, missing)
	if len(resolved) == 0 {
		return
	}
	for i := range channels {
		if channels[i].ProfileImageURL != "" {
			continue
		}
		if url, ok := resolved[normalizeLogin(channels[i].Login)]; ok {
			channels[i].ProfileImageURL = url
		}
	}
}

func (h *Handler) enrichHubMoverProfileImages(ctx context.Context, movers []HubMover, channels []HubLiveChannel) {
	if len(movers) == 0 {
		return
	}
	byLogin := make(map[string]string, len(channels))
	for _, ch := range channels {
		if ch.ProfileImageURL != "" {
			byLogin[normalizeLogin(ch.Login)] = ch.ProfileImageURL
		}
	}
	missing := make([]string, 0, len(movers))
	for i := range movers {
		if movers[i].ProfileImageURL != "" {
			continue
		}
		login := normalizeLogin(movers[i].Login)
		if url, ok := byLogin[login]; ok {
			movers[i].ProfileImageURL = url
			continue
		}
		if login != "" {
			missing = append(missing, login)
		}
	}
	if len(missing) == 0 {
		return
	}
	resolved := h.resolveHubProfileURLs(ctx, missing)
	for i := range movers {
		if movers[i].ProfileImageURL != "" {
			continue
		}
		if url, ok := resolved[normalizeLogin(movers[i].Login)]; ok {
			movers[i].ProfileImageURL = url
		}
	}
}

func (h *Handler) resolveHubProfileURLs(ctx context.Context, logins []string) map[string]string {
	resolved := make(map[string]string, len(logins))
	if len(logins) == 0 {
		return resolved
	}
	missing := make([]string, 0, len(logins))
	seen := make(map[string]struct{}, len(logins))
	for _, raw := range logins {
		login := normalizeLogin(raw)
		if login == "" {
			continue
		}
		if _, ok := seen[login]; ok {
			continue
		}
		seen[login] = struct{}{}
		if h.rdb != nil {
			if url, err := h.rdb.Get(ctx, hubProfileCachePrefix+login).Result(); err == nil && url != "" {
				resolved[login] = url
				continue
			}
		}
		missing = append(missing, login)
	}
	if len(missing) > 0 && h.helix != nil && h.helix.Enabled() {
		if profiles, err := h.helix.UsersByLogin(ctx, missing); err == nil {
			for login, prof := range profiles {
				if prof.ProfileImageURL == "" {
					continue
				}
				login = normalizeLogin(login)
				resolved[login] = prof.ProfileImageURL
				if h.rdb != nil {
					_ = h.rdb.Set(ctx, hubProfileCachePrefix+login, prof.ProfileImageURL, hubProfileCacheTTL).Err()
				}
			}
		}
	}
	return resolved
}

func hubProviderShares(emoteTotals map[string]int, total int) []HubProviderShare {
	if total <= 0 || len(emoteTotals) == 0 {
		return nil
	}
	providerTotals := map[string]int{}
	for key, count := range emoteTotals {
		if count <= 0 {
			continue
		}
		_, _, provider := splitEmoteKey(key)
		providerTotals[hubProviderLabel(provider)] += count
	}
	shares := make([]HubProviderShare, 0, len(providerTotals))
	for provider, count := range providerTotals {
		shares = append(shares, HubProviderShare{
			Provider: provider,
			Count:    count,
			SharePct: round2(float64(count) / float64(total) * 100),
		})
	}
	sort.SliceStable(shares, func(i, j int) bool {
		if shares[i].Count != shares[j].Count {
			return shares[i].Count > shares[j].Count
		}
		return shares[i].Provider < shares[j].Provider
	})
	return shares
}

func hubProviderLabel(provider string) string {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "7tv", "seventv":
		return "7TV"
	case "twitch":
		return "Twitch"
	case "ffz":
		return "FFZ"
	case "bttv":
		return "BTTV"
	default:
		return "Other"
	}
}

// buildHubCorpusPipeline assembles the hosted-safe corpus pipeline block. It
// reuses buildTop100ReadinessReport but exposes ONLY collector capacity + the
// aggregate readiness summary, plus Silver/Gold tier counts. No per-channel
// rows, admission attempts/messages, logins, stream IDs, or job errors leak.
func (h *Handler) buildHubCorpusPipeline(ctx context.Context, now time.Time) HubCorpusPipeline {
	cfg := h.corpusRuntimeConfig()
	topN := cfg.TargetTopN
	if topN <= 0 {
		topN = hubTrackerTopN
	}
	pipeline := HubCorpusPipeline{
		GeneratedAt:          now,
		State:                CorpusStatusHealthy,
		TopN:                 topN,
		LiveAdmissionEnabled: cfg.LiveAdmissionEnabled,
		LiveAdmissionTopN:    cfg.LiveAdmissionTopN,
		MaxActiveIRCChannels: cfg.MaxActiveIRCChannels,
	}

	// Errors (e.g. roster tables absent in local dev) degrade to zeroed counts;
	// collector capacity is still populated from the in-memory snapshot.
	report, err := h.buildTop100ReadinessReport(ctx, topN, cfg.LiveAdmissionEnabled, ReadinessReportOptions{SkipRollups: true})
	if err != nil {
		pipeline.State = CorpusStatusDegraded
	}
	pipeline.CollectorActive = report.CollectorActive
	pipeline.CollectorMax = report.CollectorMax
	pipeline.RuntimeConfigFingerprint = corpusRuntimeHubCacheFingerprint(cfg, report.CollectorMax)
	pipeline.MetadataSampledAgoSeconds = top500ReportMetadataSampledAgoSeconds(report)
	if report.TopN > 0 {
		pipeline.TopN = report.TopN
	}
	if err == nil {
		pipeline.State = corpusPipelineStateFromReadiness(cfg, report)
	}
	pipeline.Roster = HubTrackerSummary{
		Live:                     report.Summary.LiveRows,
		CollectorTracking:        report.Summary.CollectorTrackingRows,
		ExpectedCollectorRows:    report.Summary.ExpectedCollectorRows,
		LiveCollectorDeficitRows: report.Summary.LiveCollectorDeficitRows,
		MetadataOnly:             report.Summary.MetadataOnlyRows,
		MetadataStale:            report.Summary.MetadataStaleRows,
		AdmissionFeatureDisabled: admissionFeatureDisabledRows(cfg, report),
		AdmissionDisabled:        report.Summary.AdmissionDisabledRows,
		CapacityBlocked:          report.Summary.CapacityBlockedRows,
		Warming:                  report.Summary.WarmingRows,
		Collecting:               report.Summary.CollectingRows,
		ViewerOnly:               report.Summary.ViewerOnlyRows,
		ZeroChatAfterAge:         report.Summary.ZeroChatAfterAgeRows,
	}

	if h.store != nil {
		if counts, err := h.store.BackfillTierCounts(ctx); err == nil {
			for _, c := range counts {
				switch strings.ToLower(strings.TrimSpace(c.Tier)) {
				case "silver":
					applyHubTierCount(&pipeline.Silver, c.Status, c.Count)
				case "gold", "gold_full", "gold_lite":
					applyHubTierCount(&pipeline.Gold, c.Status, c.Count)
				}
			}
		}
		if eligible, err := h.store.CorpusSilverEligibleCount(ctx, pipeline.TopN); err == nil {
			pipeline.Silver.Eligible = eligible
		}
		if eligible, err := h.store.CorpusGoldEligibleCount(ctx); err == nil {
			pipeline.Gold.Eligible = eligible
		}
		if age, err := h.store.BackfillOldestQueuedAgeSeconds(ctx, "silver"); err == nil {
			pipeline.Silver.OldestQueuedSeconds = age
		}
		if age, err := h.store.BackfillOldestQueuedAgeSeconds(ctx, "gold", "gold_full", "gold_lite"); err == nil {
			pipeline.Gold.OldestQueuedSeconds = age
		}
	}
	return pipeline
}

func admissionFeatureDisabledRows(cfg CorpusRuntimeConfig, report Top100ReadinessReport) int {
	if cfg.LiveAdmissionEnabled || report.Summary.LiveRows <= 0 {
		return 0
	}
	return report.Summary.LiveRows
}

func top500ReportMetadataSampledAgoSeconds(report Top100ReadinessReport) *int {
	var maxAge *int
	for _, row := range report.Rows {
		if row.MetadataFreshnessSeconds == nil {
			continue
		}
		age := *row.MetadataFreshnessSeconds
		if maxAge == nil || age > *maxAge {
			ageCopy := age
			maxAge = &ageCopy
		}
	}
	return maxAge
}

func applyHubTierCount(counts *HubTierCounts, status string, n int) {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "queued":
		counts.Queued += n
	case "running":
		counts.Running += n
	case "done":
		counts.Done += n
	case "skipped":
		counts.Skipped += n
	case "failed":
		counts.Failed += n
	}
	counts.Total += n
}

func summarizeChannelWindow(rec *StreamRecord, rollups []MinuteRollup) channelWindow {
	win := channelWindow{record: rec, viewers: rec.CurrentViewers, coverageState: "stats_only"}
	rollups = filterPublicHubLiveRollups(rollups)
	if len(rollups) == 0 {
		return win
	}
	var chat, emotes, seventv, viewerSamples, minutesWithData int
	for _, ru := range rollups {
		if ru.Missing {
			continue
		}
		emoteCount := hubRollupEmoteCount(ru)
		if ru.ChatCount > 0 || emoteCount > 0 || ru.SevenTVEmoteCount > 0 || ru.ViewerSamples > 0 {
			minutesWithData++
		}
		chat += ru.ChatCount
		emotes += emoteCount
		seventv += ru.SevenTVEmoteCount
		viewerSamples += ru.ViewerSamples
	}
	minutes := math.Max(1, float64(minutesWithData))
	win.chatPerMin = round2(float64(chat) / minutes)
	win.emotesPerMin = round2(float64(emotes) / minutes)
	win.seventvPerMin = round2(float64(seventv) / minutes)
	if v := pickViewer(rollups[len(rollups)-1]); v > 0 {
		win.viewers = v
	}
	switch {
	case viewerSamples > 0 && chat > 0:
		win.coverageState = "synced"
	case chat > 0:
		win.coverageState = "chat_only"
	case viewerSamples > 0:
		win.coverageState = "viewer_only"
	default:
		win.coverageState = "stats_only"
	}
	win.trendPct = windowTrendPct(rollups)
	return win
}

// windowTrendPct compares activity (chat+emotes) in the most recent 5 minutes
// against the prior 5 minutes, returning a bounded percentage delta.
func windowTrendPct(rollups []MinuteRollup) float64 {
	rollups = filterPublicHubLiveRollups(rollups)
	n := len(rollups)
	if n < 4 {
		return 0
	}
	recentSpan := 5
	if recentSpan > n/2 {
		recentSpan = n / 2
	}
	recent := 0
	for _, ru := range rollups[n-recentSpan:] {
		recent += ru.ChatCount + hubRollupEmoteCount(ru)
	}
	prior := 0
	for _, ru := range rollups[n-2*recentSpan : n-recentSpan] {
		prior += ru.ChatCount + hubRollupEmoteCount(ru)
	}
	if prior <= 0 {
		if recent > 0 {
			return 100
		}
		return 0
	}
	delta := float64(recent-prior) / float64(prior) * 100
	return round2(math.Max(-100, math.Min(500, delta)))
}

func hubLiveActivityCounts(ru MinuteRollup) (chat, emotes, sevenTV int) {
	if !isLiveChatRollup(ru) {
		return 0, 0, 0
	}
	return ru.ChatCount, hubRollupEmoteCount(ru), ru.SevenTVEmoteCount
}

func hubRollupProviderCounts(ru MinuteRollup) (twitch, bttv, ffz int) {
	if !isLiveChatRollup(ru) || len(ru.Emotes) == 0 {
		return 0, 0, 0
	}
	for key, count := range ru.Emotes {
		if count <= 0 {
			continue
		}
		_, _, provider := splitEmoteKey(key)
		switch strings.ToLower(strings.TrimSpace(provider)) {
		case "twitch":
			twitch += count
		case "bttv":
			bttv += count
		case "ffz":
			ffz += count
		}
	}
	return twitch, bttv, ffz
}

func hubLiveViewerRollup(ru MinuteRollup) bool {
	switch ru.ChatSource {
	case RollupChatSourceGQL, RollupChatSourceIVR, ChatSourceMixed:
		return false
	}
	return true
}

// filterPublicHubLiveRollups keeps only live IRC / verified rows for hub KPIs.
func filterPublicHubLiveRollups(rollups []MinuteRollup) []MinuteRollup {
	if len(rollups) == 0 {
		return rollups
	}
	out := make([]MinuteRollup, 0, len(rollups))
	for _, ru := range rollups {
		if isLiveChatRollup(ru) || (hubLiveViewerRollup(ru) && ru.ViewerSamples > 0) {
			out = append(out, ru)
		}
	}
	return out
}

func hubRollupEmoteCount(ru MinuteRollup) int {
	total := ru.TotalEmoteCount
	if total <= 0 {
		for _, count := range ru.Emotes {
			if count > 0 {
				total += count
			}
		}
	}
	if total < ru.SevenTVEmoteCount {
		return ru.SevenTVEmoteCount
	}
	return total
}

func (h *Handler) buildHubMoments(live []*StreamRecord, windows []channelWindow, now time.Time) []HubMoment {
	moments := make([]HubMoment, 0, hubMomentsCap)
	cutoff := now.Add(-hubMomentLookback)

	byLogin := make(map[string]channelWindow, len(windows))
	for _, win := range windows {
		if win.record != nil {
			byLogin[win.record.Login] = win
		}
	}

	for _, rec := range live {
		if rec == nil || rec.StartedAt.IsZero() || rec.StartedAt.Before(cutoff) {
			continue
		}
		moments = append(moments, HubMoment{
			Kind:        "live_attach",
			Login:       rec.Login,
			DisplayName: displayNameOrLogin(rec),
			Label:       displayNameOrLogin(rec) + " went live",
			Detail:      rec.Category,
			At:          rec.StartedAt.UTC().UnixMilli(),
		})
	}

	for _, win := range windows {
		if win.trendPct >= 60 && (win.chatPerMin >= 30 || win.seventvPerMin >= 8) {
			kind := "chat_spike"
			label := displayNameOrLogin(win.record) + " chat surging"
			if win.seventvPerMin >= win.chatPerMin*0.4 && win.seventvPerMin >= 8 {
				kind = "emote_spike"
				label = displayNameOrLogin(win.record) + " emote spam spike"
			}
			moments = append(moments, HubMoment{
				Kind:        kind,
				Login:       win.record.Login,
				DisplayName: displayNameOrLogin(win.record),
				Label:       label,
				Detail:      win.record.Category,
				Magnitude:   win.trendPct,
				At:          now.UnixMilli(),
			})
		}
	}

	if h.pulseBackfill != nil {
		for _, job := range h.pulseBackfill.ListJobs(true) {
			if job.UpdatedAt.Before(cutoff) {
				continue
			}
			switch job.Status {
			case PulseBackfillQueued, PulseBackfillFetchingChat, PulseBackfillWritingRollups:
				moments = append(moments, HubMoment{
					Kind:        "backfill_queued",
					Login:       job.Login,
					DisplayName: job.Login,
					Label:       job.Login + " VOD backfill running",
					At:          job.UpdatedAt.UTC().UnixMilli(),
				})
			case PulseBackfillDone:
				moments = append(moments, HubMoment{
					Kind:        "backfill_done",
					Login:       job.Login,
					DisplayName: job.Login,
					Label:       job.Login + " backfill complete",
					At:          job.UpdatedAt.UTC().UnixMilli(),
				})
			}
		}
	}

	sort.SliceStable(moments, func(i, j int) bool { return moments[i].At > moments[j].At })
	if len(moments) > hubMomentsCap {
		moments = moments[:hubMomentsCap]
	}
	return moments
}

func cloneHubActivityPoints(src map[int64]*HubActivityPoint) map[int64]*HubActivityPoint {
	if len(src) == 0 {
		return map[int64]*HubActivityPoint{}
	}
	out := make(map[int64]*HubActivityPoint, len(src))
	for key, pt := range src {
		if pt == nil {
			continue
		}
		copied := *pt
		out[key] = &copied
	}
	return out
}

func hubActivityPointsFromRollupBuckets(buckets []MinuteRollup, providerByBucket map[int64]HubProviderBucketCounts) map[int64]*HubActivityPoint {
	activity := make(map[int64]*HubActivityPoint, len(buckets))
	for _, ru := range buckets {
		bucket := ru.MinuteTS.UTC().Truncate(time.Minute).UnixMilli()
		pt := &HubActivityPoint{
			T:               bucket,
			Chat:            ru.ChatCount,
			Emotes:          hubRollupEmoteCount(ru),
			SevenTV:         ru.SevenTVEmoteCount,
			Viewers:         pickViewer(ru),
			HasChatRollup:   ru.ChatCount > 0 || hubRollupEmoteCount(ru) > 0,
			HasViewerRollup: pickViewer(ru) > 0,
		}
		if pc, ok := providerByBucket[bucket]; ok {
			pt.Twitch = pc.Twitch
			pt.BTTV = pc.BTTV
			pt.FFZ = pc.FFZ
		}
		activity[bucket] = pt
	}
	return activity
}

// finalizeHubActivityViewers is the last viewer mutation on hub activity — Top-500
// snapshot totals must win over corpus rollups and live-pool overlay peaks.
func finalizeHubActivityViewers(activity map[int64]*HubActivityPoint, top500Buckets []Top500ViewerBucket, currentHelixSum int, now time.Time, windowMinutes int) {
	mergeTop500ViewerBucketsIntoActivity(activity, top500Buckets)
	forwardFillTop500ViewerTrail(activity, top500Buckets)
	mergeCurrentHelixLiveViewersIntoOpenBucket(activity, currentHelixSum, now, windowMinutes)
}

func forwardFillTop500ViewerTrail(activity map[int64]*HubActivityPoint, top500Buckets []Top500ViewerBucket) {
	if len(activity) == 0 || len(top500Buckets) == 0 {
		return
	}
	top500ByT := make(map[int64]int, len(top500Buckets))
	for _, bucket := range top500Buckets {
		if bucket.Viewers > 0 {
			top500ByT[bucket.T] = bucket.Viewers
		}
	}
	keys := make([]int64, 0, len(activity))
	for key := range activity {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
	running := 0
	for _, key := range keys {
		if viewers, ok := top500ByT[key]; ok && viewers > running {
			running = viewers
		}
		pt := activity[key]
		if pt == nil || running <= 0 {
			continue
		}
		if pt.Viewers > 0 && pt.Viewers < running/3 {
			pt.Viewers = running
			pt.HasViewerRollup = true
		}
	}
}

// mergeCurrentHelixLiveViewersIntoOpenBucket floors the in-progress coarse bucket with
// the current Helix live roster sum when corpus rollups are sparse.
func mergeCurrentHelixLiveViewersIntoOpenBucket(activity map[int64]*HubActivityPoint, currentHelixSum int, now time.Time, windowMinutes int) {
	if currentHelixSum <= 0 || len(activity) == 0 {
		return
	}
	bucketMin := hubActivityBucketMinutes(windowMinutes)
	bucketMs := int64(bucketMin) * 60_000
	if bucketMs <= 0 {
		bucketMs = 60_000
	}
	openKey := (now.UTC().UnixMilli() / bucketMs) * bucketMs
	pt := activity[openKey]
	if pt == nil {
		pt = &HubActivityPoint{T: openKey}
		activity[openKey] = pt
	}
	if currentHelixSum > pt.Viewers {
		pt.Viewers = currentHelixSum
		pt.HasViewerRollup = true
	}
}

func hubActivityPeakViewers(points []HubActivityPoint) (peak int, at int64) {
	for _, pt := range points {
		if pt.Viewers > peak {
			peak = pt.Viewers
			at = pt.T
		}
	}
	return peak, at
}

func hubLivePoolViewerSum(channels []HubLiveChannel) int {
	sum := 0
	for _, ch := range channels {
		sum += ch.Viewers
	}
	return sum
}

func markHubActivityBucketComplete(points []HubActivityPoint, now time.Time, bucketMinutes int) {
	if len(points) == 0 || bucketMinutes <= 0 {
		return
	}
	bucketDur := time.Duration(bucketMinutes) * time.Minute
	for i := range points {
		bucketEnd := time.UnixMilli(points[i].T).UTC().Add(bucketDur)
		points[i].BucketComplete = !bucketEnd.After(now)
	}
}

func mergeTop500ViewerBucketsIntoActivity(activity map[int64]*HubActivityPoint, top500Buckets []Top500ViewerBucket) {
	if activity == nil || len(top500Buckets) == 0 {
		return
	}
	for _, bucket := range top500Buckets {
		if bucket.Viewers <= 0 {
			continue
		}
		pt := activity[bucket.T]
		if pt == nil {
			pt = &HubActivityPoint{T: bucket.T}
			activity[bucket.T] = pt
		}
		if bucket.Viewers > pt.Viewers {
			pt.Viewers = bucket.Viewers
		}
		pt.HasViewerRollup = true
	}
}

// overlayRecentPoolHubActivity replaces coarse corpus buckets in the recent live
// window with live-pool minute rollups summed per coarse bucket (peak concurrent
// global viewers inside each bucket).
func overlayRecentPoolHubActivity(
	coarse map[int64]*HubActivityPoint,
	recentPool map[int64]*HubActivityPoint,
	recentCutoff time.Time,
	windowMinutes int,
) {
	if len(coarse) == 0 || len(recentPool) == 0 {
		return
	}
	bucketMin := hubActivityBucketMinutes(windowMinutes)
	bucketMs := int64(bucketMin) * 60_000
	if bucketMs <= 0 {
		bucketMs = 60_000
	}
	cutoffMs := recentCutoff.UTC().UnixMilli()

	coarseRecent := make(map[int64]*HubActivityPoint)
	for minuteT, minutePt := range recentPool {
		if minutePt == nil || minuteT < cutoffMs {
			continue
		}
		coarseKey := (minuteT / bucketMs) * bucketMs
		acc := coarseRecent[coarseKey]
		if acc == nil {
			acc = &HubActivityPoint{T: coarseKey}
			coarseRecent[coarseKey] = acc
		}
		if minutePt.Viewers > acc.Viewers {
			acc.Viewers = minutePt.Viewers
		}
		acc.Chat += minutePt.Chat
		acc.Emotes += minutePt.Emotes
		acc.SevenTV += minutePt.SevenTV
		acc.Twitch += minutePt.Twitch
		acc.BTTV += minutePt.BTTV
		acc.FFZ += minutePt.FFZ
		if minutePt.HasChatRollup || minutePt.Chat > 0 || minutePt.Emotes > 0 {
			acc.HasChatRollup = true
		}
		if minutePt.HasViewerRollup || minutePt.Viewers > 0 {
			acc.HasViewerRollup = true
		}
	}

	for key, pt := range coarse {
		if key < cutoffMs {
			continue
		}
		alignedKey := (key / bucketMs) * bucketMs
		overlay, ok := coarseRecent[alignedKey]
		if !ok {
			overlay, ok = coarseRecent[key]
		}
		if !ok {
			continue
		}
		if overlay.Viewers > pt.Viewers {
			pt.Viewers = overlay.Viewers
		}
		if overlay.Viewers > 0 {
			pt.HasViewerRollup = pt.HasViewerRollup || overlay.HasViewerRollup
		}
		if overlay.HasChatRollup || overlay.Chat > 0 || overlay.Emotes > 0 {
			pt.Chat = overlay.Chat
			pt.Emotes = overlay.Emotes
			pt.SevenTV = overlay.SevenTV
			pt.Twitch = overlay.Twitch
			pt.BTTV = overlay.BTTV
			pt.FFZ = overlay.FFZ
			pt.HasChatRollup = true
		}
	}
}

// peakConcurrentGlobalViewers returns the highest per-minute sum of viewer counts
// across streams in a bucket (mirrors AggregateRollupBucketsSince SQL semantics).
func peakConcurrentGlobalViewers(rows []MinuteRollup) int {
	if len(rows) == 0 {
		return 0
	}
	byMinute := map[int64]int{}
	for _, ru := range rows {
		if !hubLiveViewerRollup(ru) {
			continue
		}
		key := ru.MinuteTS.UTC().Truncate(time.Minute).UnixMilli()
		byMinute[key] += pickViewer(ru)
	}
	peak := 0
	for _, total := range byMinute {
		if total > peak {
			peak = total
		}
	}
	return peak
}

func pickViewer(ru MinuteRollup) int {
	if ru.ViewerLatest > 0 {
		return ru.ViewerLatest
	}
	if ru.ViewerMax > 0 {
		return ru.ViewerMax
	}
	return ru.ViewerAvg
}

func hubBucketEmotesFromTop(top []TopEmote) []HubBucketEmote {
	if len(top) == 0 {
		return nil
	}
	out := make([]HubBucketEmote, 0, len(top))
	for _, emote := range top {
		name := strings.TrimSpace(emote.Name)
		if name == "" || emote.Count <= 0 {
			continue
		}
		out = append(out, HubBucketEmote{
			Name:     name,
			Provider: emote.Provider,
			ImageURL: emote.ImageURL,
			Count:    emote.Count,
		})
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func attachHubActivityBucketTopEmotes(ctx context.Context, h *Handler, points []HubActivityPoint, byBucket map[int64]map[string]int) {
	if len(points) == 0 || len(byBucket) == 0 {
		return
	}
	for i := range points {
		totals := byBucket[points[i].T]
		if len(totals) == 0 {
			continue
		}
		top := TopEmotesFromRollups([]MinuteRollup{{Emotes: totals}}, hubActivityBucketTopEmotesCap)
		if h != nil {
			top = h.rewritePortalTopEmotes(ctx, top)
		}
		points[i].TopEmotes = hubBucketEmotesFromTop(top)
	}
}

func displayNameOrLogin(rec *StreamRecord) string {
	if rec == nil {
		return ""
	}
	if rec.DisplayName != "" {
		return rec.DisplayName
	}
	return rec.Login
}
