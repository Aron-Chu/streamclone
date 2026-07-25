package worker

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"

	"streamclone/internal/emote/assets"
	"streamclone/internal/emote/dict"
	"streamclone/internal/emote/flags"
	"streamclone/internal/emote/objstore"
	"streamclone/internal/emote/render"
	"streamclone/internal/emote/store"
	"streamclone/internal/metrics"
)

const (
	maxAttempts               = 3
	defaultDictionaryDebounce = 750 * time.Millisecond
	minDictionaryDebounce     = 50 * time.Millisecond
)

type Worker struct {
	st                 *store.Store
	obj                objstore.Store
	d                  *dict.Dict
	renderCfg          render.Config
	log                *slog.Logger
	dirtyChannels      chan string
	dictionaryDebounce time.Duration
	startOnce          sync.Once
}

func New(st *store.Store, obj objstore.Store, d *dict.Dict, log *slog.Logger) *Worker {
	return NewWithConfig(st, obj, d, render.Config{
		DefaultScales: []string{"1x", "2x", "3x", "4x"},
		AllowedScales: []string{"1x", "2x", "3x", "4x"},
	}, log, defaultDictionaryDebounce)
}

func NewWithDictionaryDebounce(st *store.Store, obj objstore.Store, d *dict.Dict, log *slog.Logger, debounce time.Duration) *Worker {
	return NewWithConfig(st, obj, d, render.Config{
		DefaultScales: []string{"1x", "2x", "3x", "4x"},
		AllowedScales: []string{"1x", "2x", "3x", "4x"},
	}, log, debounce)
}

func NewWithConfig(st *store.Store, obj objstore.Store, d *dict.Dict, cfg render.Config, log *slog.Logger, debounce time.Duration) *Worker {
	if debounce < minDictionaryDebounce {
		debounce = minDictionaryDebounce
	}
	return &Worker{
		st:                 st,
		obj:                obj,
		d:                  d,
		renderCfg:          cfg,
		log:                log,
		dirtyChannels:      make(chan string, 2048),
		dictionaryDebounce: debounce,
	}
}

func (w *Worker) RunPool(ctx context.Context, concurrency int) {
	w.startOnce.Do(func() {
		go w.dictionaryLoop(ctx)
	})
	for i := 0; i < concurrency; i++ {
		go w.loop(ctx)
	}
}

func (w *Worker) loop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		if err := w.processOne(ctx); err != nil {
			if err == pgx.ErrNoRows {
				select {
				case <-ctx.Done():
					return
				case <-time.After(2 * time.Second):
				}
			} else {
				w.log.Error("worker error", "err", err)
				time.Sleep(500 * time.Millisecond)
			}
		}
	}
}

func (w *Worker) processOne(ctx context.Context) error {
	job, err := w.st.ClaimJob(ctx)
	if err != nil {
		return err
	}
	started := time.Now()
	fail := func(errMsg string) error {
		result := "retry"
		if job.Attempts >= maxAttempts {
			result = "failed"
		}
		metrics.AssetProcessSeconds.WithLabelValues(result).Observe(time.Since(started).Seconds())
		return w.failOrRetry(ctx, job, errMsg)
	}

	w.log.Info("processing job", "job_id", job.ID, "emote_id", job.EmoteID)

	emote, emoteErr := w.st.GetEmote(ctx, job.EmoteID)
	provider := "custom"
	if emoteErr == nil && emote.Provider != "" {
		provider = emote.Provider
	}

	src, err := w.obj.GetSrc(ctx, job.EmoteID)
	if err != nil {
		return fail("fetch source: " + err.Error())
	}

	dir, err := os.MkdirTemp("", "emote-src-*")
	if err != nil {
		return fail("mktemp: " + err.Error())
	}
	defer os.RemoveAll(dir)

	srcPath := filepath.Join(dir, "src.webp")
	if err := os.WriteFile(srcPath, src, 0600); err != nil {
		return fail("write src: " + err.Error())
	}

	_, jobScales := render.ParseJobSourceKey(job.SourceKey)
	renditions, err := assets.RenderScales(srcPath, render.ResolveScales(
		jobScales,
		w.renderCfg.DefaultScales,
		w.renderCfg.AllowedScales,
	))
	if err != nil {
		return fail("render: " + err.Error())
	}

	var written []string
	for _, r := range renditions {
		scaleStarted := time.Now()
		if err := w.obj.Put(ctx, job.EmoteID, r.Scale, r.Data); err != nil {
			for _, scale := range written {
				_ = w.obj.Delete(ctx, job.EmoteID, scale)
			}
			metrics.EmoteRenderFailed.WithLabelValues(provider, r.Scale, "upload").Inc()
			return fail("upload " + r.Scale + ": " + err.Error())
		}
		written = append(written, r.Scale)
		metrics.EmoteRenderCompleted.WithLabelValues(provider, r.Scale).Inc()
		metrics.EmoteRenderDuration.WithLabelValues(provider, r.Scale).Observe(time.Since(scaleStarted).Seconds())
	}

	if err := w.st.SetEmoteStatus(ctx, job.EmoteID, 1); err != nil {
		for _, scale := range written {
			_ = w.obj.Delete(ctx, job.EmoteID, scale)
		}
		return fail("activate emote: " + err.Error())
	}
	w.queueAffectedDictionaries(ctx, job.EmoteID)

	metrics.AssetJobs.WithLabelValues("success").Inc()
	metrics.AssetProcessSeconds.WithLabelValues("success").Observe(time.Since(started).Seconds())
	err = w.st.FinishJob(ctx, job.ID, true, "")
	render.SyncQueueDepthMetric(ctx, w.st)
	return err
}

func (w *Worker) queueAffectedDictionaries(ctx context.Context, emoteID string) {
	if w.d == nil {
		return
	}
	channels, err := w.st.GetChannelsForEmote(ctx, emoteID)
	if err != nil {
		w.log.Warn("find affected emote channels", "emote_id", emoteID, "err", err)
		return
	}
	for _, login := range channels {
		select {
		case w.dirtyChannels <- login:
		case <-ctx.Done():
			return
		default:
			metrics.EmoteDictionaryQueueDrops.Inc()
			w.log.Warn("emote dictionary rebuild queue full", "channel", login)
		}
	}
}

func (w *Worker) dictionaryLoop(ctx context.Context) {
	if w.d == nil {
		return
	}
	pending := make(map[string]struct{})
	var timer *time.Timer
	var timerC <-chan time.Time

	for {
		select {
		case <-ctx.Done():
			if timer != nil {
				timer.Stop()
			}
			return
		case login := <-w.dirtyChannels:
			if login == "" {
				continue
			}
			pending[login] = struct{}{}
			if timer == nil {
				timer = time.NewTimer(w.dictionaryDebounce)
				timerC = timer.C
			}
		case <-timerC:
			batch := pending
			pending = make(map[string]struct{})
			timer = nil
			timerC = nil
			w.rebuildDictionaryBatch(ctx, batch)
		}
	}
}

func (w *Worker) rebuildDictionaryBatch(ctx context.Context, channels map[string]struct{}) {
	if len(channels) == 0 {
		return
	}
	started := time.Now()
	totalEntries := 0
	for login := range channels {
		count, err := w.rebuildChannelDictionary(ctx, login)
		if err != nil {
			w.log.Warn("rebuild emote dictionary", "channel", login, "err", err)
			continue
		}
		totalEntries += count
	}
	w.log.Info("rebuilt emote dictionaries", "channels", len(channels), "entries", totalEntries, "elapsed_ms", time.Since(started).Milliseconds())
}

func (w *Worker) rebuildChannelDictionary(ctx context.Context, login string) (int, error) {
	emotes, err := w.st.GetChannelEmotes(ctx, login)
	if err != nil {
		return 0, err
	}
	entries := make([]dict.EmoteEntry, 0, len(emotes))
	for _, e := range emotes {
		entries = append(entries, dict.EmoteEntry{
			Name:            e.Name,
			EmoteID:         e.EmoteID,
			ProviderEmoteID: e.ProviderEmoteID,
			ZeroWidth:       flags.IsZeroWidth(e.Flags),
			Provider:        e.Provider,
		})
	}
	if err := w.d.Rebuild(ctx, login, entries); err != nil {
		return len(entries), err
	}
	return len(entries), nil
}

func (w *Worker) failOrRetry(ctx context.Context, job *store.Job, errMsg string) error {
	w.log.Warn("job failed", "job_id", job.ID, "err", errMsg, "attempts", job.Attempts)
	var err error
	if job.Attempts >= maxAttempts {
		_ = w.st.SetEmoteStatus(ctx, job.EmoteID, 2)
		metrics.AssetJobs.WithLabelValues("failed").Inc()
		err = w.st.FinishJob(ctx, job.ID, false, errMsg)
	} else {
		metrics.AssetJobs.WithLabelValues("retry").Inc()
		err = w.st.RetryJob(ctx, job.ID, errMsg)
	}
	render.SyncQueueDepthMetric(ctx, w.st)
	return err
}
