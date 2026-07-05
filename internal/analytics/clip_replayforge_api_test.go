package analytics

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"streamclone/internal/config"
)

type fakeReplayForgeClient struct {
	calls       int
	statusCalls int
	got         ReplayForgeTriggerRequest
	resp        ReplayForgeTriggerResponse
	statusResp  ReplayForgeJobStatusResponse
	err         error
	statusErr   error
}

func (f *fakeReplayForgeClient) TriggerManual(_ context.Context, req ReplayForgeTriggerRequest) (ReplayForgeTriggerResponse, error) {
	f.calls++
	f.got = req
	if f.err != nil {
		return ReplayForgeTriggerResponse{}, f.err
	}
	if f.resp.JobID == "" {
		f.resp = ReplayForgeTriggerResponse{Status: "queued", JobID: "rf_route_1"}
	}
	return f.resp, nil
}

func (f *fakeReplayForgeClient) GetJob(_ context.Context, _ string) (ReplayForgeJobStatusResponse, error) {
	f.statusCalls++
	if f.statusErr != nil {
		return ReplayForgeJobStatusResponse{}, f.statusErr
	}
	if f.statusResp.Job == nil {
		f.statusResp = ReplayForgeJobStatusResponse{
			Job: map[string]interface{}{
				"id":    "rf_route_1",
				"state": "queued",
			},
		}
	}
	return f.statusResp, nil
}

func TestPulseClipReplayForgeRoutePreflightsSourceIntegration(t *testing.T) {
	ctx, store := setupClipCandidateStore(t)
	startedAt := time.Date(2026, 7, 4, 22, 0, 0, 0, time.UTC)
	mustExec(t, ctx, store, `
		INSERT INTO analytics_streams (stream_id, login, started_at, title, category)
		VALUES ('stream-rf-route-1', 'xqc', $1, 'Route stream', 'Just Chatting')
	`, startedAt)
	candidate := ClipCandidate{
		ID:            "cc_route_missing",
		Login:         "xqc",
		StreamID:      "stream-rf-route-1",
		OffsetSeconds: 120,
		StartSeconds:  100,
		EndSeconds:    160,
		Score:         93,
		Reason:        "chat_spike",
		SourceKind:    ClipCandidateSourceRecap,
		SourceStatus:  ClipCandidateSourceMissing,
	}
	if err := store.UpsertClipCandidates(ctx, []ClipCandidate{candidate}); err != nil {
		t.Fatalf("upsert candidate: %v", err)
	}
	fake := &fakeReplayForgeClient{}
	h := &Handler{store: store, replayForge: fake}
	r := chi.NewRouter()
	h.PulseRoutes(r)

	req := httptest.NewRequest(http.MethodPost, "/v1/pulse/clips/cc_route_missing/replayforge", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409: %s", rec.Code, rec.Body.String())
	}
	if fake.calls != 0 {
		t.Fatalf("ReplayForge called %d times for missing source", fake.calls)
	}
	var job ClipCandidateJob
	if err := json.Unmarshal(rec.Body.Bytes(), &job); err != nil {
		t.Fatalf("decode job: %v", err)
	}
	if job.Status != ClipCandidateJobSourceUnavailable || job.ErrorCode != "source_unavailable" {
		t.Fatalf("job = %+v, want source_unavailable", job)
	}
}

func TestPulseClipReplayForgeRouteRefreshesJobStatusIntegration(t *testing.T) {
	ctx, store := setupClipCandidateStore(t)
	startedAt := time.Date(2026, 7, 4, 22, 0, 0, 0, time.UTC)
	mustExec(t, ctx, store, `
		INSERT INTO analytics_streams (stream_id, login, started_at, title, category)
		VALUES ('stream-rf-route-3', 'xqc', $1, 'Route stream', 'Just Chatting')
	`, startedAt)
	vodID := "vod-route-3"
	candidate := ClipCandidate{
		ID:            "cc_route_refresh",
		Login:         "xqc",
		StreamID:      "stream-rf-route-3",
		VodID:         &vodID,
		OffsetSeconds: 120,
		StartSeconds:  100,
		EndSeconds:    160,
		Score:         93,
		Reason:        "chat_spike",
		SourceKind:    ClipCandidateSourceRecap,
		SourceStatus:  ClipCandidateSourceAvailable,
	}
	if err := store.UpsertClipCandidates(ctx, []ClipCandidate{candidate}); err != nil {
		t.Fatalf("upsert candidate: %v", err)
	}
	if _, err := store.UpsertClipCandidateJob(ctx, ClipCandidateJob{
		ID:               newClipCandidateJobID(candidate.ID, "local"),
		CandidateID:      candidate.ID,
		PrincipalID:      "local",
		PrincipalKind:    "guest",
		Status:           ClipCandidateJobQueued,
		ReplayForgeJobID: "rf_route_3",
		ReplayForgeState: "queued",
		Request:          BuildReplayForgeTriggerFromCandidate(candidate, ClipCandidateState{}),
		Response:         map[string]interface{}{"status": "queued", "job_id": "rf_route_3"},
	}); err != nil {
		t.Fatalf("upsert job: %v", err)
	}
	fake := &fakeReplayForgeClient{
		statusResp: ReplayForgeJobStatusResponse{
			Job: map[string]interface{}{
				"id":                 "rf_route_3",
				"state":              "ready",
				"artifact_available": float64(1),
			},
		},
	}
	h := &Handler{store: store, replayForge: fake}
	r := chi.NewRouter()
	h.PulseRoutes(r)

	req := httptest.NewRequest(http.MethodGet, "/v1/pulse/clips/cc_route_refresh/replayforge", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	if fake.statusCalls != 1 {
		t.Fatalf("ReplayForge status calls = %d, want 1", fake.statusCalls)
	}
	var job ClipCandidateJob
	if err := json.Unmarshal(rec.Body.Bytes(), &job); err != nil {
		t.Fatalf("decode job: %v", err)
	}
	if job.Status != ClipCandidateJobReady || job.ReplayForgeState != "ready" {
		t.Fatalf("job = %+v, want ready mirror", job)
	}
	if job.LastCheckedAt == nil {
		t.Fatalf("lastCheckedAt was not set: %+v", job)
	}
}

func TestPulseClipReplayForgeRouteSubmitsAvailableCandidateIntegration(t *testing.T) {
	ctx, store := setupClipCandidateStore(t)
	startedAt := time.Date(2026, 7, 4, 22, 0, 0, 0, time.UTC)
	mustExec(t, ctx, store, `
		INSERT INTO analytics_streams (stream_id, login, started_at, title, category)
		VALUES ('stream-rf-route-2', 'xqc', $1, 'Route stream', 'Just Chatting')
	`, startedAt)
	vodID := "vod-route-2"
	candidate := ClipCandidate{
		ID:            "cc_route_available",
		Login:         "xqc",
		StreamID:      "stream-rf-route-2",
		VodID:         &vodID,
		OffsetSeconds: 120,
		StartSeconds:  100,
		EndSeconds:    160,
		Score:         93,
		Reason:        "chat_spike",
		SourceKind:    ClipCandidateSourceRecap,
		SourceStatus:  ClipCandidateSourceAvailable,
	}
	if err := store.UpsertClipCandidates(ctx, []ClipCandidate{candidate}); err != nil {
		t.Fatalf("upsert candidate: %v", err)
	}
	fake := &fakeReplayForgeClient{resp: ReplayForgeTriggerResponse{Status: "queued", JobID: "rf_route_2"}}
	h := &Handler{store: store, replayForge: fake}
	r := chi.NewRouter()
	h.PulseRoutes(r)

	req := httptest.NewRequest(http.MethodPost, "/v1/pulse/clips/cc_route_available/replayforge", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202: %s", rec.Code, rec.Body.String())
	}
	if fake.calls != 1 {
		t.Fatalf("ReplayForge calls = %d, want 1", fake.calls)
	}
	if fake.got.MomentContext["candidate_id"] != "cc_route_available" || fake.got.MomentContext["vod_id"] != "vod-route-2" {
		t.Fatalf("ReplayForge request context = %#v", fake.got.MomentContext)
	}
	var job ClipCandidateJob
	if err := json.Unmarshal(rec.Body.Bytes(), &job); err != nil {
		t.Fatalf("decode job: %v", err)
	}
	if job.Status != ClipCandidateJobQueued || job.ReplayForgeJobID != "rf_route_2" {
		t.Fatalf("job = %+v, want queued rf_route_2", job)
	}
}

func TestReplayForgeWebhookUpdatesMirroredJobsIntegration(t *testing.T) {
	ctx, store := setupClipCandidateStore(t)
	startedAt := time.Date(2026, 7, 4, 23, 0, 0, 0, time.UTC)
	mustExec(t, ctx, store, `
		INSERT INTO analytics_streams (stream_id, login, started_at, title, category)
		VALUES ('stream-rf-webhook-1', 'xqc', $1, 'Webhook stream', 'Just Chatting')
	`, startedAt)
	vodID := "vod-webhook-1"
	candidate := ClipCandidate{
		ID:            "cc_webhook_ready",
		Login:         "xqc",
		StreamID:      "stream-rf-webhook-1",
		VodID:         &vodID,
		OffsetSeconds: 120,
		StartSeconds:  100,
		EndSeconds:    160,
		Score:         93,
		Reason:        "chat_spike",
		SourceKind:    ClipCandidateSourceRecap,
		SourceStatus:  ClipCandidateSourceAvailable,
	}
	if err := store.UpsertClipCandidates(ctx, []ClipCandidate{candidate}); err != nil {
		t.Fatalf("upsert candidate: %v", err)
	}
	if _, err := store.UpsertClipCandidateJob(ctx, ClipCandidateJob{
		ID:               newClipCandidateJobID(candidate.ID, "principal-a"),
		CandidateID:      candidate.ID,
		PrincipalID:      "principal-a",
		PrincipalKind:    "beta",
		Status:           ClipCandidateJobQueued,
		ReplayForgeJobID: "rf_webhook_1",
		ReplayForgeState: "queued",
		Request:          BuildReplayForgeTriggerFromCandidate(candidate, ClipCandidateState{}),
		Response:         map[string]interface{}{"status": "queued", "job_id": "rf_webhook_1"},
	}); err != nil {
		t.Fatalf("upsert job: %v", err)
	}
	if _, err := store.UpsertClipCandidateJob(ctx, ClipCandidateJob{
		ID:               newClipCandidateJobID(candidate.ID, "principal-b"),
		CandidateID:      candidate.ID,
		PrincipalID:      "principal-b",
		PrincipalKind:    "beta",
		Status:           ClipCandidateJobQueued,
		ReplayForgeJobID: "rf_webhook_1",
		ReplayForgeState: "queued",
		Request:          BuildReplayForgeTriggerFromCandidate(candidate, ClipCandidateState{}),
		Response:         map[string]interface{}{"status": "queued", "job_id": "rf_webhook_1"},
	}); err != nil {
		t.Fatalf("upsert second job: %v", err)
	}
	body := `{"job":{"id":"rf_webhook_1","state":"ready","artifact_available":1,"final_path":"/tmp/final.mp4","raw_path":"C:\\clips\\raw.mp4","storage_key":"clips/private/final.mp4","signed_url":"https://r2.example/final.mp4?token=secret","access_token":"secret"},"events":[{"state":"ready","signed_url":"https://r2.example/event.mp4?token=secret"}]}`
	clipperOnly := &Handler{
		store: store,
		appConfig: config.Config{
			ClipperWebhookToken: "hook-secret",
		},
		pulseHosted: PulseHostedConfig{Hosted: true},
	}
	clipperOnlyRouter := chi.NewRouter()
	clipperOnly.registerReplayForgeWebhookRoutes(clipperOnlyRouter)
	unconfiguredReq := httptest.NewRequest(http.MethodPost, "/v1/internal/replayforge/jobs/rf_webhook_1", strings.NewReader(body))
	unconfiguredReq.Header.Set("Authorization", "Bearer hook-secret")
	unconfiguredRec := httptest.NewRecorder()
	clipperOnlyRouter.ServeHTTP(unconfiguredRec, unconfiguredReq)
	if unconfiguredRec.Code != http.StatusServiceUnavailable {
		t.Fatalf("clipper-token-only status = %d, want 503: %s", unconfiguredRec.Code, unconfiguredRec.Body.String())
	}
	h := &Handler{
		store: store,
		appConfig: config.Config{
			ReplayForgeCallbackToken: "hook-secret",
		},
		pulseHosted: PulseHostedConfig{Hosted: true},
	}
	r := chi.NewRouter()
	h.registerReplayForgeWebhookRoutes(r)

	unauthReq := httptest.NewRequest(http.MethodPost, "/v1/internal/replayforge/jobs/rf_webhook_1", strings.NewReader(body))
	unauthRec := httptest.NewRecorder()
	r.ServeHTTP(unauthRec, unauthReq)
	if unauthRec.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized status = %d, want 401: %s", unauthRec.Code, unauthRec.Body.String())
	}

	wrongReq := httptest.NewRequest(http.MethodPost, "/v1/internal/replayforge/jobs/rf_webhook_1", strings.NewReader(body))
	wrongReq.Header.Set("Authorization", "Bearer wrong-secret")
	wrongRec := httptest.NewRecorder()
	r.ServeHTTP(wrongRec, wrongReq)
	if wrongRec.Code != http.StatusUnauthorized {
		t.Fatalf("wrong-token status = %d, want 401: %s", wrongRec.Code, wrongRec.Body.String())
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/internal/replayforge/jobs/rf_webhook_1", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer hook-secret")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var response struct {
		Updated int `json:"updated"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.Updated != 2 {
		t.Fatalf("response = %+v, want two mirrored jobs updated", response)
	}
	assertJSONOmitsForbiddenFields(t, rec.Body.Bytes(), []string{
		"items", "principal", "principalId", "principalKind", "final_path", "raw_path", "storage_key", "signed_url", "access_token", "/tmp", "token=secret",
	})
	for _, principalID := range []string{"principal-a", "principal-b"} {
		got, err := store.GetClipCandidateJob(ctx, candidate.ID, principalID)
		if err != nil {
			t.Fatalf("get stored job for %s: %v", principalID, err)
		}
		if got.Status != ClipCandidateJobReady || got.ReplayForgeState != "ready" || got.LastCheckedAt == nil {
			t.Fatalf("stored job for %s = %+v, want ready with lastCheckedAt", principalID, got)
		}
		body, err := json.Marshal(got)
		if err != nil {
			t.Fatalf("marshal stored job for %s: %v", principalID, err)
		}
		assertJSONOmitsForbiddenFields(t, body, []string{
			"principalId", "principalKind", "final_path", "raw_path", "storage_key", "signed_url", "access_token", "/tmp", `C:\`, "token=secret",
		})
	}

	items, _, err := store.ListClipCandidates(ctx, ListClipCandidatesFilter{
		StreamID:    candidate.StreamID,
		PrincipalID: "principal-a",
		Limit:       10,
	})
	if err != nil {
		t.Fatalf("list stored candidates: %v", err)
	}
	listBody, err := json.Marshal(items)
	if err != nil {
		t.Fatalf("marshal list response: %v", err)
	}
	assertJSONOmitsForbiddenFields(t, listBody, []string{
		"principalId", "principalKind", "final_path", "raw_path", "storage_key", "signed_url", "access_token", "/tmp", `C:\`, "token=secret",
	})

	staleReq := httptest.NewRequest(http.MethodPost, "/v1/internal/replayforge/jobs/rf_webhook_1", strings.NewReader(`{"job":{"id":"rf_webhook_1","state":"queued","signed_url":"https://r2.example/stale.mp4?token=secret"}}`))
	staleReq.Header.Set("Authorization", "Bearer hook-secret")
	staleRec := httptest.NewRecorder()
	r.ServeHTTP(staleRec, staleReq)
	if staleRec.Code != http.StatusOK {
		t.Fatalf("stale callback status = %d, want 200: %s", staleRec.Code, staleRec.Body.String())
	}
	got, err := store.GetClipCandidateJob(ctx, candidate.ID, "principal-a")
	if err != nil {
		t.Fatalf("get job after stale callback: %v", err)
	}
	if got.Status != ClipCandidateJobReady || got.ReplayForgeState != "ready" {
		t.Fatalf("stale queued callback downgraded stored job: %+v", got)
	}
}

func TestReplayForgeArtifactRegistrationStoresSafePrivateMetadataIntegration(t *testing.T) {
	ctx, store := setupClipCandidateStore(t)
	startedAt := time.Date(2026, 7, 5, 1, 0, 0, 0, time.UTC)
	mustExec(t, ctx, store, `
		INSERT INTO analytics_streams (stream_id, login, started_at, title, category)
		VALUES ('stream-rf-artifact-1', 'xqc', $1, 'Artifact stream', 'Just Chatting')
	`, startedAt)
	vodID := "vod-artifact-1"
	candidate := ClipCandidate{
		ID:            "cc_artifact_ready",
		Login:         "xqc",
		StreamID:      "stream-rf-artifact-1",
		VodID:         &vodID,
		OffsetSeconds: 120,
		StartSeconds:  100,
		EndSeconds:    160,
		Score:         93,
		Reason:        "chat_spike",
		SourceKind:    ClipCandidateSourceRecap,
		SourceStatus:  ClipCandidateSourceAvailable,
	}
	if err := store.UpsertClipCandidates(ctx, []ClipCandidate{candidate}); err != nil {
		t.Fatalf("upsert candidate: %v", err)
	}
	if _, err := store.UpsertClipCandidateJob(ctx, ClipCandidateJob{
		ID:               newClipCandidateJobID(candidate.ID, "principal-artifact"),
		CandidateID:      candidate.ID,
		PrincipalID:      "principal-artifact",
		PrincipalKind:    "beta",
		Status:           ClipCandidateJobQueued,
		ReplayForgeJobID: "rf_artifact_1",
		ReplayForgeState: "ready",
		Request:          BuildReplayForgeTriggerFromCandidate(candidate, ClipCandidateState{}),
		Response:         map[string]interface{}{"status": "ready", "job_id": "rf_artifact_1", "artifact_available": true},
	}); err != nil {
		t.Fatalf("upsert job: %v", err)
	}
	h := &Handler{
		store: store,
		appConfig: config.Config{
			ReplayForgeCallbackToken: "artifact-secret",
		},
		pulseHosted: PulseHostedConfig{Hosted: true},
	}
	r := chi.NewRouter()
	h.registerReplayForgeWebhookRoutes(r)
	body := `{"artifacts":[{"kind":"final","storage_provider":"r2","storage_key":"clips/channel=xqc/stream=stream-rf-artifact-1/candidate=cc_artifact_ready/final.mp4","media_type":"video/mp4","byte_size":12345,"checksum":"sha256:test","visibility":"private","signed_url":"https://r2.example/final.mp4?token=secret","local_path":"/tmp/replayforge/final.mp4"}]}`

	req := httptest.NewRequest(http.MethodPost, "/v1/internal/replayforge/jobs/rf_artifact_1/artifacts", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer artifact-secret")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	assertJSONOmitsForbiddenFields(t, rec.Body.Bytes(), []string{
		"storage_key", "signed_url", "local_path", "token=secret", "/tmp", "principalId", "principalKind",
	})
	if !strings.Contains(rec.Body.String(), `"/v1/pulse/clips/cc_artifact_ready/artifacts/`) {
		t.Fatalf("artifact response does not include private read path: %s", rec.Body.String())
	}

	got, err := store.GetClipCandidateJob(ctx, candidate.ID, "principal-artifact")
	if err != nil {
		t.Fatalf("get job after artifact registration: %v", err)
	}
	if got.Status != ClipCandidateJobReady {
		t.Fatalf("job status = %q, want ready after private artifact registration", got.Status)
	}
	jobBody, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("marshal job: %v", err)
	}
	assertJSONOmitsForbiddenFields(t, jobBody, []string{
		"storage_key", "signed_url", "local_path", "token=secret", "/tmp", "principalId", "principalKind",
	})
}
