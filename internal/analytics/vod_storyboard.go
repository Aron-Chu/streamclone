package analytics

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
)

const vodStoryboardMetaCacheTTL = time.Hour

var vodIDRe = regexp.MustCompile(`^\d{6,20}$`)

type vodStoryboardMeta struct {
	URLTemplate          string `json:"urlTemplate"`
	SheetWidth           int    `json:"sheetWidth"`
	SheetHeight          int    `json:"sheetHeight"`
	SheetCount           int    `json:"sheetCount"`
	TileWidth            int    `json:"tileWidth"`
	TileHeight           int    `json:"tileHeight"`
	IntervalMilliseconds int    `json:"intervalMilliseconds"`
}

type storyboardThumbResponse struct {
	SheetURL    string `json:"sheetUrl"`
	SheetWidth  int    `json:"sheetWidth"`
	SheetHeight int    `json:"sheetHeight"`
	CropX       int    `json:"cropX"`
	CropY       int    `json:"cropY"`
	CropW       int    `json:"cropW"`
	CropH       int    `json:"cropH"`
}

type cachedVodStoryboardMeta struct {
	meta      vodStoryboardMeta
	expiresAt time.Time
}

type vodStoryboardCache struct {
	mu sync.Map
}

func (c *vodStoryboardCache) get(vodID string) (vodStoryboardMeta, bool) {
	if c == nil {
		return vodStoryboardMeta{}, false
	}
	raw, ok := c.mu.Load(vodID)
	if !ok {
		return vodStoryboardMeta{}, false
	}
	entry := raw.(cachedVodStoryboardMeta)
	if time.Now().After(entry.expiresAt) {
		c.mu.Delete(vodID)
		return vodStoryboardMeta{}, false
	}
	return entry.meta, true
}

func (c *vodStoryboardCache) set(vodID string, meta vodStoryboardMeta) {
	if c == nil {
		return
	}
	c.mu.Store(vodID, cachedVodStoryboardMeta{
		meta:      meta,
		expiresAt: time.Now().Add(vodStoryboardMetaCacheTTL),
	})
}

func normalizeStoryboardRow(raw struct {
	URL      string `json:"url"`
	Width    int    `json:"width"`
	Height   int    `json:"height"`
	Count    int    `json:"count"`
	Duration int    `json:"duration"`
}) (vodStoryboardMeta, bool) {
	urlTemplate := strings.TrimSpace(raw.URL)
	if urlTemplate == "" || (!strings.Contains(urlTemplate, "{index}") && !strings.Contains(urlTemplate, "%")) {
		return vodStoryboardMeta{}, false
	}
	if !strings.Contains(urlTemplate, "{index}") {
		urlTemplate = strings.ReplaceAll(urlTemplate, "%d", "{index}")
	}
	sheetWidth := raw.Width
	if sheetWidth <= 0 {
		sheetWidth = 1600
	}
	sheetHeight := raw.Height
	if sheetHeight <= 0 {
		sheetHeight = 900
	}
	sheetCount := raw.Count
	if sheetCount <= 0 {
		sheetCount = 1
	}
	intervalMS := raw.Duration
	if intervalMS < 1000 {
		intervalMS = 30_000
	}
	return vodStoryboardMeta{
		URLTemplate:          urlTemplate,
		SheetWidth:           sheetWidth,
		SheetHeight:          sheetHeight,
		SheetCount:           sheetCount,
		TileWidth:            160,
		TileHeight:           90,
		IntervalMilliseconds: intervalMS,
	}, true
}

func storyboardTilesPerSheet(meta vodStoryboardMeta) int {
	cols := max(1, meta.SheetWidth/meta.TileWidth)
	rows := max(1, meta.SheetHeight/meta.TileHeight)
	return cols * rows
}

func resolveStoryboardThumb(meta vodStoryboardMeta, offsetSec int) (*storyboardThumbResponse, string) {
	if meta.URLTemplate == "" || meta.SheetCount <= 0 {
		return nil, "no_storyboard"
	}
	if offsetSec < 0 {
		return nil, "offset_out_of_range"
	}
	offsetMS := offsetSec * 1000
	frameIndex := offsetMS / max(1, meta.IntervalMilliseconds)
	tilesPerSheet := storyboardTilesPerSheet(meta)
	sheetIndex := frameIndex / tilesPerSheet
	if sheetIndex < 0 || sheetIndex >= meta.SheetCount {
		return nil, "offset_out_of_range"
	}
	tileInSheet := frameIndex % tilesPerSheet
	cols := max(1, meta.SheetWidth/meta.TileWidth)
	tileX := (tileInSheet % cols) * meta.TileWidth
	tileY := (tileInSheet / cols) * meta.TileHeight
	sheetURL := strings.ReplaceAll(meta.URLTemplate, "{index}", fmt.Sprintf("%d", sheetIndex))
	return &storyboardThumbResponse{
		SheetURL:    sheetURL,
		SheetWidth:  meta.SheetWidth,
		SheetHeight: meta.SheetHeight,
		CropX:       tileX,
		CropY:       tileY,
		CropW:       meta.TileWidth,
		CropH:       meta.TileHeight,
	}, ""
}

func (s *SyncService) fetchVodStoryboardMeta(ctx context.Context, vodID string) (vodStoryboardMeta, error) {
	query := `query PulseVodStoryboard($id: ID!) {
  video(id: $id) {
    lengthSeconds
    storyboards {
      type
      url
      width
      height
      count
      duration
    }
  }
}`
	body, err := json.Marshal([]map[string]any{
		{
			"query":     query,
			"variables": map[string]string{"id": vodID},
		},
	})
	if err != nil {
		return vodStoryboardMeta{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.twitchGQLURL, bytes.NewReader(body))
	if err != nil {
		return vodStoryboardMeta{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Client-Id", s.twitchClientID)

	resp, err := s.gqlHTTPClient(ctx).Do(req)
	if err != nil {
		return vodStoryboardMeta{}, err
	}
	defer resp.Body.Close()
	payload, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return vodStoryboardMeta{}, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return vodStoryboardMeta{}, fmt.Errorf("gql storyboard status %d: %s", resp.StatusCode, strings.TrimSpace(string(payload)))
	}

	var parsedBatch []struct {
		Data struct {
			Video *struct {
				Storyboards []struct {
					URL      string `json:"url"`
					Width    int    `json:"width"`
					Height   int    `json:"height"`
					Count    int    `json:"count"`
					Duration int    `json:"duration"`
				} `json:"storyboards"`
			} `json:"video"`
		} `json:"data"`
	}
	if err := json.Unmarshal(payload, &parsedBatch); err != nil {
		return vodStoryboardMeta{}, err
	}
	if len(parsedBatch) == 0 {
		return vodStoryboardMeta{}, fmt.Errorf("no_storyboard")
	}
	parsed := parsedBatch[0]
	if parsed.Data.Video == nil || len(parsed.Data.Video.Storyboards) == 0 {
		return vodStoryboardMeta{}, fmt.Errorf("no_storyboard")
	}
	meta, ok := normalizeStoryboardRow(parsed.Data.Video.Storyboards[0])
	if !ok {
		return vodStoryboardMeta{}, fmt.Errorf("no_storyboard")
	}
	return meta, nil
}

func (h *Handler) vodStoryboardMeta(ctx context.Context, vodID string) (vodStoryboardMeta, error) {
	if h == nil || h.syncService == nil {
		return vodStoryboardMeta{}, fmt.Errorf("storyboard_unavailable")
	}
	if h.storyboardCache == nil {
		h.storyboardCache = &vodStoryboardCache{}
	}
	if meta, ok := h.storyboardCache.get(vodID); ok {
		return meta, nil
	}
	meta, err := h.syncService.fetchVodStoryboardMeta(ctx, vodID)
	if err != nil {
		return vodStoryboardMeta{}, err
	}
	h.storyboardCache.set(vodID, meta)
	return meta, nil
}

func (h *Handler) vodStoryboardThumb(w http.ResponseWriter, r *http.Request) {
	vodID := strings.TrimSpace(chi.URLParam(r, "vodId"))
	if !vodIDRe.MatchString(vodID) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_vod_id"})
		return
	}
	offsetSec, err := parseOffsetSecQuery(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_offset"})
		return
	}
	meta, err := h.vodStoryboardMeta(r.Context(), vodID)
	if err != nil {
		reason := "no_storyboard"
		if strings.Contains(err.Error(), "offset") {
			reason = "offset_out_of_range"
		}
		writeJSON(w, http.StatusNotFound, map[string]string{"reason": reason})
		return
	}
	thumb, reason := resolveStoryboardThumb(meta, offsetSec)
	if thumb == nil {
		if reason == "" {
			reason = "no_storyboard"
		}
		writeJSON(w, http.StatusNotFound, map[string]string{"reason": reason})
		return
	}
	writeJSON(w, http.StatusOK, thumb)
}

func parseOffsetSecQuery(r *http.Request) (int, error) {
	raw := strings.TrimSpace(r.URL.Query().Get("offsetSec"))
	if raw == "" {
		return 0, nil
	}
	var offset int
	if _, err := fmt.Sscanf(raw, "%d", &offset); err != nil {
		return 0, err
	}
	return offset, nil
}
