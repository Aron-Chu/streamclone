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
	"streamclone/internal/emote/objstore"
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

	useSSL := strings.HasPrefix(cfg.S3Endpoint, "https://")
	endpoint := strings.TrimPrefix(cfg.S3Endpoint, "https://")
	endpoint = strings.TrimPrefix(endpoint, "http://")

	obj, err := objstore.New(endpoint, cfg.S3AccessKey, cfg.S3SecretKey, cfg.S3Bucket, useSSL)
	if err != nil {
		logger.Error("objstore init failed", "err", err)
		os.Exit(1)
	}

	if err := obj.EnsureBucket(ctx); err != nil {
		logger.Warn("ensure bucket failed", "err", err)
	}

	st := store.New(pool)
	d := dict.New(rdb, cfg.CDNPublicBase)
	twitch := helix.New(cfg.TwitchAPIURL, cfg.TwitchTokenURL, cfg.TwitchOAuthClientID, cfg.TwitchOAuthClientSecret, "streamclone/emote")
	seed := seeder.NewWithImportConcurrency(st, obj, d, logger, cfg.Upstream.SevenTVAPIURL, cfg.Upstream.SevenTVCDNURL, cfg.Upstream.FFZAPIURL, cfg.Upstream.BTTVAPIURL, twitch, cfg.EmoteImportConcurrency)
	w := worker.NewWithDictionaryDebounce(st, obj, d, logger, time.Duration(cfg.EmoteDictionaryDebounceMS)*time.Millisecond)
	workerConcurrency := cfg.EmoteWorkerConcurrency
	if workerConcurrency < 1 {
		workerConcurrency = 1
	}
	w.RunPool(ctx, workerConcurrency)

	h := api.New(st, obj, d, seed, logger, cfg.CuratorAPIToken)

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
