package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"streamclone/internal/social/twitchclips"
	"streamclone/internal/storygraph/store"
)

func (h *Handler) repairThumbnails(w http.ResponseWriter, r *http.Request) {
	limit := queryInt(r, "limit", 100)
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := h.store.ListClipsNeedingThumbnailRepair(r.Context(), limit)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	src := twitchclips.NewSource(h.cfg)
	repaired := 0
	skipped := 0
	var failures []string
	for _, row := range rows {
		clip, err := src.FetchClipByID(r.Context(), row.ExternalID)
		if err != nil {
			skipped++
			if len(failures) < 5 {
				failures = append(failures, row.ExternalID+": "+err.Error())
			}
			continue
		}
		thumb := strings.TrimSpace(clip.ThumbnailURL)
		if thumb == "" {
			skipped++
			continue
		}
		metrics := map[string]any{}
		if len(row.Metrics) > 0 {
			_ = json.Unmarshal(row.Metrics, &metrics)
		}
		metrics["thumbnail_url"] = thumb
		metrics["thumbnail_source"] = "helix"
		metrics["thumbnail_status"] = "ready"
		updated := store.MergeSocialMetrics(row.Metrics, mustMarshalMetrics(metrics))
		if err := h.store.UpdateSocialItemMetrics(r.Context(), row.ID, updated); err != nil {
			skipped++
			if len(failures) < 5 {
				failures = append(failures, row.ExternalID+": "+err.Error())
			}
			continue
		}
		repaired++
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"status":   "ok",
		"scanned":  len(rows),
		"repaired": repaired,
		"skipped":  skipped,
		"errors":   failures,
	})
}

func mustMarshalMetrics(metrics map[string]any) json.RawMessage {
	raw, err := json.Marshal(metrics)
	if err != nil {
		return json.RawMessage("{}")
	}
	return raw
}

func queryInt(r *http.Request, key string, fallback int) int {
	raw := strings.TrimSpace(r.URL.Query().Get(key))
	if raw == "" {
		return fallback
	}
	n, err := parseInt(raw)
	if err != nil {
		return fallback
	}
	return n
}

func parseInt(raw string) (int, error) {
	return strconv.Atoi(raw)
}
