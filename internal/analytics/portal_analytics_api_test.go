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
	resp := portalMinutesFromRollups(stream, rollups)
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

func TestPortalStreamMinutesEmptyRollups(t *testing.T) {
	stream := &StreamRecord{
		StreamID:  "123",
		Login:     "xqc",
		StartedAt: time.Date(2026, 6, 25, 18, 0, 0, 0, time.UTC),
	}
	resp := portalMinutesFromRollups(stream, nil)
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

func TestAllowPortalSummaryRateLimitFailOpen(t *testing.T) {
	rl := NewPulseRateLimiter(nil, 10, 5)
	ok, _ := rl.AllowPortalSummary(t.Context(), "principal-a")
	if !ok {
		t.Fatal("expected fail-open when redis nil")
	}
}
