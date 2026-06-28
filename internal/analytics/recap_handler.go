package analytics

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"

	"streamclone/internal/analytics/heatmap"
	pulserecap "streamclone/internal/analytics/recap"
)

func (h *Handler) getPulseStreamRecap(w http.ResponseWriter, r *http.Request) {
	streamID := strings.TrimSpace(chi.URLParam(r, "streamID"))
	if streamID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing_stream_id"})
		return
	}
	payload, err := h.buildPulseStreamRecap(r.Context(), streamID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "stream_not_found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, payload)
}

func (h *Handler) buildPulseStreamRecap(ctx context.Context, streamID string) (pulserecap.StreamRecap, error) {
	stream, err := h.store.StreamByID(ctx, streamID)
	if err != nil {
		return pulserecap.StreamRecap{}, err
	}
	rollups, startedAt, err := h.consolidateForHeatmap(ctx, streamID)
	if err != nil {
		return pulserecap.StreamRecap{}, err
	}
	cfg, cfgErr := heatmap.LoadScoringConfig()
	if cfgErr != nil {
		cfg = heatmap.DefaultScoringConfig()
	}
	detail := heatmap.ComputeHeatmapDetail(rollups, cfg)

	duration := 0
	if stream.EndedAt != nil && !stream.StartedAt.IsZero() {
		duration = int(stream.EndedAt.Sub(stream.StartedAt).Seconds())
	}
	if duration <= 0 && len(rollups) > 0 {
		duration = len(rollups) * 60
	}

	var vodID *string
	if strings.TrimSpace(stream.VodID) != "" {
		v := strings.TrimSpace(stream.VodID)
		vodID = &v
	}
	if startedAt.IsZero() {
		startedAt = stream.StartedAt
	}

	recap := pulserecap.Build(pulserecap.Input{
		StreamID:        stream.StreamID,
		Login:           stream.Login,
		VodID:           vodID,
		StartedAt:       startedAt,
		DurationSeconds: duration,
		Rollups:         rollups,
		Points:          detail.Points,
	})
	h.enrichRecapTopEmotes(ctx, &recap, storeRollupsFromHeatmap(rollups))
	return recap, nil
}
