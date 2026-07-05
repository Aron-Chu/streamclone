package analytics

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	"streamclone/internal/config"
)

func TestCorpusReadinessRouteReportsCriticalWithoutStore(t *testing.T) {
	h := &Handler{}
	r := chi.NewRouter()
	h.Routes(r)

	req := httptest.NewRequest(http.MethodGet, "/v1/corpus/readiness", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rec.Code)
	}
	var payload CorpusReadinessReport
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode readiness: %v", err)
	}
	if payload.Status != CorpusStatusCritical {
		t.Fatalf("status = %q, want critical", payload.Status)
	}
	if payload.Components.Database.Status != CorpusStatusCritical {
		t.Fatalf("database status = %q, want critical", payload.Components.Database.Status)
	}
}

func TestCorpusReadinessInternalRouteAlias(t *testing.T) {
	h := &Handler{}
	r := chi.NewRouter()
	h.Routes(r)

	req := httptest.NewRequest(http.MethodGet, "/v1/internal/corpus/readiness", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rec.Code)
	}
	var payload CorpusReadinessReport
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode readiness: %v", err)
	}
	if payload.Desired.TargetTopN != DefaultTop500MetadataTopN {
		t.Fatalf("desired target topN = %d, want default", payload.Desired.TargetTopN)
	}
}

func TestCorpusPipelineStateFromReadinessFlagsCriticalTrackerFailures(t *testing.T) {
	cfg := CorpusRuntimeConfig{
		MetadataEnabled:      true,
		MetadataWriteEnabled: true,
		LiveAdmissionEnabled: true,
	}
	report := Top100ReadinessReport{
		CollectorMax: 50,
		Summary: Top100ReadinessSummary{
			LiveRows:              95,
			CollectorTrackingRows: 2,
			ExpectedCollectorRows: 50,
			MetadataStaleRows:     95,
		},
	}
	if got := corpusPipelineStateFromReadiness(cfg, report); got != CorpusStatusCritical {
		t.Fatalf("state with stale metadata = %q, want critical", got)
	}

	report.Summary.MetadataStaleRows = 0
	report.Summary.AdmissionDisabledRows = 95
	if got := corpusPipelineStateFromReadiness(cfg, report); got != CorpusStatusCritical {
		t.Fatalf("state with disabled admission rows = %q, want critical", got)
	}
}

func TestCorpusPipelineStateFromReadinessFlagsCollectorDeficit(t *testing.T) {
	cfg := CorpusRuntimeConfig{
		MetadataEnabled:      true,
		MetadataWriteEnabled: true,
		LiveAdmissionEnabled: true,
	}
	report := Top100ReadinessReport{
		CollectorMax: 50,
		Summary: Top100ReadinessSummary{
			LiveRows:                 95,
			CollectorTrackingRows:    40,
			ExpectedCollectorRows:    50,
			LiveCollectorDeficitRows: 10,
		},
	}
	if got := corpusPipelineStateFromReadiness(cfg, report); got != CorpusStatusDegraded {
		t.Fatalf("state with collector deficit = %q, want degraded", got)
	}
}

func TestQueueReadinessFlagsEligibleButEmptyQueue(t *testing.T) {
	report := CorpusReadinessReport{}
	component := queueReadinessComponent("Silver", true, "silver", CorpusBackfillTierSummary{Eligible: 3}, &report)

	if component.Status != CorpusStatusDegraded {
		t.Fatalf("queue status = %q, want degraded", component.Status)
	}
	if len(report.Issues) != 1 || report.Issues[0].Code != "silver_eligible_but_queue_empty" {
		t.Fatalf("issues = %#v, want silver eligible empty queue issue", report.Issues)
	}
	if !testContainsString(component.ReasonCodes, "silver_queue_empty_with_eligible_streams") {
		t.Fatalf("reason codes = %#v, want Phase 2 silver queue reason", component.ReasonCodes)
	}
}

func TestQueueReadinessExplainsDisabledQueue(t *testing.T) {
	report := CorpusReadinessReport{}
	component := queueReadinessComponent("Gold", false, "gold", CorpusBackfillTierSummary{Eligible: 2}, &report)

	if component.Status != CorpusStatusDisabled {
		t.Fatalf("queue status = %q, want disabled", component.Status)
	}
	if len(report.Issues) != 0 {
		t.Fatalf("disabled queue should explain via component, not issues: %#v", report.Issues)
	}
}

func TestIRCCollectorReadinessFlagsZeroChatAfterAge(t *testing.T) {
	report := CorpusReadinessReport{}
	component := ircCollectorReadinessComponent(Top100ReadinessReport{
		CollectorMax: 10,
		Summary: Top100ReadinessSummary{
			LiveRows:              20,
			CollectorTrackingRows: 10,
			ZeroChatAfterAgeRows:  4,
		},
	}, &report)

	if component.Status != CorpusStatusDegraded {
		t.Fatalf("irc status = %q, want degraded", component.Status)
	}
	if len(report.Issues) != 1 || report.Issues[0].Code != "zero_chat_after_age" {
		t.Fatalf("issues = %#v, want zero-chat issue", report.Issues)
	}
	if !testContainsString(component.ReasonCodes, "rollup_collapse") {
		t.Fatalf("reason codes = %#v, want rollup_collapse", component.ReasonCodes)
	}
}

func TestEmoteHistoryCorpusComponentMapsStaleness(t *testing.T) {
	report := CorpusReadinessReport{}
	component := emoteHistoryCorpusComponent(EmoteHistoryJobConfig{SnapshotEnabled: true, NormalizeEnabled: true}, EmoteHistoryReadinessResponse{
		Status:      emoteHistoryStatusDegraded,
		ReasonCodes: []string{"no_recent_snapshots", "no_recent_normalized_usage"},
	}, &report)

	if component.Status != CorpusStatusDegraded {
		t.Fatalf("status = %q, want degraded", component.Status)
	}
	if !testContainsString(component.ReasonCodes, "emote_history_stale") {
		t.Fatalf("reason codes = %#v, want emote_history_stale", component.ReasonCodes)
	}
	if len(report.Issues) != 1 || report.Issues[0].Code != "emote_history_stale" {
		t.Fatalf("issues = %#v, want emote_history_stale issue", report.Issues)
	}
}

func TestRateLimitReadinessComponentFlagsGQLRateLimit(t *testing.T) {
	report := CorpusReadinessReport{}
	component := rateLimitReadinessComponent(CorpusGoldSegmentSummary{RateLimitedBuckets: 1}, &report)

	if component.Status != CorpusStatusDegraded {
		t.Fatalf("status = %q, want degraded", component.Status)
	}
	if !testContainsString(component.ReasonCodes, "gql_rate_limited") {
		t.Fatalf("reason codes = %#v, want gql_rate_limited", component.ReasonCodes)
	}
	if len(report.Issues) != 1 || report.Issues[0].Code != "gql_rate_limited" {
		t.Fatalf("issues = %#v, want gql_rate_limited issue", report.Issues)
	}
}

func TestCorpusActualStateIncludesGoldSegmentCounts(t *testing.T) {
	report := CorpusReadinessReport{GoldSegments: CorpusGoldSegmentSummary{Queued: 2, Running: 1, Done: 3, Failed: 4, DeadLetter: 5, RateLimitedBuckets: 1}}
	actual := corpusActualState(report)

	if actual.GoldSegmentsQueued != 2 || actual.GoldSegmentsRunning != 1 || actual.GoldSegmentsDone != 3 || actual.GoldSegmentsFailed != 4 || actual.GoldSegmentsDeadLetter != 5 || actual.GQLRateLimitedBuckets != 1 {
		t.Fatalf("actual gold segments = %+v", actual)
	}
}

func testContainsString(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}

func TestCorpusRuntimeConfigFromAppPreservesLiveAdmissionTopN5000(t *testing.T) {
	cfg := config.Config{
		Top500MetadataTopN:       1000,
		PulseTop500AdmissionTopN: 5000,
		PulseMaxActiveChannels:   5000,
	}
	runtime := CorpusRuntimeConfigFromApp(cfg)
	if runtime.TargetTopN != 1000 {
		t.Fatalf("TargetTopN = %d, want metadata cap 1000", runtime.TargetTopN)
	}
	if runtime.LiveAdmissionTopN != 5000 {
		t.Fatalf("LiveAdmissionTopN = %d, want 5000", runtime.LiveAdmissionTopN)
	}
	if runtime.MaxActiveIRCChannels != 5000 {
		t.Fatalf("MaxActiveIRCChannels = %d, want 5000", runtime.MaxActiveIRCChannels)
	}
}
