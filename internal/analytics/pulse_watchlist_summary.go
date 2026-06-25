package analytics

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	pulseSummaryWatchlistCap    = 10
	pulseSummaryArrayCap        = 3
	pulseSummaryCacheTTL        = 30 * time.Second
	pulseSummaryRateLimitPerMin = 30 // per principal; hosted summary polling default
	pulseSummaryCacheKeyPrefix  = "sp:pulse:summary:"
	pulseSummaryRLKeyPrefix     = "sp:rl:summary:"
)

var errSummaryUnavailable = errors.New("summary_unavailable")

// PulseWatchlistSummary is the BEACON watchlist aggregate (requirements §12).
type PulseWatchlistSummary struct {
	LiveCount       int                              `json:"liveCount"`
	RecapReadyCount int                              `json:"recapReadyCount"`
	AttentionCount  int                              `json:"attentionCount"`
	ProtectedCount  int                              `json:"protectedCount"`
	CurrentChannel  *PulseWatchlistSummaryChannel    `json:"currentChannel,omitempty"`
	LiveNow         []PulseWatchlistSummaryLiveNow   `json:"liveNow"`
	Recaps          []PulseWatchlistSummaryRecap     `json:"recaps"`
	Moments         []PulseWatchlistSummaryMoment    `json:"moments"`
	Attention       []PulseWatchlistSummaryAttention `json:"attention"`
}

type PulseWatchlistSummaryChannel struct {
	Login         string `json:"login"`
	DisplayName   string `json:"displayName"`
	IsLive        bool   `json:"isLive"`
	Protected     bool   `json:"protected"`
	CoverageState string `json:"coverageState,omitempty"`
	Action        string `json:"action,omitempty"`
}

type PulseWatchlistSummaryLiveNow struct {
	Login       string `json:"login"`
	DisplayName string `json:"displayName"`
	AvatarURL   string `json:"avatarUrl,omitempty"`
	Category    string `json:"category,omitempty"`
	ViewerCount *int   `json:"viewerCount,omitempty"`
	HeatTier    string `json:"heatTier"`
	SignalType  string `json:"signalType,omitempty"`
	Confidence  string `json:"confidence,omitempty"`
}

type PulseWatchlistSummaryRecap struct {
	StreamID        string `json:"streamId"`
	Login           string `json:"login"`
	Title           string `json:"title"`
	EndedAt         string `json:"endedAt"`
	DurationSeconds *int   `json:"durationSeconds,omitempty"`
	Status          string `json:"status"`
}

type PulseWatchlistSummaryMoment struct {
	ID             string `json:"id"`
	Login          string `json:"login"`
	StreamID       string `json:"streamId"`
	OffsetSeconds  *int   `json:"offsetSeconds,omitempty"`
	Title          string `json:"title"`
	SignalType     string `json:"signalType,omitempty"`
	Confidence     string `json:"confidence,omitempty"`
	Saved          bool   `json:"saved"`
}

type PulseWatchlistSummaryAttention struct {
	Kind    string `json:"kind"`
	Login   string `json:"login"`
	Message string `json:"message"`
	Action  string `json:"action,omitempty"`
}

type watchlistSummaryStore interface {
	ListPulseWatchlist(ctx context.Context, principalID string) ([]PulseWatchlistEntry, error)
	LatestStreamsByLogins(ctx context.Context, logins []string) (map[string]*StreamRecord, error)
	ListPulseBookmarks(ctx context.Context, filter ListPulseBookmarksFilter) ([]PulseBookmark, string, error)
}

type watchlistSummaryContext struct {
	isTracked       func(string) bool
	backfillRunning func(string) bool
	backfillFailed  func(string) bool
}

func (c watchlistSummaryContext) withDefaults() watchlistSummaryContext {
	if c.isTracked == nil {
		c.isTracked = func(string) bool { return false }
	}
	if c.backfillRunning == nil {
		c.backfillRunning = func(string) bool { return false }
	}
	if c.backfillFailed == nil {
		c.backfillFailed = func(string) bool { return false }
	}
	return c
}

func (h *Handler) getPulseWatchlistSummary(w http.ResponseWriter, r *http.Request) {
	if !h.enforceSummaryRateLimit(w, r) {
		return
	}

	principal, ok := pulsePrincipalFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}

	loginHint := strings.TrimSpace(r.URL.Query().Get("login"))
	if loginHint != "" {
		if _, ok := validLogin(loginHint); !ok {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_login"})
			return
		}
	}

	cacheKey := pulseSummaryCacheKey(principal.ID, loginHint)
	if payload, hit := h.loadPulseWatchlistSummaryCache(r.Context(), cacheKey); hit {
		w.Header().Set("X-Cache", "HIT")
		writeJSON(w, http.StatusOK, payload)
		return
	}

	store := h.watchlistSummaryStore()
	if store == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "summary_unavailable"})
		return
	}

	payload, err := h.buildPulseWatchlistSummary(r.Context(), store, principal.ID, loginHint, h.watchlistSummaryContext())
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "summary_unavailable"})
		return
	}

	h.savePulseWatchlistSummaryCache(r.Context(), cacheKey, payload)
	w.Header().Set("X-Cache", "MISS")
	writeJSON(w, http.StatusOK, payload)
}

func (h *Handler) watchlistSummaryStore() watchlistSummaryStore {
	if h == nil || h.store == nil {
		return nil
	}
	return h.store
}

func (h *Handler) watchlistSummaryContext() watchlistSummaryContext {
	ctx := watchlistSummaryContext{
		isTracked: func(string) bool { return false },
		backfillRunning: func(string) bool { return false },
		backfillFailed:  func(string) bool { return false },
	}
	if h != nil {
		if h.collector != nil {
			ctx.isTracked = h.isLoginTracked
		}
		if h.pulseBackfill != nil {
			ctx.backfillRunning = func(streamID string) bool {
				job := h.pulseBackfill.ActiveJobForStream(streamID)
				return job != nil && !isPulseBackfillTerminal(job.Status)
			}
			ctx.backfillFailed = h.pulseBackfill.BackfillFailedForStream
		}
	}
	return ctx
}

func (h *Handler) buildPulseWatchlistSummary(
	ctx context.Context,
	store watchlistSummaryStore,
	principalID string,
	loginHint string,
	summaryCtx watchlistSummaryContext,
) (PulseWatchlistSummary, error) {
	items, err := store.ListPulseWatchlist(ctx, principalID)
	if err != nil {
		return PulseWatchlistSummary{}, errSummaryUnavailable
	}

	protectedCount := 0
	watchlistByLogin := make(map[string]PulseWatchlistEntry, len(items))
	for _, item := range items {
		watchlistByLogin[item.Login] = item
		if item.AlwaysTrack {
			protectedCount++
		}
	}

	processed := capWatchlistEntries(items, pulseSummaryWatchlistCap)
	processedLogins := make([]string, 0, len(processed))
	for _, item := range processed {
		processedLogins = append(processedLogins, item.Login)
	}

	streams, _ := store.LatestStreamsByLogins(ctx, processedLogins)
	if streams == nil {
		streams = map[string]*StreamRecord{}
	}

	bookmarks, _, _ := store.ListPulseBookmarks(ctx, ListPulseBookmarksFilter{
		PrincipalID: principalID,
		Limit:       pulseSummaryArrayCap,
	})

	summary := assemblePulseWatchlistSummary(assembleWatchlistSummaryInput{
		Processed:        processed,
		WatchlistByLogin: watchlistByLogin,
		Streams:          streams,
		Bookmarks:        bookmarks,
		LoginHint:        loginHint,
		ProtectedCount:   protectedCount,
		SummaryCtx:       summaryCtx.withDefaults(),
	})
	return summary, nil
}

type assembleWatchlistSummaryInput struct {
	Processed        []PulseWatchlistEntry
	WatchlistByLogin map[string]PulseWatchlistEntry
	Streams          map[string]*StreamRecord
	Bookmarks        []PulseBookmark
	LoginHint        string
	ProtectedCount   int
	SummaryCtx       watchlistSummaryContext
}

func assemblePulseWatchlistSummary(in assembleWatchlistSummaryInput) PulseWatchlistSummary {
	liveNow := make([]PulseWatchlistSummaryLiveNow, 0, pulseSummaryArrayCap)
	recaps := make([]PulseWatchlistSummaryRecap, 0, pulseSummaryArrayCap)
	liveCount := 0
	recapReadyCount := 0

	for _, item := range in.Processed {
		stream := in.Streams[item.Login]
		if stream != nil && stream.EndedAt == nil {
			liveCount++
			if len(liveNow) < pulseSummaryArrayCap {
				liveNow = append(liveNow, liveNowRowFromStream(stream))
			}
		}
	}

	endedCandidates := make([]PulseWatchlistSummaryRecap, 0, len(in.Processed))
	for _, item := range in.Processed {
		stream := in.Streams[item.Login]
		if stream == nil || stream.EndedAt == nil {
			continue
		}
		recap := recapRowFromStream(stream)
		endedCandidates = append(endedCandidates, recap)
		if recap.Status == "ready" {
			recapReadyCount++
		}
	}
	sort.Slice(endedCandidates, func(i, j int) bool {
		return endedCandidates[i].EndedAt > endedCandidates[j].EndedAt
	})
	if len(endedCandidates) > pulseSummaryArrayCap {
		endedCandidates = endedCandidates[:pulseSummaryArrayCap]
	}
	recaps = append(recaps, endedCandidates...)

	moments := make([]PulseWatchlistSummaryMoment, 0, pulseSummaryArrayCap)
	for _, bm := range in.Bookmarks {
		if len(moments) >= pulseSummaryArrayCap {
			break
		}
		moments = append(moments, momentRowFromBookmark(bm))
	}

	attentionAll := buildSummaryAttention(in.Processed, in.Streams, in.SummaryCtx)
	attentionCount := len(attentionAll)
	attention := attentionAll
	if len(attention) > pulseSummaryArrayCap {
		attention = attention[:pulseSummaryArrayCap]
	}

	out := PulseWatchlistSummary{
		LiveCount:       liveCount,
		RecapReadyCount: recapReadyCount,
		AttentionCount:  attentionCount,
		ProtectedCount:  in.ProtectedCount,
		LiveNow:         liveNow,
		Recaps:          recaps,
		Moments:         moments,
		Attention:       attention,
	}
	if out.LiveNow == nil {
		out.LiveNow = []PulseWatchlistSummaryLiveNow{}
	}
	if out.Recaps == nil {
		out.Recaps = []PulseWatchlistSummaryRecap{}
	}
	if out.Moments == nil {
		out.Moments = []PulseWatchlistSummaryMoment{}
	}
	if out.Attention == nil {
		out.Attention = []PulseWatchlistSummaryAttention{}
	}

	if in.LoginHint != "" {
		if login, ok := validLogin(in.LoginHint); ok {
			entry, onWatchlist := in.WatchlistByLogin[login]
			protected := onWatchlist && entry.AlwaysTrack
			stream := in.Streams[login]
			tracked := in.SummaryCtx.isTracked(login)
			backfillRunning := false
			backfillFailed := false
			if stream != nil {
				backfillRunning = in.SummaryCtx.backfillRunning(stream.StreamID)
				backfillFailed = in.SummaryCtx.backfillFailed(stream.StreamID)
			}
			out.CurrentChannel = buildSummaryCurrentChannel(login, stream, protected, tracked, backfillRunning, backfillFailed)
		}
	}
	return out
}

func capWatchlistEntries(items []PulseWatchlistEntry, cap int) []PulseWatchlistEntry {
	if cap <= 0 || len(items) <= cap {
		return items
	}
	return items[:cap]
}

func liveNowRowFromStream(stream *StreamRecord) PulseWatchlistSummaryLiveNow {
	displayName := stream.DisplayName
	if displayName == "" {
		displayName = stream.Login
	}
	row := PulseWatchlistSummaryLiveNow{
		Login:       stream.Login,
		DisplayName: displayName,
		HeatTier:    "unknown",
		SignalType:  "unknown",
		Confidence:  "unknown",
	}
	if stream.ProfileImageURL != "" {
		row.AvatarURL = stream.ProfileImageURL
	}
	if stream.Category != "" {
		row.Category = stream.Category
	}
	if stream.CurrentViewers > 0 {
		v := stream.CurrentViewers
		row.ViewerCount = &v
	}
	return row
}

func recapRowFromStream(stream *StreamRecord) PulseWatchlistSummaryRecap {
	status := "pending"
	if stream.ChatMessages > 0 || stream.ViewerSamples > 0 {
		status = "ready"
	}
	title := stream.Title
	if title == "" {
		title = stream.Login + " stream"
	}
	endedAt := ""
	if stream.EndedAt != nil {
		endedAt = stream.EndedAt.UTC().Format(time.RFC3339)
	}
	recap := PulseWatchlistSummaryRecap{
		StreamID: stream.StreamID,
		Login:    stream.Login,
		Title:    title,
		EndedAt:  endedAt,
		Status:   status,
	}
	if stream.EndedAt != nil && !stream.StartedAt.IsZero() {
		dur := int(stream.EndedAt.Sub(stream.StartedAt).Seconds())
		if dur > 0 {
			recap.DurationSeconds = &dur
		}
	}
	return recap
}

func momentRowFromBookmark(bm PulseBookmark) PulseWatchlistSummaryMoment {
	title := bm.Label
	if title == "" {
		title = "Saved moment"
	}
	streamID := ""
	if bm.StreamID != nil {
		streamID = *bm.StreamID
	}
	offset := bm.OffsetSeconds
	m := PulseWatchlistSummaryMoment{
		ID:         bm.ID,
		Login:      bm.Login,
		StreamID:   streamID,
		Title:      title,
		SignalType: "unknown",
		Confidence: "unknown",
		Saved:      true,
	}
	if offset > 0 {
		m.OffsetSeconds = &offset
	}
	return m
}

func buildSummaryAttention(
	entries []PulseWatchlistEntry,
	streams map[string]*StreamRecord,
	summaryCtx watchlistSummaryContext,
) []PulseWatchlistSummaryAttention {
	out := make([]PulseWatchlistSummaryAttention, 0, len(entries))
	for _, entry := range entries {
		stream := streams[entry.Login]
		tracked := summaryCtx.isTracked(entry.Login)
		isLive := stream != nil && stream.EndedAt == nil
		streamID := ""
		if stream != nil {
			streamID = stream.StreamID
		}
		backfillRunning := streamID != "" && summaryCtx.backfillRunning(streamID)
		backfillFailed := streamID != "" && summaryCtx.backfillFailed(streamID)

		switch {
		case entry.AlwaysTrack && !isLive && !tracked:
			out = append(out, PulseWatchlistSummaryAttention{
				Kind:    "protected_waiting",
				Login:   entry.Login,
				Message: "Protected — waiting for next stream",
				Action:  "open_channel",
			})
		case backfillFailed:
			out = append(out, PulseWatchlistSummaryAttention{
				Kind:    "backfill_failed",
				Login:   entry.Login,
				Message: "Could not load missed moments",
				Action:  "load_missed",
			})
		case backfillRunning:
			out = append(out, PulseWatchlistSummaryAttention{
				Kind:    "partial_coverage",
				Login:   entry.Login,
				Message: "Loading missed chat replay…",
				Action:  "load_missed",
			})
		case stream != nil && strings.TrimSpace(stream.VodID) != "" && !tracked && stream.ChatMessages == 0:
			out = append(out, PulseWatchlistSummaryAttention{
				Kind:    "vod_available",
				Login:   entry.Login,
				Message: "VOD available for missed moments",
				Action:  "load_missed",
			})
		case stream != nil && tracked && !stream.StartedAt.IsZero() && stream.ChatMessages == 0 && isLive:
			out = append(out, PulseWatchlistSummaryAttention{
				Kind:    "partial_coverage",
				Login:   entry.Login,
				Message: "Collecting live moments",
				Action:  "open_channel",
			})
		case !entry.AlwaysTrack && tracked:
			out = append(out, PulseWatchlistSummaryAttention{
				Kind:    "protect_recommended",
				Login:   entry.Login,
				Message: "Protect to capture future streams from the start",
				Action:  "protect",
			})
		}
	}
	return out
}

func buildSummaryCurrentChannel(
	login string,
	stream *StreamRecord,
	protected bool,
	tracked bool,
	backfillRunning bool,
	backfillFailed bool,
) *PulseWatchlistSummaryChannel {
	displayName := login
	isLive := false
	if stream != nil {
		if stream.DisplayName != "" {
			displayName = stream.DisplayName
		}
		isLive = stream.EndedAt == nil
	}
	cov := summaryCoverageState(stream, tracked, backfillRunning, backfillFailed)
	ch := &PulseWatchlistSummaryChannel{
		Login:         login,
		DisplayName:   displayName,
		IsLive:        isLive,
		Protected:     protected,
		CoverageState: cov,
	}
	return ch
}

func summaryCoverageState(stream *StreamRecord, tracked, backfillRunning, backfillFailed bool) string {
	switch {
	case backfillRunning:
		return CoverageStateBackfillRunning
	case backfillFailed:
		return CoverageStateBackfillFailed
	case stream == nil:
		if tracked {
			return CoverageStatePartialTracking
		}
		return CoverageStatePartialTracking
	case stream.EndedAt == nil && tracked:
		return CoverageStatePartialTracking
	case strings.TrimSpace(stream.VodID) != "" && stream.ChatMessages == 0:
		return CoverageStateWaitingForVOD
	case stream.EndedAt != nil && stream.ChatMessages > 0:
		return CoverageStateFullStreamTracked
	case stream.EndedAt != nil:
		return CoverageStatePartialTracking
	default:
		return CoverageStatePartialTracking
	}
}

func pulseSummaryCacheKey(principalID, loginHint string) string {
	key := pulseSummaryCacheKeyPrefix + principalID
	if login, ok := validLogin(loginHint); ok {
		key += ":" + login
	}
	return key
}

func (h *Handler) loadPulseWatchlistSummaryCache(ctx context.Context, key string) (PulseWatchlistSummary, bool) {
	if h == nil || h.rdb == nil {
		return PulseWatchlistSummary{}, false
	}
	cached, err := h.rdb.Get(ctx, key).Bytes()
	if err != nil || len(cached) == 0 {
		return PulseWatchlistSummary{}, false
	}
	var payload PulseWatchlistSummary
	if json.Unmarshal(cached, &payload) != nil {
		return PulseWatchlistSummary{}, false
	}
	return payload, true
}

func (h *Handler) savePulseWatchlistSummaryCache(ctx context.Context, key string, payload PulseWatchlistSummary) {
	if h == nil || h.rdb == nil {
		return
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return
	}
	_ = h.rdb.Set(ctx, key, body, pulseSummaryCacheTTL).Err()
}

func (h *Handler) enforceSummaryRateLimit(w http.ResponseWriter, r *http.Request) bool {
	if h.rateLimiter == nil || !h.pulseHosted.Hosted {
		return true
	}
	principal, ok := pulsePrincipalFromContext(r.Context())
	if !ok {
		return true
	}
	allowed, retryAfter := h.rateLimiter.AllowSummary(r.Context(), principal.ID)
	if allowed {
		return true
	}
	if retryAfter <= 0 {
		retryAfter = time.Minute
	}
	seconds := int(retryAfter.Seconds() + 0.999)
	w.Header().Set("Retry-After", strconv.Itoa(seconds))
	writeJSON(w, http.StatusTooManyRequests, map[string]any{
		"error":             "rate_limited",
		"hint":              "Summary limit exceeded; retry later",
		"scope":             "summary",
		"retryAfterSeconds": seconds,
	})
	return false
}
