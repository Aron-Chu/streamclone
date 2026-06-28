package analytics

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"streamclone/internal/metrics"
)

const defaultIVRBaseURL = "https://logs.ivr.fi"

// GoldIVRAllowlist holds staged rollout entries (login and/or Twitch channel user ID).
type GoldIVRAllowlist struct {
	Logins     map[string]struct{}
	ChannelIDs map[string]struct{}
}

// GoldIVRConfig controls IVR Gold Lite accelerator behavior (all off by default).
type GoldIVRConfig struct {
	Enabled                     bool
	LiteEnabled                 bool
	CanonicalReplace            bool
	PeaksOnlyEnabled            bool
	PeaksOnlyMaxMinutes         int
	PeaksOnlyMinChatCount       int
	BaseURL                     string
	MaxBytesPerJob              int64
	MaxMessagesPerJob           int
	MaxDurationMinutes          int
	HTTPTimeout                 time.Duration
	MaxRetries                  int
	Allowlist                   GoldIVRAllowlist
	ParserErrorRateMax          float64
	MinMessagesPerWindow        int
	ShadowMode                  bool
	ShadowArtifactDir           string
	ShadowArtifactRetentionDays int
	ShadowArtifactMaxFiles      int
}

// GoldIVRAttemptResult summarizes one accelerator pass before GQL.
type GoldIVRAttemptResult struct {
	Attempted            bool
	PreflightHit         bool
	Imported             bool
	PeaksOnly            bool
	ShadowOnly           bool
	FallbackGQL          bool
	Messages             int
	MinutesWritten       int
	ShadowScorePct       float64
	ShadowRecommendation string
	ShadowArtifactPath   string
	Reason               string
}

// GoldIVRService implements IVR preflight + provisional Gold Lite import.
type GoldIVRService struct {
	cfg    GoldIVRConfig
	store  *Store
	client *http.Client
	log    *slog.Logger

	preflightMu    sync.Mutex
	preflightCache map[string]ivrPreflightCacheEntry
}

type ivrPreflightCacheEntry struct {
	hit       bool
	expiresAt time.Time
}

type ivrRawMessage struct {
	ID          string            `json:"id"`
	Text        string            `json:"text"`
	DisplayName string            `json:"displayName"`
	Username    string            `json:"username"`
	Timestamp   string            `json:"timestamp"`
	Tags        map[string]string `json:"tags"`
}

type ivrImportStats struct {
	messages      int
	bytesRead     int64
	parserErrors  int
	windows       []ChatSourceWindow
	minuteBuckets map[time.Time]int
	emoteBuckets  map[time.Time]map[string]int
}

func (s *ivrImportStats) minuteRollups(anchor time.Time) []MinuteRollup {
	out := make([]MinuteRollup, 0, len(s.minuteBuckets))
	for minute, count := range s.minuteBuckets {
		emotes := s.emoteBuckets[minute]
		if emotes == nil {
			emotes = map[string]int{}
		}
		totalEmotes := 0
		for _, c := range emotes {
			totalEmotes += c
		}
		_ = anchor
		out = append(out, MinuteRollup{
			MinuteTS:          minute,
			ChatCount:         count,
			TotalEmoteCount:   totalEmotes,
			SevenTVEmoteCount: 0,
			Emotes:            emotes,
		})
	}
	return out
}

func NewGoldIVRService(cfg GoldIVRConfig, store *Store, client *http.Client, log *slog.Logger) *GoldIVRService {
	if strings.TrimSpace(cfg.BaseURL) == "" {
		cfg.BaseURL = defaultIVRBaseURL
	}
	if cfg.HTTPTimeout <= 0 {
		cfg.HTTPTimeout = 30 * time.Second
	}
	if cfg.MaxBytesPerJob <= 0 {
		cfg.MaxBytesPerJob = 64 << 20
	}
	if cfg.MaxMessagesPerJob <= 0 {
		cfg.MaxMessagesPerJob = 500_000
	}
	if cfg.MaxDurationMinutes <= 0 {
		cfg.MaxDurationMinutes = 180
	}
	if cfg.MaxRetries <= 0 {
		cfg.MaxRetries = 2
	}
	if cfg.ParserErrorRateMax <= 0 {
		cfg.ParserErrorRateMax = 0.001
	}
	if cfg.MinMessagesPerWindow <= 0 {
		cfg.MinMessagesPerWindow = 1
	}
	if cfg.PeaksOnlyMaxMinutes <= 0 {
		cfg.PeaksOnlyMaxMinutes = defaultIVRPeaksMaxMinutes
	}
	if cfg.PeaksOnlyMinChatCount <= 0 {
		cfg.PeaksOnlyMinChatCount = defaultIVRPeaksMinChatCount
	}
	if cfg.ShadowArtifactRetentionDays <= 0 {
		cfg.ShadowArtifactRetentionDays = 7
	}
	if cfg.ShadowArtifactMaxFiles <= 0 {
		cfg.ShadowArtifactMaxFiles = 1000
	}
	if client == nil {
		client = &http.Client{Timeout: cfg.HTTPTimeout}
	}
	if log == nil {
		log = slog.Default()
	}
	return &GoldIVRService{
		cfg:            cfg,
		store:          store,
		client:         client,
		log:            log,
		preflightCache: map[string]ivrPreflightCacheEntry{},
	}
}

// ParseGoldIVRAllowlist parses comma-separated allowlist entries.
// Accepts lowercase Twitch logins (letters/digits/underscore) and numeric channel user IDs.
// Display names are not supported — use login or broadcaster ID only.
// When GOLD_IVR is enabled, an empty allowlist denies all channels (safe staged rollout).
func ParseGoldIVRAllowlist(raw string) GoldIVRAllowlist {
	out := GoldIVRAllowlist{
		Logins:     map[string]struct{}{},
		ChannelIDs: map[string]struct{}{},
	}
	for _, part := range strings.Split(raw, ",") {
		entry := strings.TrimSpace(part)
		if entry == "" {
			continue
		}
		if isNumericChannelID(entry) {
			out.ChannelIDs[entry] = struct{}{}
			continue
		}
		out.Logins[strings.ToLower(entry)] = struct{}{}
	}
	return out
}

func isNumericChannelID(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// allowed reports whether IVR Lite may run for a channel and why.
func (g *GoldIVRService) allowed(login, channelID string) (bool, string) {
	if !g.cfg.Enabled {
		return false, "ivr_disabled"
	}
	if g.cfg.CanonicalReplace {
		return false, "ivr_canonical_replace_blocked"
	}
	if !g.cfg.LiteEnabled && !g.cfg.ShadowMode && !g.cfg.PeaksOnlyEnabled {
		return false, "ivr_lite_disabled"
	}
	if g.cfg.LiteEnabled && g.cfg.PeaksOnlyEnabled {
		return false, "ivr_lite_and_peaks_only_mutually_exclusive"
	}
	allow := g.cfg.Allowlist
	if len(allow.Logins) == 0 && len(allow.ChannelIDs) == 0 {
		return false, "allowlist_empty"
	}
	loginKey := strings.ToLower(strings.TrimSpace(login))
	channelKey := strings.TrimSpace(channelID)
	if loginKey != "" {
		if _, ok := allow.Logins[loginKey]; ok {
			return true, "allowlist_login:" + loginKey
		}
	}
	if channelKey != "" {
		if _, ok := allow.ChannelIDs[channelKey]; ok {
			return true, "allowlist_channel_id:" + channelKey
		}
	}
	return false, "allowlist_miss"
}

// TryAccelerator runs IVR preflight + provisional import ahead of canonical GQL.
func (g *GoldIVRService) TryAccelerator(ctx context.Context, streamID, login string) GoldIVRAttemptResult {
	out := GoldIVRAttemptResult{FallbackGQL: true}
	if g == nil || g.store == nil {
		out.Reason = "ivr_not_configured"
		return out
	}

	stream, err := g.store.StreamByID(ctx, streamID)
	if err != nil || stream == nil {
		out.Reason = "stream_not_found"
		metrics.GoldIVRJobsFailedTotal.Inc()
		metrics.GoldIVRJobsFallbackGQLTotal.Inc()
		return out
	}
	channelID := strings.TrimSpace(stream.BroadcasterID)
	if channelID == "" {
		out.Reason = "missing_channel_id"
		metrics.GoldIVRJobsFailedTotal.Inc()
		metrics.GoldIVRJobsFallbackGQLTotal.Inc()
		return out
	}
	allowed, allowReason := g.allowed(login, channelID)
	if !allowed {
		out.Reason = allowReason
		return out
	}
	out.Attempted = true
	metrics.GoldIVRJobsTotal.Inc()

	windowStart := stream.StartedAt.UTC()
	windowEnd := time.Now().UTC()
	if stream.EndedAt != nil {
		windowEnd = stream.EndedAt.UTC()
	}
	if windowEnd.Before(windowStart) || windowEnd.Equal(windowStart) {
		out.Reason = "invalid_stream_window"
		metrics.GoldIVRJobsFailedTotal.Inc()
		metrics.GoldIVRJobsFallbackGQLTotal.Inc()
		return out
	}
	maxDur := time.Duration(g.cfg.MaxDurationMinutes) * time.Minute
	if windowEnd.Sub(windowStart) > maxDur {
		windowEnd = windowStart.Add(maxDur)
	}

	hit, preflightReason := g.preflightCoverage(ctx, channelID, windowStart)
	if !hit {
		out.Reason = preflightReason
		metrics.GoldIVRJobsFallbackGQLTotal.Inc()
		g.log.Info("gold ivr preflight miss",
			"stream_id", streamID, "login", login, "channel_id", channelID, "reason", preflightReason)
		return out
	}
	out.PreflightHit = true

	existing, err := g.store.RollupsByStream(ctx, streamID)
	if err != nil {
		out.Reason = "rollup_load_failed"
		metrics.GoldIVRJobsFailedTotal.Inc()
		metrics.GoldIVRJobsFallbackGQLTotal.Inc()
		return out
	}
	livePct := estimateLiveCoveragePct(existing, windowStart, windowEnd)

	importExisting := existing
	if g.cfg.ShadowMode {
		// Shadow/compare must read full IVR window even when GQL canonical minutes exist.
		importExisting = nil
	}
	stats, importErr := g.importWindow(ctx, channelID, windowStart, windowEnd, importExisting)
	if importErr != nil {
		out.Reason = importErr.Error()
		metrics.GoldIVRJobsFailedTotal.Inc()
		metrics.GoldIVRQualityFailTotal.Inc()
		metrics.GoldIVRJobsFallbackGQLTotal.Inc()
		g.log.Warn("gold ivr import failed",
			"stream_id", streamID, "login", login, "err", importErr)
		return out
	}
	if stats.messages == 0 {
		out.Reason = "zero_messages"
		metrics.GoldIVRJobsFallbackGQLTotal.Inc()
		return out
	}
	if err := g.qualityCheck(stats); err != nil {
		out.Reason = err.Error()
		metrics.GoldIVRQualityFailTotal.Inc()
		metrics.GoldIVRJobsFailedTotal.Inc()
		metrics.GoldIVRJobsFallbackGQLTotal.Inc()
		return out
	}

	rollups := stats.minuteRollups(windowStart)
	shadowScore, shadowRec := shadowCompareRollups(existing, rollups)
	out.Messages = stats.messages
	out.MinutesWritten = len(rollups)
	out.ShadowScorePct = shadowScore
	out.ShadowRecommendation = shadowRec

	if g.cfg.ShadowMode {
		metrics.GoldIVRShadowJobsTotal.Inc()
		metrics.GoldIVRShadowMessagesTotal.Add(float64(stats.messages))
		metrics.GoldIVRShadowDuplicateAdjustedScore.Observe(shadowScore)
		metrics.GoldIVRShadowRecommendationTotal.WithLabelValues(shadowRec).Inc()
		metrics.GoldIVRShadowSuccessTotal.Inc()
		out.ShadowOnly = true
		out.Reason = "ivr_shadow_complete"
		priority := gqlPriorityFromIVRRollups(rollups, stats.messages)
		peakRollups := selectIVRPeakRollups(rollups, g.cfg.PeaksOnlyMaxMinutes, g.cfg.PeaksOnlyMinChatCount)
		artifact := GoldIVRShadowArtifact{
			StreamID:                  streamID,
			VodID:                     strings.TrimSpace(stream.VodID),
			ChannelID:                 channelID,
			Login:                     login,
			WindowStart:               windowStart,
			WindowEnd:                 windowEnd,
			IVRMessageCount:           stats.messages,
			ExistingRollupMinutes:     countActiveRollupMinutes(existing),
			RawSuitabilityPct:         shadowScore,
			DedupedSuitabilityPct:     shadowScore,
			AdjustedSuitabilityPct:    shadowScore,
			PeakOverlapTop3Pct:        peakOverlapTopN(rollups, existing, 3),
			ShapeSimilarityPct:        shadowScore,
			Recommendation:            shadowRec,
			GQLPriorityRecommendation: priority,
			PeakMinuteTimestamps:      peakMinuteTimestamps(peakRollups),
			WroteRollups:              false,
			UpdatedStreamMetadata:     false,
			ShadowOnly:                true,
			Success:                   true,
			CreatedAt:                 time.Now().UTC(),
		}
		artifactDir := resolveGoldIVRShadowArtifactDir(g.cfg)
		if path, err := writeGoldIVRShadowArtifact(artifactDir, artifact); err != nil {
			g.log.Warn("gold ivr shadow artifact write failed", "stream_id", streamID, "err", err)
		} else {
			out.ShadowArtifactPath = path
			artifact.ArtifactPath = path
			_ = pruneGoldIVRShadowArtifacts(artifactDir, g.cfg.ShadowArtifactRetentionDays, g.cfg.ShadowArtifactMaxFiles)
		}
		g.log.Info("gold ivr shadow run complete",
			"stream_id", streamID,
			"login", login,
			"channel_id", channelID,
			"allowlist", allowReason,
			"messages", stats.messages,
			"minutes", len(rollups),
			"shadow_score_pct", shadowScore,
			"shadow_recommendation", shadowRec,
			"gql_priority_recommendation", priority,
			"shadow_artifact_path", out.ShadowArtifactPath,
		)
		return out
	}

	if g.cfg.PeaksOnlyEnabled {
		peakRollups := selectIVRPeakRollups(rollups, g.cfg.PeaksOnlyMaxMinutes, g.cfg.PeaksOnlyMinChatCount)
		if len(peakRollups) == 0 {
			out.Reason = "zero_peak_minutes"
			metrics.GoldIVRJobsFallbackGQLTotal.Inc()
			return out
		}
		if err := g.store.BulkUpsertProvisionalIVRChatRollups(ctx, streamID, peakRollups); err != nil {
			out.Reason = "peaks_rollup_write_failed"
			metrics.GoldIVRJobsFailedTotal.Inc()
			metrics.GoldIVRJobsFallbackGQLTotal.Inc()
			return out
		}
		ivrPct := coveragePctForMinutes(len(peakRollups), windowStart, windowEnd)
		meta := StreamChatSourceMetadata{
			ChatState:        ChatStateIVRLite,
			ChatSource:       ChatSourceIVR,
			SourceConfidence: SourceConfidenceProvisional,
			ChatCoveragePct:  ivrPct + livePct,
			IVRCoveragePct:   ivrPct,
			LiveCoveragePct:  livePct,
			ChatSourceDetail: "ivr_peaks_only",
		}
		if b, err := json.Marshal(map[string]any{
			"peak_minutes": peakMinuteTimestamps(peakRollups),
			"peak_count":   len(peakRollups),
		}); err == nil {
			meta.SourceWindows = b
		}
		if err := g.store.UpsertStreamChatSourceMetadata(ctx, streamID, meta); err != nil {
			g.log.Warn("gold ivr peaks metadata write failed", "stream_id", streamID, "err", err)
		}
		metrics.GoldIVRJobsSuccessTotal.Inc()
		metrics.GoldIVRMessagesImportedTotal.Add(float64(stats.messages))
		metrics.GoldIVRRollupMinutesWrittenTotal.Add(float64(len(peakRollups)))
		metrics.GoldSourceStateTotal.WithLabelValues(ChatStateIVRLite).Inc()
		metrics.GoldChatSourceTotal.WithLabelValues(ChatSourceIVR).Inc()
		out.Imported = true
		out.PeaksOnly = true
		out.FallbackGQL = true
		out.Messages = stats.messages
		out.MinutesWritten = len(peakRollups)
		out.Reason = "ivr_peaks_only_provisional"
		g.log.Info("gold ivr peaks-only import complete",
			"stream_id", streamID,
			"login", login,
			"peak_minutes", len(peakRollups),
			"messages", stats.messages,
		)
		return out
	}

	if !g.cfg.LiteEnabled {
		out.Reason = "ivr_lite_disabled"
		return out
	}

	if err := g.store.BulkUpsertProvisionalIVRChatRollups(ctx, streamID, rollups); err != nil {
		out.Reason = "rollup_write_failed"
		metrics.GoldIVRJobsFailedTotal.Inc()
		metrics.GoldIVRJobsFallbackGQLTotal.Inc()
		return out
	}

	ivrPct := coveragePctForMinutes(len(rollups), windowStart, windowEnd)
	chatState := deriveStreamChatStateFromRollups(existing, livePct, ivrPct, 0, false)
	chatSource, confidence := deriveStreamChatSource(livePct, ivrPct, 0)
	meta := StreamChatSourceMetadata{
		ChatState:        chatState,
		ChatSource:       chatSource,
		SourceConfidence: confidence,
		ChatCoveragePct:  ivrPct + livePct,
		IVRCoveragePct:   ivrPct,
		LiveCoveragePct:  livePct,
		ChatSourceDetail: "ivr_ndjson",
	}
	if b, err := json.Marshal(stats.windows); err == nil {
		meta.SourceWindows = b
	}
	if err := g.store.UpsertStreamChatSourceMetadata(ctx, streamID, meta); err != nil {
		g.log.Warn("gold ivr metadata write failed", "stream_id", streamID, "err", err)
	}

	metrics.GoldIVRJobsSuccessTotal.Inc()
	metrics.GoldIVRMessagesImportedTotal.Add(float64(stats.messages))
	metrics.GoldIVRBytesImportedTotal.Add(float64(stats.bytesRead))
	metrics.GoldIVRParserErrorsTotal.Add(float64(stats.parserErrors))
	metrics.GoldIVRRollupMinutesWrittenTotal.Add(float64(len(rollups)))
	metrics.GoldSourceStateTotal.WithLabelValues(chatState).Inc()
	metrics.GoldChatSourceTotal.WithLabelValues(chatSource).Inc()

	out.Imported = true
	out.FallbackGQL = true // GQL canonical job still required
	out.Messages = stats.messages
	out.MinutesWritten = len(rollups)
	out.Reason = "ivr_lite_provisional"
	g.log.Info("gold ivr lite import complete",
		"stream_id", streamID,
		"login", login,
		"channel_id", channelID,
		"allowlist", allowReason,
		"messages", stats.messages,
		"minutes", len(rollups),
		"chat_state", chatState,
		"live_pct", livePct,
		"ivr_pct", ivrPct,
	)
	return out
}

func (g *GoldIVRService) preflightCoverage(ctx context.Context, channelID string, day time.Time) (bool, string) {
	metrics.GoldIVRPreflightTotal.Inc()
	cacheKey := channelID + ":" + day.Format("2006-01-02")
	g.preflightMu.Lock()
	if entry, ok := g.preflightCache[cacheKey]; ok && time.Now().Before(entry.expiresAt) {
		g.preflightMu.Unlock()
		metrics.GoldIVRCoverageCacheHitTotal.Inc()
		if entry.hit {
			metrics.GoldIVRPreflightHitTotal.Inc()
			return true, "ivr_coverage_hit_cached"
		}
		metrics.GoldIVRPreflightMissTotal.Inc()
		return false, "ivr_coverage_miss_cached"
	}
	g.preflightMu.Unlock()

	listURL := g.cfg.BaseURL + "/list?" + url.Values{"channelid": {channelID}}.Encode()
	body, status, err := g.httpGet(ctx, listURL)
	if err != nil {
		metrics.GoldIVRPreflightErrorTotal.Inc()
		return false, "ivr_preflight_error"
	}
	hit := status == http.StatusOK && len(body) > 2 && !strings.Contains(strings.ToLower(string(body)), "not found")
	g.preflightMu.Lock()
	g.preflightCache[cacheKey] = ivrPreflightCacheEntry{hit: hit, expiresAt: time.Now().Add(6 * time.Hour)}
	g.preflightMu.Unlock()
	if hit {
		metrics.GoldIVRPreflightHitTotal.Inc()
		return true, "ivr_coverage_hit"
	}
	metrics.GoldIVRPreflightMissTotal.Inc()
	return false, "ivr_coverage_miss"
}

func (g *GoldIVRService) importWindow(
	ctx context.Context,
	channelID string,
	from, to time.Time,
	existing []MinuteRollup,
) (*ivrImportStats, error) {
	covered := existingChatMinutes(existing)
	ndjsonURL := fmt.Sprintf("%s/channelid/%s?%s",
		strings.TrimRight(g.cfg.BaseURL, "/"),
		url.PathEscape(channelID),
		url.Values{
			"from":   {from.Format(time.RFC3339)},
			"to":     {to.Format(time.RFC3339)},
			"ndjson": {"1"},
		}.Encode(),
	)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, ndjsonURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/x-ndjson, application/json")
	req.Header.Set("User-Agent", "StreamcloneGoldIVR/1.0")

	resp, err := g.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, errors.New("ivr_ndjson_not_found")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("ivr_ndjson_status_%d", resp.StatusCode)
	}

	stats := &ivrImportStats{
		windows: []ChatSourceWindow{{
			Source:     ChatSourceIVR,
			Confidence: SourceConfidenceProvisional,
			Start:      from,
			End:        to,
		}},
		minuteBuckets: map[time.Time]int{},
		emoteBuckets:  map[time.Time]map[string]int{},
	}

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		stats.bytesRead += int64(len(line) + 1)
		if stats.bytesRead > g.cfg.MaxBytesPerJob {
			return stats, errors.New("ivr_byte_cap_exceeded")
		}
		if stats.messages >= g.cfg.MaxMessagesPerJob {
			return stats, errors.New("ivr_message_cap_exceeded")
		}
		if len(line) == 0 {
			continue
		}
		var raw ivrRawMessage
		if err := json.Unmarshal(line, &raw); err != nil {
			stats.parserErrors++
			continue
		}
		ts, err := time.Parse(time.RFC3339, strings.Replace(raw.Timestamp, "Z", "+00:00", 1))
		if err != nil {
			stats.parserErrors++
			continue
		}
		ts = ts.UTC()
		if ts.Before(from) || ts.After(to) || ts.After(time.Now().UTC().Add(time.Minute)) {
			stats.parserErrors++
			continue
		}
		minute := ts.Truncate(time.Minute)
		if covered[minute] {
			continue
		}
		stats.messages++
		stats.minuteBuckets[minute]++
		if text := strings.TrimSpace(raw.Text); text != "" {
			if stats.emoteBuckets[minute] == nil {
				stats.emoteBuckets[minute] = map[string]int{}
			}
			for _, token := range strings.Fields(text) {
				if strings.HasPrefix(token, ":") || strings.Contains(token, ":") {
					stats.emoteBuckets[minute][token]++
				}
			}
		}
	}
	if err := scanner.Err(); err != nil && !errors.Is(err, io.EOF) {
		return stats, err
	}
	stats.windows[0].MessageCount = stats.messages
	stats.windows[0].ParserErrors = stats.parserErrors
	return stats, nil
}

func (g *GoldIVRService) qualityCheck(stats *ivrImportStats) error {
	total := stats.messages + stats.parserErrors
	if total == 0 {
		return errors.New("ivr_quality_no_messages")
	}
	rate := float64(stats.parserErrors) / float64(total)
	if rate > g.cfg.ParserErrorRateMax {
		return fmt.Errorf("ivr_quality_parser_rate_%.4f", rate)
	}
	if stats.messages < g.cfg.MinMessagesPerWindow {
		return errors.New("ivr_quality_zero_active_window")
	}
	return nil
}

func (g *GoldIVRService) httpGet(ctx context.Context, rawURL string) ([]byte, int, error) {
	var lastErr error
	for attempt := 0; attempt <= g.cfg.MaxRetries; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
		if err != nil {
			return nil, 0, err
		}
		req.Header.Set("User-Agent", "StreamcloneGoldIVR/1.0")
		resp, err := g.client.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		resp.Body.Close()
		if readErr != nil {
			lastErr = readErr
			continue
		}
		return body, resp.StatusCode, nil
	}
	return nil, 0, lastErr
}

func existingChatMinutes(rollups []MinuteRollup) map[time.Time]bool {
	out := map[time.Time]bool{}
	for _, r := range rollups {
		if r.ChatCount > 0 {
			out[r.MinuteTS.UTC().Truncate(time.Minute)] = true
		}
	}
	return out
}

func estimateLiveCoveragePct(rollups []MinuteRollup, start, end time.Time) float64 {
	if end.Before(start) {
		return 0
	}
	span := int(end.Sub(start).Minutes())
	if span <= 0 {
		span = 1
	}
	liveMinutes := 0
	for _, r := range rollups {
		if isLiveChatRollup(r) {
			liveMinutes++
		}
	}
	return float64(liveMinutes) / float64(span) * 100
}

func coveragePctForMinutes(minutes int, start, end time.Time) float64 {
	span := int(end.Sub(start).Minutes())
	if span <= 0 {
		span = 1
	}
	return float64(minutes) / float64(span) * 100
}

// MarkStreamGQLCanonical upgrades stream metadata after canonical GQL completes.
func MarkStreamGQLCanonical(ctx context.Context, store *Store, streamID string, coveragePct float64) error {
	if store == nil || streamID == "" {
		return nil
	}
	meta := StreamChatSourceMetadata{
		ChatState:        ChatStateGQLGold,
		ChatSource:       ChatSourceGQL,
		SourceConfidence: SourceConfidenceCanonical,
		ChatCoveragePct:  coveragePct,
		GQLCoveragePct:   coveragePct,
		ChatSourceDetail: "gql_vod_comments",
	}
	now := time.Now().UTC()
	meta.LastSourceUpgrade = &now
	metrics.GoldSourceStateTotal.WithLabelValues(ChatStateGQLGold).Inc()
	metrics.GoldChatSourceTotal.WithLabelValues(ChatSourceGQL).Inc()
	return store.UpsertStreamChatSourceMetadata(ctx, streamID, meta)
}

// shadowCompareRollups scores IVR provisional minutes against existing rollups without writing.
func shadowCompareRollups(existing, ivr []MinuteRollup) (scorePct float64, recommendation string) {
	baseline := map[int64]int{}
	for _, r := range existing {
		if r.ChatCount <= 0 {
			continue
		}
		key := r.MinuteTS.UTC().Truncate(time.Minute).Unix()
		if isGQLCanonicalRollup(r) {
			baseline[key] = r.ChatCount
		}
	}
	if len(baseline) == 0 {
		for _, r := range existing {
			if r.ChatCount <= 0 {
				continue
			}
			key := r.MinuteTS.UTC().Truncate(time.Minute).Unix()
			baseline[key] = r.ChatCount
		}
	}
	var agreements []float64
	ivrTotal, baselineTotal := 0, 0
	for _, r := range ivr {
		ivrTotal += r.ChatCount
		key := r.MinuteTS.UTC().Truncate(time.Minute).Unix()
		base := baseline[key]
		baselineTotal += base
		if base == 0 && r.ChatCount == 0 {
			continue
		}
		denom := base
		if r.ChatCount > denom {
			denom = r.ChatCount
		}
		if denom <= 0 {
			denom = 1
		}
		agreements = append(agreements, float64(minInt(base, r.ChatCount))/float64(denom))
	}
	if len(agreements) == 0 {
		return 0, "shadow_no_overlap"
	}
	scorePct = 0
	for _, a := range agreements {
		scorePct += a
	}
	scorePct = scorePct / float64(len(agreements)) * 100
	switch {
	case scorePct >= 95:
		recommendation = "experimental_lite_chat_peaks"
	case scorePct >= 85:
		recommendation = "shadow_only_or_peaks_only"
	case scorePct >= 70:
		recommendation = "hold"
	default:
		recommendation = "reject_lite_for_now"
	}
	_ = ivrTotal
	_ = baselineTotal
	return scorePct, recommendation
}

func countActiveRollupMinutes(rollups []MinuteRollup) int {
	n := 0
	for _, r := range rollups {
		if r.ChatCount > 0 {
			n++
		}
	}
	return n
}

// ReconcileShadowAfterGQL compares a prior shadow IVR import to post-GQL canonical rollups.
// It re-fetches IVR read-only and writes a reconciliation artifact; no DB rollup/metadata writes.
func (g *GoldIVRService) ReconcileShadowAfterGQL(ctx context.Context, streamID, login string, shadow GoldIVRAttemptResult) (string, error) {
	if g == nil || g.store == nil || !shadow.ShadowOnly {
		return "", nil
	}
	stream, err := g.store.StreamByID(ctx, streamID)
	if err != nil || stream == nil {
		return "", err
	}
	windowStart := stream.StartedAt.UTC()
	windowEnd := time.Now().UTC()
	if stream.EndedAt != nil {
		windowEnd = stream.EndedAt.UTC()
	}
	maxDur := time.Duration(g.cfg.MaxDurationMinutes) * time.Minute
	if windowEnd.Sub(windowStart) > maxDur {
		windowEnd = windowStart.Add(maxDur)
	}
	channelID := strings.TrimSpace(stream.BroadcasterID)
	existing, err := g.store.RollupsByStream(ctx, streamID)
	if err != nil {
		return "", err
	}
	// Re-fetch IVR without skipping minutes that GQL already covered — otherwise
	// reconcile silently no-ops after canonical rollups land.
	stats, importErr := g.importWindow(ctx, channelID, windowStart, windowEnd, nil)
	if importErr != nil {
		return "", importErr
	}
	if stats.messages == 0 {
		g.log.Warn("gold ivr shadow reconcile skipped: zero ivr messages after gql",
			"stream_id", streamID, "login", login)
		return "", nil
	}
	ivrRollups := stats.minuteRollups(windowStart)
	var gqlCanonical []MinuteRollup
	for _, r := range existing {
		if isGQLCanonicalRollup(r) && r.ChatCount > 0 {
			gqlCanonical = append(gqlCanonical, r)
		}
	}
	if len(gqlCanonical) == 0 {
		return "", nil
	}
	score := reconciliationScorePct(ivrRollups, gqlCanonical)
	peak3 := peakOverlapTopN(ivrRollups, gqlCanonical, 3)
	peak5 := peakOverlapTopN(ivrRollups, gqlCanonical, 5)
	rec := reconciliationRecommendation(score, peak3)
	priority := gqlPriorityFromIVRRollups(ivrRollups, stats.messages)
	peakOverlapPass := peak3 >= 66
	artifact := GoldIVRReconciliationArtifact{
		StreamID:                     streamID,
		VodID:                        strings.TrimSpace(stream.VodID),
		Login:                        login,
		ShadowArtifactPath:           shadow.ShadowArtifactPath,
		ShadowRecommendation:         shadow.ShadowRecommendation,
		ShadowScorePct:               shadow.ShadowScorePct,
		GQLCanonicalMinutes:          len(gqlCanonical),
		GQLPeakOverlapTop3Pct:        peak3,
		GQLPeakOverlapTop5Pct:        peak5,
		ReconciliationScorePct:       score,
		ReconciliationRecommendation: rec,
		GQLPriorityRecommendation:    priority,
		GQLCanonicalPresent:          len(gqlCanonical) > 0,
		PeakOverlapPass:              peakOverlapPass,
		WroteRollups:                 false,
		UpdatedStreamMetadata:        false,
		CreatedAt:                    time.Now().UTC(),
	}
	path, err := writeGoldIVRReconciliationArtifact(resolveGoldIVRShadowArtifactDir(g.cfg), artifact)
	if err != nil {
		if g.log != nil {
			g.log.Warn("gold ivr reconciliation artifact write failed (best-effort)",
				"stream_id", streamID, "login", login, "err", err)
		}
		return "", nil
	}
	artifact.ArtifactPath = path
	g.log.Info("gold ivr shadow reconciled after gql",
		"stream_id", streamID,
		"login", login,
		"reconciliation_score_pct", score,
		"reconciliation_recommendation", rec,
		"gql_priority_recommendation", priority,
		"artifact_path", path,
		"shadow_artifact_path", shadow.ShadowArtifactPath,
	)
	return path, nil
}
