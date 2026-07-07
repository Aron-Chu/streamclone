package analytics

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestBuildReplayForgeTriggerFromCandidatePreservesMomentContext(t *testing.T) {
	vodID := "12345"
	minuteTS := time.Date(2026, 7, 4, 21, 30, 0, 0, time.UTC)
	candidate := ClipCandidate{
		ID:             "cc_replayforge",
		Login:          "xqc",
		StreamID:       "stream-1",
		VodID:          &vodID,
		StreamTitle:    "Late night set",
		StreamCategory: "Just Chatting",
		MinuteTS:       &minuteTS,
		OffsetSeconds:  1234,
		StartSeconds:   1210,
		EndSeconds:     1270,
		Score:          94,
		Confidence:     0.87,
		Reason:         "emote_spike",
		ChatCount:      240,
		EmoteCount:     190,
		ViewerCount:    42000,
		SourceKind:     ClipCandidateSourceRecap,
		SourceStatus:   ClipCandidateSourceAvailable,
		TopEmotes: []ClipCandidateEmote{{
			Provider: "seventv",
			ID:       "7tv-1",
			Name:     "KEKW",
			Count:    90,
			ImageURL: "https://cdn.example/kekw.webp",
		}},
	}

	req := BuildReplayForgeTriggerFromCandidate(candidate, ClipCandidateState{
		Status: ClipCandidateStatusSaved,
	})

	if req.Channel != "xqc" {
		t.Fatalf("channel = %q, want xqc", req.Channel)
	}
	if req.Title != "Late night set" {
		t.Fatalf("title = %q, want stream title", req.Title)
	}
	if req.Duration != 60 || req.FinalDuration != 60 {
		t.Fatalf("duration/final_duration = %d/%d, want 60/60", req.Duration, req.FinalDuration)
	}
	ctx := req.MomentContext
	assertMomentContextString(t, ctx, "candidate_id", "cc_replayforge")
	assertMomentContextString(t, ctx, "stream_id", "stream-1")
	assertMomentContextString(t, ctx, "vod_id", "12345")
	assertMomentContextString(t, ctx, "pick_reason", "emote_spike")
	assertMomentContextFloat(t, ctx, "vod_offset_seconds", 1234)
	assertMomentContextFloat(t, ctx, "clip_start_seconds", 1210)
	assertMomentContextFloat(t, ctx, "clip_end_seconds", 1270)
	assertMomentContextFloat(t, ctx, "moment_score", 94)
	emotes, ok := ctx["top_emotes"].([]map[string]interface{})
	if !ok || len(emotes) != 1 {
		t.Fatalf("top_emotes = %#v, want one mapped emote", ctx["top_emotes"])
	}
	if emotes[0]["image_url"] != "https://cdn.example/kekw.webp" {
		t.Fatalf("top emote image_url = %#v", emotes[0]["image_url"])
	}
}

func TestReplayForgeHTTPClientSendsBearerAndParsesQueuedJob(t *testing.T) {
	var gotPath, gotAuth string
	var gotBody ReplayForgeTriggerRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		writeJSON(w, http.StatusAccepted, map[string]string{"status": "queued", "job_id": "rf_123"})
	}))
	defer srv.Close()

	client := NewReplayForgeHTTPClient(srv.URL, "secret-token")
	resp, err := client.TriggerManual(context.Background(), ReplayForgeTriggerRequest{
		Channel: "xqc",
		Title:   "Good bit",
		MomentContext: map[string]interface{}{
			"candidate_id": "cc_1",
		},
	})
	if err != nil {
		t.Fatalf("TriggerManual: %v", err)
	}
	if gotPath != "/v1/triggers/manual" {
		t.Fatalf("path = %q, want /v1/triggers/manual", gotPath)
	}
	if gotAuth != "Bearer secret-token" {
		t.Fatalf("authorization = %q, want bearer token", gotAuth)
	}
	if gotBody.Channel != "xqc" || gotBody.Title != "Good bit" {
		t.Fatalf("body = %+v", gotBody)
	}
	if resp.Status != "queued" || resp.JobID != "rf_123" {
		t.Fatalf("response = %+v", resp)
	}
}

func TestReplayForgeHTTPClientTreatsSuppressedDuplicateAsExistingJob(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusAccepted, map[string]string{
			"status":          "suppressed",
			"reason":          "duplicate",
			"existing_job_id": "rf_existing",
		})
	}))
	defer srv.Close()

	client := NewReplayForgeHTTPClient(srv.URL, "")
	resp, err := client.TriggerManual(context.Background(), ReplayForgeTriggerRequest{
		Channel: "xqc",
	})
	if err != nil {
		t.Fatalf("TriggerManual suppressed duplicate: %v", err)
	}
	if resp.Status != "suppressed" || resp.JobID != "rf_existing" || resp.ExistingJobID != "rf_existing" {
		t.Fatalf("response = %+v, want suppressed existing job", resp)
	}
}

func TestReplayForgeHTTPClientFetchesJobStatus(t *testing.T) {
	var gotPath, gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"job": map[string]interface{}{
				"id":                 "rf_123",
				"state":              "ready",
				"artifact_available": float64(1),
			},
			"events": []map[string]interface{}{{"state": "ready"}},
		})
	}))
	defer srv.Close()

	client := NewReplayForgeHTTPClient(srv.URL, "secret-token")
	resp, err := client.GetJob(context.Background(), "rf_123")
	if err != nil {
		t.Fatalf("GetJob: %v", err)
	}
	if gotPath != "/v1/jobs/rf_123" {
		t.Fatalf("path = %q, want /v1/jobs/rf_123", gotPath)
	}
	if gotAuth != "Bearer secret-token" {
		t.Fatalf("authorization = %q, want bearer token", gotAuth)
	}
	if resp.State() != "ready" {
		t.Fatalf("state = %q, want ready", resp.State())
	}
	if resp.Job["id"] != "rf_123" {
		t.Fatalf("job = %#v", resp.Job)
	}
}

func TestReplayForgeStatusMappingRequiresArtifactForReady(t *testing.T) {
	studioReady := ReplayForgeJobStatusResponse{Job: map[string]interface{}{
		"id":                 "rf_studio",
		"state":              "ready",
		"artifact_available": float64(0),
	}}
	if got := clipCandidateJobStatusFromReplayForgeStatus(studioReady); got != ClipCandidateJobQueued {
		t.Fatalf("studio ready status = %q, want queued until artifact exists", got)
	}
	exportReady := ReplayForgeJobStatusResponse{Job: map[string]interface{}{
		"id":                 "rf_export",
		"state":              "ready",
		"artifact_available": float64(1),
	}}
	if got := clipCandidateJobStatusFromReplayForgeStatus(exportReady); got != ClipCandidateJobReady {
		t.Fatalf("export ready status = %q, want ready", got)
	}
}

func TestReplayForgeStatusResponseMapSanitizesArtifactsAndTokens(t *testing.T) {
	status := ReplayForgeJobStatusResponse{
		Job: map[string]interface{}{
			"id":                 "rf_sensitive",
			"state":              "failed",
			"artifact_available": true,
			"final_path":         "/tmp/replayforge/final.mp4",
			"raw_path":           `C:\clips\raw.mp4`,
			"storage_key":        "clips/private/final.mp4",
			"signed_url":         "https://r2.example/final.mp4?token=secret",
			"access_token":       "secret-token",
			"failure_code":       "download_failed",
			"error_message":      "VOD unavailable",
			"message":            "failed reading /tmp/replayforge/raw.mp4?token=secret",
		},
		Events: []map[string]interface{}{{
			"state":      "failed",
			"signed_url": "https://r2.example/event.mp4?token=secret",
		}},
	}

	body, err := json.Marshal(status.ResponseMap())
	if err != nil {
		t.Fatalf("marshal sanitized status: %v", err)
	}
	assertJSONOmitsForbiddenFields(t, body, []string{
		"final_path", "raw_path", "storage_key", "signed_url", "access_token",
		"/tmp", `C:\`, "token=secret", "events",
	})
	raw := string(body)
	for _, required := range []string{"rf_sensitive", "failed", "download_failed", "VOD unavailable", "redacted", "artifact_available"} {
		if !strings.Contains(raw, required) {
			t.Fatalf("sanitized response missing %q: %s", required, raw)
		}
	}
}

func TestBuildReplayForgeTriggerFromCandidateNormalizesPartialOverrideRange(t *testing.T) {
	vodID := "12345"
	candidate := ClipCandidate{
		ID:            "cc_bad_range",
		Login:         "xqc",
		StreamID:      "stream-1",
		VodID:         &vodID,
		StartSeconds:  100,
		EndSeconds:    160,
		OffsetSeconds: 120,
		Reason:        "chat_spike",
		SourceKind:    ClipCandidateSourceRecap,
		SourceStatus:  ClipCandidateSourceAvailable,
	}
	start := 240
	req := BuildReplayForgeTriggerFromCandidate(candidate, ClipCandidateState{
		StartSecondsOverride: &start,
	})
	if req.MomentContext["clip_start_seconds"] != float64(240) || req.MomentContext["clip_end_seconds"] != float64(245) {
		t.Fatalf("context range = %#v/%#v, want normalized 240/245", req.MomentContext["clip_start_seconds"], req.MomentContext["clip_end_seconds"])
	}
	if req.Duration != 5 || req.FinalDuration != 5 {
		t.Fatalf("duration = %d/%d, want 5/5", req.Duration, req.FinalDuration)
	}
}

func TestClipCandidateReplayForgeJobMirrorIntegration(t *testing.T) {
	ctx, store := setupClipCandidateStore(t)
	startedAt := time.Date(2026, 7, 4, 20, 0, 0, 0, time.UTC)
	mustExec(t, ctx, store, `
		INSERT INTO analytics_streams (stream_id, login, started_at, title, category)
		VALUES ('stream-rf-1', 'xqc', $1, 'ReplayForge stream', 'Just Chatting')
	`, startedAt)
	vodID := "vod-rf-1"
	candidate := ClipCandidate{
		ID:            "cc_replayforge_store",
		Login:         "xqc",
		StreamID:      "stream-rf-1",
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
	request := BuildReplayForgeTriggerFromCandidate(candidate, ClipCandidateState{Status: ClipCandidateStatusSaved})
	job, err := store.UpsertClipCandidateJob(ctx, ClipCandidateJob{
		ID:               newClipCandidateJobID(candidate.ID, "principal-a"),
		CandidateID:      candidate.ID,
		PrincipalID:      "principal-a",
		PrincipalKind:    "beta",
		Status:           ClipCandidateJobQueued,
		ReplayForgeJobID: "rf_123",
		ReplayForgeState: "queued",
		Request:          request,
		Response:         map[string]interface{}{"status": "queued", "job_id": "rf_123"},
	})
	if err != nil {
		t.Fatalf("upsert job: %v", err)
	}
	if job.ID == "" || job.ReplayForgeJobID != "rf_123" {
		t.Fatalf("job = %+v", job)
	}
	got, err := store.GetClipCandidateJob(ctx, candidate.ID, "principal-a")
	if err != nil {
		t.Fatalf("get job: %v", err)
	}
	if got.Status != ClipCandidateJobQueued || got.Request.Channel != "xqc" {
		t.Fatalf("stored job = %+v", got)
	}
	other, err := store.GetClipCandidateJob(ctx, candidate.ID, "principal-b")
	if err == nil || other.ID != "" {
		t.Fatalf("job mirror leaked across principals: job=%+v err=%v", other, err)
	}
}

func TestClipCandidateListIncludesReplayForgeJobIntegration(t *testing.T) {
	ctx, store := setupClipCandidateStore(t)
	startedAt := time.Date(2026, 7, 4, 20, 0, 0, 0, time.UTC)
	mustExec(t, ctx, store, `
		INSERT INTO analytics_streams (stream_id, login, started_at, title, category)
		VALUES ('stream-rf-list-1', 'xqc', $1, 'ReplayForge stream', 'Just Chatting')
	`, startedAt)
	vodID := "vod-rf-list-1"
	candidate := ClipCandidate{
		ID:            "cc_replayforge_list",
		Login:         "xqc",
		StreamID:      "stream-rf-list-1",
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
		ReplayForgeJobID: "rf_list_1",
		ReplayForgeState: "queued",
		Request:          BuildReplayForgeTriggerFromCandidate(candidate, ClipCandidateState{}),
	}); err != nil {
		t.Fatalf("upsert job: %v", err)
	}

	items, _, err := store.ListClipCandidates(ctx, ListClipCandidatesFilter{
		StreamID:    "stream-rf-list-1",
		PrincipalID: "principal-a",
		Limit:       10,
	})
	if err != nil {
		t.Fatalf("list candidates: %v", err)
	}
	if len(items) != 1 || items[0].Job == nil {
		t.Fatalf("items = %+v, want job mirror on candidate", items)
	}
	if items[0].Job.ReplayForgeJobID != "rf_list_1" || items[0].Job.Status != ClipCandidateJobQueued {
		t.Fatalf("candidate job = %+v", items[0].Job)
	}
}

func assertMomentContextString(t *testing.T, ctx map[string]interface{}, key, want string) {
	t.Helper()
	got, _ := ctx[key].(string)
	if got != want {
		t.Fatalf("moment_context[%s] = %#v, want %q", key, ctx[key], want)
	}
}

func assertMomentContextFloat(t *testing.T, ctx map[string]interface{}, key string, want float64) {
	t.Helper()
	got, ok := ctx[key].(float64)
	if !ok || got != want {
		t.Fatalf("moment_context[%s] = %#v, want %v", key, ctx[key], want)
	}
}
