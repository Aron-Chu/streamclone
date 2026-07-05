package analytics

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"

	"streamclone/internal/analytics/recap"
)

func (h *Handler) registerPulseClipRoutes(r chi.Router) {
	r.Get("/clips", h.listPulseClips)
	r.Get("/clips/preview", h.previewPulseClips)
	r.Post("/clips/generate", h.generatePulseClips)
	r.Patch("/clips/{id}", h.updatePulseClipCandidateState)
	r.Get("/clips/{id}/replayforge", h.refreshPulseClipReplayForgeJob)
	r.Post("/clips/{id}/replayforge", h.sendPulseClipToReplayForge)
}

func (h *Handler) listPulseClips(w http.ResponseWriter, r *http.Request) {
	principalID := "local"
	if h.pulseHosted.Hosted {
		principal, ok := h.requireHostedPrivateClipsPrincipal(w, r)
		if !ok {
			return
		}
		principalID = principal.ID
	} else if p, ok := pulsePrincipalFromContext(r.Context()); ok {
		principalID = p.ID
	}
	if h.store == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "store_unavailable"})
		return
	}
	query := r.URL.Query()
	opts, err := clipCandidatePreviewOptionsFromQuery(h.clipCandidateBuildOptions(), query)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_controls"})
		return
	}
	filter := ListClipCandidatesFilter{
		Login:        normalizeLogin(query.Get("login")),
		StreamID:     strings.TrimSpace(query.Get("streamId")),
		Status:       strings.TrimSpace(strings.ToLower(query.Get("status"))),
		PrincipalID:  principalID,
		Limit:        parseBookmarkLimit(query.Get("limit")),
		MinChatCount: clipMaxInt(0, opts.MinChatCount),
		MaxChatCount: clipMaxInt(0, opts.MaxChatCount),
	}
	if cursor := strings.TrimSpace(query.Get("cursor")); cursor != "" {
		parsed, legacy, err := parseClipCandidateCursor(cursor)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_cursor"})
			return
		}
		if legacy {
			filter.Before = parsed.CreatedAt
		} else {
			filter.Cursor = &parsed
		}
	}
	if filter.StreamID != "" {
		if err := h.ensureClipCandidatesForStreamWithOptions(r.Context(), filter.StreamID, opts); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "stream_not_found"})
				return
			}
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "clip_candidates_unavailable"})
			return
		}
	} else if shouldSeedRecentClipCandidates(filter) {
		if err := h.ensureRecentClipCandidates(r.Context(), defaultClipSeedStreamLimit); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "clip_candidates_unavailable"})
			return
		}
	}
	items, nextCursor, err := h.store.ListClipCandidates(r.Context(), filter)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "clips_unavailable"})
		return
	}
	writeJSON(w, http.StatusOK, ClipCandidateListResponse{Items: items, NextCursor: nextCursor})
}

func (h *Handler) previewPulseClips(w http.ResponseWriter, r *http.Request) {
	if h.pulseHosted.Hosted {
		if _, ok := h.requireHostedPrivateClipsPrincipal(w, r); !ok {
			return
		}
	}
	query := r.URL.Query()
	streamID := strings.TrimSpace(query.Get("streamId"))
	if streamID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing_stream_id"})
		return
	}
	if h.store == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "store_unavailable"})
		return
	}
	opts, err := clipCandidatePreviewOptionsFromQuery(h.clipCandidateBuildOptions(), query)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_controls"})
		return
	}
	stream, err := h.store.StreamByID(r.Context(), streamID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "stream_not_found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "stream_unavailable"})
		return
	}
	rec, err := h.buildPulseStreamRecap(r.Context(), streamID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "stream_not_found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "clip_preview_unavailable"})
		return
	}
	items := BuildClipCandidatesFromRecap(stream, rec, opts)
	writeJSON(w, http.StatusOK, ClipCandidatePreviewResponse{
		Items:     items,
		Controls:  clipCandidatePreviewControlsFromOptions(opts),
		Source:    clipCandidatePreviewSourceFromRecap(stream, rec),
		Summary:   clipCandidateTuningSummaryFromRecap(rec, items, opts),
		Persisted: false,
	})
}

func (h *Handler) generatePulseClips(w http.ResponseWriter, r *http.Request) {
	if !h.requirePulseWrite(w) {
		return
	}
	if h.pulseHosted.Hosted {
		if _, ok := h.requireHostedPrivateClipsPrincipal(w, r); !ok {
			return
		}
	}
	query := r.URL.Query()
	streamID := strings.TrimSpace(query.Get("streamId"))
	if streamID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing_stream_id"})
		return
	}
	if h.store == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "store_unavailable"})
		return
	}
	opts, err := clipCandidatePreviewOptionsFromQuery(h.clipCandidateBuildOptions(), query)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_controls"})
		return
	}
	stream, rec, items, err := h.buildClipCandidatesForStream(r.Context(), streamID, opts)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "stream_not_found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "clip_candidates_unavailable"})
		return
	}
	if err := h.store.UpsertClipCandidates(r.Context(), items); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "clip_candidates_unavailable"})
		return
	}
	writeJSON(w, http.StatusOK, ClipCandidatePreviewResponse{
		Items:     items,
		Controls:  clipCandidatePreviewControlsFromOptions(opts),
		Source:    clipCandidatePreviewSourceFromRecap(stream, rec),
		Summary:   clipCandidateTuningSummaryFromRecap(rec, items, opts),
		Persisted: true,
	})
}

func (h *Handler) ensureClipCandidatesForStream(ctx context.Context, streamID string) error {
	return h.ensureClipCandidatesForStreamWithOptions(ctx, streamID, h.clipCandidateBuildOptions())
}

func (h *Handler) ensureClipCandidatesForStreamWithOptions(ctx context.Context, streamID string, opts ClipCandidateBuildOptions) error {
	if h == nil || h.store == nil {
		return errors.New("store unavailable")
	}
	_, _, candidates, err := h.buildClipCandidatesForStream(ctx, streamID, opts)
	if err != nil {
		return err
	}
	return h.store.UpsertClipCandidates(ctx, candidates)
}

func (h *Handler) buildClipCandidatesForStream(ctx context.Context, streamID string, opts ClipCandidateBuildOptions) (*StreamRecord, recap.StreamRecap, []ClipCandidate, error) {
	stream, err := h.store.StreamByID(ctx, streamID)
	if err != nil {
		return nil, recap.StreamRecap{}, nil, err
	}
	rec, err := h.buildPulseStreamRecap(ctx, streamID)
	if err != nil {
		return nil, recap.StreamRecap{}, nil, err
	}
	candidates := BuildClipCandidatesFromRecap(stream, rec, opts)
	return stream, rec, candidates, nil
}

func shouldSeedRecentClipCandidates(filter ListClipCandidatesFilter) bool {
	return filter.StreamID == "" &&
		filter.Login == "" &&
		filter.Cursor == nil &&
		filter.Before.IsZero() &&
		(filter.Status == "" || filter.Status == ClipCandidateStatusNew)
}

func (h *Handler) ensureRecentClipCandidates(ctx context.Context, limit int) error {
	if h == nil || h.store == nil {
		return errors.New("store unavailable")
	}
	streamIDs, err := h.store.RecentStreamsForClipCandidateSeeding(ctx, limit)
	if err != nil {
		return err
	}
	for _, streamID := range streamIDs {
		if err := h.ensureClipCandidatesForStream(ctx, streamID); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				continue
			}
			return err
		}
	}
	return nil
}

func clipCandidatePreviewOptionsFromQuery(base ClipCandidateBuildOptions, query url.Values) (ClipCandidateBuildOptions, error) {
	opts := base
	var err error
	if opts.MaxCandidates, err = clipQueryInt(query, opts.MaxCandidates, "maxCandidates", "max"); err != nil {
		return ClipCandidateBuildOptions{}, err
	}
	if opts.MinScore, err = clipQueryInt(query, opts.MinScore, "minScore", "scoreMin"); err != nil {
		return ClipCandidateBuildOptions{}, err
	}
	if opts.MinChatCount, err = clipQueryInt(query, opts.MinChatCount, "minChatCount", "minChat", "chatMin"); err != nil {
		return ClipCandidateBuildOptions{}, err
	}
	if opts.MaxChatCount, err = clipQueryInt(query, opts.MaxChatCount, "maxChatCount", "maxChat", "chatMax"); err != nil {
		return ClipCandidateBuildOptions{}, err
	}
	if opts.MinEmoteCount, err = clipQueryInt(query, opts.MinEmoteCount, "minEmoteCount", "minEmotes", "emoteMin"); err != nil {
		return ClipCandidateBuildOptions{}, err
	}
	if opts.MinProviderEmoteCount, err = clipQueryInt(query, opts.MinProviderEmoteCount, "minProviderEmoteCount", "minProviderEmotes", "providerMin"); err != nil {
		return ClipCandidateBuildOptions{}, err
	}
	if opts.MinNonMissingRollupMinutes, err = clipQueryInt(query, opts.MinNonMissingRollupMinutes, "minNonMissingRollupMinutes", "minRollupMinutes"); err != nil {
		return ClipCandidateBuildOptions{}, err
	}
	if opts.DuplicateRadiusSeconds, err = clipQueryInt(query, opts.DuplicateRadiusSeconds, "duplicateRadiusSeconds", "dedupeSeconds"); err != nil {
		return ClipCandidateBuildOptions{}, err
	}
	if opts.MaxCandidatesPerHour, err = clipQueryInt(query, opts.MaxCandidatesPerHour, "maxCandidatesPerHour", "maxPerHour"); err != nil {
		return ClipCandidateBuildOptions{}, err
	}
	if opts.MinConfidence, err = clipQueryFloat(query, opts.MinConfidence, "minConfidence", "confidenceMin"); err != nil {
		return ClipCandidateBuildOptions{}, err
	}
	if value, ok := firstClipQueryValue(query, "providerEmoteProvider", "provider"); ok {
		opts.ProviderEmoteProvider = strings.TrimSpace(strings.ToLower(value))
	}
	if opts.RequireSourceAvailable, err = clipQueryBool(query, opts.RequireSourceAvailable, "requireSourceAvailable", "sourceRequired"); err != nil {
		return ClipCandidateBuildOptions{}, err
	}
	return opts, nil
}

func clipQueryInt(query url.Values, fallback int, keys ...string) (int, error) {
	value, ok := firstClipQueryValue(query, keys...)
	if !ok {
		return fallback, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < 0 {
		return 0, errors.New("invalid integer")
	}
	return parsed, nil
}

func clipQueryFloat(query url.Values, fallback float64, keys ...string) (float64, error) {
	value, ok := firstClipQueryValue(query, keys...)
	if !ok {
		return fallback, nil
	}
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil || parsed < 0 || parsed > 1 {
		return 0, errors.New("invalid float")
	}
	return parsed, nil
}

func clipQueryBool(query url.Values, fallback bool, keys ...string) (bool, error) {
	value, ok := firstClipQueryValue(query, keys...)
	if !ok {
		return fallback, nil
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return false, err
	}
	return parsed, nil
}

func firstClipQueryValue(query url.Values, keys ...string) (string, bool) {
	for _, key := range keys {
		if value := strings.TrimSpace(query.Get(key)); value != "" {
			return value, true
		}
	}
	return "", false
}

func (h *Handler) updatePulseClipCandidateState(w http.ResponseWriter, r *http.Request) {
	if !h.requirePulseWrite(w) {
		return
	}
	principal := PulsePrincipal{}
	if h.pulseHosted.Hosted {
		var ok bool
		principal, ok = h.requireHostedPrivateClipsPrincipal(w, r)
		if !ok {
			return
		}
	}
	if h.store == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "store_unavailable"})
		return
	}
	candidateID := strings.TrimSpace(chi.URLParam(r, "id"))
	if candidateID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing_id"})
		return
	}
	var req UpdateClipCandidateStateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_body"})
		return
	}
	patch, err := normalizeClipCandidateStatePatch(req)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if !h.pulseHosted.Hosted {
		if p, ok := pulsePrincipalFromContext(r.Context()); ok {
			principal = p
		}
	}
	if principal.ID == "" {
		principal = PulsePrincipal{ID: "local", Kind: "guest"}
	}
	state, err := h.store.UpdateClipCandidateState(r.Context(), candidateID, principal, patch)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "clip_candidate_not_found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "clip_state_unavailable"})
		return
	}
	writeJSON(w, http.StatusOK, state)
}

func (h *Handler) sendPulseClipToReplayForge(w http.ResponseWriter, r *http.Request) {
	if !h.requirePulseWrite(w) {
		return
	}
	principal := h.clipCandidatePrincipal(w, r)
	if principal.ID == "" {
		return
	}
	if h.store == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "store_unavailable"})
		return
	}
	candidateID := strings.TrimSpace(chi.URLParam(r, "id"))
	if candidateID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing_id"})
		return
	}
	if existing, err := h.store.GetClipCandidateJob(r.Context(), candidateID, principal.ID); err == nil && existing.ReplayForgeJobID != "" && existing.Status != ClipCandidateJobFailed {
		writeJSON(w, http.StatusOK, existing)
		return
	}
	candidate, err := h.store.GetClipCandidate(r.Context(), candidateID, principal.ID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "clip_candidate_not_found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "clip_candidate_unavailable"})
		return
	}
	state := ClipCandidateState{Status: ClipCandidateStatusNew}
	if candidate.State != nil {
		state = *candidate.State
	}
	if candidate.SourceStatus != ClipCandidateSourceAvailable || candidate.VodID == nil || strings.TrimSpace(*candidate.VodID) == "" {
		job := h.clipCandidateJobForFailure(candidate, principal, state, ClipCandidateJobSourceUnavailable, "source_unavailable", "candidate source video is unavailable")
		stored, err := h.store.UpsertClipCandidateJob(r.Context(), job)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "clip_job_unavailable"})
			return
		}
		writeJSON(w, http.StatusConflict, stored)
		return
	}
	if h.replayForge == nil {
		job := h.clipCandidateJobForFailure(candidate, principal, state, ClipCandidateJobFailed, "replayforge_unconfigured", "ReplayForge client is not configured")
		_, _ = h.store.UpsertClipCandidateJob(r.Context(), job)
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "replayforge_unconfigured"})
		return
	}
	request := BuildReplayForgeTriggerFromCandidate(candidate, state)
	response, err := h.replayForge.TriggerManual(r.Context(), request)
	if err != nil {
		job := h.clipCandidateJobForFailure(candidate, principal, state, ClipCandidateJobFailed, "replayforge_unavailable", err.Error())
		_, _ = h.store.UpsertClipCandidateJob(r.Context(), job)
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "replayforge_unavailable"})
		return
	}
	now := time.Now().UTC()
	job, err := h.store.UpsertClipCandidateJob(r.Context(), ClipCandidateJob{
		ID:               newClipCandidateJobID(candidate.ID, principal.ID),
		CandidateID:      candidate.ID,
		PrincipalID:      principal.ID,
		PrincipalKind:    principal.Kind,
		Status:           ClipCandidateJobQueued,
		ReplayForgeJobID: response.JobID,
		ReplayForgeState: response.Status,
		Request:          request,
		Response: map[string]interface{}{
			"status":          response.Status,
			"job_id":          response.JobID,
			"existing_job_id": response.ExistingJobID,
			"reason":          response.Reason,
		},
		SubmittedAt: &now,
	})
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "clip_job_unavailable"})
		return
	}
	writeJSON(w, http.StatusAccepted, job)
}

func (h *Handler) refreshPulseClipReplayForgeJob(w http.ResponseWriter, r *http.Request) {
	if !h.requirePulseWrite(w) {
		return
	}
	principal := h.clipCandidatePrincipal(w, r)
	if principal.ID == "" {
		return
	}
	if h.store == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "store_unavailable"})
		return
	}
	candidateID := strings.TrimSpace(chi.URLParam(r, "id"))
	if candidateID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing_id"})
		return
	}
	job, err := h.store.GetClipCandidateJob(r.Context(), candidateID, principal.ID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "clip_job_not_found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "clip_job_unavailable"})
		return
	}
	if strings.TrimSpace(job.ReplayForgeJobID) == "" {
		writeJSON(w, http.StatusOK, job)
		return
	}
	if h.replayForge == nil {
		job.ErrorCode = "replayforge_unconfigured"
		job.ErrorMessage = "ReplayForge client is not configured"
		job.LastCheckedAt = timeNowPtr()
		_, _ = h.store.UpsertClipCandidateJob(r.Context(), job)
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "replayforge_unconfigured"})
		return
	}
	status, err := h.replayForge.GetJob(r.Context(), job.ReplayForgeJobID)
	if err != nil {
		job.ErrorCode = "replayforge_unavailable"
		job.ErrorMessage = err.Error()
		job.LastCheckedAt = timeNowPtr()
		_, _ = h.store.UpsertClipCandidateJob(r.Context(), job)
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "replayforge_unavailable"})
		return
	}
	applyReplayForgeStatusToClipCandidateJob(&job, status)
	stored, err := h.store.UpsertClipCandidateJob(r.Context(), job)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "clip_job_unavailable"})
		return
	}
	writeJSON(w, http.StatusOK, stored)
}

func (h *Handler) clipCandidatePrincipal(w http.ResponseWriter, r *http.Request) PulsePrincipal {
	if h.pulseHosted.Hosted {
		principal, ok := h.requireHostedPrivateClipsPrincipal(w, r)
		if !ok {
			return PulsePrincipal{}
		}
		return principal
	}
	if p, ok := pulsePrincipalFromContext(r.Context()); ok {
		return p
	}
	return PulsePrincipal{ID: "local", Kind: "guest"}
}

func (h *Handler) clipCandidateJobForFailure(candidate ClipCandidate, principal PulsePrincipal, state ClipCandidateState, status, code, message string) ClipCandidateJob {
	return ClipCandidateJob{
		ID:            newClipCandidateJobID(candidate.ID, principal.ID),
		CandidateID:   candidate.ID,
		PrincipalID:   principal.ID,
		PrincipalKind: principal.Kind,
		Status:        status,
		Request:       BuildReplayForgeTriggerFromCandidate(candidate, state),
		ErrorCode:     code,
		ErrorMessage:  message,
		LastCheckedAt: timeNowPtr(),
	}
}
