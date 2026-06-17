package main

import (
	"context"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"streamclone/internal/analytics"
	"streamclone/internal/analytics/chatreplay"
	"streamclone/internal/analytics/heatmap"
	"streamclone/internal/chat/enrich"
	"streamclone/internal/chat/ircconn"
	"streamclone/internal/config"
	"streamclone/internal/httpx"
	"streamclone/internal/log"
	"streamclone/internal/metrics"
	"streamclone/internal/timeseries"
)

func main() {
	cfg, err := config.Load()
	logger := log.New("analytics", cfg.LogLevel)
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

	tsWriter := timeseries.NewAsyncWriter(timeseries.Config{
		Enabled:      cfg.TimeseriesEnabled,
		Backend:      cfg.TimeseriesBackend,
		URL:          cfg.InfluxDBURL,
		Token:        cfg.InfluxDBToken,
		Org:          cfg.InfluxDBOrg,
		Bucket:       cfg.InfluxDBBucket,
		WriteTimeout: time.Duration(cfg.TimeseriesWriteTimeoutMS) * time.Millisecond,
		QueueSize:    cfg.TimeseriesQueueSize,
	}, logger)
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if err := tsWriter.Close(shutdownCtx); err != nil {
			logger.Warn("time-series writer shutdown timed out", "err", err)
		}
	}()

	store := analytics.NewStore(pool).WithTelemetry(tsWriter)
	if cfg.TimeseriesEnabled && cfg.TimeseriesBackfillOnStart {
		status := tsWriter.Status()
		if status.Configured {
			go func() {
				deadline := time.Now().Add(2 * time.Minute)
				for attempt := 1; ; attempt++ {
					backfillCtx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
					logger.Info("time-series backfill started", "attempt", attempt)
					summary, err := store.BackfillTimeseries(backfillCtx, 500)
					cancel()
					if err == nil {
						logger.Info("time-series backfill completed", "streams", summary.StreamCount, "rollups", summary.RollupCount, "exported", summary.ExportedCount)
						return
					}
					if time.Now().After(deadline) {
						logger.Warn("time-series backfill failed", "attempt", attempt, "err", err)
						return
					}
					delay := time.Duration(attempt*2) * time.Second
					if delay > 10*time.Second {
						delay = 10 * time.Second
					}
					logger.Warn("time-series backfill attempt failed; retrying", "attempt", attempt, "retryIn", delay.String(), "err", err)
					time.Sleep(delay)
				}
			}()
		} else {
			logger.Warn("time-series backfill skipped; writer is not configured", "state", status.State, "err", status.LastError)
		}
	}
	helix := analytics.NewHelixClient(
		cfg.TwitchAPIURL,
		cfg.TwitchTokenURL,
		cfg.TwitchOAuthClientID,
		cfg.TwitchOAuthClientSecret,
		cfg.Upstream.UserAgent,
	)
	enricher := enrich.New(rdb, cfg.DeltaDebounceMS, logger)
	if err := store.EnsureAlwaysTrackedTable(ctx); err != nil {
		logger.Error("always tracked table ensure failed", "err", err)
	}
	dbAlways, err := store.GetAlwaysTracked(ctx)
	if err != nil {
		logger.Error("failed to load always tracked from db", "err", err)
	}
	allAlways := append(cfg.AlwaysTrackedChannels, dbAlways...)

	var collector *analytics.Collector
	irc := ircconn.NewManager(cfg.Upstream.TwitchIRCURL, cfg.MaxChannelsPerSocket, func(line string) {
		if collector != nil {
			collector.HandleIRCLine(line)
		}
	}, logger)
	collector = analytics.NewCollector(
		store,
		helix,
		irc,
		enricher,
		logger,
		cfg.MaxConcurrentTrackedChannels,
		cfg.AnalyticsPollInterval,
		time.Duration(cfg.AnalyticsRetentionDays)*24*time.Hour,
		cfg.AnalyticsTopEmotesPerMinute,
	).WithAlwaysTracked(allAlways)
	collector.Start(ctx)
	defer collector.Stop()

	syncService := analytics.NewSyncService(
		store,
		enricher,
		helix,
		cfg.EmoteServiceURL,
		cfg.ScraperAPIURL,
		cfg.ScraperAPIKey,
		cfg.TwitchGQLURL,
		cfg.TwitchClientID,
		cfg.Upstream.UserAgent,
		logger,
		rdb,
		time.Duration(cfg.AnalyticsVODGQLPageDelayMS)*time.Millisecond,
		cfg.AnalyticsVODGQLConcurrency,
		cfg.AnalyticsVODGQLConcurrencyMin,
		cfg.AnalyticsVODGQLConcurrencyMax,
		cfg.AnalyticsVODGQLSegmentSeconds,
		cfg.AnalyticsVODGQLDenseSegmentSeconds,
		cfg.AnalyticsVODGQLHotSegmentPageThreshold,
		cfg.AnalyticsVODGQLHotSlowAdvanceSec,
		cfg.AnalyticsVODGQLHotSlowAdvancePages,
		cfg.AnalyticsVODGQLHotCommentsPerPage,
		cfg.AnalyticsVODGQLPriorityEdgeSeconds,
		cfg.AnalyticsVODGQLIncrementalDB,
		cfg.AnalyticsTrackerScrapeMS,
		cfg.AnalyticsPassTTMaxAge,
		cfg.AnalyticsTTMaxAgeMS,
		cfg.AnalyticsTTStaleMaxAgeMS,
		cfg.AnalyticsTTPrefetchEnabled,
		cfg.AnalyticsTTDirectHTTPEnabled,
		cfg.AnalyticsTTDirectHTTPStaleOnly,
		cfg.AnalyticsTTDirectHTTPTimeoutMS,
	)
	handler := analytics.NewHandler(store, collector, helix, syncService)
	heatmapCache := heatmap.NewCache(rdb, logger)
	handler.WithHeatmapCache(heatmapCache)
	handler.WithTimeseries(tsWriter)
	chatReplayStore := chatreplay.NewStore(pool)
	chatReplayHandler := chatreplay.NewHandler(chatReplayStore).WithLogger(logger).WithIngestEnabled(func() bool {
		return cfg.ChatLogPersistEnabled
	})

	retentionWorker := chatreplay.NewRetentionWorker(chatReplayStore, cfg.AnalyticsVODChatRetentionDays, logger)
	retentionWorker.Start(ctx)
	liveRetentionWorker := chatreplay.NewLiveRetentionWorker(chatReplayStore, cfg.ChatLogRetentionDays, logger)
	liveRetentionWorker.Start(ctx)
	srv := httpx.New("analytics", cfg.HTTPAddr, logger, metrics.HTTPMiddleware("analytics"), httpx.CORS, httpx.NewRateLimiter(20, 40).Middleware)
	srv.AddReady(func(ctx context.Context) error {
		return store.Ping(ctx)
	})
	handler.Routes(srv.Router)
	chatReplayHandler.Routes(srv.Router)

	if err := srv.Run(ctx); err != nil {
		logger.Error("server stopped", "err", err)
		os.Exit(1)
	}
}
