package analytics

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
)

func TestPortalStreamDetailOmitsRollups(t *testing.T) {
	// Shape-only test — no DB required (handler integration covered in CI with postgres).
	detail := PortalStreamDetail{
		Channel: "xqc",
		State:   "historical",
		Sources: []SourceStatus{{Source: "analytics_db", State: "ready"}},
	}
	body, err := json.Marshal(detail)
	if err != nil {
		t.Fatal(err)
	}
	raw := strings.ToLower(string(body))
	for _, forbidden := range []string{"rollups", "messages", "operator", "gql", "corpus", "archive"} {
		if strings.Contains(raw, forbidden) {
			t.Fatalf("portal detail must not contain %q", forbidden)
		}
	}
}

func TestPortalAnalyticsJSONForbiddenKeys(t *testing.T) {
	detail := PortalStreamDetail{
		Channel: "xqc",
		State:   "historical",
		Sources: []SourceStatus{{Source: "analytics_db", State: "ready"}},
	}
	body, err := json.Marshal(detail)
	if err != nil {
		t.Fatal(err)
	}
	raw := strings.ToLower(string(body))
	for _, forbidden := range []string{"rollups", "messages", "operator", "gql", "corpus", "archive"} {
		if strings.Contains(raw, forbidden) {
			t.Fatalf("portal detail must not contain %q", forbidden)
		}
	}
}

func TestPortalSyncStatusSanitizedShape(t *testing.T) {
	st := PortalSyncStatus{Phase: "completed", Message: "Done"}
	body, err := json.Marshal(st)
	if err != nil {
		t.Fatal(err)
	}
	raw := string(body)
	for _, forbidden := range []string{"network", "tracker", "commentsFetched"} {
		if strings.Contains(raw, forbidden) {
			t.Fatalf("sanitized sync status leaked %q", forbidden)
		}
	}
}

func TestPortalStreamMinutesSanitizedShape(t *testing.T) {
	start := time.Date(2026, 6, 25, 18, 0, 0, 0, time.UTC)
	stream := &StreamRecord{
		StreamID:  "123",
		Login:     "xqc",
		StartedAt: start,
	}
	rollups := []MinuteRollup{{
		MinuteTS:          start.Add(2 * time.Minute),
		ViewerAvg:         100,
		ViewerLatest:      120,
		ChatCount:         50,
		SevenTVEmoteCount: 10,
		Emotes:            map[string]int{"7tv:1:KEKW": 10},
	}}
	resp := portalMinutesFromRollups(stream, rollups, false)
	body, err := json.Marshal(resp)
	if err != nil {
		t.Fatal(err)
	}
	raw := strings.ToLower(string(body))
	for _, forbidden := range []string{"emotes", "rollups", "messages", "operator", "gql", "corpus", "archive"} {
		if strings.Contains(raw, forbidden) {
			t.Fatalf("portal minutes must not contain %q", forbidden)
		}
	}
	if len(resp.Minutes) != 1 || resp.Minutes[0].OffsetSeconds != 120 {
		t.Fatalf("unexpected minute offset: %+v", resp.Minutes)
	}
	if resp.CoverageStartOffsetSeconds != 120 {
		t.Fatalf("expected coverage start 120, got %d", resp.CoverageStartOffsetSeconds)
	}
}

func TestPortalMinutesCacheKeyIncludesProvisionalFlag(t *testing.T) {
	if portalMinutesCacheKey("abc", false) == portalMinutesCacheKey("abc", true) {
		t.Fatal("cache keys must differ when includeProvisionalPeaks changes")
	}
	if !strings.Contains(portalMinutesCacheKey("abc", true), "provisional_peaks") {
		t.Fatal("provisional peaks cache key suffix missing")
	}
}

func TestPortalStreamMinutesEmptyRollups(t *testing.T) {
	stream := &StreamRecord{
		StreamID:  "123",
		Login:     "xqc",
		StartedAt: time.Date(2026, 6, 25, 18, 0, 0, 0, time.UTC),
	}
	resp := portalMinutesFromRollups(stream, nil, false)
	if len(resp.Minutes) != 0 {
		t.Fatalf("expected empty minutes, got %d", len(resp.Minutes))
	}
	body, err := json.Marshal(resp)
	if err != nil {
		t.Fatal(err)
	}
	raw := strings.ToLower(string(body))
	for _, forbidden := range []string{"emotes", "rollups", "messages"} {
		if strings.Contains(raw, forbidden) {
			t.Fatalf("portal minutes must not contain %q", forbidden)
		}
	}
}

func TestPortalChannelEmotesSanitizedShape(t *testing.T) {
	latestUsage := time.Date(2026, 6, 25, 19, 0, 0, 0, time.UTC)
	resp := PortalChannelEmotesResponse{
		Login:   "xqc",
		Range:   "30d",
		AsOf:    latestUsage,
		Sources: []SourceStatus{{Source: "analytics_db", State: "ready"}},
		Coverage: PortalEmoteCoverage{
			ChatCoveragePct:      98.5,
			MinutesWithData:      120,
			NormalizedMinutes:    118,
			IdentityResolvedRows: 50,
			IdentityTotalRows:    52,
		},
		Freshness: PortalEmoteFreshness{
			LatestUsageAt:        &latestUsage,
			ProviderState:        "ready",
			ProviderStalenessSec: 60,
			UsageStalenessSec:    30,
		},
		IdentityResolutionPct: 96.15,
		TotalEmoteUses:        400,
		EmotesPerMinute:       3.39,
		SevenTVSharePct:       82,
		UniqueEmotes:          12,
		TopEmotes: []PortalChannelEmote{{
			Provider:           "seventv",
			ProviderEmoteID:    "abc",
			Name:               "KEKW",
			UseCount:           120,
			MinutesSeen:        42,
			SharePct:           30,
			IdentityResolution: "provider_id",
			Confidence:         100,
		}},
		TopMoments: []PortalEmoteMoment{{
			StreamID:        "stream-1",
			StartedAt:       latestUsage.Add(-time.Hour),
			OffsetSeconds:   120,
			Href:            "/analytics/xqc/s/stream-1?t=120#emotes",
			UseCount:        33,
			TopEmoteName:    "KEKW",
			Provider:        "seventv",
			ProviderEmoteID: "abc",
		}},
	}
	body, err := json.Marshal(resp)
	if err != nil {
		t.Fatal(err)
	}
	raw := strings.ToLower(string(body))
	for _, required := range []string{"coverage", "freshness", "identityresolutionpct", "topemotes", "topmoments"} {
		if !strings.Contains(raw, required) {
			t.Fatalf("portal emotes response missing %q in %s", required, raw)
		}
	}
	for _, forbidden := range []string{"rawchat", "chattext", "message", "fragments", "chatter", "username", "userlogin", "userid", "leaderboard", "operator", "gql", "corpus"} {
		if strings.Contains(raw, forbidden) {
			t.Fatalf("portal emotes response must not contain %q: %s", forbidden, raw)
		}
	}
	assertJSONOmitsForbiddenFields(t, body, []string{"raw", "payload", "chatText", "message", "userId", "chatter", "chatterId", "userLogin", "individualChatterRankings", "leaderboard"})
}

func TestPortalChannelEmotesRouteStoreUnavailable(t *testing.T) {
	h := &Handler{}
	r := chi.NewRouter()
	h.PortalRoutes(r)

	req := httptest.NewRequest(http.MethodGet, "/v1/portal/analytics/channels/xqc/emotes?range=30d", nil)
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
}

func TestPortalStreamMinutesUnauthorizedHosted(t *testing.T) {
	h := &Handler{
		pulseHosted: PulseHostedConfig{Hosted: true, BetaKeys: []string{"secret-one"}},
	}
	r := chi.NewRouter()
	h.PortalRoutes(r)

	req := httptest.NewRequest(http.MethodGet, "/v1/portal/analytics/streams/stream-1/minutes", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestHostedPortalChannelStreamsUnauthorizedWithoutAuth(t *testing.T) {
	h := &Handler{
		pulseHosted: PulseHostedConfig{Hosted: true, BetaKeys: []string{"secret-one"}},
	}
	r := chi.NewRouter()
	h.PortalRoutes(r)

	req := httptest.NewRequest(http.MethodGet, "/v1/portal/analytics/channels/ludwig/streams", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
	var payload map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if payload["error"] != "unauthorized" {
		t.Fatalf("payload = %#v", payload)
	}
	if strings.Contains(strings.ToLower(rec.Body.String()), `"items"`) {
		t.Fatalf("unauthorized response must not include items, got %s", rec.Body.String())
	}
}

func TestHostedPortalChannelStreamsAllowsBetaKey(t *testing.T) {
	h := &Handler{
		pulseHosted: PulseHostedConfig{Hosted: true, BetaKeys: []string{"secret-one"}},
	}
	r := chi.NewRouter()
	h.PortalRoutes(r)

	req := httptest.NewRequest(http.MethodGet, "/v1/portal/analytics/channels/ludwig/streams", nil)
	req.Header.Set("X-Streamclone-Beta-Key", "secret-one")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rec.Code)
	}
	var payload map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if payload["error"] != "store_unavailable" {
		t.Fatalf("payload = %#v, want store_unavailable", payload)
	}
}

func TestNonHostedPortalChannelStreamsAllowsGuest(t *testing.T) {
	h := &Handler{
		pulseHosted: PulseHostedConfig{Hosted: false},
	}
	r := chi.NewRouter()
	h.PortalRoutes(r)

	req := httptest.NewRequest(http.MethodGet, "/v1/portal/analytics/channels/ludwig/streams", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rec.Code)
	}
	var payload map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if payload["error"] != "store_unavailable" {
		t.Fatalf("payload = %#v, want store_unavailable", payload)
	}
}

func TestAllowPortalSummaryRateLimitFailOpen(t *testing.T) {
	rl := NewPulseRateLimiter(nil, 10, 5)
	ok, _ := rl.AllowPortalSummary(t.Context(), "principal-a")
	if !ok {
		t.Fatal("expected fail-open when redis nil")
	}
}

func assertJSONOmitsForbiddenFields(t *testing.T, body []byte, forbidden []string) {
	t.Helper()
	var payload any
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("decode public JSON: %v", err)
	}
	forbiddenSet := map[string]struct{}{}
	for _, key := range forbidden {
		forbiddenSet[strings.ToLower(key)] = struct{}{}
	}
	walkPublicJSONKeys(t, payload, forbiddenSet)
}

func walkPublicJSONKeys(t *testing.T, value any, forbidden map[string]struct{}) {
	t.Helper()
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			if _, blocked := forbidden[strings.ToLower(key)]; blocked {
				t.Fatalf("public JSON contains forbidden field %q", key)
			}
			walkPublicJSONKeys(t, child, forbidden)
		}
	case []any:
		for _, child := range typed {
			walkPublicJSONKeys(t, child, forbidden)
		}
	}
}
