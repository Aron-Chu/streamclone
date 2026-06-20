package ingest

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"

	"streamclone/internal/config"
	"streamclone/internal/social"
	"streamclone/internal/social/reddit"
	"streamclone/internal/social/streamerbans"
	"streamclone/internal/social/twitchclips"
	"streamclone/internal/social/youtube"
	"streamclone/internal/storygraph/bans"
	"streamclone/internal/storygraph/cluster"
	"streamclone/internal/storygraph/entity"
	"streamclone/internal/storygraph/evidenceurl"
	"streamclone/internal/storygraph/matcher"
	"streamclone/internal/storygraph/moments"
	"streamclone/internal/storygraph/preview"
	"streamclone/internal/storygraph/reliability"
	"streamclone/internal/storygraph/score"
	"streamclone/internal/storygraph/store"
)

// Options configures background workers.
type Options struct {
	Store             *store.Store
	Reliability       *reliability.Registry
	Redis             *redis.Client
	Logger            *slog.Logger
	Config            config.Config
	Health            *Health
	SamplerHealth     *DirectorySamplerHealth
	WindowScoreHealth *WindowScoreHealth
}

// Workers runs fingerprint pull, source ingest, trend sampling, and retention reaper.
type Workers struct {
	opts              Options
	cancel            context.CancelFunc
	wg                sync.WaitGroup
	matcher           *matcher.LexicalMatcher
	cluster           *cluster.Service
	score             *score.Engine
	window            *score.WindowEngine
	trend             *score.TrendSampler
	entity            *entity.Resolver
	puller            *moments.Puller
	preview           *preview.Hydrator
	backfill          *BackfillRunner
	samplerHealth     *DirectorySamplerHealth
	windowScoreHealth *WindowScoreHealth
	backfillOnce       sync.Once
	redditRepairOnce   sync.Once
	entityBackfillOnce sync.Once
	redditCommentHydrations int
	lastIngestAtMu          sync.RWMutex
	lastIngestAt            time.Time
	spreadBackfill          *spreadBackfillCoordinator
	redditLoginCursor       int
}

func NewWorkers(opts Options) *Workers {
	if opts.Reliability == nil {
		opts.Reliability = reliability.NewRegistry(nil)
		opts.Reliability.SeedDefaults()
	}
	if opts.SamplerHealth == nil {
		opts.SamplerHealth = NewDirectorySamplerHealth()
	}
	if opts.WindowScoreHealth == nil {
		opts.WindowScoreHealth = NewWindowScoreHealth()
	}
	ent := entity.New(opts.Store)
	w := &Workers{
		opts: opts,
		matcher: matcher.NewLexical(matcher.Config{
			LinkThreshold:   opts.Config.MatchLinkThreshold,
			ReviewThreshold: opts.Config.MatchReviewThreshold,
		}),
		cluster: cluster.New(opts.Store),
		score:   score.New(opts.Reliability),
		window:  score.NewWindowEngine(opts.Store, opts.Reliability),
		trend: score.NewTrendSampler(opts.Config.PulseWireEnabled, opts.Store, opts.Reliability, map[string]social.SocialSource{
			"reddit":      reddit.NewSource(opts.Config),
			"youtube":     youtube.NewSource(opts.Config),
			"twitchclips": twitchclips.NewSource(opts.Config),
		}),
		entity:            ent,
		puller:            moments.NewPuller(opts.Config, opts.Store, ent, opts.Logger),
		preview:           preview.NewHydrator(opts.Logger),
		samplerHealth:     opts.SamplerHealth,
		windowScoreHealth: opts.WindowScoreHealth,
		backfill: NewBackfillRunner(BackfillOptions{
			Store:       opts.Store,
			Reliability: opts.Reliability,
			Logger:      opts.Logger,
			Config:      opts.Config,
			Budget: social.Budget{
				MaxItems:          24,
				MaxBrowserFetches: socialBrowserFetchBudget(opts.Config),
			},
		}),
		spreadBackfill: newSpreadBackfillCoordinator(),
	}
	w.backfill.SetPersist(w.persistAndMatch)
	return w
}

func (w *Workers) maybeLearnRedditAlias(ctx context.Context, entityID int64, item social.Item) {
	if strings.TrimSpace(item.EntityTwitchLogin) == "" {
		return
	}
	if flair := strings.TrimSpace(item.FlairText); flair != "" {
		_ = w.entity.AppendAlias(ctx, entityID, "reddit", flair)
	}
	if display := strings.TrimSpace(item.EntityDisplayName); display != "" &&
		!strings.EqualFold(display, item.EntityTwitchLogin) {
		_ = w.entity.AppendAlias(ctx, entityID, "reddit", display)
	}
}

// SamplerHealth returns directory sampler status for APIs.
func (w *Workers) SamplerHealth() *DirectorySamplerHealth { return w.samplerHealth }

// WindowScoreHealth returns window score recompute status for APIs.
func (w *Workers) WindowScoreHealth() *WindowScoreHealth { return w.windowScoreHealth }

func (w *Workers) Start(parent context.Context) {
	ctx, cancel := context.WithCancel(parent)
	w.cancel = cancel
	go w.recomputeWindowScores(ctx)
	w.wg.Add(5)
	go w.runRetention(ctx)
	go w.runIngest(ctx)
	go w.runFingerprints(ctx)
	go w.runTrendSampler(ctx)
	go w.runDirectorySampler(ctx)
}

func (w *Workers) Stop() {
	if w.cancel != nil {
		w.cancel()
	}
	w.wg.Wait()
}

func (w *Workers) runRetention(ctx context.Context) {
	defer w.wg.Done()
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			n, err := w.opts.Store.DeleteExpiredItems(ctx)
			if err != nil {
				w.opts.Logger.Warn("retention reaper failed", "err", err)
			} else if n > 0 {
				w.opts.Logger.Info("expired social items purged", "count", n)
			}
			if w.opts.Config.PulseWireEnabled {
				dn, err := w.opts.Store.DeleteExpiredDirectorySamples(ctx, w.opts.Config.PulseDirectoryRetentionDays)
				if err != nil {
					w.opts.Logger.Warn("directory sample retention failed", "err", err)
				} else if dn > 0 {
					w.opts.Logger.Info("expired directory samples purged", "count", dn)
				}
			}
		}
	}
}

func (w *Workers) runIngest(ctx context.Context) {
	defer w.wg.Done()
	w.ingestAll(ctx)
	ticker := time.NewTicker(w.opts.Config.IngestPollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.ingestAll(ctx)
		}
	}
}

func (w *Workers) runTrendSampler(ctx context.Context) {
	defer w.wg.Done()
	ticker := time.NewTicker(w.opts.Config.IngestPollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			ids, err := w.opts.Store.ListSampleableClusterIDs(ctx)
			if err != nil {
				w.opts.Logger.Warn("trend sampler list clusters failed", "err", err)
				continue
			}
			for _, id := range ids {
				if err := w.trend.Sample(ctx, id); err != nil {
					w.opts.Logger.Warn("trend sample failed", "cluster", id, "err", err)
				}
			}
		}
	}
}

func (w *Workers) runFingerprints(ctx context.Context) {
	defer w.wg.Done()
	ticker := time.NewTicker(w.opts.Config.FingerprintPollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.pullAndAttachOrigins(ctx)
		}
	}
}

func (w *Workers) pullAndAttachOrigins(ctx context.Context) {
	if w.opts.Store == nil || w.puller == nil {
		return
	}
	since := time.Now().Add(-24 * time.Hour)
	streams, err := w.opts.Store.ListOriginCandidateStreams(ctx, w.opts.Config.AlwaysTrackedChannels, since, 30)
	if err != nil {
		w.opts.Logger.Warn("origin stream discovery failed", "err", err)
		return
	}
	for _, stream := range streams {
		if ctx.Err() != nil {
			return
		}
		written, err := w.puller.PullStream(ctx, stream.StreamID, stream.VODID, stream.Login)
		if err != nil {
			w.opts.Logger.Warn("origin fingerprint pull failed", "stream", stream.StreamID, "login", stream.Login, "err", err)
			continue
		}
		if written > 0 {
			w.opts.Logger.Info("origin fingerprints pulled", "stream", stream.StreamID, "login", stream.Login, "count", written)
		}
		if err := w.attachOriginsForStream(ctx, stream); err != nil {
			w.opts.Logger.Warn("origin attach failed", "stream", stream.StreamID, "login", stream.Login, "err", err)
		}
	}
}

func (w *Workers) attachOriginsForStream(ctx context.Context, stream store.OriginCandidateStream) error {
	entity, err := w.opts.Store.EntityByLogin(ctx, stream.Login)
	if err != nil || entity == nil {
		return err
	}
	fps, err := w.opts.Store.ListFingerprintsForStream(ctx, stream.StreamID, 50)
	if err != nil || len(fps) == 0 {
		return err
	}
	from := stream.StartedAt.Add(-30 * time.Minute)
	to := stream.LastSeenAt.Add(6 * time.Hour)
	clusters, err := w.opts.Store.ListOriginClusterCandidates(ctx, entity.ID, from, to, 25)
	if err != nil || len(clusters) == 0 {
		return err
	}
	for _, cluster := range clusters {
		bestID := int64(0)
		bestConfidence := 0.0
		bestOffset := stream.StartedAt
		for _, fp := range fps {
			confidence := originMatchConfidence(cluster.Title, fp)
			if confidence > bestConfidence {
				bestConfidence = confidence
				bestID = fp.ID
				bestOffset = stream.StartedAt.Add(time.Duration(fp.VODOffsetS) * time.Second)
			}
		}
		if bestID == 0 || bestConfidence < 0.55 {
			if err := w.opts.Store.MarkOriginSearched(ctx, cluster.ID, "searched_missing"); err != nil {
				return err
			}
			continue
		}
		if err := w.opts.Store.AttachOriginToCluster(ctx, cluster.ID, bestID, bestConfidence, bestOffset); err != nil {
			return err
		}
		w.opts.Logger.Info("pulse origin attached", "cluster", cluster.ID, "stream", stream.StreamID, "confidence", bestConfidence)
	}
	return nil
}

func originMatchConfidence(title string, fp store.MomentFingerprint) float64 {
	title = strings.TrimSpace(title)
	if title == "" {
		return 0
	}
	quotes := decodeQuoteStrings(fp.TranscriptKW)
	best := 0.0
	for _, quote := range quotes {
		if score := matcher.TitleSimilarity(title, quote); score > best {
			best = score
		}
	}
	if summary := strings.TrimSpace(fp.ChatSpikeSummary); summary != "" {
		if score := matcher.TitleSimilarity(title, summary); score > best {
			best = score
		}
	}
	if fp.OriginConfidence != nil && best > 0 {
		best = (best * 0.8) + (*fp.OriginConfidence * 0.2)
	}
	if best > 1 {
		return 1
	}
	return best
}

func decodeQuoteStrings(raw json.RawMessage) []string {
	if len(raw) == 0 {
		return nil
	}
	var quotes []string
	if err := json.Unmarshal(raw, &quotes); err == nil {
		return quotes
	}
	return nil
}

func (w *Workers) ingestAll(ctx context.Context) {
	w.redditCommentHydrations = 0
	if n, err := w.opts.Store.BackfillPlaceholderRedditSocialText(ctx, 100); err != nil {
		w.opts.Logger.Warn("placeholder reddit text backfill failed", "err", err)
	} else {
		w.opts.Logger.Info("placeholder reddit social text backfilled", "count", n)
	}
	if n, err := w.opts.Store.BackfillPlaceholderClusterTitles(ctx, 100); err != nil {
		w.opts.Logger.Warn("placeholder title backfill failed", "err", err)
	} else {
		w.opts.Logger.Info("placeholder cluster titles backfilled", "count", n)
	}
	if n, err := w.opts.Store.BackfillRedditCanonicalURLs(ctx, 200); err != nil {
		w.opts.Logger.Warn("reddit canonical url backfill failed", "err", err)
	} else if n > 0 {
		w.opts.Logger.Info("reddit canonical urls backfilled", "count", n)
	}
	w.runRedditPlaceholderRepair(ctx)
	w.runEntityTitleBackfill(ctx)
	sources := []social.SocialSource{
		reddit.NewSource(w.opts.Config),
	}
	if w.opts.Config.StreamerbansIngestEnabled {
		sources = append(sources, streamerbans.NewSource(w.opts.Config))
	}
	sources = append(sources, twitchclips.NewSource(w.opts.Config))
	// YouTube search uses the same browser scraper as Analytics TwitchTracker.
	// Run it last and skip when scraper health is bad so Reddit LSF ingest keeps working.
	scraperReady := ScraperReady(ctx, w.opts.Config)
	if scraperReady {
		sources = append(sources, youtube.NewSource(w.opts.Config))
	} else {
		w.opts.Logger.Warn("skipping youtube ingest this cycle; shared scraper unhealthy")
		if w.opts.Health != nil {
			w.opts.Health.RecordSkip("youtube", "shared scraper unhealthy")
		}
	}
	ytKeywords := w.expandedYouTubeKeywords(ctx)
	baseQ := social.Query{
		Since:    time.Now().Add(-48 * time.Hour),
		Keywords: ytKeywords,
	}
	minNonTwitchItems := 4
	nonTwitchIngested := 0
	for _, src := range sources {
		name := src.Name()
		if name == "youtube" && len(ytKeywords) == 0 {
			continue
		}
		if name == "youtube" && !scraperReady {
			continue
		}
		if name == "streamerbans" && !w.opts.Config.StreamerbansIngestEnabled {
			continue
		}
		q := baseQ
		switch name {
		case "reddit":
			q.Budget = social.Budget{MaxItems: 28, MaxBrowserFetches: socialBrowserFetchBudget(w.opts.Config)}
		case "streamerbans":
			q.Budget = social.Budget{MaxItems: 8, MaxBrowserFetches: socialBrowserFetchBudget(w.opts.Config)}
		case "youtube":
			q.Budget = social.Budget{MaxItems: 12, MaxBrowserFetches: youtubeBrowserFetchBudget(w.opts.Config)}
		case "twitchclips":
			q.Budget = social.Budget{MaxItems: minInt(20, w.opts.Config.PulseDirectoryTopN/4)}
			if q.Budget.MaxItems <= 0 {
				q.Budget.MaxItems = 12
			}
		}
		if name != "twitchclips" && nonTwitchIngested < minNonTwitchItems {
			if q.Budget.MaxItems < minNonTwitchItems {
				q.Budget.MaxItems = minNonTwitchItems
			}
		}
		if err := src.Healthy(ctx); err != nil {
			w.opts.Logger.Warn("source unhealthy", "source", name, "err", err)
			if w.opts.Health != nil {
				w.opts.Health.RecordSkip(name, err.Error())
			}
			continue
		}
		if name == "twitchclips" {
			w.ingestDirectorySeededClips(ctx, src, q)
			continue
		}
		page, err := src.Search(ctx, q)
		if err != nil {
			w.opts.Logger.Warn("source ingest failed", "source", name, "err", err)
			if w.opts.Health != nil {
				w.opts.Health.RecordFail(name, err)
			}
			continue
		}
		if w.opts.Health != nil {
			w.opts.Health.RecordOK(name, len(page.Items))
		}
		if name != "twitchclips" {
			nonTwitchIngested += len(page.Items)
		}
		for _, item := range page.Items {
			w.persistAndMatch(ctx, name, item)
		}
		if name == "reddit" {
			w.ingestTieredRedditLogins(ctx)
		}
	}
	w.setLastIngestAt(time.Now().UTC())
	w.recomputeWindowScores(ctx)
	w.runBackfillOnce(ctx)
	w.runRedditThumbnailRepair(ctx)
	w.runRedditMetricsRepair(ctx)
}

func (w *Workers) runRedditPlaceholderRepair(ctx context.Context) {
	w.redditRepairOnce.Do(func() {
		remaining, err := w.opts.Store.CountPlaceholderRedditSocialText(ctx)
		if err != nil {
			w.opts.Logger.Warn("placeholder reddit repair count failed", "err", err)
			return
		}
		if remaining == 0 {
			return
		}
		src := reddit.NewSource(w.opts.Config)
		if err := src.Healthy(ctx); err != nil {
			w.opts.Logger.Warn("placeholder reddit repair skipped", "err", err)
			return
		}
		page, err := src.Search(ctx, social.Query{
			Budget: social.Budget{
				MaxItems:          28,
				MaxBrowserFetches: socialBrowserFetchBudget(w.opts.Config),
			},
		})
		if err != nil {
			w.opts.Logger.Warn("placeholder reddit repair ingest failed", "err", err)
			return
		}
		repaired := 0
		for _, item := range page.Items {
			w.persistAndMatch(ctx, "reddit", item)
			repaired++
		}
		w.opts.Logger.Info("placeholder reddit repair re-ingest complete", "items", repaired, "remaining_before", remaining)
	})
}

func (w *Workers) runRedditMetricsRepair(ctx context.Context) {
	remaining, err := w.opts.Store.CountRedditZeroScoreMetrics(ctx)
	if err != nil {
		w.opts.Logger.Warn("reddit zero-score repair count failed", "err", err)
		return
	}
	if remaining == 0 {
		return
	}
	rows, err := w.opts.Store.ListRedditNeedingMetricsRepair(ctx, 5)
	if err != nil {
		w.opts.Logger.Warn("reddit metrics repair list failed", "err", err)
		return
	}
	repaired := 0
	for _, row := range rows {
		if w.backfillRedditSocialMetrics(ctx, row.ID, row.Metrics, row.URL) {
			repaired++
		}
	}
	if repaired > 0 {
		w.opts.Logger.Info("reddit metrics repair complete", "items", repaired, "remaining_before", remaining)
	}
}

func (w *Workers) runRedditThumbnailRepair(ctx context.Context) {
	remaining, err := w.opts.Store.CountRedditNeedingThumbnailRepair(ctx)
	if err != nil {
		w.opts.Logger.Warn("reddit thumbnail repair count failed", "err", err)
		return
	}
	if remaining == 0 {
		return
	}
	rows, err := w.opts.Store.ListRedditNeedingThumbnailRepair(ctx, 3)
	if err != nil {
		w.opts.Logger.Warn("reddit thumbnail repair list failed", "err", err)
		return
	}
	repaired := 0
	for _, row := range rows {
		if w.backfillRedditSocialMetrics(ctx, row.ID, nil, row.URL) {
			repaired++
		}
	}
	if repaired > 0 {
		w.opts.Logger.Info("reddit thumbnail repair complete", "items", repaired, "remaining_before", remaining)
	}
}

func (w *Workers) backfillRedditSocialMetrics(ctx context.Context, socialID int64, metrics json.RawMessage, postURL string) bool {
	existing := map[string]any{}
	if len(metrics) > 0 {
		_ = json.Unmarshal(metrics, &existing)
	}
	needsThumb := strings.TrimSpace(stringFromAny(existing["thumbnail_url"])) == ""
	needsScore := metricFloat(existing["score"]) == 0
	needsComments := metricFloat(existing["comments"]) == 0
	needsSelftext := strings.TrimSpace(stringFromAny(existing["selftext"])) == ""
	needsExternal := strings.TrimSpace(stringFromAny(existing["external_url"])) == ""
	if !needsThumb && !needsScore && !needsComments && !needsSelftext && !needsExternal {
		return false
	}
	src := reddit.NewSource(w.opts.Config)
	meta, ok := src.FetchPostMeta(ctx, postURL)
	if !ok {
		return false
	}
	patch := map[string]any{}
	if needsThumb && meta.Thumbnail != "" {
		patch["thumbnail_url"] = meta.Thumbnail
		patch["thumbnail_source"] = "reddit_json"
		patch["thumbnail_status"] = "ready"
	}
	if needsScore && meta.Score > 0 {
		patch["score"] = float64(meta.Score)
	}
	if needsComments && meta.Comments > 0 {
		patch["comments"] = float64(meta.Comments)
	}
	if needsSelftext && meta.SelfText != "" {
		patch["selftext"] = meta.SelfText
	}
	if needsExternal && meta.ExternalURL != "" {
		patch["external_url"] = meta.ExternalURL
	}
	if len(patch) == 0 {
		return false
	}
	incoming, _ := json.Marshal(patch)
	merged := store.MergeSocialMetrics(metrics, incoming)
	if err := w.opts.Store.UpdateSocialItemMetrics(ctx, socialID, merged); err != nil {
		w.opts.Logger.Warn("reddit metrics backfill failed", "item", socialID, "err", err)
		return false
	}
	return true
}

func metricFloat(v any) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case int:
		return float64(n)
	case json.Number:
		f, _ := n.Float64()
		return f
	default:
		return 0
	}
}

func stringFromAny(v any) string {
	if v == nil {
		return ""
	}
	return strings.TrimSpace(strings.Trim(strings.TrimSpace(fmt.Sprint(v)), `"`))
}

func (w *Workers) runEntityTitleBackfill(ctx context.Context) {
	w.entityBackfillOnce.Do(func() {
		rows, err := w.opts.Store.ListClustersMissingEntity(ctx, 120)
		if err != nil {
			w.opts.Logger.Warn("entity title backfill list failed", "err", err)
			return
		}
		if len(rows) == 0 {
			return
		}
		titleHints := append([]string{}, w.opts.Config.AlwaysTrackedChannels...)
		titleHints = append(titleHints, w.opts.Config.StorygraphYTKeywords...)
		if seeds, err := w.opts.Store.DirectorySeedLogins(ctx, 300); err == nil {
			titleHints = append(titleHints, seeds...)
		}
		attached := 0
		for _, row := range rows {
			entityID, _, err := w.entity.ResolveLoginFromTitle(ctx, row.Title, titleHints)
			if err != nil || entityID == nil {
				continue
			}
			if err := w.opts.Store.UpdateClusterMeta(ctx, row.ID, entityID, "", "", "", ""); err != nil {
				w.opts.Logger.Warn("entity title backfill attach failed", "cluster_id", row.ID, "err", err)
				continue
			}
			attached++
		}
		if attached > 0 {
			w.opts.Logger.Info("entity title backfill attached", "count", attached, "candidates", len(rows))
		}
	})
}

func (w *Workers) recomputeWindowScores(ctx context.Context) {
	if w.window == nil {
		return
	}
	if err := w.window.RecomputeAll(ctx, time.Now().UTC()); err != nil {
		w.opts.Logger.Warn("window score recompute failed", "err", err)
		if w.windowScoreHealth != nil {
			w.windowScoreHealth.RecordFailure(err.Error())
		}
		return
	}
	if w.windowScoreHealth != nil {
		w.windowScoreHealth.RecordSuccess()
	}
}

func (w *Workers) runBackfillOnce(ctx context.Context) {
	if w.backfill == nil {
		return
	}
	w.backfillOnce.Do(func() {
		go func() {
			counts, err := w.backfill.Run(ctx, "reddit", "youtube")
			if err != nil {
				w.opts.Logger.Warn("backfill run failed", "err", err)
				return
			}
			if len(counts) > 0 {
				w.opts.Logger.Info("backfill imported", "counts", counts)
				w.recomputeWindowScores(ctx)
			}
		}()
	})
}

var youtubeNewsTerms = []string{"drama", "clip", "reaction", "ban", "exposed", "funny", "crash out", "apology"}

func (w *Workers) expandedYouTubeKeywords(ctx context.Context) []string {
	seen := map[string]struct{}{}
	var out []string
	add := func(kw string) {
		kw = strings.TrimSpace(kw)
		if kw == "" {
			return
		}
		key := strings.ToLower(kw)
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		out = append(out, kw)
	}
	for _, kw := range w.opts.Config.StorygraphYTKeywords {
		add(kw)
	}
	if len(out) == 0 {
		for _, login := range w.opts.Config.AlwaysTrackedChannels {
			add(login)
		}
	}
	if seeds, err := w.opts.Store.DirectorySeedLogins(ctx, 25); err == nil {
		for _, login := range seeds {
			add(login)
			for _, term := range youtubeNewsTerms {
				if len(out) >= 48 {
					break
				}
				add(login + " " + term)
			}
		}
	}
	for _, term := range youtubeNewsTerms {
		if len(out) >= 48 {
			break
		}
		add("streamer " + term)
	}
	if len(out) > 48 {
		out = out[:48]
	}
	return out
}

func (w *Workers) persistAndMatch(ctx context.Context, sourceName string, item social.Item) {
	if sourceName == "youtube" {
		yt := youtube.NewSource(w.opts.Config)
		yt.EnrichItemText(ctx, &item)
	}
	metrics := w.buildSocialMetrics(sourceName, item)
	expires := item.ExpiresAt
	if expires.IsZero() {
		expires = time.Now().Add(time.Duration(w.opts.Config.SocialRetentionDays) * 24 * time.Hour)
	}
	prov, _ := json.Marshal(item.Provenance)
	hash := item.SnapshotSHA256
	if len(hash) == 0 {
		h := sha256.Sum256([]byte(item.URL + item.Text))
		hash = h[:]
	}
	entityID, err := w.resolveEntity(ctx, item)
	if err != nil {
		w.opts.Logger.Warn("entity resolve failed", "source", sourceName, "text", item.Text, "err", err)
	}
	if entityID != nil && sourceName == "reddit" {
		w.maybeLearnRedditAlias(ctx, *entityID, item)
	}
	socialID, err := w.opts.Store.UpsertSocialItem(ctx, store.SocialItem{
		Source:       item.Source,
		Kind:         item.Kind,
		ExternalID:   item.ExternalID,
		URL:          item.URL,
		Author:       item.Author,
		CreatedAtSrc: ptrTime(item.CreatedAt),
		Text:         item.Text,
		Metrics:      metrics,
		EntityID:     entityID,
		ExpiresAt:    expires,
	}, prov, hash)
	if err != nil {
		return
	}
	if err := bans.UpsertFromSocialItem(ctx, w.opts.Store, sourceName, socialID, item); err != nil {
		w.opts.Logger.Warn("ban event upsert failed", "source", sourceName, "item", socialID, "err", err)
	}
	canonicalURLs := collectCanonicalURLs(item)
	clusterID, alreadyLinked, matchKind, err := w.cluster.EnsureWireStory(ctx, socialID, entityID, item.Text, sourceName, item.FlairText, canonicalURLs)
	if err != nil {
		return
	}
	sourceType := sourceTypeFor(sourceName)
	weight := w.opts.Reliability.Weight(sourceType)
	var evidenceID *int64
	if !alreadyLinked {
		id, _ := w.opts.Store.InsertEvidence(ctx, store.Evidence{
			ClusterID:  clusterID,
			ItemID:     &socialID,
			SourceType: sourceType,
			SourceURL:  item.URL,
			MatchConf:  wireMatchConfidence(sourceName, entityID != nil, matchKind),
			Weight:     weight,
			OccurredAt: ptrTime(item.CreatedAt),
		})
		if id > 0 {
			evidenceID = &id
		}
	}
	w.attachEvidencePreviews(ctx, clusterID, evidenceID, item, sourceName, previewMatchKind(matchKind))
	if sourceName == "reddit" {
		w.backfillRedditSocialMetrics(ctx, socialID, metrics, item.URL)
	}
	_ = w.score.RefreshConfidence(ctx, w.opts.Store, clusterID)
	if err := w.trend.Sample(ctx, clusterID); err != nil {
		w.opts.Logger.Warn("trend sample failed", "cluster", clusterID, "err", err)
	}
}

func (w *Workers) attachEvidencePreviews(ctx context.Context, clusterID int64, evidenceID *int64, item social.Item, sourceName, matchKind string) {
	seen := map[string]struct{}{}
	attach := func(rawURL, titleHint string) {
		if rawURL == "" {
			return
		}
		link, ok := evidenceurl.Canonicalize(rawURL)
		if !ok || !evidenceurl.Attachable(link) {
			return
		}
		if _, exists := seen[link.CanonicalURL]; exists {
			return
		}
		seen[link.CanonicalURL] = struct{}{}
		if _, _, err := w.preview.AttachURL(ctx, w.opts.Store, clusterID, evidenceID, link.CanonicalURL, matchKind, "", titleHint); err != nil {
			w.opts.Logger.Warn("evidence preview attach failed", "url", link.CanonicalURL, "err", err)
		}
	}
	attach(item.URL, strings.TrimSpace(item.Text))
	for _, link := range evidenceurl.Extract(item.Text) {
		if !evidenceurl.Attachable(link) {
			continue
		}
		attach(link.CanonicalURL, "")
	}
	for _, media := range item.Media {
		if strings.EqualFold(strings.TrimSpace(media.Kind), "image") {
			w.attachImagePreview(ctx, clusterID, evidenceID, item.URL, media.URL, strings.TrimSpace(item.Text))
			continue
		}
		attach(media.URL, "")
	}
	if sourceName == "reddit" && item.Kind == "post" && item.URL != "" {
		w.attachRedditCommentLinks(ctx, clusterID, evidenceID, item.URL)
	}
}

func (w *Workers) attachRedditCommentLinks(ctx context.Context, clusterID int64, evidenceID *int64, postURL string) {
	const maxCommentHydrationsPerCycle = 2
	if socialBrowserFetchBudget(w.opts.Config) > 0 && w.redditCommentHydrations >= maxCommentHydrationsPerCycle {
		if w.opts.Health != nil {
			w.opts.Health.RecordDetailSkip("reddit", "comments", "comment hydration capped for this ingest cycle")
		}
		return
	}
	src := reddit.NewSource(w.opts.Config)
	fetcher, ok := interface{}(src).(social.CommentFetcher)
	if !ok {
		if w.opts.Health != nil {
			w.opts.Health.RecordDetailSkip("reddit", "comments", "comment fetcher unavailable")
		}
		return
	}
	page, err := fetcher.Comments(ctx, postURL, social.Query{Budget: social.Budget{MaxItems: 8, MaxBrowserFetches: socialBrowserFetchBudget(w.opts.Config)}})
	if err != nil {
		if w.opts.Health != nil {
			w.opts.Health.RecordDetailFail("reddit", "comments", err)
		}
		w.opts.Logger.Warn("reddit comment fetch failed", "url", postURL, "err", err)
		return
	}
	if w.opts.Health != nil {
		w.opts.Health.RecordDetailOK("reddit", "comments", len(page.Items))
	}
	w.redditCommentHydrations++
	for _, canonicalURL := range extractAttachableCommentLinks(page.Items) {
		if _, _, err := w.preview.AttachURL(ctx, w.opts.Store, clusterID, evidenceID, canonicalURL, "url", "reddit comment link", ""); err != nil {
			w.opts.Logger.Warn("comment preview attach failed", "url", canonicalURL, "err", err)
		}
	}
}

func extractAttachableCommentLinks(comments []social.Item) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0)
	for _, comment := range comments {
		for _, link := range evidenceurl.Extract(comment.Text) {
			if !evidenceurl.Attachable(link) {
				continue
			}
			if _, ok := seen[link.CanonicalURL]; ok {
				continue
			}
			seen[link.CanonicalURL] = struct{}{}
			out = append(out, link.CanonicalURL)
		}
	}
	return out
}

func thumbnailSourceFor(sourceName string, item social.Item) string {
	switch sourceName {
	case "twitchclips":
		return "helix"
	case "streamerbans":
		return "streamerbans"
	case "reddit":
		if strings.Contains(strings.ToLower(item.Provenance.SourceAPI), "oauth") {
			return "reddit_oauth"
		}
		return "reddit_lsf"
	default:
		return ""
	}
}

func (w *Workers) buildSocialMetrics(sourceName string, item social.Item) json.RawMessage {
	payload := map[string]any{}
	for key, value := range item.Metrics {
		payload[key] = value
	}
	category := cluster.ClassifyCategory(item.Text, item.FlairText)
	if sourceName == "streamerbans" {
		category = "bans"
	}
	if category != "" {
		payload["pulse_category"] = category
	}
	if flair := strings.TrimSpace(item.FlairText); flair != "" {
		payload["flair"] = flair
	}
	thumbSource := thumbnailSourceFor(sourceName, item)
	for _, media := range item.Media {
		if !strings.EqualFold(strings.TrimSpace(media.Kind), "image") {
			continue
		}
		if thumb := strings.TrimSpace(media.URL); thumb != "" {
			payload["thumbnail_url"] = thumb
			if thumbSource != "" {
				payload["thumbnail_source"] = thumbSource
				payload["thumbnail_status"] = "ready"
			}
			break
		}
	}
	if sourceName == "twitchclips" {
		if _, ok := payload["thumbnail_url"]; !ok {
			payload["thumbnail_status"] = "pending"
		}
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return json.RawMessage("{}")
	}
	return raw
}

func (w *Workers) attachImagePreview(ctx context.Context, clusterID int64, evidenceID *int64, canonicalURL, imageURL, titleHint string) {
	if evidenceID == nil || imageURL == "" {
		return
	}
	link, ok := evidenceurl.Canonicalize(canonicalURL)
	if !ok {
		link, ok = evidenceurl.Canonicalize(imageURL)
	}
	if !ok {
		return
	}
	now := time.Now()
	expires := now.Add(7 * 24 * time.Hour)
	nextFetch := expires
	preview := store.EvidencePreview{
		CanonicalURL:  link.CanonicalURL,
		Platform:      link.Platform,
		Title:         strings.TrimSpace(titleHint),
		ThumbnailURL:  strings.TrimSpace(imageURL),
		PreviewStatus: "ready",
		FetchedAt:     now,
		ExpiresAt:     expires,
		NextFetchAt:   &nextFetch,
	}
	previewID, err := w.opts.Store.UpsertEvidencePreview(ctx, preview)
	if err != nil {
		w.opts.Logger.Warn("clip thumbnail preview failed", "url", imageURL, "err", err)
		return
	}
	if err := w.opts.Store.LinkEvidencePreview(ctx, clusterID, evidenceID, previewID, "media", "clip thumbnail"); err != nil {
		w.opts.Logger.Warn("clip thumbnail link failed", "url", imageURL, "err", err)
	}
}

func ptrTime(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}

func collectCanonicalURLs(item social.Item) []string {
	seen := map[string]struct{}{}
	var out []string
	add := func(raw string) {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			return
		}
		link, ok := evidenceurl.Canonicalize(raw)
		if !ok || !evidenceurl.Attachable(link) {
			return
		}
		if _, exists := seen[link.CanonicalURL]; exists {
			return
		}
		seen[link.CanonicalURL] = struct{}{}
		out = append(out, link.CanonicalURL)
	}
	add(item.URL)
	for _, link := range evidenceurl.Extract(item.Text) {
		add(link.CanonicalURL)
	}
	for _, media := range item.Media {
		if strings.EqualFold(strings.TrimSpace(media.Kind), "image") {
			continue
		}
		add(media.URL)
	}
	return out
}

func (w *Workers) resolveEntity(ctx context.Context, item social.Item) (*int64, error) {
	if resolved, err := w.entity.ResolveItem(ctx, item); resolved != nil || err != nil {
		return resolved, err
	}
	titleHints := append([]string{}, w.opts.Config.AlwaysTrackedChannels...)
	titleHints = append(titleHints, w.opts.Config.StorygraphYTKeywords...)
	if seeds, err := w.opts.Store.DirectorySeedLogins(ctx, 300); err == nil {
		titleHints = append(titleHints, seeds...)
	}
	id, _, err := w.entity.ResolveLoginFromTitle(ctx, item.Text, titleHints)
	return id, err
}

func sourceTypeFor(sourceName string) string {
	switch sourceName {
	case "youtube":
		return "youtube_video"
	case "twitchclips":
		return "twitch_clip"
	case "streamerbans":
		return "streamerbans_post"
	default:
		return sourceName + "_thread"
	}
}

func previewMatchKind(matchKind string) string {
	switch matchKind {
	case "canonical_url", "title_similarity", "exact_title", "item_dedup":
		return matchKind
	default:
		return "url"
	}
}

func wireMatchConfidence(sourceName string, resolvedEntity bool, matchKind string) float64 {
	switch matchKind {
	case "canonical_url":
		return 0.96
	case "exact_title":
		return 0.90
	case "title_similarity":
		return 0.82
	case "item_dedup":
		return 0.99
	}
	switch sourceName {
	case "twitchclips":
		if resolvedEntity {
			return 0.92
		}
		return 0.84
	case "youtube":
		if resolvedEntity {
			return 0.78
		}
		return 0.66
	case "streamerbans":
		if resolvedEntity {
			return 0.88
		}
		return 0.80
	default:
		if resolvedEntity {
			return 0.72
		}
		return 0.58
	}
}

func (w *Workers) ingestDirectorySeededClips(ctx context.Context, src social.SocialSource, baseQ social.Query) {
	logins, err := w.opts.Store.DirectorySeedLogins(ctx, w.opts.Config.PulseDirectoryTopN)
	if err != nil || len(logins) == 0 {
		page, searchErr := src.Search(ctx, baseQ)
		if searchErr != nil {
			w.opts.Logger.Warn("source ingest failed", "source", "twitchclips", "err", searchErr)
			if w.opts.Health != nil {
				w.opts.Health.RecordFail("twitchclips", searchErr)
			}
			return
		}
		if w.opts.Health != nil {
			w.opts.Health.RecordOK("twitchclips", len(page.Items))
		}
		for _, item := range page.Items {
			w.persistAndMatch(ctx, "twitchclips", item)
		}
		return
	}
	maxItems := baseQ.Budget.MaxItems
	if maxItems <= 0 {
		maxItems = 12
	}
	perLogin := maxInt(1, maxItems/maxInt(1, len(logins)))
	if perLogin > 3 {
		perLogin = 3
	}
	total := 0
	for _, login := range logins {
		if total >= maxItems {
			break
		}
		q := baseQ
		q.Entity = social.EntityRef{TwitchLogin: login}
		q.Budget = social.Budget{MaxItems: perLogin}
		page, err := src.Search(ctx, q)
		if err != nil {
			continue
		}
		for _, item := range page.Items {
			w.persistAndMatch(ctx, "twitchclips", item)
			total++
			if total >= maxItems {
				break
			}
		}
	}
	if w.opts.Health != nil {
		w.opts.Health.RecordOK("twitchclips", total)
	}
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func socialBrowserFetchBudget(cfg config.Config) int {
	if cfg.SocialBrowserFetchBudget <= 0 {
		return -1
	}
	return cfg.SocialBrowserFetchBudget
}

func youtubeBrowserFetchBudget(cfg config.Config) int {
	if cfg.YouTubeBrowserFetchBudget <= 0 {
		return -1
	}
	return cfg.YouTubeBrowserFetchBudget
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
