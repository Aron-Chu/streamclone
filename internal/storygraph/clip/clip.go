package clip

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"streamclone/internal/config"
	"streamclone/internal/storygraph/store"
)

// Bridge triggers ReplayForge manual clips from story origins.
type Bridge struct {
	cfg    config.Config
	client *http.Client
}

func New(cfg config.Config) *Bridge {
	return &Bridge{
		cfg:    cfg,
		client: &http.Client{Timeout: 30 * time.Second},
	}
}

// TriggerForStory creates a clip job for a story's origin fingerprint.
func (b *Bridge) TriggerForStory(ctx context.Context, st *store.Store, clusterID int64) (map[string]any, error) {
	card, err := st.GetStory(ctx, clusterID, "local")
	if err != nil {
		return nil, err
	}
	if card == nil || card.Origin == nil {
		return nil, fmt.Errorf("story has no origin moment")
	}
	if strings.TrimSpace(card.Origin.VODID) == "" {
		return nil, fmt.Errorf("story origin has no VOD id")
	}
	channel := ""
	if card.Entity != nil {
		channel = card.Entity.TwitchLogin
	}
	body := map[string]any{
		"channel":            channel,
		"stream_id":          card.Origin.StreamID,
		"vod_id":             card.Origin.VODID,
		"vod_offset_seconds": card.Origin.VODOffsetS,
		"moment_context": map[string]any{
			"stream_id":          card.Origin.StreamID,
			"vod_id":             card.Origin.VODID,
			"vod_offset_seconds": card.Origin.VODOffsetS,
			"source_kind":        "vod",
			"moment_score":       0,
			"pick_reason":        "pulse_wire",
		},
	}
	payload, _ := json.Marshal(body)
	url := strings.TrimRight(b.cfg.ClipperServiceURL, "/") + "/v1/triggers/manual"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := b.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var out map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return map[string]any{"status": resp.Status, "clipper": url}, nil
	}
	out["status"] = resp.StatusCode
	return out, nil
}
