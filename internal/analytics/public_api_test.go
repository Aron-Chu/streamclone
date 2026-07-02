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

func TestPublicStatsResponseShape(t *testing.T) {
	payload := PublicStatsResponse{
		StreamsTracked:        10,
		MomentsDetected:       20,
		ChatMessagesProcessed: 30,
		EmotesIndexed:         40,
		VodsAnalyzed:          5,
		UpdatedAt:             time.Now().UTC(),
	}
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	if forbidden := forbiddenPublicJSONFields(body); len(forbidden) > 0 {
		t.Fatalf("unexpected fields in stats payload: %v", forbidden)
	}
	var decoded map[string]any
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"streamsTracked", "momentsDetected", "chatMessagesProcessed", "emotesIndexed", "vodsAnalyzed", "updatedAt"} {
		if _, ok := decoded[key]; !ok {
			t.Fatalf("missing key %q", key)
		}
	}
}

func TestPublicStatusResponseShape(t *testing.T) {
	payload := PublicStatusResponse{
		Status:    "operational",
		API:       "up",
		Degraded:  false,
		Incident:  nil,
		UpdatedAt: time.Now().UTC(),
	}
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	if forbidden := forbiddenPublicJSONFields(body); len(forbidden) > 0 {
		t.Fatalf("unexpected fields in status payload: %v", forbidden)
	}
}

func TestPublicRoutesUnauthenticated(t *testing.T) {
	h := &Handler{
		pulseHosted: PulseHostedConfig{Hosted: true, BetaKeys: []string{"secret-one"}},
	}
	r := chi.NewRouter()
	h.Routes(r)

	for _, path := range []string{"/v1/public/stats", "/v1/public/status"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		if rec.Code == http.StatusUnauthorized {
			t.Fatalf("%s should be public, got 401", path)
		}
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/pulse/bookmarks", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("gated pulse route status = %d, want 401", rec.Code)
	}
}

func TestPublicStatsCacheHit(t *testing.T) {
	body, _ := json.Marshal(PublicStatsResponse{
		StreamsTracked: 42,
		UpdatedAt:      time.Now().UTC(),
	})
	var cached PublicStatsResponse
	if err := json.Unmarshal(body, &cached); err != nil {
		t.Fatal(err)
	}
	if cached.StreamsTracked != 42 {
		t.Fatalf("cache decode streamsTracked = %d, want 42", cached.StreamsTracked)
	}
}

func TestPublicAggregateStatsMomentsDetectedUsesPeakRollups(t *testing.T) {
	query := strings.ToLower(publicAggregateStatsQuery)
	if strings.Contains(query, "pulse_bookmarks") {
		t.Fatal("momentsDetected must not count user bookmarks")
	}
	for _, want := range []string{"analytics_minute_rollups", "chat_source_detail", "chat_count > 0"} {
		if !strings.Contains(query, want) {
			t.Fatalf("momentsDetected query missing %q: %s", want, publicAggregateStatsQuery)
		}
	}
}

func TestPublicHubPrewarmOptionsInclude24h(t *testing.T) {
	got := map[int]bool{}
	for _, opts := range publicHubPrewarmOptions() {
		got[normalizePublicHubOptions(opts).ActivityWindowMinutes] = true
	}
	for _, want := range []int{hubActivityWindowMinutes, 24 * 60, 7 * 24 * 60} {
		if !got[want] {
			t.Fatalf("prewarm options missing %d minutes: %+v", want, got)
		}
	}
}

func forbiddenPublicJSONFields(body []byte) []string {
	var decoded map[string]any
	if err := json.Unmarshal(body, &decoded); err != nil {
		return []string{"invalid_json"}
	}
	banned := []string{"login", "principal", "principalId", "queue", "cap", "channelCount", "channels", "active", "max"}
	found := make([]string, 0)
	for _, key := range banned {
		if _, ok := decoded[key]; ok {
			found = append(found, key)
		}
	}
	for key := range decoded {
		lower := strings.ToLower(key)
		if strings.Contains(lower, "principal") || strings.Contains(lower, "login") {
			found = append(found, key)
		}
	}
	return found
}
