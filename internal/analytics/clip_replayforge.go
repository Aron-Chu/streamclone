package analytics

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	ClipCandidateJobQueued            = "queued"
	ClipCandidateJobReady             = "ready"
	ClipCandidateJobFailed            = "failed"
	ClipCandidateJobSourceUnavailable = "source_unavailable"
)

type ReplayForgeClient interface {
	TriggerManual(ctx context.Context, req ReplayForgeTriggerRequest) (ReplayForgeTriggerResponse, error)
	GetJob(ctx context.Context, jobID string) (ReplayForgeJobStatusResponse, error)
}

type ReplayForgeTriggerRequest struct {
	Channel       string                 `json:"channel"`
	Title         string                 `json:"title,omitempty"`
	Duration      int                    `json:"duration,omitempty"`
	FinalDuration int                    `json:"final_duration,omitempty"`
	MomentContext map[string]interface{} `json:"moment_context,omitempty"`
}

type ReplayForgeTriggerResponse struct {
	Status        string `json:"status"`
	JobID         string `json:"job_id"`
	ExistingJobID string `json:"existing_job_id,omitempty"`
	Reason        string `json:"reason,omitempty"`
}

type ReplayForgeJobStatusResponse struct {
	Job    map[string]interface{}   `json:"job"`
	Events []map[string]interface{} `json:"events,omitempty"`
}

func (r ReplayForgeJobStatusResponse) State() string {
	if r.Job == nil {
		return ""
	}
	value, _ := r.Job["state"].(string)
	return strings.TrimSpace(value)
}

func (r ReplayForgeJobStatusResponse) ResponseMap() map[string]interface{} {
	out := map[string]interface{}{}
	if r.Job != nil {
		job := map[string]interface{}{}
		copyReplayForgeJobField(job, r.Job, "id")
		copyReplayForgeJobField(job, r.Job, "state")
		copyReplayForgeJobField(job, r.Job, "artifact_available")
		copyReplayForgeJobField(job, r.Job, "failure_code")
		copyReplayForgeJobField(job, r.Job, "error_code")
		copyReplayForgeJobField(job, r.Job, "error_message")
		copyReplayForgeJobField(job, r.Job, "message")
		copyReplayForgeJobField(job, r.Job, "reason")
		if len(job) > 0 {
			out["job"] = job
		}
	}
	return out
}

func copyReplayForgeJobField(dst, src map[string]interface{}, key string) {
	if dst == nil || src == nil {
		return
	}
	value, ok := src[key]
	if !ok || value == nil {
		return
	}
	switch v := value.(type) {
	case string:
		dst[key] = sanitizeReplayForgeStatusText(v)
	case bool, int, int64, float64, float32:
		dst[key] = v
	}
}

func sanitizeReplayForgeStatusText(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	lower := strings.ToLower(value)
	for _, marker := range []string{"://", "token=", "access_token", "signed_url", "storage_key", "/tmp/", "\\tmp\\", ":\\"} {
		if strings.Contains(lower, marker) {
			return "redacted"
		}
	}
	if strings.HasPrefix(value, "/") || strings.HasPrefix(value, "\\") {
		return "redacted"
	}
	if len(value) > 300 {
		return value[:300]
	}
	return value
}

type ClipCandidateJob struct {
	ID               string                    `json:"id"`
	CandidateID      string                    `json:"candidateId"`
	PrincipalID      string                    `json:"-"`
	PrincipalKind    string                    `json:"-"`
	Status           string                    `json:"status"`
	ReplayForgeJobID string                    `json:"replayForgeJobId,omitempty"`
	ReplayForgeState string                    `json:"replayForgeState,omitempty"`
	Request          ReplayForgeTriggerRequest `json:"request,omitempty"`
	Response         map[string]interface{}    `json:"response,omitempty"`
	ErrorCode        string                    `json:"errorCode,omitempty"`
	ErrorMessage     string                    `json:"errorMessage,omitempty"`
	SubmittedAt      *time.Time                `json:"submittedAt,omitempty"`
	LastCheckedAt    *time.Time                `json:"lastCheckedAt,omitempty"`
	CreatedAt        time.Time                 `json:"createdAt,omitempty"`
	UpdatedAt        time.Time                 `json:"updatedAt,omitempty"`
}

type ReplayForgeHTTPClient struct {
	baseURL    string
	token      string
	httpClient *http.Client
}

func NewReplayForgeHTTPClient(baseURL, token string) *ReplayForgeHTTPClient {
	return &ReplayForgeHTTPClient{
		baseURL:    strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		token:      strings.TrimSpace(token),
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

func (c *ReplayForgeHTTPClient) TriggerManual(ctx context.Context, req ReplayForgeTriggerRequest) (ReplayForgeTriggerResponse, error) {
	if c == nil || c.baseURL == "" {
		return ReplayForgeTriggerResponse{}, errors.New("replayforge_unconfigured")
	}
	endpoint, err := url.JoinPath(c.baseURL, "/v1/triggers/manual")
	if err != nil {
		return ReplayForgeTriggerResponse{}, err
	}
	body, err := json.Marshal(req)
	if err != nil {
		return ReplayForgeTriggerResponse{}, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return ReplayForgeTriggerResponse{}, err
	}
	httpReq.Header.Set("Accept", "application/json")
	httpReq.Header.Set("Content-Type", "application/json")
	if c.token != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.token)
	}
	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return ReplayForgeTriggerResponse{}, err
	}
	defer resp.Body.Close()
	var payload ReplayForgeTriggerResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return ReplayForgeTriggerResponse{}, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		if payload.Status == "" {
			payload.Status = fmt.Sprintf("http_%d", resp.StatusCode)
		}
		return ReplayForgeTriggerResponse{}, fmt.Errorf("replayforge_%s", payload.Status)
	}
	if strings.TrimSpace(payload.JobID) == "" && strings.TrimSpace(payload.ExistingJobID) != "" {
		payload.JobID = strings.TrimSpace(payload.ExistingJobID)
	}
	if strings.TrimSpace(payload.JobID) == "" {
		return ReplayForgeTriggerResponse{}, errors.New("replayforge_missing_job_id")
	}
	return payload, nil
}

func (c *ReplayForgeHTTPClient) GetJob(ctx context.Context, jobID string) (ReplayForgeJobStatusResponse, error) {
	if c == nil || c.baseURL == "" {
		return ReplayForgeJobStatusResponse{}, errors.New("replayforge_unconfigured")
	}
	jobID = strings.TrimSpace(jobID)
	if jobID == "" {
		return ReplayForgeJobStatusResponse{}, errors.New("replayforge_missing_job_id")
	}
	endpoint, err := url.JoinPath(c.baseURL, "/v1/jobs", jobID)
	if err != nil {
		return ReplayForgeJobStatusResponse{}, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return ReplayForgeJobStatusResponse{}, err
	}
	httpReq.Header.Set("Accept", "application/json")
	if c.token != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.token)
	}
	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return ReplayForgeJobStatusResponse{}, err
	}
	defer resp.Body.Close()
	var payload ReplayForgeJobStatusResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return ReplayForgeJobStatusResponse{}, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		state := payload.State()
		if state == "" {
			state = fmt.Sprintf("http_%d", resp.StatusCode)
		}
		return ReplayForgeJobStatusResponse{}, fmt.Errorf("replayforge_%s", state)
	}
	if payload.Job == nil {
		return ReplayForgeJobStatusResponse{}, errors.New("replayforge_missing_job")
	}
	return payload, nil
}

func clipCandidateJobStatusFromReplayForgeStatus(status ReplayForgeJobStatusResponse) string {
	switch strings.TrimSpace(strings.ToLower(status.State())) {
	case "ready":
		if replayForgeArtifactAvailable(status.Job) {
			return ClipCandidateJobReady
		}
		return ClipCandidateJobQueued
	case "failed", "purged":
		return ClipCandidateJobFailed
	default:
		return ClipCandidateJobQueued
	}
}

func applyReplayForgeStatusToClipCandidateJob(job *ClipCandidateJob, status ReplayForgeJobStatusResponse) {
	if job == nil {
		return
	}
	incomingStatus := clipCandidateJobStatusFromReplayForgeStatus(status)
	if clipCandidateJobPreservesTerminal(job.Status, incomingStatus) {
		job.LastCheckedAt = timeNowPtr()
		return
	}
	job.Status = incomingStatus
	job.ReplayForgeState = status.State()
	job.Response = status.ResponseMap()
	if incomingStatus == ClipCandidateJobFailed {
		job.ErrorCode, job.ErrorMessage = replayForgeStatusFailure(status)
	} else {
		job.ErrorCode = ""
		job.ErrorMessage = ""
	}
	job.LastCheckedAt = timeNowPtr()
}

func clipCandidateJobPreservesTerminal(existingStatus, incomingStatus string) bool {
	switch strings.TrimSpace(strings.ToLower(existingStatus)) {
	case ClipCandidateJobReady, ClipCandidateJobFailed:
		return incomingStatus == ClipCandidateJobQueued
	default:
		return false
	}
}

func replayForgeStatusFailure(status ReplayForgeJobStatusResponse) (string, string) {
	if clipCandidateJobStatusFromReplayForgeStatus(status) != ClipCandidateJobFailed {
		return "", ""
	}
	code := replayForgeStatusString(status.Job, "failure_code", "error_code", "code", "reason")
	message := replayForgeStatusString(status.Job, "error_message", "message", "error", "reason")
	state := strings.TrimSpace(status.State())
	if code == "" {
		code = state
	}
	if message == "" && state != "" {
		message = "ReplayForge reported " + state
	}
	return code, message
}

func replayForgeStatusString(job map[string]interface{}, keys ...string) string {
	if job == nil {
		return ""
	}
	for _, key := range keys {
		value, _ := job[key].(string)
		if strings.TrimSpace(value) != "" {
			return sanitizeReplayForgeStatusText(value)
		}
	}
	return ""
}

func replayForgeArtifactAvailable(job map[string]interface{}) bool {
	if job == nil {
		return false
	}
	switch value := job["artifact_available"].(type) {
	case bool:
		return value
	case int:
		return value != 0
	case int64:
		return value != 0
	case float64:
		return value != 0
	case string:
		v := strings.TrimSpace(strings.ToLower(value))
		return v == "1" || v == "true" || v == "yes"
	default:
		return false
	}
}

func BuildReplayForgeTriggerFromCandidate(candidate ClipCandidate, state ClipCandidateState) ReplayForgeTriggerRequest {
	startSeconds := candidate.StartSeconds
	endSeconds := candidate.EndSeconds
	if state.StartSecondsOverride != nil {
		startSeconds = *state.StartSecondsOverride
	}
	if state.EndSecondsOverride != nil {
		endSeconds = *state.EndSecondsOverride
	}
	if endSeconds <= startSeconds {
		endSeconds = startSeconds + 5
	}
	duration := clipMaxInt(5, endSeconds-startSeconds)
	if duration > 90 {
		duration = 90
	}
	finalDuration := duration
	if finalDuration > 60 {
		finalDuration = 60
	}
	title := strings.TrimSpace(candidate.StreamTitle)
	if state.TitleOverride != nil && strings.TrimSpace(*state.TitleOverride) != "" {
		title = strings.TrimSpace(*state.TitleOverride)
	}
	if title == "" {
		title = candidate.Login + " clip"
	}
	ctx := map[string]interface{}{
		"candidate_id":       candidate.ID,
		"stream_id":          candidate.StreamID,
		"vod_offset_seconds": float64(candidate.OffsetSeconds),
		"clip_start_seconds": float64(startSeconds),
		"clip_end_seconds":   float64(endSeconds),
		"moment_score":       float64(candidate.Score),
		"confidence":         candidate.Confidence,
		"pick_reason":        candidate.Reason,
		"chat_per_min":       float64(candidate.ChatCount),
		"emote_count":        float64(candidate.EmoteCount),
		"viewer_count":       float64(candidate.ViewerCount),
		"source_kind":        candidate.SourceKind,
		"source_status":      candidate.SourceStatus,
	}
	if candidate.VodID != nil && strings.TrimSpace(*candidate.VodID) != "" {
		ctx["vod_id"] = strings.TrimSpace(*candidate.VodID)
	}
	if candidate.MinuteTS != nil {
		ctx["minute_ts"] = candidate.MinuteTS.UTC().Format(time.RFC3339)
	}
	if candidate.StreamCategory != "" {
		ctx["category"] = candidate.StreamCategory
	}
	if len(candidate.TopEmotes) > 0 {
		emotes := make([]map[string]interface{}, 0, len(candidate.TopEmotes))
		for _, emote := range candidate.TopEmotes {
			item := map[string]interface{}{
				"name":     emote.Name,
				"count":    float64(emote.Count),
				"provider": emote.Provider,
			}
			if emote.ID != "" {
				item["id"] = emote.ID
			}
			if emote.ImageURL != "" {
				item["image_url"] = emote.ImageURL
			}
			emotes = append(emotes, item)
		}
		ctx["top_emotes"] = emotes
	}
	return ReplayForgeTriggerRequest{
		Channel:       candidate.Login,
		Title:         title,
		Duration:      duration,
		FinalDuration: finalDuration,
		MomentContext: ctx,
	}
}

func newClipCandidateJobID(candidateID, principalID string) string {
	sum := sha1.Sum([]byte(candidateID + ":" + principalID))
	return "ccj_" + hex.EncodeToString(sum[:8])
}
