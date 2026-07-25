package main

import (
	"context"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"streamclone/internal/config"
	"streamclone/internal/emote/api"
	"streamclone/internal/emote/dict"
	"streamclone/internal/emote/eventapi"
	"streamclone/internal/emote/objstore"
	"streamclone/internal/emote/preload"
	"streamclone/internal/emote/render"
	"streamclone/internal/emote/seeder"
	"streamclone/internal/emote/store"
	"streamclone/internal/emote/worker"
	"streamclone/internal/httpx"
	"streamclone/internal/log"
	"streamclone/internal/metadata/helix"
	"streamclone/internal/metrics"
)

func main() {
	cfg, err := config.Load()
	logger := log.New("emote", cfg.LogLevel)
	if err != nil {
		logger.Error("config load failed", "err", err)
		os.Exit(1)
	}
	if err := cfg.ValidateEmoteObjectStorage(); err != nil {
		logger.Error("emote object storage config invalid", "err", err)
		os.Exit(1)
	}

	ctx := context.Background()

	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		logger.Error("db connect failed", "err", err)
		os.Exit(1)
	}
	defer pool.Close()

	opt, err := redis.ParseURL(cfg.RedisURL)
	if err != nil {
		logger.Error("redis url parse failed", "err", err)
		os.Exit(1)
	}
	rdb := redis.NewClient(opt)
	defer rdb.Close()

	localObj, err := newObjectStore(cfg.S3Endpoint, cfg.S3AccessKey, cfg.S3SecretKey, cfg.S3Bucket, cfg.S3Prefix)
	if err != nil {
		logger.Error("local objstore init failed", "err", err)
		os.Exit(1)
	}

	var secondaryObj objstore.Store
	if cfg.EmoteObjectSecondaryEnabled {
		secondaryObj, err = newObjectStore(
			cfg.EmoteObjectSecondaryEndpoint,
			cfg.EmoteObjectSecondaryAccessKey,
			cfg.EmoteObjectSecondarySecretKey,
			cfg.EmoteObjectSecondaryBucket,
			cfg.EmoteObjectSecondaryPrefix,
		)
		if err != nil {
			logger.Error("secondary objstore init failed", "err", err)
			os.Exit(1)
		}
	}
	obj, err := objstore.NewReadThroughStore(localObj, secondaryObj, objstore.ReadThroughOptions{
		PrimarySecondary: cfg.EmoteObjectSecondaryPrimary,
		DualWrite:        cfg.EmoteObjectDualWrite,
		ReadThrough:      cfg.EmoteObjectReadThrough,
	})
	if err != nil {
		logger.Error("objstore routing init failed", "err", err)
		os.Exit(1)
	}
	if err := obj.EnsureBucket(ctx, cfg.S3PublicRead); err != nil {
		logger.Warn("ensure bucket failed", "err", err)
	}

	st := store.New(pool)
	if n, err := st.RepairProviderMetadataReady(ctx); err != nil {
		logger.Warn("startup provider metadata repair failed", "err", err)
	} else if n > 0 {
		logger.Info("startup provider metadata repair", "count", n)
	}
	d := dict.NewWithTTL(rdb, cfg.CDNPublicBase, cfg.EmoteDictionaryTTL)
	backfillLegacyTTLs := func() {
		opts := dict.LegacyBackfillOptions{
			ScanCount:  500,
			BatchSize:  cfg.EmoteDictionaryLegacyBatchSize,
			BatchPause: cfg.EmoteDictionaryLegacyBatchPause,
			TTLJitter:  cfg.EmoteDictionaryLegacyJitter,
		}
		if n, err := d.BackfillLegacyTTLsWithOptions(ctx, opts); err != nil {
			logger.Warn("legacy emote dictionary ttl backfill failed", "err", err)
		} else {
			logger.Info("legacy emote dictionary ttl backfill complete",
				"updated", n,
				"ttl", cfg.EmoteDictionaryTTL.String(),
				"jitter", cfg.EmoteDictionaryLegacyJitter.String(),
				"batch_size", cfg.EmoteDictionaryLegacyBatchSize,
				"batch_pause", cfg.EmoteDictionaryLegacyBatchPause.String(),
			)
		}
	}
	renderCfg := render.ConfigFromApp(cfg)
	rq := render.NewQueue(st, obj, renderCfg, logger)
	render.StartObserveConsumer(ctx, rdb, rq, logger)
	render.SyncQueueDepthMetric(ctx, st)
	twitch := helix.New(cfg.TwitchAPIURL, cfg.TwitchTokenURL, cfg.TwitchOAuthClientID, cfg.TwitchOAuthClientSecret, "streamclone/emote")
	seed := seeder.NewWithRenderQueue(st, obj, d, rq, logger, cfg.Upstream.SevenTVAPIURL, cfg.Upstream.SevenTVCDNURL, cfg.Upstream.FFZAPIURL, cfg.Upstream.BTTVAPIURL, twitch, cfg.EmoteImportConcurrency)
	w := worker.NewWithConfig(st, obj, d, renderCfg, logger, time.Duration(cfg.EmoteDictionaryDebounceMS)*time.Millisecond)
	workerConcurrency := cfg.EmoteWorkerConcurrency
	if workerConcurrency < 1 {
		workerConcurrency = 1
	}
	w.RunPool(ctx, workerConcurrency)

	eventSub := eventapi.New(st, seed, d, logger)
	eventSub.Start(ctx)
	defer eventSub.Stop()

	if cfg.EmoteRosterPreloadEnabled {
		rosterPreloader := preload.NewRosterPreloader(
			cfg.MetadataServiceURL,
			cfg.EmoteRosterPreloadTopN,
			cfg.AlwaysTrackedChannels,
			seed,
			twitch,
			logger,
		)
		preload.StartRosterPreloaderAfterInitial(
			ctx,
			rosterPreloader,
			cfg.EmoteRosterPreloadInterval,
			logger,
			backfillLegacyTTLs,
		)
		logger.Info("emote roster preloader started",
			"interval", cfg.EmoteRosterPreloadInterval.String(),
			"top_n", cfg.EmoteRosterPreloadTopN,
			"always_tracked", len(cfg.AlwaysTrackedChannels),
		)
	} else {
		go backfillLegacyTTLs()
	}

	h := api.NewWithRenderQueue(st, obj, d, seed, rq, logger, cfg.CuratorAPIToken)
	h.SetEventSubscriber(eventSub)

	srv := httpx.New("emote", cfg.HTTPAddr, logger, metrics.HTTPMiddleware("emote"), httpx.CORS)
	srv.AddReady(func(ctx context.Context) error {
		return st.Ping(ctx)
	})
	srv.AddReady(func(ctx context.Context) error {
		return d.Ping(ctx)
	})
	h.Routes(srv.Router)

	if err := srv.Run(ctx); err != nil {
		logger.Error("server stopped", "err", err)
		os.Exit(1)
	}
}

func newObjectStore(endpoint, accessKey, secretKey, bucket, prefix string) (*objstore.Client, error) {
	useSSL := strings.HasPrefix(strings.TrimSpace(endpoint), "https://")
	endpoint = strings.TrimSpace(endpoint)
	endpoint = strings.TrimPrefix(endpoint, "https://")
	endpoint = strings.TrimPrefix(endpoint, "http://")
	return objstore.New(endpoint, accessKey, secretKey, bucket, prefix, useSSL)
}
