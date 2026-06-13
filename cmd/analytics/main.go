package main

import (
	"context"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"streamclone/internal/analytics"
	"streamclone/internal/chat/enrich"
	"streamclone/internal/chat/ircconn"
	"streamclone/internal/config"
	"streamclone/internal/httpx"
	"streamclone/internal/log"
	"streamclone/internal/metrics"
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

	store := analytics.NewStore(pool)
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
	srv := httpx.New("analytics", cfg.HTTPAddr, logger, metrics.HTTPMiddleware("analytics"), httpx.CORS, httpx.NewRateLimiter(20, 40).Middleware)
	srv.AddReady(func(ctx context.Context) error {
		return store.Ping(ctx)
	})
	handler.Routes(srv.Router)

	if err := srv.Run(ctx); err != nil {
		logger.Error("server stopped", "err", err)
		os.Exit(1)
	}
}
