package analytics

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
)

const (
	pulseVODUserRetryCooldown  = 15 * time.Minute
	pulseVODAdminRetryCooldown = time.Minute
)

type PulseVODRetryResponse struct {
	OK                 bool   `json:"ok"`
	StreamID           string `json:"streamId"`
	Login              string `json:"login"`
	VodID              string `json:"vodId,omitempty"`
	Status             string `json:"status"`
	ManualRetryAllowed bool   `json:"manualRetryAllowed"`
	RetryAfterSeconds  int    `json:"retryAfterSeconds,omitempty"`
}

func (h *Handler) extensionPulseVodRetry(w http.ResponseWriter, r *http.Request) {
	if !h.requirePulseWrite(w) {
		return
	}
	if !h.enforceBackfillRateLimit(w, r) {
		return
	}
	login, ok := validLogin(chi.URLParam(r, "login"))
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_channel"})
		return
	}
	var req struct {
		StreamID string `json:"streamId"`
	}
	if r.Body != nil {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_body"})
			return
		}
	}

	streamID := strings.TrimSpace(req.StreamID)
	if streamID == "" {
		stream, err := h.latestPulseRetryStream(r.Context(), login)
		if err != nil {
			writePulseRetryStreamError(w, err)
			return
		}
		streamID = stream.StreamID
	}

	if principal, ok := pulsePrincipalFromContext(r.Context()); ok {
		key := "sp:vodretry:user:" + principal.ID + ":" + streamID
		if allowed, retryAfter := h.allowPulseRetryCooldown(r.Context(), key, pulseVODUserRetryCooldown); !allowed {
			writePulseRetryCooldown(w, retryAfter)
			return
		}
	}

	resp, err := h.retryPulseVODResolution(r.Context(), login, streamID, "manual_retry")
	if err != nil {
		writePulseRetryError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) adminPulseVodRetry(w http.ResponseWriter, r *http.Request) {
	if !h.requirePulseWrite(w) {
		return
	}
	streamID := strings.TrimSpace(chi.URLParam(r, "streamID"))
	if streamID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing_stream_id"})
		return
	}
	key := "sp:vodretry:admin:" + streamID
	if allowed, retryAfter := h.allowPulseRetryCooldown(r.Context(), key, pulseVODAdminRetryCooldown); !allowed {
		writePulseRetryCooldown(w, retryAfter)
		return
	}
	resp, err := h.retryPulseVODResolution(r.Context(), "", streamID, "admin_retry")
	if err != nil {
		writePulseRetryError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) latestPulseRetryStream(ctx context.Context, login string) (*StreamRecord, error) {
	if h == nil || h.store == nil {
		return nil, errPulseRetryStoreUnavailable
	}
	stream, err := h.store.LatestStreamByLogin(ctx, login)
	if err != nil {
		return nil, err
	}
	return stream, nil
}

func (h *Handler) retryPulseVODResolution(ctx context.Context, login, streamID, source string) (PulseVODRetryResponse, error) {
	if h == nil || h.store == nil {
		return PulseVODRetryResponse{}, errPulseRetryStoreUnavailable
	}
	stream, err := h.store.StreamByID(ctx, streamID)
	if err != nil {
		return PulseVODRetryResponse{}, err
	}
	login = normalizeLogin(login)
	if login == "" {
		login = normalizeLogin(stream.Login)
	}
	if login == "" || normalizeLogin(stream.Login) != login {
		return PulseVODRetryResponse{}, errPulseRetryLoginMismatch
	}

	if vodID := strings.TrimSpace(stream.VodID); vodID != "" {
		h.invalidatePulseBFFCache(ctx, login)
		return PulseVODRetryResponse{
			OK:                 true,
			StreamID:           stream.StreamID,
			Login:              login,
			VodID:              vodID,
			Status:             "available",
			ManualRetryAllowed: true,
		}, nil
	}

	runtime := h.pulseRuntimeConfig()
	if !runtime.HelixVodEnabled || h.helix == nil || !h.helix.Enabled() {
		return PulseVODRetryResponse{}, ErrPulseVODValidationUnavailable
	}
	broadcasterID := h.helix.ResolveBroadcasterID(ctx, login, stream.BroadcasterID)
	if broadcasterID == "" {
		return PulseVODRetryResponse{}, ErrPulseVODValidationUnavailable
	}

	now := time.Now().UTC()
	resolved, helixErr := h.helix.VideoIDByStreamID(ctx, broadcasterID, stream.StreamID)
	if helixErr != nil {
		_ = h.store.RecordPulseVODResolutionAttempt(ctx, PulseVODResolutionAttemptInput{
			StreamID:           stream.StreamID,
			Login:              login,
			TwitchStreamID:     stream.StreamID,
			BroadcasterID:      broadcasterID,
			Source:             source,
			Status:             "error",
			Attempts:           1,
			LastAttemptAt:      &now,
			ManualRetryAllowed: true,
			ErrorCode:          "helix_error",
		})
		return PulseVODRetryResponse{}, ErrPulseVODValidationUnavailable
	}
	if resolved != "" {
		vodID, err := validatePulseVODCandidate(*stream, resolved)
		if err != nil {
			return PulseVODRetryResponse{}, err
		}
		if err := h.store.SetStreamVodID(ctx, stream.StreamID, vodID, source); err != nil {
			return PulseVODRetryResponse{}, err
		}
		h.invalidatePulseBFFCache(ctx, login)
		return PulseVODRetryResponse{
			OK:                 true,
			StreamID:           stream.StreamID,
			Login:              login,
			VodID:              vodID,
			Status:             "available",
			ManualRetryAllowed: true,
		}, nil
	}

	status := "waiting"
	var finalizedAt *time.Time
	errorCode := ""
	if stream.EndedAt != nil {
		status = "unavailable"
		finalizedAt = &now
		errorCode = "vod_unavailable"
		if err := h.store.MarkStreamVodUnlinked(ctx, stream.StreamID); err != nil {
			return PulseVODRetryResponse{}, err
		}
	}
	_ = h.store.RecordPulseVODResolutionAttempt(ctx, PulseVODResolutionAttemptInput{
		StreamID:           stream.StreamID,
		Login:              login,
		TwitchStreamID:     stream.StreamID,
		BroadcasterID:      broadcasterID,
		Source:             source,
		Status:             status,
		Attempts:           1,
		LastAttemptAt:      &now,
		FinalizedAt:        finalizedAt,
		ManualRetryAllowed: true,
		ErrorCode:          errorCode,
	})
	h.invalidatePulseBFFCache(ctx, login)
	return PulseVODRetryResponse{
		OK:                 false,
		StreamID:           stream.StreamID,
		Login:              login,
		Status:             status,
		ManualRetryAllowed: true,
	}, nil
}

func (h *Handler) allowPulseRetryCooldown(ctx context.Context, key string, window time.Duration) (bool, time.Duration) {
	if h == nil || h.rdb == nil || strings.TrimSpace(key) == "" || window <= 0 {
		return true, 0
	}
	ok, err := h.rdb.SetNX(ctx, key, "1", window).Result()
	if err != nil || ok {
		return true, 0
	}
	ttl, err := h.rdb.TTL(ctx, key).Result()
	if err != nil || ttl <= 0 {
		ttl = window
	}
	return false, ttl
}

var (
	errPulseRetryStoreUnavailable = errors.New("store_unavailable")
	errPulseRetryLoginMismatch    = errors.New("stream_login_mismatch")
)

func writePulseRetryStreamError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, errPulseRetryStoreUnavailable):
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "store_unavailable"})
	case errors.Is(err, pgx.ErrNoRows):
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "stream_not_found"})
	default:
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
}

func writePulseRetryError(w http.ResponseWriter, err error) {
	if errors.Is(err, ErrPulseInvalidVODID) ||
		errors.Is(err, ErrPulseVODStreamMismatch) ||
		errors.Is(err, ErrPulseVODValidationUnavailable) {
		status, code := pulseVodValidationHTTPError(err)
		writeJSON(w, status, map[string]string{"error": code})
		return
	}
	switch {
	case errors.Is(err, errPulseRetryStoreUnavailable):
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "store_unavailable"})
	case errors.Is(err, errPulseRetryLoginMismatch):
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "stream_login_mismatch"})
	case errors.Is(err, pgx.ErrNoRows):
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "stream_not_found"})
	default:
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
	}
}

func writePulseRetryCooldown(w http.ResponseWriter, retryAfter time.Duration) {
	if retryAfter <= 0 {
		retryAfter = pulseVODUserRetryCooldown
	}
	seconds := int(retryAfter.Seconds() + 0.999)
	w.Header().Set("Retry-After", strconv.Itoa(seconds))
	writeJSON(w, http.StatusTooManyRequests, PulseVODRetryResponse{
		OK:                 false,
		Status:             "error",
		ManualRetryAllowed: false,
		RetryAfterSeconds:  seconds,
	})
}
