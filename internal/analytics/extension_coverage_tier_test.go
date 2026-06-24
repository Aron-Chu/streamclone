package analytics

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
)

func TestMapExtensionCoverageTierActive(t *testing.T) {
	tier, reasons := mapExtensionCoverageTier(extensionCoverageInputs{
		login:    "xqc",
		tracking: true,
		hostedCap: ExtensionHostedCapStatus{ActiveLimit: 10, ActiveAvailable: true},
	})
	if tier != CoverageTierActiveLiveCoverage {
		t.Fatalf("tier = %q, want %q", tier, CoverageTierActiveLiveCoverage)
	}
	if len(reasons) == 0 || reasons[0] != reasonActiveCollector {
		t.Fatalf("reasons = %#v", reasons)
	}
}

func TestMapExtensionCoverageTierMetadataOnly(t *testing.T) {
	now := time.Now().UTC()
	tier, reasons := mapExtensionCoverageTier(extensionCoverageInputs{
		login: "shroud",
		stream: &StreamRecord{
			StreamID:       "s1",
			Title:          "Just chatting",
			CurrentViewers: 12000,
			StartedAt:      now.Add(-30 * time.Minute),
			LastSeenAt:     now,
			ViewerSource:   ViewerSourceLive,
		},
		isLive: true,
		hostedCap: ExtensionHostedCapStatus{ActiveLimit: 10, ActiveAvailable: true},
	})
	if tier != CoverageTierTop500MetadataOnly {
		t.Fatalf("tier = %q, want %q", tier, CoverageTierTop500MetadataOnly)
	}
	if reasons[0] != reasonMetadataWithoutChat {
		t.Fatalf("reasons = %#v", reasons)
	}
}

func TestMapExtensionCoverageTierStaleMetadataDowngrades(t *testing.T) {
	stale := time.Now().UTC().Add(-2 * time.Hour)
	tier, reasons := mapExtensionCoverageTier(extensionCoverageInputs{
		login: "stalechan",
		stream: &StreamRecord{
			StreamID:       "s1",
			Title:          "Old title",
			CurrentViewers: 500,
			StartedAt:      stale,
			LastSeenAt:     stale,
			ViewerSource:   ViewerSourceLive,
		},
		isLive: true,
		hostedCap: ExtensionHostedCapStatus{ActiveAvailable: true},
	})
	if tier != CoverageTierOnDemandAvailable {
		t.Fatalf("tier = %q, want %q", tier, CoverageTierOnDemandAvailable)
	}
	if len(reasons) == 0 || reasons[0] != reasonMetadataStale {
		t.Fatalf("reasons = %#v, want metadata_stale", reasons)
	}
}

func TestMapExtensionCoverageTierOfflineMetadataNotTop500(t *testing.T) {
	ended := time.Now().UTC().Add(-24 * time.Hour)
	tier, reasons := mapExtensionCoverageTier(extensionCoverageInputs{
		login: "offlinechan",
		stream: &StreamRecord{
			StreamID: "s1",
			Title:    "Yesterday stream",
			EndedAt:  &ended,
		},
		isLive: false,
		hostedCap: ExtensionHostedCapStatus{ActiveAvailable: true},
	})
	if tier != CoverageTierOnDemandAvailable {
		t.Fatalf("tier = %q, want %q", tier, CoverageTierOnDemandAvailable)
	}
	if len(reasons) == 0 || reasons[0] != reasonMetadataOffline {
		t.Fatalf("reasons = %#v, want metadata_offline_not_live", reasons)
	}
}

func TestMapExtensionCoverageTierBudgetLimited(t *testing.T) {
	active := 10
	tier, reasons := mapExtensionCoverageTier(extensionCoverageInputs{
		login: "newchan",
		hostedCap: ExtensionHostedCapStatus{
			ActiveLimit:     10,
			ActiveCount:     &active,
			ActiveAvailable: false,
		},
	})
	if tier != CoverageTierBudgetLimited {
		t.Fatalf("tier = %q, want %q", tier, CoverageTierBudgetLimited)
	}
	if reasons[0] != reasonHostedCapFull {
		t.Fatalf("reasons = %#v", reasons)
	}
}

func TestMapExtensionCoverageTierUnknown(t *testing.T) {
	tier, reasons := mapExtensionCoverageTier(extensionCoverageInputs{
		login:     "unknownchan",
		hostedCap: ExtensionHostedCapStatus{ActiveAvailable: true},
	})
	if tier != CoverageTierUnknownOrUnsupported {
		t.Fatalf("tier = %q, want %q", tier, CoverageTierUnknownOrUnsupported)
	}
	if reasons[0] != reasonNoStreamRecord {
		t.Fatalf("reasons = %#v", reasons)
	}
}

func TestMapExtensionCoverageTierOnDemand(t *testing.T) {
	tier, _ := mapExtensionCoverageTier(extensionCoverageInputs{
		login: "smallstreamer",
		stream: &StreamRecord{
			StreamID: "s-small",
		},
		hostedCap: ExtensionHostedCapStatus{ActiveAvailable: true},
	})
	if tier != CoverageTierOnDemandAvailable {
		t.Fatalf("tier = %q, want %q", tier, CoverageTierOnDemandAvailable)
	}
}

func TestMapExtensionCoverageTierHistoricalEnriched(t *testing.T) {
	ended := time.Now().UTC().Add(-2 * time.Hour)
	tier, reasons := mapExtensionCoverageTier(extensionCoverageInputs{
		login: "historychan",
		stream: &StreamRecord{
			StreamID: "s-old",
			EndedAt:  &ended,
		},
		historicalChat: true,
		hostedCap:      ExtensionHostedCapStatus{ActiveAvailable: true},
	})
	if tier != CoverageTierHistoricalEnriched {
		t.Fatalf("tier = %q, want %q", tier, CoverageTierHistoricalEnriched)
	}
	if reasons[0] != reasonHistoricalAvailable {
		t.Fatalf("reasons = %#v", reasons)
	}
}

func TestExtensionCoverageResponseShape(t *testing.T) {
	now := time.Now().UTC()
	resp := assembleExtensionCoverageResponse(extensionCoverageInputs{
		login: "xqc",
		stream: &StreamRecord{
			StreamID:       "s1",
			BroadcasterID:  "123",
			DisplayName:    "xQc",
			Title:          "Live title",
			Category:       "Just Chatting",
			CurrentViewers: 50000,
			StartedAt:      now,
			LastSeenAt:     now,
		},
		isLive:   true,
		tracking: true,
		hostedCap: ExtensionHostedCapStatus{
			ActiveLimit:     10,
			ActiveCount:     intPtr(3),
			ActiveAvailable: true,
		},
	})

	raw, err := json.Marshal(resp)
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{
		"login", "channelId", "displayName", "coverageTier", "hostedCap",
		"liveMetadata", "dataAvailability", "actions", "reasonCodes",
	} {
		if _, ok := decoded[key]; !ok {
			t.Fatalf("missing key %q in response", key)
		}
	}
	if decoded["coverageTier"] != CoverageTierActiveLiveCoverage {
		t.Fatalf("coverageTier = %#v", decoded["coverageTier"])
	}
}

func TestExtensionCoverageEndpointInvalidLogin(t *testing.T) {
	h := &Handler{}
	r := chi.NewRouter()
	h.ExtensionRoutes(r)

	req := httptest.NewRequest(http.MethodGet, "/v1/extension/pulse/channels/INVALID!!/coverage", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestExtensionCoverageEndpointNilStoreUnknown(t *testing.T) {
	c := NewCollector(&fakeStore{}, fakeProvider{}, &fakeJoiner{}, nil, nilLogger(), 10, time.Hour, time.Hour, 200)
	h := &Handler{collector: c, pulseHosted: PulseHostedConfig{MaxActiveChannels: 10}}
	r := chi.NewRouter()
	h.ExtensionRoutes(r)

	req := httptest.NewRequest(http.MethodGet, "/v1/extension/pulse/channels/unknownchan/coverage", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	var body ExtensionCoverageTierResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.CoverageTier != CoverageTierUnknownOrUnsupported {
		t.Fatalf("coverageTier = %q", body.CoverageTier)
	}
}

func TestExtensionCoverageEndpointCapFull(t *testing.T) {
	c := NewCollector(&fakeStore{}, fakeProvider{}, &fakeJoiner{}, nil, nilLogger(), 2, time.Hour, time.Hour, 200)
	c.tracked["one"] = &trackedChannel{login: "one"}
	c.tracked["two"] = &trackedChannel{login: "two"}
	h := &Handler{collector: c, pulseHosted: PulseHostedConfig{MaxActiveChannels: 2}}
	r := chi.NewRouter()
	h.ExtensionRoutes(r)

	req := httptest.NewRequest(http.MethodGet, "/v1/extension/pulse/channels/newchan/coverage", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	var body ExtensionCoverageTierResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.CoverageTier != CoverageTierBudgetLimited {
		t.Fatalf("coverageTier = %q", body.CoverageTier)
	}
	if body.Actions.CanStartTracking {
		t.Fatal("expected canStartTracking=false when cap full")
	}
}

func TestExtensionCoverageEndpointActiveTracked(t *testing.T) {
	c := NewCollector(&fakeStore{}, fakeProvider{}, &fakeJoiner{}, nil, nilLogger(), 10, time.Hour, time.Hour, 200)
	c.tracked["xqc"] = &trackedChannel{login: "xqc", currentStreamID: "stream-active"}
	h := &Handler{collector: c, pulseHosted: PulseHostedConfig{MaxActiveChannels: 10}}
	r := chi.NewRouter()
	h.ExtensionRoutes(r)

	req := httptest.NewRequest(http.MethodGet, "/v1/extension/pulse/channels/xqc/coverage", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	var body ExtensionCoverageTierResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.CoverageTier != CoverageTierActiveLiveCoverage {
		t.Fatalf("coverageTier = %q", body.CoverageTier)
	}
}

func TestExtensionCoverageActionsNoSideEffects(t *testing.T) {
	in := extensionCoverageInputs{
		login:    "xqc",
		tracking: false,
		stream:   &StreamRecord{StreamID: "s1", Title: "T"},
		isLive:   true,
		hostedCap: ExtensionHostedCapStatus{ActiveAvailable: true},
	}
	tier, _ := mapExtensionCoverageTier(in)
	actions := buildExtensionCoverageActions(in, tier)
	if actions.CanBackfillMissedMoments {
		t.Fatal("metadata-only path must not allow backfill actions")
	}
}

func TestAssembleExtensionCoverageBenchmarkReadModel(t *testing.T) {
	in := extensionCoverageInputs{
		login: "bench",
		stream: &StreamRecord{
			StreamID:       "s1",
			Title:          "Bench",
			CurrentViewers: 1000,
			StartedAt:      time.Now().UTC(),
			LastSeenAt:     time.Now().UTC(),
		},
		isLive: true,
		hostedCap: ExtensionHostedCapStatus{
			ActiveLimit:     10,
			ActiveAvailable: true,
		},
	}
	start := time.Now()
	for i := 0; i < 5000; i++ {
		_ = assembleExtensionCoverageResponse(in)
	}
	if elapsed := time.Since(start); elapsed > 200*time.Millisecond {
		t.Fatalf("5000 read-model assemblies took %v, want well under 200ms", elapsed)
	}
}

func TestExtensionCoverageBudgetLimitedActions(t *testing.T) {
	active := 10
	in := extensionCoverageInputs{
		login: "blocked",
		stream: &StreamRecord{
			StreamID:       "s1",
			Title:          "Popular stream",
			CurrentViewers: 20000,
		},
		isLive: true,
		hostedCap: ExtensionHostedCapStatus{
			ActiveLimit:     10,
			ActiveCount:     &active,
			ActiveAvailable: false,
		},
	}
	tier, _ := mapExtensionCoverageTier(in)
	if tier != CoverageTierBudgetLimited {
		t.Fatalf("tier = %q", tier)
	}
	actions := buildExtensionCoverageActions(in, tier)
	if actions.CanLoadChatAnalytics || actions.CanStartTracking {
		t.Fatalf("actions = %+v", actions)
	}
}
