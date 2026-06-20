package ingest

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"streamclone/internal/social"
	"streamclone/internal/social/reddit"
	"streamclone/internal/social/twitchclips"
)

const spreadBackfillCooldown = 5 * time.Minute

// ErrSpreadBackfillCooldown is returned when a login was backfilled too recently.
var ErrSpreadBackfillCooldown = errors.New("spread backfill cooldown")

// SpreadBackfillCounts summarizes one channel backfill run.
type SpreadBackfillCounts struct {
	Reddit     int `json:"reddit"`
	Clips      int `json:"clips"`
	Reattached int `json:"reattached"`
}

// SpreadBackfillMeta is the client-visible backfill state for one login.
type SpreadBackfillMeta struct {
	State       string     `json:"state"`
	RequestedAt *time.Time `json:"requestedAt,omitempty"`
}

type spreadBackfillCoordinator struct {
	mu        sync.Mutex
	lastReq   map[string]time.Time
	state     map[string]SpreadBackfillMeta
	inFlight  sync.Map
}

func newSpreadBackfillCoordinator() *spreadBackfillCoordinator {
	return &spreadBackfillCoordinator{
		lastReq: map[string]time.Time{},
		state:   map[string]SpreadBackfillMeta{},
	}
}

func normalizeSpreadLogin(login string) string {
	return strings.ToLower(strings.TrimSpace(login))
}

func (c *spreadBackfillCoordinator) meta(login string) SpreadBackfillMeta {
	login = normalizeSpreadLogin(login)
	c.mu.Lock()
	defer c.mu.Unlock()
	if row, ok := c.state[login]; ok {
		return row
	}
	return SpreadBackfillMeta{State: "idle"}
}

func (c *spreadBackfillCoordinator) setState(login string, state string, requestedAt *time.Time) {
	login = normalizeSpreadLogin(login)
	c.mu.Lock()
	defer c.mu.Unlock()
	c.state[login] = SpreadBackfillMeta{State: state, RequestedAt: requestedAt}
}

func (c *spreadBackfillCoordinator) request(login string) (SpreadBackfillMeta, bool, error) {
	login = normalizeSpreadLogin(login)
	if login == "" {
		return SpreadBackfillMeta{}, false, errors.New("invalid login")
	}
	now := time.Now()
	c.mu.Lock()
	if last, ok := c.lastReq[login]; ok && now.Sub(last) < spreadBackfillCooldown {
		c.mu.Unlock()
		return SpreadBackfillMeta{}, false, ErrSpreadBackfillCooldown
	}
	c.lastReq[login] = now
	c.state[login] = SpreadBackfillMeta{State: "warming", RequestedAt: &now}
	c.mu.Unlock()
	if _, loaded := c.inFlight.LoadOrStore(login, struct{}{}); loaded {
		return SpreadBackfillMeta{State: "warming", RequestedAt: &now}, false, nil
	}
	return SpreadBackfillMeta{State: "warming", RequestedAt: &now}, true, nil
}

func (c *spreadBackfillCoordinator) finish(login string) {
	login = normalizeSpreadLogin(login)
	c.inFlight.Delete(login)
	c.setState(login, "ready", c.meta(login).RequestedAt)
}

func (c *spreadBackfillCoordinator) releaseWithoutRun(login string) {
	login = normalizeSpreadLogin(login)
	c.inFlight.Delete(login)
}

// LastIngestAt returns the timestamp of the most recent ingestAll cycle.
func (w *Workers) LastIngestAt() *time.Time {
	w.lastIngestAtMu.RLock()
	defer w.lastIngestAtMu.RUnlock()
	if w.lastIngestAt.IsZero() {
		return nil
	}
	ts := w.lastIngestAt
	return &ts
}

func (w *Workers) setLastIngestAt(ts time.Time) {
	w.lastIngestAtMu.Lock()
	defer w.lastIngestAtMu.Unlock()
	w.lastIngestAt = ts
}

// SpreadBackfillMeta returns the backfill state for a channel login.
func (w *Workers) SpreadBackfillMeta(login string) SpreadBackfillMeta {
	if w.spreadBackfill == nil {
		return SpreadBackfillMeta{State: "idle"}
	}
	return w.spreadBackfill.meta(login)
}

// RequestSpreadBackfill kicks off an async per-login spread backfill when allowed.
func (w *Workers) RequestSpreadBackfill(ctx context.Context, login string) (SpreadBackfillMeta, error) {
	if w.spreadBackfill == nil {
		return SpreadBackfillMeta{}, errors.New("spread backfill unavailable")
	}
	meta, shouldRun, err := w.spreadBackfill.request(login)
	if err != nil {
		return meta, err
	}
	if shouldRun {
		go w.runSpreadBackfillJob(context.Background(), login)
	}
	return meta, nil
}

func (w *Workers) runSpreadBackfillJob(ctx context.Context, login string) {
	defer w.spreadBackfill.finish(login)
	counts := w.RunSpreadBackfill(ctx, login)
	w.opts.Logger.Info("spread backfill complete", "login", login, "counts", counts)
}

// RunSpreadBackfill ingests Reddit + clips for one login and reattaches unresolved mentions.
func (w *Workers) RunSpreadBackfill(ctx context.Context, login string) SpreadBackfillCounts {
	login = normalizeSpreadLogin(login)
	if login == "" {
		return SpreadBackfillCounts{}
	}
	runCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	counts := SpreadBackfillCounts{}
	since := time.Now().Add(-7 * 24 * time.Hour)

	redditSrc := reddit.NewSource(w.opts.Config)
	if err := redditSrc.Healthy(runCtx); err == nil {
		page, err := redditSrc.Search(runCtx, social.Query{
			Entity: social.EntityRef{TwitchLogin: login},
			Since:  since,
			Budget: social.Budget{
				MaxItems:          8,
				MaxBrowserFetches: socialBrowserFetchBudget(w.opts.Config),
			},
		})
		if err == nil {
			counts.Reddit = len(page.Items)
			for _, item := range page.Items {
				w.persistAndMatch(runCtx, "reddit", item)
			}
		}
	}

	clipsSrc := twitchclips.NewSource(w.opts.Config)
	if err := clipsSrc.Healthy(runCtx); err == nil {
		page, err := clipsSrc.Search(runCtx, social.Query{
			Entity: social.EntityRef{TwitchLogin: login},
			Since:  since,
			Budget: social.Budget{MaxItems: 6},
		})
		if err == nil {
			counts.Clips = len(page.Items)
			for _, item := range page.Items {
				w.persistAndMatch(runCtx, "twitchclips", item)
			}
		}
	}

	counts.Reattached = w.reattachUnresolvedForLogin(runCtx, login)
	return counts
}

func (w *Workers) reattachUnresolvedForLogin(ctx context.Context, login string) int {
	rows, err := w.opts.Store.ListUnresolvedSocialItemsMentioningLogin(ctx, login, 20)
	if err != nil || len(rows) == 0 {
		return 0
	}
	attached := 0
	for _, row := range rows {
		item := social.Item{
			Source:     row.Source,
			Kind:       row.Kind,
			ExternalID: row.ExternalID,
			URL:        row.URL,
			Author:     row.Author,
			Text:       row.Text,
			CreatedAt:  derefTime(row.CreatedAtSrc),
		}
		entityID, err := w.resolveEntity(ctx, item)
		if err != nil || entityID == nil {
			continue
		}
		if err := w.opts.Store.AttachSocialItemEntity(ctx, row.ID, *entityID); err != nil {
			continue
		}
		clusterID, err := w.opts.Store.ClusterByItemID(ctx, row.ID)
		if err == nil && clusterID > 0 {
			_ = w.opts.Store.UpdateClusterMeta(ctx, clusterID, entityID, "", "", "", "")
		}
		attached++
	}
	return attached
}

func (w *Workers) ingestTieredRedditLogins(ctx context.Context) {
	targets := w.spreadRedditTargets(ctx)
	if len(targets) == 0 {
		return
	}
	const maxPerCycle = 3
	cursor := w.redditLoginCursor % len(targets)
	w.redditLoginCursor = (cursor + maxPerCycle) % len(targets)

	src := reddit.NewSource(w.opts.Config)
	if err := src.Healthy(ctx); err != nil {
		return
	}
	since := time.Now().Add(-7 * 24 * time.Hour)
	total := 0
	for i := 0; i < maxPerCycle; i++ {
		login := targets[(cursor+i)%len(targets)]
		page, err := src.Search(ctx, social.Query{
			Entity: social.EntityRef{TwitchLogin: login},
			Since:  since,
			Budget: social.Budget{
				MaxItems:          8,
				MaxBrowserFetches: socialBrowserFetchBudget(w.opts.Config),
			},
		})
		if err != nil {
			w.opts.Logger.Warn("tiered reddit login search failed", "login", login, "err", err)
			continue
		}
		total += len(page.Items)
		for _, item := range page.Items {
			w.persistAndMatch(ctx, "reddit", item)
		}
	}
	if total > 0 && w.opts.Health != nil {
		w.opts.Health.RecordDetailOK("reddit", "per_login", total)
	}
}

func (w *Workers) spreadRedditTargets(ctx context.Context) []string {
	seen := map[string]struct{}{}
	var out []string
	add := func(login string) {
		login = normalizeSpreadLogin(login)
		if login == "" {
			return
		}
		if _, ok := seen[login]; ok {
			return
		}
		seen[login] = struct{}{}
		out = append(out, login)
	}
	for _, login := range w.opts.Config.AlwaysTrackedChannels {
		add(login)
	}
	if seeds, err := w.opts.Store.DirectorySeedLogins(ctx, 25); err == nil {
		for _, login := range seeds {
			add(login)
		}
	}
	return out
}

func derefTime(ts *time.Time) time.Time {
	if ts == nil {
		return time.Time{}
	}
	return *ts
}
