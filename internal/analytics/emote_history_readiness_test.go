package analytics

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
)

func TestEmoteHistoryReadinessRouteReportsUnavailableWithoutStore(t *testing.T) {
	h := (&Handler{}).WithEmoteHistoryJobs(EmoteHistoryJobConfig{SnapshotEnabled: true, NormalizeEnabled: true})
	r := chi.NewRouter()
	h.Routes(r)

	req := httptest.NewRequest(http.MethodGet, "/v1/internal/emote-history/readiness", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rec.Code)
	}
	var payload EmoteHistoryReadinessResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode readiness: %v", err)
	}
	if payload.Status != emoteHistoryStatusUnhealthy {
		t.Fatalf("status = %q, want %q", payload.Status, emoteHistoryStatusUnhealthy)
	}
	if !hasEmoteHistoryReason(payload.ReasonCodes, "store_unavailable") {
		t.Fatalf("reason codes = %v, want store_unavailable", payload.ReasonCodes)
	}
}

func TestEmoteHistoryReadinessFinalStatus(t *testing.T) {
	report := EmoteHistoryReadinessResponse{}
	report.addReason("normalize_job_disabled")
	report.addReason("snapshot_job_disabled")
	report.addReason("normalize_job_disabled")
	report.finalizeStatus()

	if report.Status != emoteHistoryStatusDegraded {
		t.Fatalf("status = %q, want degraded", report.Status)
	}
	if len(report.ReasonCodes) != 2 || report.ReasonCodes[0] != "normalize_job_disabled" || report.ReasonCodes[1] != "snapshot_job_disabled" {
		t.Fatalf("unexpected reason codes: %v", report.ReasonCodes)
	}
	if len(report.Sources) != 1 || report.Sources[0].State != "limited" {
		t.Fatalf("unexpected sources: %+v", report.Sources)
	}
}

func TestEmoteHistoryReadinessConfigReportsIntervals(t *testing.T) {
	h := (&Handler{}).WithEmoteHistoryJobs(EmoteHistoryJobConfig{
		SnapshotEnabled:    true,
		SnapshotInterval:   6 * time.Hour,
		SnapshotBatchSize:  25,
		NormalizeEnabled:   true,
		NormalizeInterval:  15 * time.Minute,
		NormalizeSince:     30 * 24 * time.Hour,
		NormalizeBatchSize: 10,
	})
	report, err := h.emoteHistoryReadiness(t.Context(), time.Date(2026, 6, 25, 20, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("readiness: %v", err)
	}
	if report.Config.SnapshotIntervalSeconds != int64((6*time.Hour).Seconds()) || report.Config.NormalizeIntervalSeconds != int64((15*time.Minute).Seconds()) {
		t.Fatalf("unexpected interval config: %+v", report.Config)
	}
	if report.Config.NormalizeSinceSeconds != int64((30*24*time.Hour).Seconds()) || report.Config.SnapshotBatchSize != 25 || report.Config.NormalizeBatchSize != 10 {
		t.Fatalf("unexpected job config: %+v", report.Config)
	}
}

func TestEmoteHistoryReadinessSanitizedRejectsUnsafeKeys(t *testing.T) {
	if !emoteHistoryReadinessSanitized([]byte(`{"sources":[{"message":"Coverage incomplete"}]}`)) {
		t.Fatal("expected safe status message to pass")
	}
	if emoteHistoryReadinessSanitized([]byte(`{"rawChat":"hello"}`)) {
		t.Fatal("expected rawChat key to fail")
	}
}

func hasEmoteHistoryReason(reasons []string, want string) bool {
	for _, reason := range reasons {
		if reason == want {
			return true
		}
	}
	return false
}
