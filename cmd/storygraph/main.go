package main

import (
	"context"
	"os"

	_ "streamclone/internal/social/kick"
	_ "streamclone/internal/social/reddit"
	_ "streamclone/internal/social/streamerbans"
	_ "streamclone/internal/social/twitchclips"
	_ "streamclone/internal/social/xrecent"
	_ "streamclone/internal/social/youtube"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"streamclone/internal/config"
	"streamclone/internal/httpx"
	"streamclone/internal/log"
	"streamclone/internal/metrics"
	"streamclone/internal/storygraph/api"
	"streamclone/internal/storygraph/ingest"
	"streamclone/internal/storygraph/reliability"
	"streamclone/internal/storygraph/store"
)

func main() {
	cfg, err := config.Load()
	logger := log.New("storygraph", cfg.LogLevel)
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

	st := store.New(pool).WithArchiveProtectRetention(cfg.ArchiveProtectRetention)
	rel := reliability.NewRegistry(pool)
	if err := rel.Load(ctx); err != nil {
		logger.Warn("reliability registry load failed; using defaults", "err", err)
		rel.SeedDefaults()
	}

	var handler *api.Handler
	if cfg.PulseWireEnabled {
		health := ingest.NewHealth()
		samplerHealth := ingest.NewDirectorySamplerHealth()
		windowScoreHealth := ingest.NewWindowScoreHealth()
		workers := ingest.NewWorkers(ingest.Options{
			Store:             st,
			Reliability:       rel,
			Redis:             rdb,
			Logger:            logger,
			Config:            cfg,
			Health:            health,
			SamplerHealth:     samplerHealth,
			WindowScoreHealth: windowScoreHealth,
		})
		workers.Start(ctx)
		defer workers.Stop()
		handler = api.New(api.Options{
			Store:             st,
			Reliability:       rel,
			Redis:             rdb,
			Logger:            logger,
			Config:            cfg,
			Enabled:           cfg.PulseWireEnabled,
			SetupControlToken: cfg.SetupControlToken,
			IngestHealth:      health,
			SamplerHealth:     samplerHealth,
			WindowScoreHealth: windowScoreHealth,
			Workers:           workers,
		})
	} else {
		handler = api.New(api.Options{
			Store:             st,
			Reliability:       rel,
			Redis:             rdb,
			Logger:            logger,
			Config:            cfg,
			Enabled:           cfg.PulseWireEnabled,
			SetupControlToken: cfg.SetupControlToken,
		})
	}

	srv := httpx.New("storygraph", cfg.HTTPAddr, logger, metrics.HTTPMiddleware("storygraph"), httpx.CORS, httpx.NewRateLimiter(20, 40).Middleware)
	srv.AddReady(func(ctx context.Context) error {
		return st.Ping(ctx)
	})
	handler.Routes(srv.Router)

	if err := srv.Run(ctx); err != nil {
		logger.Error("server stopped", "err", err)
		os.Exit(1)
	}
}
