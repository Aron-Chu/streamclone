package ingest

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"streamclone/internal/storygraph/store"
)

type metadataStreamPage struct {
	Items  []metadataStream `json:"items"`
	Cursor string           `json:"cursor"`
}

type metadataStream struct {
	ID           string `json:"id"`
	Login        string `json:"login"`
	DisplayName  string `json:"displayName"`
	Viewers      int    `json:"viewers"`
	Category     string `json:"category"`
	IsLive       bool   `json:"isLive"`
}

func (w *Workers) runDirectorySampler(ctx context.Context) {
	defer w.wg.Done()
	if !w.opts.Config.PulseWireEnabled {
		return
	}
	interval := w.opts.Config.PulseDirectorySampleInterval
	if interval <= 0 {
		interval = 10 * time.Minute
	}
	failureInterval := 2 * time.Minute
	// Metadata can return transient 502 for a few seconds right after compose up.
	time.Sleep(8 * time.Second)
	next := interval
	for {
		w.sampleDirectory(ctx)
		if w.samplerHealth != nil && !w.samplerHealth.Snapshot().Healthy {
			next = failureInterval
		} else {
			next = interval
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(next):
		}
	}
}

func (w *Workers) sampleDirectory(ctx context.Context) {
	topN := w.opts.Config.PulseDirectoryTopN
	if topN <= 0 {
		topN = 200
	}
	if w.samplerHealth != nil {
		w.samplerHealth.RecordAttempt()
	}
	runID := uuid.NewString()
	sampledAt := time.Now().UTC()
	samples, err := w.fetchDirectorySamplesWithRetry(ctx, topN, runID, sampledAt)
	if err != nil {
		w.opts.Logger.Warn("directory sampler fetch failed", "err", err)
		return
	}
	if len(samples) == 0 {
		return
	}
	if err := w.opts.Store.InsertDirectorySamples(ctx, samples); err != nil {
		w.opts.Logger.Warn("directory sampler insert failed", "err", err)
		if w.samplerHealth != nil {
			retryAt := sampledAt.Add(5 * time.Minute)
			w.samplerHealth.RecordFailure(err.Error(), &retryAt)
		}
		return
	}
	if err := w.computeRisingScores(ctx, samples, sampledAt); err != nil {
		w.opts.Logger.Warn("rising score compute failed", "err", err)
	}
	w.recomputeWindowScores(ctx)
	historyDays, _ := w.opts.Store.DirectorySampleHistoryDays(ctx)
	if w.samplerHealth != nil {
		w.samplerHealth.RecordSuccess(len(samples), historyDays)
	}
	w.opts.Logger.Info("directory sample complete", "run", runID, "count", len(samples))
}

func (w *Workers) fetchDirectorySamplesWithRetry(ctx context.Context, topN int, runID string, sampledAt time.Time) ([]store.DirectorySample, error) {
	delays := []time.Duration{0, 2 * time.Second, 5 * time.Second, 10 * time.Second, 15 * time.Second, 30 * time.Second}
	var lastErr error
	for attempt, delay := range delays {
		if delay > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(delay):
			}
		}
		samples, err := w.fetchDirectorySamples(ctx, topN, runID, sampledAt)
		if err == nil {
			return samples, nil
		}
		lastErr = err
		if !isRetryableMetadataErr(err) || attempt == len(delays)-1 {
			break
		}
		w.opts.Logger.Warn("directory sampler retry", "attempt", attempt+1, "err", err)
	}
	if w.samplerHealth != nil {
		retryAt := time.Now().UTC().Add(5 * time.Minute)
		w.samplerHealth.RecordFailure(lastErr.Error(), &retryAt)
	}
	return nil, lastErr
}

func isRetryableMetadataErr(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "502") ||
		strings.Contains(msg, "503") ||
		strings.Contains(msg, "504") ||
		strings.Contains(msg, "429") ||
		strings.Contains(msg, "timeout") ||
		strings.Contains(msg, "connection refused")
}

func (w *Workers) fetchDirectorySamples(ctx context.Context, topN int, runID string, sampledAt time.Time) ([]store.DirectorySample, error) {
	base := strings.TrimRight(strings.TrimSpace(w.opts.Config.MetadataServiceURL), "/")
	if base == "" {
		return nil, fmt.Errorf("metadata service url not configured")
	}
	client := &http.Client{Timeout: 20 * time.Second}
	pageSize := 25
	cursor := ""
	out := make([]store.DirectorySample, 0, topN)
	rank := 1
	for len(out) < topN {
		limit := pageSize
		if remaining := topN - len(out); remaining < limit {
			limit = remaining
		}
		u, err := url.Parse(base + "/v1/streams")
		if err != nil {
			return nil, err
		}
		q := u.Query()
		q.Set("limit", strconv.Itoa(limit))
		if cursor != "" {
			q.Set("cursor", cursor)
		}
		u.RawQuery = q.Encode()
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
		if err != nil {
			return nil, err
		}
		resp, err := client.Do(req)
		if err != nil {
			return nil, err
		}
		body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			pageErr := fmt.Errorf("metadata streams status %d", resp.StatusCode)
			if len(out) > 0 && isRetryableMetadataErr(pageErr) {
				w.opts.Logger.Warn("directory sampler partial page", "collected", len(out), "err", pageErr)
				return out, nil
			}
			return nil, pageErr
		}
		var page metadataStreamPage
		if err := json.Unmarshal(body, &page); err != nil {
			return nil, err
		}
		if len(page.Items) == 0 {
			break
		}
		for _, stream := range page.Items {
			login := normalizeDirectoryLogin(stream.Login)
			if login == "" {
				continue
			}
			out = append(out, store.DirectorySample{
				TwitchLogin:  login,
				TwitchID:     strings.TrimSpace(stream.ID),
				DisplayName:  strings.TrimSpace(stream.DisplayName),
				Category:     strings.TrimSpace(stream.Category),
				Viewers:      stream.Viewers,
				Rank:         rank,
				IsLive:       stream.IsLive,
				SampleRunID:  runID,
				SampledAt:    sampledAt,
			})
			rank++
			if len(out) >= topN {
				break
			}
		}
		cursor = strings.TrimSpace(page.Cursor)
		if cursor == "" {
			break
		}
	}
	return out, nil
}

func (w *Workers) computeRisingScores(ctx context.Context, latest []store.DirectorySample, at time.Time) error {
	windows := []struct {
		label string
		since time.Time
	}{
		{"today", startOfDay(at)},
		{"24h", at.Add(-24 * time.Hour)},
		{"7d", at.Add(-7 * 24 * time.Hour)},
	}
	for _, win := range windows {
		var rows []store.RisingRow
		for _, now := range latest {
			prev, err := w.opts.Store.FirstSampleSince(ctx, now.TwitchLogin, win.since)
			if err != nil {
				return err
			}
			viewersPrev := 0
			rankPrev := 0
			newEntrant := prev == nil
			if prev != nil {
				viewersPrev = prev.Viewers
				rankPrev = prev.Rank
			}
			viewerDeltaPct := 0.0
			if viewersPrev > 0 {
				viewerDeltaPct = float64(now.Viewers-viewersPrev) / float64(viewersPrev) * 100
			} else if now.Viewers > 0 {
				viewerDeltaPct = 100
			}
			rankDelta := 0
			if rankPrev > 0 && now.Rank > 0 {
				rankDelta = rankPrev - now.Rank
			}
			clipVel, _ := w.opts.Store.ClipVelocityForLogin(ctx, now.TwitchLogin, win.since)
			score := risingScore(viewerDeltaPct, rankDelta, newEntrant, clipVel, now.Viewers)
			rows = append(rows, store.RisingRow{
				TwitchLogin:    now.TwitchLogin,
				Window:         win.label,
				ViewersNow:     now.Viewers,
				ViewersPrev:    viewersPrev,
				ViewerDeltaPct: viewerDeltaPct,
				RankNow:        now.Rank,
				RankPrev:       rankPrev,
				RankDelta:      rankDelta,
				NewEntrant:     newEntrant,
				ClipVelocity:   clipVel,
				RisingScore:    score,
				ComputedAt:     at,
			})
		}
		if err := w.opts.Store.UpsertRisingRows(ctx, rows); err != nil {
			return err
		}
	}
	return nil
}

func risingScore(viewerDeltaPct float64, rankDelta int, newEntrant bool, clipVel float64, viewersNow int) float64 {
	viewerComponent := clamp(viewerDeltaPct/50, -1, 2) * 40
	rankComponent := clamp(float64(rankDelta)/20, -1, 2) * 30
	entrantBonus := 0.0
	if newEntrant && viewersNow >= 500 {
		entrantBonus = 15
	}
	clipComponent := clamp(clipVel, 0, 5) * 3
	return viewerComponent + rankComponent + entrantBonus + clipComponent
}

func clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func startOfDay(t time.Time) time.Time {
	t = t.UTC()
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}

func normalizeDirectoryLogin(login string) string {
	var out strings.Builder
	for _, r := range strings.ToLower(strings.TrimSpace(login)) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' {
			out.WriteRune(r)
		}
	}
	return out.String()
}
