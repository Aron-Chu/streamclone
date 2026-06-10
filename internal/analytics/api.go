package analytics

import (
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/redis/go-redis/v9"
)

var loginRe = regexp.MustCompile(`^[a-z0-9][a-z0-9_]{2,24}$`)

type Handler struct {
	store       *Store
	collector   *Collector
	helix       *HelixClient
	syncService *SyncService
}

func NewHandler(store *Store, collector *Collector, helix *HelixClient, syncService *SyncService) *Handler {
	return &Handler{store: store, collector: collector, helix: helix, syncService: syncService}
}

func (h *Handler) Routes(r chi.Router) {
	r.Route("/v1/analytics", func(r chi.Router) {
		r.Get("/always-tracked", h.getAlwaysTracked)
		r.Post("/always-tracked", h.setAlwaysTracked)
		r.Post("/channels/{login}/watch", h.watchChannel)
		r.Get("/channels/{login}/live", h.channelLive)
		r.Get("/channels/{login}/streams", h.channelStreams)
		r.Get("/streams/{streamID}", h.streamDetail)
		r.Post("/streams/{streamID}/sync", h.syncStream)
		r.Get("/streams/{streamID}/sync/status", h.syncStreamStatus)
		r.Get("/streams/{streamID}/games", h.getStreamGames)
	})
}

func (h *Handler) getAlwaysTracked(w http.ResponseWriter, r *http.Request) {
	logins := h.collector.GetAlwaysTracked()
	writeJSON(w, http.StatusOK, map[string][]string{"channels": logins})
}

func (h *Handler) setAlwaysTracked(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Channel string `json:"channel"`
		Track   bool   `json:"track"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_body"})
		return
	}
	login, ok := validLogin(req.Channel)
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_channel"})
		return
	}
	resp := h.collector.SetAlwaysTracked(r.Context(), login, req.Track)
	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) watchChannel(w http.ResponseWriter, r *http.Request) {
	login, ok := validLogin(chi.URLParam(r, "login"))
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_channel"})
		return
	}
	resp := h.collector.Watch(r.Context(), login)
	status := http.StatusOK
	if !resp.Tracking {
		status = http.StatusAccepted
	}
	writeJSON(w, status, resp)
}

func (h *Handler) channelLive(w http.ResponseWriter, r *http.Request) {
	login, ok := validLogin(chi.URLParam(r, "login"))
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_channel"})
		return
	}
	stream, err := h.store.LatestStreamByLogin(r.Context(), login)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeJSON(w, http.StatusOK, StreamDetailResponse{
				Channel:   login,
				State:     "not_collected",
				Rollups:   []MinuteRollup{},
				TopEmotes: []TopEmote{},
				Sources:   []SourceStatus{{Source: "analytics_db", State: "unavailable", Message: "No recent data"}},
				UpdatedAt: time.Now().UnixMilli(),
			})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	h.writeStreamDetail(w, r, stream, http.StatusOK)
}

func (h *Handler) channelStreams(w http.ResponseWriter, r *http.Request) {
	login, ok := validLogin(chi.URLParam(r, "login"))
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_channel"})
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	streams, err := h.store.StreamsByLogin(r.Context(), login, limit)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, StreamsResponse{
		Channel:   login,
		Items:     streams,
		Sources:   []SourceStatus{{Source: "analytics_db", State: "ready"}},
		UpdatedAt: time.Now().UnixMilli(),
	})
}

func (h *Handler) streamDetail(w http.ResponseWriter, r *http.Request) {
	streamID := strings.TrimSpace(chi.URLParam(r, "streamID"))
	if streamID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing_stream_id"})
		return
	}
	stream, err := h.store.StreamByID(r.Context(), streamID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			h.writeMissingStreamDetail(w, r, streamID)
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	h.writeStreamDetail(w, r, stream, http.StatusOK)
}

func (h *Handler) writeMissingStreamDetail(w http.ResponseWriter, r *http.Request, streamID string) {
	if h.syncService != nil {
		if status, statusErr := h.syncService.GetSyncStatus(r.Context(), streamID); statusErr == nil && status != nil && !status.Phase.IsTerminal() {
			writeJSON(w, http.StatusOK, StreamDetailResponse{
				Channel:   "",
				State:     "syncing",
				SyncPhase: string(status.Phase),
				Rollups:   []MinuteRollup{},
				TopEmotes: []TopEmote{},
				Sources:   []SourceStatus{{Source: "analytics_db", State: "syncing", Message: status.Message}},
				UpdatedAt: time.Now().UnixMilli(),
			})
			return
		}
	}
	channel := ""
	if login, ok := validLogin(r.URL.Query().Get("channel")); ok {
		channel = login
		upsertErr := h.store.UpsertStreamPlaceholder(r.Context(), streamID, "", login, "", time.Time{})
		if upsertErr == nil {
			if stream, err := h.store.StreamByID(r.Context(), streamID); err == nil {
				h.writeStreamDetail(w, r, stream, http.StatusOK)
				return
			}
		}
	}
	writeJSON(w, http.StatusOK, StreamDetailResponse{
		Channel:   channel,
		State:     "not_collected",
		Rollups:   []MinuteRollup{},
		TopEmotes: []TopEmote{},
		Sources:   []SourceStatus{{Source: "analytics_db", State: "unavailable", Message: "Stream not synced yet"}},
		UpdatedAt: time.Now().UnixMilli(),
	})
}

func (h *Handler) writeStreamDetail(w http.ResponseWriter, r *http.Request, stream *StreamRecord, status int) {
	sparse := r == nil || r.URL.Query().Get("sparse") != "false"
	rollups, err := h.store.RollupsByStream(r.Context(), stream.StreamID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	for i := range rollups {
		rollups[i] = normalizeRollup(rollups[i], 200)
	}
	topEmotes := TopEmotesFromRollups(rollups, 50)
	emoteKeys := make([]string, 0, len(topEmotes))
	for _, emote := range topEmotes {
		emoteKeys = append(emoteKeys, emote.Key)
	}
	state := "historical"
	if stream.EndedAt == nil {
		state = "live"
	}
	vodID := strings.TrimSpace(stream.VodID)
	broadcasterID := NormalizeBroadcasterID(stream.BroadcasterID)
	if broadcasterID == "" && h.helix != nil && h.helix.Enabled() && stream.Login != "" {
		broadcasterID = h.helix.ResolveBroadcasterID(r.Context(), stream.Login, "")
	}
	if vodID == "" && h.helix != nil && h.helix.Enabled() && broadcasterID != "" {
		if resolved, _ := h.helix.VideoIDByStreamID(r.Context(), broadcasterID, stream.StreamID); resolved != "" {
			vodID = resolved
			_ = h.store.SetStreamVodID(r.Context(), stream.StreamID, vodID, "helix_stream_match")
		}
	}
	var responseRollups []MinuteRollup
	if sparse {
		responseRollups = slimRollupsForChart(rollups, emoteKeys)
	} else {
		startAt, endAt := normalizeStreamWindow(stream.StartedAt, stream.EndedAt)
		responseRollups = fillMissingRollups(rollups, startAt, endAt)
	}
	vodDurationSec := 0
	if vodID != "" && h.helix != nil && h.helix.Enabled() {
		if d, err := h.helix.VideoDurationSeconds(r.Context(), vodID); err == nil {
			vodDurationSec = d
		}
	}
	chatCoverage := chatCoverageSummary(rollups, stream, vodDurationSec)

	responseState := state
	var syncPhase string
	if h.syncService != nil {
		if syncStatus, syncErr := h.syncService.GetSyncStatus(r.Context(), stream.StreamID); syncErr == nil && syncStatus != nil && !syncStatus.Phase.IsTerminal() {
			responseState = "syncing"
			syncPhase = string(syncStatus.Phase)
		}
	}

	writeJSON(w, status, StreamDetailResponse{
		Channel:         stream.Login,
		State:           responseState,
		SyncPhase:       syncPhase,
		Stream:          stream,
		Rollups:         responseRollups,
		TopEmotes:       topEmotes,
		Sources:         []SourceStatus{{Source: "analytics_db", State: "ready"}},
		UpdatedAt:       time.Now().UnixMilli(),
		VodID:           vodID,
		ChatCoveragePct: chatCoverage.CoveragePct,
		VodDurationSec:  vodDurationSec,
		ChatCoverage:    &chatCoverage,
	})
}

func validLogin(value string) (string, bool) {
	login := normalizeLogin(value)
	return login, loginRe.MatchString(login)
}

func normalizeLogin(login string) string {
	return strings.ToLower(strings.TrimSpace(login))
}

func minuteBucketKey(t time.Time) string {
	return t.UTC().Truncate(time.Minute).Format("2006-01-02T15:04:05Z07:00")
}

func mergeMinuteRollups(prev, item MinuteRollup) MinuteRollup {
	if item.ViewerAvg > prev.ViewerAvg {
		prev.ViewerAvg = item.ViewerAvg
	}
	if item.ViewerMax > prev.ViewerMax {
		prev.ViewerMax = item.ViewerMax
	}
	if item.ViewerLatest > prev.ViewerLatest {
		prev.ViewerLatest = item.ViewerLatest
	}
	if item.ViewerSamples > prev.ViewerSamples {
		prev.ViewerSamples = item.ViewerSamples
	}
	if item.ChatCount > prev.ChatCount {
		prev.ChatCount = item.ChatCount
	}
	if item.TotalEmoteCount > prev.TotalEmoteCount {
		prev.TotalEmoteCount = item.TotalEmoteCount
	}
	if item.SevenTVEmoteCount > prev.SevenTVEmoteCount {
		prev.SevenTVEmoteCount = item.SevenTVEmoteCount
	}
	if prev.Emotes == nil {
		prev.Emotes = map[string]int{}
	}
	for key, count := range item.Emotes {
		if count > prev.Emotes[key] {
			prev.Emotes[key] = count
		}
	}
	if !item.Missing {
		prev.Missing = false
	}
	return prev
}

func consolidateRollupsByMinute(in []MinuteRollup) map[string]MinuteRollup {
	out := make(map[string]MinuteRollup, len(in))
	for _, item := range in {
		bucket := item.MinuteTS.UTC().Truncate(time.Minute)
		key := minuteBucketKey(bucket)
		item.MinuteTS = bucket
		if item.Emotes == nil {
			item.Emotes = map[string]int{}
		}
		prev, ok := out[key]
		if !ok {
			out[key] = item
			continue
		}
		out[key] = mergeMinuteRollups(prev, item)
	}
	return out
}

func normalizeStreamWindow(startedAt time.Time, endedAt *time.Time) (time.Time, *time.Time) {
	if endedAt == nil || startedAt.IsZero() {
		return startedAt, endedAt
	}
	if !endedAt.Before(startedAt) {
		return startedAt, endedAt
	}
	clamped := startedAt.UTC().Truncate(time.Minute).Add(time.Minute)
	return startedAt, &clamped
}

func fillMissingRollups(in []MinuteRollup, startedAt time.Time, endedAt *time.Time) []MinuteRollup {
	startedAt, endedAt = normalizeStreamWindow(startedAt, endedAt)
	if startedAt.IsZero() {
		if len(in) < 2 {
			return in
		}
		out := make([]MinuteRollup, 0, len(in))
		for i, item := range in {
			if i > 0 {
				prev := in[i-1].MinuteTS
				if item.MinuteTS.Sub(prev) > 24*time.Hour {
					prev = item.MinuteTS.Add(-24 * time.Hour)
				}
				for ts := prev.Add(time.Minute); ts.Before(item.MinuteTS); ts = ts.Add(time.Minute) {
					out = append(out, MinuteRollup{MinuteTS: ts, Emotes: map[string]int{}, Missing: true})
				}
			}
			out = append(out, item)
		}
		return out
	}

	startMin := startedAt.UTC().Truncate(time.Minute)
	
	var endMin time.Time
	if endedAt != nil {
		endMin = endedAt.UTC().Truncate(time.Minute)
	} else {
		endMin = time.Now().UTC().Truncate(time.Minute)
	}

	// Safety: prevent padding spans greater than 24 hours to avoid infinite loops / OOM
	if endMin.Sub(startMin) > 24*time.Hour {
		startMin = endMin.Add(-24 * time.Hour)
	}

	minuteSpan := int(endMin.Sub(startMin) / time.Minute)
	if minuteSpan < 0 {
		endMin = startMin
		minuteSpan = 0
	}

	out := make([]MinuteRollup, 0, minuteSpan+1)
	existing := consolidateRollupsByMinute(in)

	for ts := startMin; !ts.After(endMin); ts = ts.Add(time.Minute) {
		if item, ok := existing[minuteBucketKey(ts)]; ok {
			out = append(out, item)
		} else {
			out = append(out, MinuteRollup{
				MinuteTS: ts,
				Emotes:   map[string]int{},
				Missing:  true,
			})
		}
	}

	return out
}

func slimRollupsForChart(in []MinuteRollup, emoteKeys []string) []MinuteRollup {
	if len(in) == 0 {
		return in
	}
	keySet := make(map[string]struct{}, len(emoteKeys))
	for _, key := range emoteKeys {
		if key == "" {
			continue
		}
		keySet[key] = struct{}{}
	}
	out := make([]MinuteRollup, len(in))
	for i, item := range in {
		if len(keySet) == 0 || len(item.Emotes) == 0 {
			item.Emotes = nil
		} else {
			slim := make(map[string]int, len(keySet))
			for key := range keySet {
				if count, ok := item.Emotes[key]; ok && count > 0 {
					slim[key] = count
				}
			}
			if len(slim) == 0 {
				item.Emotes = nil
			} else {
				item.Emotes = slim
			}
		}
		out[i] = item
	}
	return out
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func (h *Handler) syncStream(w http.ResponseWriter, r *http.Request) {
	streamID := strings.TrimSpace(chi.URLParam(r, "streamID"))
	if streamID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing_stream_id"})
		return
	}
	channelOpt := normalizeLogin(r.URL.Query().Get("channel"))
	viewersOnly := strings.EqualFold(r.URL.Query().Get("viewers_only"), "true") ||
		strings.EqualFold(r.URL.Query().Get("viewers_only"), "1") ||
		strings.EqualFold(r.URL.Query().Get("mode"), "viewers")
	forceChat := strings.EqualFold(r.URL.Query().Get("force_chat"), "true") ||
		strings.EqualFold(r.URL.Query().Get("force_chat"), "1")
	accepted, status, err := h.syncService.TryStartSync(r.Context(), streamID, channelOpt, viewersOnly, forceChat, strings.TrimSpace(r.URL.Query().Get("vod_id")))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	code := http.StatusAccepted
	if !accepted {
		code = http.StatusOK
	}
	writeJSON(w, code, StartSyncResponse{Accepted: accepted, Status: status})
}

func (h *Handler) syncStreamStatus(w http.ResponseWriter, r *http.Request) {
	streamID := strings.TrimSpace(chi.URLParam(r, "streamID"))
	if streamID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing_stream_id"})
		return
	}
	status, err := h.syncService.GetSyncStatus(r.Context(), streamID)
	if err != nil {
		if errors.Is(err, redis.Nil) {
			writeJSON(w, http.StatusOK, map[string]string{"phase": "idle"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, status)
}

func (h *Handler) getStreamGames(w http.ResponseWriter, r *http.Request) {
	streamID := strings.TrimSpace(chi.URLParam(r, "streamID"))
	if streamID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing_stream_id"})
		return
	}
	segments, err := h.store.GetGameSegments(r.Context(), streamID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if segments == nil {
		segments = []GameSegment{}
	}
	writeJSON(w, http.StatusOK, segments)
}
