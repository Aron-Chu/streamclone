package moments

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"log/slog"

	"streamclone/internal/config"
	"streamclone/internal/storygraph/entity"
	"streamclone/internal/storygraph/fingerprint"
	"streamclone/internal/storygraph/store"
)

// Puller fetches replay heatmap peaks from analytics and builds fingerprints.
type Puller struct {
	cfg    config.Config
	client *http.Client
	logger *slog.Logger
	store  *store.Store
	entity *entity.Resolver
}

func NewPuller(cfg config.Config, st *store.Store, ent *entity.Resolver, logger *slog.Logger) *Puller {
	return &Puller{
		cfg:    cfg,
		client: &http.Client{Timeout: 2 * time.Minute},
		logger: logger,
		store:  st,
		entity: ent,
	}
}

type heatmapResponse struct {
	StreamID string `json:"streamId"`
	Points   []struct {
		OffsetSeconds int    `json:"offsetSeconds"`
		Score         int    `json:"score"`
		Reason        string `json:"reason"`
		TopEmotes     []struct {
			ID       string `json:"id"`
			Name     string `json:"name"`
			Count    int    `json:"count"`
			Provider string `json:"provider"`
		} `json:"topEmotes"`
	} `json:"points"`
}

type chatReplayResponse struct {
	Messages []struct {
		Text string `json:"text"`
	} `json:"messages"`
}

// PullStream builds moment_fingerprints for heatmap peaks on a stream.
func (p *Puller) PullStream(ctx context.Context, streamID, vodID, login string) (int, error) {
	base := strings.TrimRight(p.cfg.AnalyticsServiceURL, "/")
	u := fmt.Sprintf("%s/v1/analytics/streams/%s/replay-heatmap", base, streamID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return 0, err
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return 0, fmt.Errorf("heatmap status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var hm heatmapResponse
	if err := json.NewDecoder(resp.Body).Decode(&hm); err != nil {
		return 0, err
	}
	var entityID *int64
	if login != "" {
		id, ok, err := p.entity.ResolveTwitchLogin(ctx, login, "", login, nil)
		if err != nil {
			return 0, err
		}
		if ok {
			entityID = &id
		}
	}
	written := 0
	for _, pt := range hm.Points {
		if pt.Score < 40 {
			continue
		}
		quotes, _ := p.pullQuotes(ctx, streamID, pt.OffsetSeconds)
		emotes := make([]fingerprint.EmoteCount, 0, len(pt.TopEmotes))
		for _, e := range pt.TopEmotes {
			emotes = append(emotes, fingerprint.EmoteCount{
				ID: e.ID, Name: e.Name, Provider: e.Provider, Count: e.Count,
			})
		}
		fp := fingerprint.MomentFingerprint{
			EntityID:   entityID,
			StreamID:   streamID,
			VODOffsetS: pt.OffsetSeconds,
			Quotes:     quotes,
			TopEmotes:  emotes,
			Version:    1,
		}
		confidence := float64(pt.Score) / 100
		if confidence > 1 {
			confidence = 1
		}
		_, err := p.store.InsertFingerprint(ctx, store.MomentFingerprint{
			EntityID:         entityID,
			StreamID:         fp.StreamID,
			VODID:            strings.TrimSpace(vodID),
			VODOffsetS:       fp.VODOffsetS,
			TranscriptKW:     fp.QuotesJSON(),
			TopEmotes:        fp.TopEmotesJSON(),
			ChatSpikeSummary: strings.TrimSpace(pt.Reason),
			OriginConfidence: &confidence,
			FPVersion:        fp.Version,
		})
		if err != nil {
			p.logger.Warn("fingerprint insert failed", "stream", streamID, "offset", pt.OffsetSeconds, "err", err)
			continue
		}
		written++
	}
	return written, nil
}

func (p *Puller) pullQuotes(ctx context.Context, streamID string, offset int) ([]string, error) {
	start := offset - 60
	if start < 0 {
		start = 0
	}
	end := offset + 60
	base := strings.TrimRight(p.cfg.AnalyticsServiceURL, "/")
	u := fmt.Sprintf("%s/v1/analytics/streams/%s/chat-replay?offsetStart=%d&offsetEnd=%d", base, streamID, start, end)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, nil
	}
	var cr chatReplayResponse
	if err := json.NewDecoder(resp.Body).Decode(&cr); err != nil {
		return nil, err
	}
	counts := map[string]int{}
	for _, m := range cr.Messages {
		t := strings.TrimSpace(m.Text)
		if len(t) < 8 || len(t) > 120 {
			continue
		}
		counts[t]++
	}
	type kv struct {
		text  string
		count int
	}
	ranked := make([]kv, 0, len(counts))
	for t, c := range counts {
		ranked = append(ranked, kv{t, c})
	}
	for i := 0; i < len(ranked); i++ {
		for j := i + 1; j < len(ranked); j++ {
			if ranked[j].count > ranked[i].count {
				ranked[i], ranked[j] = ranked[j], ranked[i]
			}
		}
	}
	limit := 5
	if len(ranked) < limit {
		limit = len(ranked)
	}
	out := make([]string, 0, limit)
	for i := 0; i < limit; i++ {
		out = append(out, ranked[i].text)
	}
	return out, nil
}
