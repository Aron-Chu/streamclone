package analytics

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
)

// TestPublicAPICompatHubShape guards existing /v1/public/hub JSON keys.
func TestPublicAPICompatHubShape(t *testing.T) {
	h := testPublicHubHandler(t)
	req := httptest.NewRequest(http.MethodGet, "/v1/public/hub?activityWindow=30m", nil)
	rec := httptest.NewRecorder()
	h.getPublicHub(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{
		"generatedAt", "poolSize", "corpus", "coverage", "corpusPipeline",
		"activity", "emoteIntel", "topEmotes", "topMovers", "liveChannels", "moments",
	} {
		if _, ok := payload[key]; !ok {
			t.Fatalf("missing required hub key %q", key)
		}
	}
	// Additive ingest block may be absent when engine nil; when present must not break shape.
	if ingest, ok := payload["ingest"]; ok {
		block, ok := ingest.(map[string]any)
		if !ok {
			t.Fatalf("ingest block type = %T", ingest)
		}
		if _, ok := block["state"]; !ok {
			t.Fatal("ingest.state missing")
		}
	}
}

func TestPublicAPICompatHubMomentsShape(t *testing.T) {
	h := testPublicHubHandler(t)
	req := httptest.NewRequest(http.MethodGet, "/v1/public/hub/moments?bucketT=1719000000000&activityWindow=24h", nil)
	rec := httptest.NewRecorder()
	h.getPublicHubMoments(rec, req)
	if rec.Code != http.StatusOK && rec.Code != http.StatusBadRequest && rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("unexpected status = %d", rec.Code)
	}
	if rec.Code == http.StatusOK {
		var payload map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
			t.Fatal(err)
		}
		for _, key := range []string{"bucketT", "moments", "status", "activityWindowMinutes"} {
			if _, ok := payload[key]; !ok {
				t.Fatalf("missing moments key %q", key)
			}
		}
	}
}

func TestPublicAPICompatExtensionHealthShape(t *testing.T) {
	h := &Handler{}
	r := chi.NewRouter()
	h.ExtensionRoutes(r)
	req := httptest.NewRequest(http.MethodGet, "/v1/extension/health", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"ok", "version", "time", "hostedMode", "degraded", "routes", "capabilities"} {
		if _, ok := payload[key]; !ok {
			t.Fatalf("missing health key %q", key)
		}
	}
}

func testPublicHubHandler(t *testing.T) *Handler {
	t.Helper()
	return NewHandler(nil, NewCollector(nil, nil, nil, nil, nil, 50, 0, 0, 0), nil, nil)
}
