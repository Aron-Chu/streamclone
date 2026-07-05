package analytics

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"streamclone/internal/analytics/recap"
	"streamclone/internal/config"
)

func TestBuildClipCandidatesFromRecapPreservesFactsAndBounds(t *testing.T) {
	startedAt := time.Date(2026, 7, 4, 19, 0, 0, 0, time.UTC)
	stream := &StreamRecord{
		StreamID:  "stream-1",
		Login:     "XQC",
		Title:     "July run",
		Category:  "Just Chatting",
		StartedAt: startedAt,
		VodID:     "vod-1",
	}
	rec := recap.StreamRecap{
		StreamID:        "stream-1",
		Login:           "xqc",
		DurationSeconds: 600,
		ClipCandidates: []recap.Moment{
			{
				OffsetSeconds: 10,
				Score:         92,
				Reasons:       []string{"emote_spike"},
				ChatCount:     240,
				EmoteCount:    180,
				ViewerCount:   1500,
				TopEmotes: []recap.Emote{{
					Code:     "KEKW",
					Provider: "seventv",
					ID:       "7tv-1",
					ImageURL: "https://cdn.example/KEKW.webp",
					Count:    99,
				}},
			},
			{OffsetSeconds: 300, Score: 81, Reasons: []string{"chat_spike"}, ChatCount: 120},
			{OffsetSeconds: 360, Score: 79, Reasons: []string{"viewer_spike"}, ChatCount: 110},
		},
	}

	got := BuildClipCandidatesFromRecap(stream, rec, ClipCandidateBuildOptions{MaxCandidates: 2})
	if len(got) != 2 {
		t.Fatalf("len(candidates) = %d, want 2", len(got))
	}
	first := got[0]
	if first.ID == "" {
		t.Fatal("candidate ID must be deterministic and non-empty")
	}
	again := BuildClipCandidatesFromRecap(stream, rec, ClipCandidateBuildOptions{MaxCandidates: 2})
	if len(again) != 2 || again[0].ID != first.ID {
		t.Fatalf("candidate ID must be stable across rebuilds: first=%q again=%q", first.ID, again[0].ID)
	}
	if first.Login != "xqc" || first.StreamID != "stream-1" || first.VodID == nil || *first.VodID != "vod-1" {
		t.Fatalf("candidate stream identity mismatch: %+v", first)
	}
	if first.StartSeconds != 0 || first.EndSeconds != 50 {
		t.Fatalf("candidate range = %d-%d, want 0-50", first.StartSeconds, first.EndSeconds)
	}
	if first.SourceKind != ClipCandidateSourceRecap || first.SourceStatus != ClipCandidateSourceAvailable {
		t.Fatalf("source = %q/%q, want recap/available", first.SourceKind, first.SourceStatus)
	}
	if first.Reason != "emote_spike" || first.ChatCount != 240 || first.EmoteCount != 180 || first.ViewerCount != 1500 {
		t.Fatalf("candidate signals not preserved: %+v", first)
	}
	if len(first.TopEmotes) != 1 || first.TopEmotes[0].Name != "KEKW" || first.TopEmotes[0].ImageURL == "" {
		t.Fatalf("top emotes not preserved: %+v", first.TopEmotes)
	}
}

func TestBuildClipCandidatesFromRecapMarksMissingSourceWithoutVod(t *testing.T) {
	startedAt := time.Date(2026, 7, 4, 19, 0, 0, 0, time.UTC)
	stream := &StreamRecord{
		StreamID:  "stream-2",
		Login:     "ludwig",
		StartedAt: startedAt,
	}
	rec := recap.StreamRecap{
		StreamID:        "stream-2",
		Login:           "ludwig",
		DurationSeconds: 120,
		ClipCandidates: []recap.Moment{{
			OffsetSeconds: 90,
			Score:         88,
			Reasons:       []string{"chat_spike"},
			ChatCount:     75,
		}},
	}

	got := BuildClipCandidatesFromRecap(stream, rec, ClipCandidateBuildOptions{})
	if len(got) != 1 {
		t.Fatalf("len(candidates) = %d, want 1", len(got))
	}
	if got[0].SourceStatus != ClipCandidateSourceMissing {
		t.Fatalf("source status = %q, want missing", got[0].SourceStatus)
	}
	if got[0].StartSeconds != 70 || got[0].EndSeconds != 120 {
		t.Fatalf("candidate range = %d-%d, want 70-120", got[0].StartSeconds, got[0].EndSeconds)
	}
}

func TestBuildClipCandidatesFromRecapAppliesQualityFilters(t *testing.T) {
	startedAt := time.Date(2026, 7, 4, 19, 0, 0, 0, time.UTC)
	stream := &StreamRecord{
		StreamID:  "stream-policy-1",
		Login:     "xqc",
		StartedAt: startedAt,
		VodID:     "vod-policy-1",
	}
	rec := recap.StreamRecap{
		StreamID:        "stream-policy-1",
		Login:           "xqc",
		DurationSeconds: 900,
		ClipCandidates: []recap.Moment{
			{OffsetSeconds: 60, Score: 69, Reasons: []string{"low_score"}, ChatCount: 500, EmoteCount: 500, TopEmotes: []recap.Emote{{Code: "KEKW", Provider: "seventv", Count: 500}}},
			{OffsetSeconds: 120, Score: 90, Reasons: []string{"low_chat"}, ChatCount: 10, EmoteCount: 500, TopEmotes: []recap.Emote{{Code: "KEKW", Provider: "seventv", Count: 500}}},
			{OffsetSeconds: 180, Score: 91, Reasons: []string{"low_emote"}, ChatCount: 500, EmoteCount: 20, TopEmotes: []recap.Emote{{Code: "KEKW", Provider: "seventv", Count: 20}}},
			{OffsetSeconds: 240, Score: 92, Reasons: []string{"low_provider"}, ChatCount: 500, EmoteCount: 500, TopEmotes: []recap.Emote{{Code: "Pog", Provider: "twitch", Count: 500}}},
			{OffsetSeconds: 300, Score: 93, Reasons: []string{"keeper"}, ChatCount: 500, EmoteCount: 500, TopEmotes: []recap.Emote{{Code: "OMEGALUL", Provider: "seventv", Count: 120}}},
		},
	}

	got := BuildClipCandidatesFromRecap(stream, rec, ClipCandidateBuildOptions{
		MinScore:               70,
		MinChatCount:           100,
		MinEmoteCount:          100,
		MinProviderEmoteCount:  50,
		ProviderEmoteProvider:  "seventv",
		RequireSourceAvailable: true,
	})

	if len(got) != 1 {
		t.Fatalf("len(candidates) = %d, want one keeper: %+v", len(got), got)
	}
	if got[0].Reason != "keeper" || got[0].OffsetSeconds != 300 {
		t.Fatalf("candidate = %+v, want keeper moment", got[0])
	}
	if got[0].CoverageState != "ready" {
		t.Fatalf("coverage state = %q, want ready", got[0].CoverageState)
	}
}

func TestBuildClipCandidatesFromRecapAppliesConfidenceAndCoveragePolicy(t *testing.T) {
	startedAt := time.Date(2026, 7, 4, 19, 0, 0, 0, time.UTC)
	stream := &StreamRecord{
		StreamID:  "stream-policy-coverage",
		Login:     "xqc",
		StartedAt: startedAt,
		VodID:     "vod-policy-coverage",
	}
	rec := recap.StreamRecap{
		StreamID:                "stream-policy-coverage",
		Login:                   "xqc",
		DurationSeconds:         900,
		NonMissingRollupMinutes: 4,
		MissingWindows:          []recap.MissingWindow{{StartSeconds: 120, EndSeconds: 179}},
		ClipCandidates: []recap.Moment{
			{OffsetSeconds: 60, Score: 95, Confidence: 0.44, Reasons: []string{"low_confidence"}, ChatCount: 300, EmoteCount: 200},
			{OffsetSeconds: 140, Score: 94, Confidence: 0.90, Reasons: []string{"missing_window"}, ChatCount: 300, EmoteCount: 200},
			{OffsetSeconds: 240, Score: 93, Confidence: 0.91, Reasons: []string{"keeper"}, ChatCount: 300, EmoteCount: 200},
		},
	}

	got := BuildClipCandidatesFromRecap(stream, rec, ClipCandidateBuildOptions{
		MinConfidence:              0.5,
		MinNonMissingRollupMinutes: 3,
	})

	if len(got) != 1 {
		t.Fatalf("len(candidates) = %d, want one keeper: %+v", len(got), got)
	}
	if got[0].Reason != "keeper" || got[0].Confidence != 0.91 {
		t.Fatalf("candidate = %+v, want keeper with confidence", got[0])
	}

	rec.NonMissingRollupMinutes = 2
	if got := BuildClipCandidatesFromRecap(stream, rec, ClipCandidateBuildOptions{MinNonMissingRollupMinutes: 3}); len(got) != 0 {
		t.Fatalf("len(candidates) = %d, want 0 when rollup coverage below policy", len(got))
	}
}

func TestBuildClipCandidatesFromRecapAppliesChatMinMaxPolicy(t *testing.T) {
	startedAt := time.Date(2026, 7, 4, 19, 0, 0, 0, time.UTC)
	stream := &StreamRecord{
		StreamID:  "stream-policy-chat-range",
		Login:     "xqc",
		StartedAt: startedAt,
		VodID:     "vod-policy-chat-range",
	}
	rec := recap.StreamRecap{
		StreamID:        "stream-policy-chat-range",
		Login:           "xqc",
		DurationSeconds: 600,
		ClipCandidates: []recap.Moment{
			{OffsetSeconds: 60, Score: 95, Reasons: []string{"too_low"}, ChatCount: 140, EmoteCount: 100},
			{OffsetSeconds: 120, Score: 94, Reasons: []string{"too_high"}, ChatCount: 260, EmoteCount: 100},
			{OffsetSeconds: 180, Score: 93, Reasons: []string{"keeper"}, ChatCount: 190, EmoteCount: 100},
		},
	}

	got := BuildClipCandidatesFromRecap(stream, rec, ClipCandidateBuildOptions{
		MinChatCount: 150,
		MaxChatCount: 220,
	})

	if len(got) != 1 {
		t.Fatalf("len(candidates) = %d, want one in-range candidate: %+v", len(got), got)
	}
	if got[0].Reason != "keeper" || got[0].ChatCount != 190 {
		t.Fatalf("candidate = %+v, want keeper with chat count 190", got[0])
	}
}

func TestClipCandidateTuningSummaryExplainsChatRange(t *testing.T) {
	rec := recap.StreamRecap{
		ClipCandidates: []recap.Moment{
			{OffsetSeconds: 60, Score: 95, Reasons: []string{"too_low"}, ChatCount: 140, EmoteCount: 10},
			{OffsetSeconds: 120, Score: 94, Reasons: []string{"keeper"}, ChatCount: 190, EmoteCount: 20},
			{OffsetSeconds: 180, Score: 93, Reasons: []string{"too_high"}, ChatCount: 260, EmoteCount: 30},
		},
	}
	selected := []ClipCandidate{{Reason: "keeper", ChatCount: 190}}

	got := clipCandidateTuningSummaryFromRecap(rec, selected, ClipCandidateBuildOptions{
		MinChatCount: 150,
		MaxChatCount: 220,
	})

	if got.CandidatePoolCount != 3 || got.SelectedCount != 1 {
		t.Fatalf("summary counts = %+v, want pool 3 selected 1", got)
	}
	if got.MinChatObserved != 140 || got.MaxChatObserved != 260 {
		t.Fatalf("observed chat range = %d-%d, want 140-260", got.MinChatObserved, got.MaxChatObserved)
	}
	if got.BelowMinChatCount != 1 || got.AboveMaxChatCount != 1 || got.InChatRangeCount != 1 {
		t.Fatalf("chat range counts = %+v, want below 1 above 1 in range 1", got)
	}
}

func TestBuildClipCandidatesFromRecapAppliesDuplicateRadiusAndHourlyCaps(t *testing.T) {
	startedAt := time.Date(2026, 7, 4, 19, 0, 0, 0, time.UTC)
	stream := &StreamRecord{
		StreamID:  "stream-policy-2",
		Login:     "xqc",
		StartedAt: startedAt,
		VodID:     "vod-policy-2",
	}
	rec := recap.StreamRecap{
		StreamID:        "stream-policy-2",
		Login:           "xqc",
		DurationSeconds: 7200,
		ClipCandidates: []recap.Moment{
			{OffsetSeconds: 120, Score: 97, Reasons: []string{"first"}, ChatCount: 200, EmoteCount: 200},
			{OffsetSeconds: 150, Score: 96, Reasons: []string{"duplicate_window"}, ChatCount: 200, EmoteCount: 200},
			{OffsetSeconds: 900, Score: 95, Reasons: []string{"second"}, ChatCount: 200, EmoteCount: 200},
			{OffsetSeconds: 1800, Score: 94, Reasons: []string{"hour_cap_drop"}, ChatCount: 200, EmoteCount: 200},
			{OffsetSeconds: 3900, Score: 93, Reasons: []string{"next_hour"}, ChatCount: 200, EmoteCount: 200},
		},
	}

	got := BuildClipCandidatesFromRecap(stream, rec, ClipCandidateBuildOptions{
		MaxCandidates:          10,
		DuplicateRadiusSeconds: 60,
		MaxCandidatesPerHour:   2,
	})

	if reasons := clipCandidateReasons(got); strings.Join(reasons, ",") != "first,second,next_hour" {
		t.Fatalf("candidate reasons = %v, want first, second, next_hour", reasons)
	}
}

func TestBuildClipCandidatesFromRecapAllowsConfiguredStreamCapAboveDefault(t *testing.T) {
	startedAt := time.Date(2026, 7, 4, 19, 0, 0, 0, time.UTC)
	stream := &StreamRecord{
		StreamID:  "stream-policy-cap",
		Login:     "xqc",
		StartedAt: startedAt,
		VodID:     "vod-policy-cap",
	}
	moments := make([]recap.Moment, 0, 6)
	for i := 0; i < 6; i++ {
		moments = append(moments, recap.Moment{
			OffsetSeconds: 60 + i*120,
			Score:         90 - i,
			Reasons:       []string{fmt.Sprintf("candidate_%d", i)},
			ChatCount:     100,
			EmoteCount:    100,
		})
	}
	rec := recap.StreamRecap{
		StreamID:        "stream-policy-cap",
		Login:           "xqc",
		DurationSeconds: 1200,
		ClipCandidates:  moments,
	}

	got := BuildClipCandidatesFromRecap(stream, rec, ClipCandidateBuildOptions{MaxCandidates: 6})
	if len(got) != 6 {
		t.Fatalf("len(candidates) = %d, want configured cap of 6", len(got))
	}
}

func TestWithAppConfigConfiguresClipCandidateBuildOptions(t *testing.T) {
	h := (&Handler{}).WithAppConfig(config.Config{
		PulseClipMaxCandidates:              8,
		PulseClipMinScore:                   70,
		PulseClipMinConfidence:              0.67,
		PulseClipMinChatCount:               50,
		PulseClipMaxChatCount:               500,
		PulseClipMinEmoteCount:              25,
		PulseClipMinProviderEmoteCount:      12,
		PulseClipProviderEmoteProvider:      "7TV",
		PulseClipMinNonMissingRollupMinutes: 6,
		PulseClipDuplicateRadiusSeconds:     90,
		PulseClipMaxCandidatesPerHour:       3,
		PulseClipRequireSourceAvailable:     true,
	})

	got := h.clipCandidateBuildOptions()
	if got.MaxCandidates != 8 ||
		got.MinScore != 70 ||
		got.MinConfidence != 0.67 ||
		got.MinChatCount != 50 ||
		got.MaxChatCount != 500 ||
		got.MinEmoteCount != 25 ||
		got.MinProviderEmoteCount != 12 ||
		got.ProviderEmoteProvider != "7tv" ||
		got.MinNonMissingRollupMinutes != 6 ||
		got.DuplicateRadiusSeconds != 90 ||
		got.MaxCandidatesPerHour != 3 ||
		!got.RequireSourceAvailable {
		t.Fatalf("clip candidate options = %+v, want app config values", got)
	}
}

func clipCandidateReasons(items []ClipCandidate) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		out = append(out, item.Reason)
	}
	return out
}

func TestNormalizeClipCandidateStatePatchAllowsOnlyPrivateReviewStatuses(t *testing.T) {
	status := "saved"
	title := "  Good bit  "
	start := 42
	patch, err := normalizeClipCandidateStatePatch(UpdateClipCandidateStateRequest{
		Status:               &status,
		TitleOverride:        &title,
		StartSecondsOverride: &start,
	})
	if err != nil {
		t.Fatalf("normalize patch: %v", err)
	}
	if patch.Status == nil || *patch.Status != ClipCandidateStatusSaved {
		t.Fatalf("status = %#v, want saved", patch.Status)
	}
	if patch.TitleOverride == nil || *patch.TitleOverride != "Good bit" {
		t.Fatalf("title override = %#v, want trimmed title", patch.TitleOverride)
	}

	rendering := "rendering"
	if _, err := normalizeClipCandidateStatePatch(UpdateClipCandidateStateRequest{Status: &rendering}); err == nil {
		t.Fatal("rendering must not be accepted by Phase 1 private queue patch")
	}
}

func TestClipCandidateCursorRoundTripAndLegacyTimestamp(t *testing.T) {
	createdAt := time.Date(2026, 7, 4, 20, 10, 11, 12, time.UTC)
	cursor := encodeClipCandidateCursor(ClipCandidate{
		ID:        "cc_cursor_roundtrip",
		Score:     87,
		CreatedAt: createdAt,
	})
	got, legacy, err := parseClipCandidateCursor(cursor)
	if err != nil {
		t.Fatalf("parse cursor: %v", err)
	}
	if legacy {
		t.Fatal("encoded cursor should not be treated as legacy timestamp")
	}
	if got.ID != "cc_cursor_roundtrip" || got.Score != 87 || !got.CreatedAt.Equal(createdAt) {
		t.Fatalf("cursor = %+v, want id/score/createdAt round-trip", got)
	}

	legacyCursor := createdAt.Format(time.RFC3339Nano)
	got, legacy, err = parseClipCandidateCursor(legacyCursor)
	if err != nil {
		t.Fatalf("parse legacy cursor: %v", err)
	}
	if !legacy || !got.CreatedAt.Equal(createdAt) {
		t.Fatalf("legacy cursor = %+v legacy=%v, want timestamp-compatible cursor", got, legacy)
	}
}

func TestPulseClipsRouteIsPrivateAndStoreBacked(t *testing.T) {
	h := &Handler{pulseHosted: PulseHostedConfig{Hosted: true, BetaKeys: []string{"secret-one"}}}
	r := chi.NewRouter()
	h.PulseRoutes(r)

	guestReq := httptest.NewRequest(http.MethodGet, "/v1/pulse/clips", nil)
	guestRec := httptest.NewRecorder()
	r.ServeHTTP(guestRec, guestReq)
	if guestRec.Code != http.StatusUnauthorized {
		t.Fatalf("guest status = %d, want 401 unauthorized before store access", guestRec.Code)
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/pulse/clips", nil)
	req.Header.Set("X-Streamclone-Beta-Key", "secret-one")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rec.Code)
	}
	var payload map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode error payload: %v", err)
	}
	if payload["error"] != "store_unavailable" {
		t.Fatalf("payload = %#v, want store_unavailable", payload)
	}
	if strings.Contains(strings.ToLower(rec.Body.String()), "render") {
		t.Fatalf("phase 1 clips route must not expose render side effects: %s", rec.Body.String())
	}
}

func TestPulseClipPreviewRouteRequiresStreamID(t *testing.T) {
	h := &Handler{}
	r := chi.NewRouter()
	h.PulseRoutes(r)

	req := httptest.NewRequest(http.MethodGet, "/v1/pulse/clips/preview?maxCandidates=3", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400: %s", rec.Code, rec.Body.String())
	}
	var payload map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode error payload: %v", err)
	}
	if payload["error"] != "missing_stream_id" {
		t.Fatalf("payload = %#v, want missing_stream_id", payload)
	}
}

func TestPulseClipPreviewRouteAppliesChatMinMaxControlsWithoutPersistingIntegration(t *testing.T) {
	ctx, store := setupClipCandidateStore(t)
	startedAt := time.Date(2026, 7, 4, 21, 0, 0, 0, time.UTC)
	mustExec(t, ctx, store, `
		INSERT INTO analytics_streams (
			stream_id, broadcaster_id, login, display_name, title, category, tags,
			started_at, ended_at, last_seen_at, vod_id, canonical_stream_id
		)
		VALUES ('stream-preview-1', 'b-preview', 'previewer', 'Previewer', 'Preview stream', 'Just Chatting', '[]'::jsonb, $1, $2, $2, 'vod-preview-1', 'stream-preview-1')
	`, startedAt, startedAt.Add(5*time.Minute))
	for i, chat := range []int{12, 220, 45, 180, 30} {
		emotes := chat / 2
		mustExec(t, ctx, store, `
			INSERT INTO analytics_minute_rollups (
				stream_id, minute_ts, viewer_avg, viewer_max, viewer_latest, viewer_samples,
				chat_count, total_emote_count, seventv_emote_count, emotes_json,
				chat_source, source_confidence, chat_source_detail
			)
			VALUES ('stream-preview-1', $1, 1000, 1000, 1000, 1, $2, $3, $3, '{}'::jsonb, 'gql', 'verified', 'test')
		`, startedAt.Add(time.Duration(i)*time.Minute), chat, emotes)
	}

	h := &Handler{store: store}
	r := chi.NewRouter()
	h.PulseRoutes(r)

	req := httptest.NewRequest(http.MethodGet, "/v1/pulse/clips/preview?streamId=stream-preview-1&minChatCount=150&maxChatCount=200&maxCandidates=1&minScore=1", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var payload struct {
		Items    []ClipCandidate `json:"items"`
		Controls struct {
			MaxCandidates int `json:"maxCandidates"`
			MinChatCount  int `json:"minChatCount"`
			MaxChatCount  int `json:"maxChatCount"`
			MinScore      int `json:"minScore"`
		} `json:"controls"`
		Summary struct {
			CandidatePoolCount int `json:"candidatePoolCount"`
			SelectedCount      int `json:"selectedCount"`
			MinChatObserved    int `json:"minChatObserved"`
			MaxChatObserved    int `json:"maxChatObserved"`
			BelowMinChatCount  int `json:"belowMinChatCount"`
			AboveMaxChatCount  int `json:"aboveMaxChatCount"`
			InChatRangeCount   int `json:"inChatRangeCount"`
		} `json:"summary"`
		Persisted bool `json:"persisted"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode preview payload: %v", err)
	}
	if payload.Persisted {
		t.Fatalf("preview persisted flag = true, want false")
	}
	if payload.Controls.MaxCandidates != 1 || payload.Controls.MinChatCount != 150 || payload.Controls.MaxChatCount != 200 || payload.Controls.MinScore != 1 {
		t.Fatalf("controls = %+v, want query controls", payload.Controls)
	}
	if payload.Summary.CandidatePoolCount == 0 || payload.Summary.SelectedCount != 1 {
		t.Fatalf("summary = %+v, want non-empty pool and one selected", payload.Summary)
	}
	if payload.Summary.BelowMinChatCount == 0 || payload.Summary.AboveMaxChatCount == 0 || payload.Summary.InChatRangeCount == 0 {
		t.Fatalf("summary chat buckets = %+v, want below/above/in-range counts", payload.Summary)
	}
	if len(payload.Items) != 1 {
		t.Fatalf("len(items) = %d, want one strict preview candidate: %+v", len(payload.Items), payload.Items)
	}
	if payload.Items[0].ChatCount < 150 {
		t.Fatalf("preview candidate chat count = %d, want >= 150: %+v", payload.Items[0].ChatCount, payload.Items[0])
	}
	if payload.Items[0].ChatCount > 200 {
		t.Fatalf("preview candidate chat count = %d, want <= 200: %+v", payload.Items[0].ChatCount, payload.Items[0])
	}
	var stored int
	if err := store.db.QueryRow(ctx, `SELECT COUNT(*) FROM clip_candidates`).Scan(&stored); err != nil {
		t.Fatalf("count stored candidates: %v", err)
	}
	if stored != 0 {
		t.Fatalf("stored candidates = %d, want preview to avoid persistence", stored)
	}
}

func TestPulseClipGenerateRouteAppliesControlsAndPersistsIntegration(t *testing.T) {
	ctx, store := setupClipCandidateStore(t)
	startedAt := time.Date(2026, 7, 4, 21, 30, 0, 0, time.UTC)
	mustExec(t, ctx, store, `
		INSERT INTO analytics_streams (
			stream_id, broadcaster_id, login, display_name, title, category, tags,
			started_at, ended_at, last_seen_at, vod_id, canonical_stream_id
		)
		VALUES ('stream-generate-1', 'b-generate', 'generator', 'Generator', 'Generate stream', 'Just Chatting', '[]'::jsonb, $1, $2, $2, 'vod-generate-1', 'stream-generate-1')
	`, startedAt, startedAt.Add(5*time.Minute))
	for i, chat := range []int{10, 240, 60, 190, 40} {
		emotes := chat / 2
		mustExec(t, ctx, store, `
			INSERT INTO analytics_minute_rollups (
				stream_id, minute_ts, viewer_avg, viewer_max, viewer_latest, viewer_samples,
				chat_count, total_emote_count, seventv_emote_count, emotes_json,
				chat_source, source_confidence, chat_source_detail
			)
			VALUES ('stream-generate-1', $1, 1200, 1200, 1200, 1, $2, $3, $3, '{}'::jsonb, 'gql', 'verified', 'test')
		`, startedAt.Add(time.Duration(i)*time.Minute), chat, emotes)
	}

	h := &Handler{store: store}
	r := chi.NewRouter()
	h.PulseRoutes(r)

	req := httptest.NewRequest(http.MethodPost, "/v1/pulse/clips/generate?streamId=stream-generate-1&minChatCount=150&maxChatCount=220&maxCandidates=3&minScore=1", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var payload ClipCandidatePreviewResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode generate payload: %v", err)
	}
	if !payload.Persisted {
		t.Fatalf("persisted = false, want true")
	}
	if payload.Controls.MinChatCount != 150 || payload.Controls.MaxChatCount != 220 || payload.Controls.MaxCandidates != 3 || payload.Controls.MinScore != 1 {
		t.Fatalf("controls = %+v, want generated query controls", payload.Controls)
	}
	if len(payload.Items) != 1 {
		t.Fatalf("len(items) = %d, want one generated candidate: %+v", len(payload.Items), payload.Items)
	}
	if payload.Items[0].ChatCount < 150 || payload.Items[0].ChatCount > 220 {
		t.Fatalf("generated candidate chat count = %d, want 150-220: %+v", payload.Items[0].ChatCount, payload.Items[0])
	}
	var stored int
	if err := store.db.QueryRow(ctx, `SELECT COUNT(*) FROM clip_candidates WHERE stream_id='stream-generate-1'`).Scan(&stored); err != nil {
		t.Fatalf("count generated candidates: %v", err)
	}
	if stored != len(payload.Items) {
		t.Fatalf("stored candidates = %d, want %d", stored, len(payload.Items))
	}
}

func TestPulseClipsRouteFiltersPersistedQueueByChatRangeIntegration(t *testing.T) {
	ctx, store := setupClipCandidateStore(t)
	startedAt := time.Date(2026, 7, 4, 21, 45, 0, 0, time.UTC)
	mustExec(t, ctx, store, `
		INSERT INTO analytics_streams (
			stream_id, broadcaster_id, login, display_name, title, category, tags,
			started_at, ended_at, last_seen_at, vod_id, canonical_stream_id
		)
		VALUES ('stream-list-filter-1', 'b-list-filter', 'listfilter', 'ListFilter', 'List filter stream', 'Just Chatting', '[]'::jsonb, $1, $2, $2, 'vod-list-filter-1', 'stream-list-filter-1')
	`, startedAt, startedAt.Add(5*time.Minute))
	for i, chat := range []int{40, 260, 120, 190, 30} {
		emotes := chat / 2
		mustExec(t, ctx, store, `
			INSERT INTO analytics_minute_rollups (
				stream_id, minute_ts, viewer_avg, viewer_max, viewer_latest, viewer_samples,
				chat_count, total_emote_count, seventv_emote_count, emotes_json,
				chat_source, source_confidence, chat_source_detail
			)
			VALUES ('stream-list-filter-1', $1, 800, 800, 800, 1, $2, $3, $3, '{}'::jsonb, 'gql', 'verified', 'test')
		`, startedAt.Add(time.Duration(i)*time.Minute), chat, emotes)
	}

	h := &Handler{store: store}
	r := chi.NewRouter()
	h.PulseRoutes(r)

	generateReq := httptest.NewRequest(http.MethodPost, "/v1/pulse/clips/generate?streamId=stream-list-filter-1&minScore=1&maxCandidates=5", nil)
	generateRec := httptest.NewRecorder()
	r.ServeHTTP(generateRec, generateReq)
	if generateRec.Code != http.StatusOK {
		t.Fatalf("generate status = %d, want 200: %s", generateRec.Code, generateRec.Body.String())
	}

	listReq := httptest.NewRequest(http.MethodGet, "/v1/pulse/clips?streamId=stream-list-filter-1&minChatCount=150&maxChatCount=220&limit=10", nil)
	listRec := httptest.NewRecorder()
	r.ServeHTTP(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("list status = %d, want 200: %s", listRec.Code, listRec.Body.String())
	}
	var payload ClipCandidateListResponse
	if err := json.Unmarshal(listRec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode list payload: %v", err)
	}
	if len(payload.Items) == 0 {
		t.Fatalf("filtered queue returned no items, want in-range candidates")
	}
	for _, item := range payload.Items {
		if item.ChatCount < 150 || item.ChatCount > 220 {
			t.Fatalf("filtered queue item chat count = %d, want 150-220: %+v", item.ChatCount, item)
		}
	}
}

func TestPulseClipsRouteSeedsRecentCandidatesWhenNoStreamIDIntegration(t *testing.T) {
	ctx, store := setupClipCandidateStore(t)
	startedAt := time.Date(2026, 7, 4, 22, 0, 0, 0, time.UTC)
	mustExec(t, ctx, store, `
		INSERT INTO analytics_streams (
			stream_id, broadcaster_id, login, display_name, title, category, tags,
			started_at, ended_at, last_seen_at, vod_id, canonical_stream_id
		)
		VALUES ('stream-seed-1', 'b-seed', 'seeded', 'Seeded', 'Seed stream', 'Just Chatting', '[]'::jsonb, $1, $2, $2, 'vod-seed-1', 'stream-seed-1')
	`, startedAt, startedAt.Add(5*time.Minute))
	for i, chat := range []int{18, 260, 35, 210, 20} {
		mustExec(t, ctx, store, `
			INSERT INTO analytics_minute_rollups (
				stream_id, minute_ts, viewer_avg, viewer_max, viewer_latest, viewer_samples,
				chat_count, total_emote_count, seventv_emote_count, emotes_json,
				chat_source, source_confidence, chat_source_detail
			)
			VALUES ('stream-seed-1', $1, 900, 900, 900, 1, $2, $3, $3, '{}'::jsonb, 'gql', 'verified', 'test')
		`, startedAt.Add(time.Duration(i)*time.Minute), chat, chat/2)
	}

	h := &Handler{store: store}
	r := chi.NewRouter()
	h.PulseRoutes(r)

	req := httptest.NewRequest(http.MethodGet, "/v1/pulse/clips?limit=2", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var payload ClipCandidateListResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode seeded list: %v", err)
	}
	if len(payload.Items) == 0 {
		t.Fatalf("items empty, want default list to seed recent stream candidates")
	}
	if payload.Items[0].StreamID != "stream-seed-1" || payload.Items[0].ChatCount <= 0 {
		t.Fatalf("seeded candidate = %+v, want stream-seed-1 with chat signal", payload.Items[0])
	}
	var stored int
	if err := store.db.QueryRow(ctx, `SELECT COUNT(*) FROM clip_candidates WHERE stream_id='stream-seed-1'`).Scan(&stored); err != nil {
		t.Fatalf("count seeded candidates: %v", err)
	}
	if stored == 0 {
		t.Fatalf("stored candidates = 0, want recent seeding to persist candidates for the queue")
	}
}

func TestHostedPulseClipRoutesRejectGuestPrincipals(t *testing.T) {
	h := &Handler{pulseHosted: PulseHostedConfig{Hosted: true, BetaKeys: []string{"secret-one"}}}
	r := chi.NewRouter()
	h.PulseRoutes(r)

	cases := []struct {
		name   string
		method string
		path   string
		body   string
	}{
		{name: "list", method: http.MethodGet, path: "/v1/pulse/clips"},
		{name: "generate", method: http.MethodPost, path: "/v1/pulse/clips/generate"},
		{name: "patch", method: http.MethodPatch, path: "/v1/pulse/clips/cc_private", body: `{"status":"saved"}`},
		{name: "send", method: http.MethodPost, path: "/v1/pulse/clips/cc_private/replayforge"},
		{name: "refresh", method: http.MethodGet, path: "/v1/pulse/clips/cc_private/replayforge"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
			if tc.body != "" {
				req.Header.Set("Content-Type", "application/json")
			}
			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, req)
			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d, want 401: %s", rec.Code, rec.Body.String())
			}
		})
	}
}

func TestClipCandidateJSONOmitsPrivateAndRawFields(t *testing.T) {
	candidate := ClipCandidate{
		ID:            "cc_test",
		Login:         "xqc",
		StreamID:      "stream-1",
		OffsetSeconds: 120,
		StartSeconds:  100,
		EndSeconds:    160,
		Score:         91,
		Reason:        "chat_spike",
		TopEmotes: []ClipCandidateEmote{{
			Name:     "OMEGALUL",
			Provider: "seventv",
			ID:       "provider-id",
			ImageURL: "https://cdn.example/emote.webp",
			Count:    44,
		}},
		SourceKind:   ClipCandidateSourceRecap,
		SourceStatus: ClipCandidateSourceAvailable,
		State: &ClipCandidateState{
			ID:          "state-1",
			CandidateID: "cc_test",
			Status:      ClipCandidateStatusNew,
		},
	}
	body, err := json.Marshal(candidate)
	if err != nil {
		t.Fatal(err)
	}
	assertJSONOmitsForbiddenFields(t, body, []string{
		"rawChat", "chatText", "message", "messages", "fragments", "chatter",
		"userLogin", "userId", "operator", "gql", "corpus", "archive", "storageKey",
		"token", "webhook", "principalId", "principalKind",
	})
	raw := string(body)
	for _, required := range []string{"topEmotes", "sourceStatus", "state", "offsetSeconds"} {
		if !strings.Contains(raw, required) {
			t.Fatalf("candidate json missing %q: %s", required, raw)
		}
	}
}

func TestClipCandidateStorePrincipalIsolationIntegration(t *testing.T) {
	ctx, store := setupClipCandidateStore(t)
	startedAt := time.Date(2026, 7, 4, 20, 0, 0, 0, time.UTC)
	mustExec(t, ctx, store, `
		INSERT INTO analytics_streams (stream_id, login, started_at, title, category)
		VALUES ('stream-clip-1', 'xqc', $1, 'Test stream', 'Just Chatting')
	`, startedAt)
	vodID := "vod-clip-1"
	candidates := []ClipCandidate{{
		ID:            "cc_store_test",
		Login:         "xqc",
		StreamID:      "stream-clip-1",
		VodID:         &vodID,
		OffsetSeconds: 120,
		StartSeconds:  100,
		EndSeconds:    160,
		Score:         93,
		Reason:        "chat_spike",
		TopEmotes:     []ClipCandidateEmote{{Name: "KEKW", Provider: "seventv", Count: 12}},
		SourceKind:    ClipCandidateSourceRecap,
		SourceStatus:  ClipCandidateSourceAvailable,
	}}
	if err := store.UpsertClipCandidates(ctx, candidates); err != nil {
		t.Fatalf("upsert candidates: %v", err)
	}
	saved := ClipCandidateStatusSaved
	if _, err := store.UpdateClipCandidateState(ctx, "cc_store_test", PulsePrincipal{ID: "principal-a", Kind: "beta"}, clipCandidateStatePatch{Status: &saved}); err != nil {
		t.Fatalf("update principal-a: %v", err)
	}
	title := "Keeper moment"
	state, err := store.UpdateClipCandidateState(ctx, "cc_store_test", PulsePrincipal{ID: "principal-a", Kind: "beta"}, clipCandidateStatePatch{TitleOverride: &title})
	if err != nil {
		t.Fatalf("title-only update principal-a: %v", err)
	}
	if state.Status != ClipCandidateStatusSaved || state.TitleOverride == nil || *state.TitleOverride != title {
		t.Fatalf("title-only update reset state: %+v", state)
	}
	dismissed := ClipCandidateStatusDismissed
	if _, err := store.UpdateClipCandidateState(ctx, "cc_store_test", PulsePrincipal{ID: "principal-b", Kind: "beta"}, clipCandidateStatePatch{Status: &dismissed}); err != nil {
		t.Fatalf("update principal-b: %v", err)
	}
	aItems, _, err := store.ListClipCandidates(ctx, ListClipCandidatesFilter{StreamID: "stream-clip-1", PrincipalID: "principal-a", Limit: 10})
	if err != nil {
		t.Fatalf("list principal-a: %v", err)
	}
	if len(aItems) != 1 || aItems[0].State == nil || aItems[0].State.Status != ClipCandidateStatusSaved {
		t.Fatalf("principal-a state = %+v", aItems)
	}
	bItems, _, err := store.ListClipCandidates(ctx, ListClipCandidatesFilter{StreamID: "stream-clip-1", PrincipalID: "principal-b", Status: ClipCandidateStatusSaved, Limit: 10})
	if err != nil {
		t.Fatalf("list principal-b saved: %v", err)
	}
	if len(bItems) != 0 {
		t.Fatalf("principal-b saved filter returned %d items, want 0", len(bItems))
	}
}

func TestPulseClipRoutesUseSameLocalPrincipalForListAndPatchIntegration(t *testing.T) {
	ctx, store := setupClipCandidateStore(t)
	startedAt := time.Date(2026, 7, 4, 20, 0, 0, 0, time.UTC)
	mustExec(t, ctx, store, `
		INSERT INTO analytics_streams (stream_id, login, started_at, title, category)
		VALUES ('stream-route-principal-1', 'routeprincipal', $1, 'Route principal stream', 'Just Chatting')
	`, startedAt)
	vodID := "vod-route-principal-1"
	if err := store.UpsertClipCandidates(ctx, []ClipCandidate{{
		ID:            "cc_route_principal",
		Login:         "routeprincipal",
		StreamID:      "stream-route-principal-1",
		VodID:         &vodID,
		OffsetSeconds: 120,
		StartSeconds:  100,
		EndSeconds:    160,
		Score:         93,
		Reason:        "chat_spike",
		SourceKind:    ClipCandidateSourceRecap,
		SourceStatus:  ClipCandidateSourceAvailable,
	}}); err != nil {
		t.Fatalf("upsert candidate: %v", err)
	}
	h := &Handler{store: store}
	r := chi.NewRouter()
	h.PulseRoutes(r)

	saveReq := httptest.NewRequest(http.MethodPatch, "/v1/pulse/clips/cc_route_principal", strings.NewReader(`{"status":"saved"}`))
	saveReq.Header.Set("Content-Type", "application/json")
	saveRec := httptest.NewRecorder()
	r.ServeHTTP(saveRec, saveReq)
	if saveRec.Code != http.StatusOK {
		t.Fatalf("save status = %d, want 200: %s", saveRec.Code, saveRec.Body.String())
	}

	savedReq := httptest.NewRequest(http.MethodGet, "/v1/pulse/clips?login=routeprincipal&status=saved", nil)
	savedRec := httptest.NewRecorder()
	r.ServeHTTP(savedRec, savedReq)
	if savedRec.Code != http.StatusOK {
		t.Fatalf("saved list status = %d, want 200: %s", savedRec.Code, savedRec.Body.String())
	}
	var savedList ClipCandidateListResponse
	if err := json.Unmarshal(savedRec.Body.Bytes(), &savedList); err != nil {
		t.Fatalf("decode saved list: %v", err)
	}
	if len(savedList.Items) != 1 || savedList.Items[0].State == nil || savedList.Items[0].State.Status != ClipCandidateStatusSaved {
		t.Fatalf("saved list = %+v, want one saved local state", savedList.Items)
	}

	dismissReq := httptest.NewRequest(http.MethodPatch, "/v1/pulse/clips/cc_route_principal", strings.NewReader(`{"status":"dismissed"}`))
	dismissReq.Header.Set("Content-Type", "application/json")
	dismissRec := httptest.NewRecorder()
	r.ServeHTTP(dismissRec, dismissReq)
	if dismissRec.Code != http.StatusOK {
		t.Fatalf("dismiss status = %d, want 200: %s", dismissRec.Code, dismissRec.Body.String())
	}

	dismissedReq := httptest.NewRequest(http.MethodGet, "/v1/pulse/clips?login=routeprincipal&status=dismissed", nil)
	dismissedRec := httptest.NewRecorder()
	r.ServeHTTP(dismissedRec, dismissedReq)
	if dismissedRec.Code != http.StatusOK {
		t.Fatalf("dismissed list status = %d, want 200: %s", dismissedRec.Code, dismissedRec.Body.String())
	}
	var dismissedList ClipCandidateListResponse
	if err := json.Unmarshal(dismissedRec.Body.Bytes(), &dismissedList); err != nil {
		t.Fatalf("decode dismissed list: %v", err)
	}
	if len(dismissedList.Items) != 1 || dismissedList.Items[0].State == nil || dismissedList.Items[0].State.Status != ClipCandidateStatusDismissed {
		t.Fatalf("dismissed list = %+v, want one dismissed local state", dismissedList.Items)
	}
}

func TestPulseClipsRouteConsumesCursorIntegration(t *testing.T) {
	ctx, store := setupClipCandidateStore(t)
	startedAt := time.Date(2026, 7, 4, 20, 0, 0, 0, time.UTC)
	mustExec(t, ctx, store, `
		INSERT INTO analytics_streams (stream_id, login, started_at, title, category)
		VALUES ('stream-cursor-1', 'xqc', $1, 'Cursor stream', 'Just Chatting')
	`, startedAt)
	vodID := "vod-cursor-1"
	candidates := []ClipCandidate{
		{
			ID:            "cc_cursor_high",
			Login:         "xqc",
			StreamID:      "stream-cursor-1",
			VodID:         &vodID,
			OffsetSeconds: 120,
			StartSeconds:  100,
			EndSeconds:    160,
			Score:         95,
			Reason:        "chat_spike",
			SourceKind:    ClipCandidateSourceRecap,
			SourceStatus:  ClipCandidateSourceAvailable,
		},
		{
			ID:            "cc_cursor_low",
			Login:         "xqc",
			StreamID:      "stream-cursor-1",
			VodID:         &vodID,
			OffsetSeconds: 220,
			StartSeconds:  200,
			EndSeconds:    260,
			Score:         80,
			Reason:        "emote_spike",
			SourceKind:    ClipCandidateSourceRecap,
			SourceStatus:  ClipCandidateSourceAvailable,
		},
	}
	if err := store.UpsertClipCandidates(ctx, candidates); err != nil {
		t.Fatalf("upsert candidates: %v", err)
	}
	mustExec(t, ctx, store, `UPDATE clip_candidates SET created_at=$1 WHERE id='cc_cursor_high'`, startedAt.Add(2*time.Minute))
	mustExec(t, ctx, store, `UPDATE clip_candidates SET created_at=$1 WHERE id='cc_cursor_low'`, startedAt.Add(3*time.Minute))

	h := &Handler{store: store}
	r := chi.NewRouter()
	h.PulseRoutes(r)

	firstReq := httptest.NewRequest(http.MethodGet, "/v1/pulse/clips?login=xqc&limit=1", nil)
	firstRec := httptest.NewRecorder()
	r.ServeHTTP(firstRec, firstReq)
	if firstRec.Code != http.StatusOK {
		t.Fatalf("first page status = %d: %s", firstRec.Code, firstRec.Body.String())
	}
	var first ClipCandidateListResponse
	if err := json.Unmarshal(firstRec.Body.Bytes(), &first); err != nil {
		t.Fatalf("decode first page: %v", err)
	}
	if len(first.Items) != 1 || first.Items[0].ID != "cc_cursor_high" || first.NextCursor == "" {
		t.Fatalf("first page = %+v, want high candidate and cursor", first)
	}

	secondReq := httptest.NewRequest(http.MethodGet, "/v1/pulse/clips?login=xqc&limit=1&cursor="+first.NextCursor, nil)
	secondRec := httptest.NewRecorder()
	r.ServeHTTP(secondRec, secondReq)
	if secondRec.Code != http.StatusOK {
		t.Fatalf("second page status = %d: %s", secondRec.Code, secondRec.Body.String())
	}
	var second ClipCandidateListResponse
	if err := json.Unmarshal(secondRec.Body.Bytes(), &second); err != nil {
		t.Fatalf("decode second page: %v", err)
	}
	if len(second.Items) != 1 || second.Items[0].ID != "cc_cursor_low" {
		t.Fatalf("second page = %+v, want low candidate after cursor", second)
	}
}

func setupClipCandidateStore(t *testing.T) (context.Context, *Store) {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	if os.Getenv("INTEGRATION") == "" {
		t.Skip("set INTEGRATION=1 to run clip candidate integration tests")
	}
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://app:test@localhost:15432/emotes?sslmode=disable"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	admin, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("admin pgxpool.New: %v", err)
	}
	t.Cleanup(admin.Close)
	schema := fmt.Sprintf("clip_candidates_test_%d", time.Now().UnixNano())
	if _, err := admin.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatalf("create schema: %v", err)
	}
	t.Cleanup(func() {
		_, _ = admin.Exec(context.Background(), `DROP SCHEMA IF EXISTS `+schema+` CASCADE`)
	})

	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatalf("parse config: %v", err)
	}
	if cfg.ConnConfig.RuntimeParams == nil {
		cfg.ConnConfig.RuntimeParams = map[string]string{}
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatalf("test pgxpool.NewWithConfig: %v", err)
	}
	t.Cleanup(pool.Close)
	store := NewStore(pool)
	mustExec(t, ctx, store, `
		CREATE TABLE analytics_streams (
			stream_id TEXT PRIMARY KEY,
			canonical_stream_id TEXT,
			broadcaster_id TEXT NOT NULL DEFAULT '',
			login TEXT NOT NULL,
			display_name TEXT NOT NULL DEFAULT '',
			profile_image_url TEXT NOT NULL DEFAULT '',
			description TEXT NOT NULL DEFAULT '',
			started_at TIMESTAMPTZ NOT NULL,
			title TEXT,
			category TEXT,
			tags JSONB NOT NULL DEFAULT '[]'::jsonb,
			language TEXT NOT NULL DEFAULT '',
			thumbnail_url TEXT NOT NULL DEFAULT '',
			ended_at TIMESTAMPTZ,
			last_seen_at TIMESTAMPTZ NOT NULL DEFAULT now(),
			current_viewers INT NOT NULL DEFAULT 0,
			avg_viewers INT NOT NULL DEFAULT 0,
			peak_viewers INT NOT NULL DEFAULT 0,
			viewer_samples INT NOT NULL DEFAULT 0,
			chat_messages BIGINT NOT NULL DEFAULT 0,
			total_emote_uses BIGINT NOT NULL DEFAULT 0,
			seventv_emote_uses BIGINT NOT NULL DEFAULT 0,
			vod_id TEXT NOT NULL DEFAULT '',
			vod_source TEXT NOT NULL DEFAULT '',
			viewer_source TEXT NOT NULL DEFAULT 'unknown',
			created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
		)
	`)
	mustExec(t, ctx, store, `
		CREATE TABLE analytics_stream_aliases (
			alias_stream_id TEXT PRIMARY KEY,
			canonical_stream_id TEXT NOT NULL,
			alias_kind TEXT NOT NULL DEFAULT 'test'
		)
	`)
	mustExec(t, ctx, store, `
		CREATE TABLE analytics_minute_rollups (
			stream_id TEXT NOT NULL REFERENCES analytics_streams(stream_id) ON DELETE CASCADE,
			minute_ts TIMESTAMPTZ NOT NULL,
			viewer_avg INT NOT NULL DEFAULT 0,
			viewer_max INT NOT NULL DEFAULT 0,
			viewer_latest INT NOT NULL DEFAULT 0,
			viewer_samples INT NOT NULL DEFAULT 0,
			chat_count INT NOT NULL DEFAULT 0,
			total_emote_count INT NOT NULL DEFAULT 0,
			seventv_emote_count INT NOT NULL DEFAULT 0,
			emotes_json JSONB NOT NULL DEFAULT '{}'::jsonb,
			chat_source TEXT NOT NULL DEFAULT '',
			source_confidence TEXT NOT NULL DEFAULT '',
			chat_source_detail TEXT NOT NULL DEFAULT '',
			updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
			PRIMARY KEY (stream_id, minute_ts)
		)
	`)
	for _, migration := range []string{
		"000062_auto_clipper_candidates.up.sql",
		"000063_auto_clipper_replayforge_jobs.up.sql",
	} {
		body, err := os.ReadFile(filepath.Join("..", "..", "migrations", migration))
		if err != nil {
			t.Fatalf("read migration %s: %v", migration, err)
		}
		if _, err := pool.Exec(ctx, string(body)); err != nil {
			t.Fatalf("apply migration %s: %v", migration, err)
		}
	}
	return ctx, store
}
