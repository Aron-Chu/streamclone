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
	"streamclone/internal/archive"
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
	if corpusWorkersExplicitlyDisabled() {
		cfg.ArchiveEnabled = false
		cfg.BronzeEnabled = false
		cfg.BackfillEnabled = false
		cfg.GoldBackfillEnabled = false
		cfg.Tier0Enabled = false
		logger.Info("CORPUS_WORKERS_ENABLED=false — corpus plane off for this process")
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

	store := analytics.NewStore(pool).
		WithTelemetry(tsWriter).
		WithArchiveProtectRetention(cfg.ArchiveProtectRetention)
	if cfg.BackfillEnabled {
		store = store.WithPostEndDetector(analytics.NewPostEndDetector(
			pool, cfg.PostEndWaitMin, cfg.PostEndWaitMax, cfg.PostEndCoveragePct,
		))
	}
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
	if report, err := store.CleanupSessionStubs(ctx, allAlways); err != nil {
		logger.Warn("prefetch stub cleanup failed", "err", err)
	} else if report.StubSessionsMerged > 0 || len(report.OrphanAliasesMerged) > 0 {
		logger.Info("prefetch stub cleanup completed",
			"stubs_merged", report.StubSessionsMerged,
			"orphan_aliases", len(report.OrphanAliasesMerged),
		)
	}

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

	var archiveExporter analytics.SyncArchiveExporter
	var archiveSyncExporter *archive.SyncExporter
	var archiveWriter *archive.Writer
	if cfg.ArchiveEnabled && cfg.ArchiveStorageProvider == "azure" {
		blob, blobErr := archive.NewAzureBlobStore(archive.AzureConfig{
			StorageAccount:       cfg.ArchiveAzureStorageAccount,
			Container:            cfg.ArchiveAzureContainer,
			Prefix:               cfg.ArchiveAzurePrefix,
			ConnectionStringFile: cfg.ArchiveAzureConnectionStringFile,
		})
		if blobErr != nil {
			logger.Error("archive blob init failed", "err", blobErr)
			os.Exit(1)
		}
		manifest := archive.NewManifestStore(pool)
		archiveWriter = archive.NewWriter(blob, manifest)
		archiveSyncExporter = archive.NewSyncExporter(archiveWriter, archive.NewPgxAnalyticsDB(pool))
		archiveExporter = archiveSyncExporter
		logger.Info("archive export enabled", "account", cfg.ArchiveAzureStorageAccount, "container", cfg.ArchiveAzureContainer)
	} else if cfg.ArchiveExportOnSync {
		logger.Warn("ARCHIVE_EXPORT_ON_SYNC is enabled but ARCHIVE_ENABLED is false; sync will stop at export_pending")
	}

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
		cfg.AnalyticsVODGQLQuietSegmentSeconds,
		cfg.AnalyticsVODGQLHotSegmentPageThreshold,
		cfg.AnalyticsVODGQLHotSlowAdvanceSec,
		cfg.AnalyticsVODGQLHotSlowAdvancePages,
		cfg.AnalyticsVODGQLHotCommentsPerPage,
		cfg.AnalyticsVODGQLPriorityEdgeSeconds,
		cfg.AnalyticsVODGQLIncrementalDB,
		cfg.AnalyticsVODGQLDeferSummaryRefresh,
		cfg.AnalyticsVODGQLRollupFlushSegments,
		time.Duration(cfg.AnalyticsVODGQLRollupFlushMS)*time.Millisecond,
		cfg.AnalyticsTrackerScrapeMS,
		cfg.AnalyticsTTSyncTimeoutMS,
		cfg.AnalyticsTTBackgroundRetryEnabled,
		cfg.AnalyticsPassTTMaxAge,
		cfg.AnalyticsTTMaxAgeMS,
		cfg.AnalyticsTTStaleMaxAgeMS,
		cfg.AnalyticsTTPrefetchEnabled,
		cfg.AnalyticsTTDirectHTTPEnabled,
		cfg.AnalyticsTTDirectHTTPStaleOnly,
		cfg.AnalyticsTTDirectHTTPTimeoutMS,
		cfg.AnalyticsTTViewerSmoothWindow,
	).WithArchiveExportOnSync(cfg.ArchiveExportOnSync, archiveExporter)

	startArchiveWorkers(ctx, cfg, pool, syncService, archiveSyncExporter, archiveWriter, logger)

	if cfg.Tier0Enabled {
		roster := analytics.NewRosterSyncer(pool, cfg.MetadataServiceURL, cfg.Tier0RosterTopN, allAlways)
		if archiveWriter != nil {
			roster = roster.WithArchiveExporter(archiveWriter)
		}
		analytics.StartRosterWorker(ctx, roster, cfg.Tier0RosterInterval, logger)
		sampler := analytics.NewViewerSampler(store, helix, roster, cfg.Tier0SampleInterval, logger)
		analytics.StartViewerSampler(ctx, sampler)
		logger.Info("tier-0 roster and viewer sampler started",
			"roster_interval", cfg.Tier0RosterInterval.String(),
			"sample_interval", cfg.Tier0SampleInterval.String(),
			"top_n", cfg.Tier0RosterTopN,
		)
	}
	if cfg.BackfillEnabled {
		worker := analytics.NewBackfillWorker(pool, syncService, archiveExporter, cfg.BackfillWorkerInterval)
		if cfg.GoldBackfillEnabled {
			worker = worker.WithGoldSyncTimeout(time.Duration(cfg.GoldSyncTimeoutMS) * time.Millisecond)
			if archiveSyncExporter != nil {
				worker = worker.WithVODChatExporter(archiveSyncExporter)
			}
		}
		analytics.StartBackfillWorker(ctx, worker, logger)
		logger.Info("backfill worker started",
			"interval", cfg.BackfillWorkerInterval.String(),
			"gold_enabled", cfg.GoldBackfillEnabled,
		)
	}
	if cfg.GoldBackfillEnabled {
		goldRules := analytics.NewGoldRulesEngine(allAlways, cfg.GoldMinPeakViewers, cfg.GoldMinDurationMinutes)
		goldEnqueuer := analytics.NewGoldEnqueuer(pool, goldRules, cfg.GoldEnqueuerInterval)
		analytics.StartGoldEnqueuer(ctx, goldEnqueuer, logger)
		logger.Info("gold enqueuer started", "interval", cfg.GoldEnqueuerInterval.String())
	}
	if cfg.BronzeEnabled {
		if archiveWriter == nil {
			logger.Warn("BRONZE_ENABLED is true but archive writer is not configured; bronze indexer disabled")
		} else {
			indexer := analytics.NewBronzeIndexer(
				pool,
				helix,
				cfg.MetadataServiceURL,
				cfg.TwitchTrackerAPIURL,
				cfg.Upstream.UserAgent,
				cfg.BronzeTopN,
				allAlways,
				cfg.BronzeHelixConcurrency,
				cfg.BronzeTTSummaryConcurrency,
			).WithWriter(archiveWriter)
			analytics.StartBronzeWorker(ctx, indexer, cfg.BronzeWorkerInterval, logger)
			logger.Info("bronze indexer started",
				"interval", cfg.BronzeWorkerInterval.String(),
				"top_n", cfg.BronzeTopN,
				"helix_concurrency", cfg.BronzeHelixConcurrency,
				"tt_concurrency", cfg.BronzeTTSummaryConcurrency,
			)
		}
	}

	handler := analytics.NewHandler(store, collector, helix, syncService)
	heatmapCache := heatmap.NewCache(rdb, logger)
	handler.WithHeatmapCache(heatmapCache)
	handler.WithTimeseries(tsWriter)
	adminArchiveHandler := analytics.NewAdminArchiveHandler(pool, cfg)
	chatReplayStore := chatreplay.NewStore(pool).WithArchiveProtectRetention(cfg.ArchiveProtectRetention)
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
	adminArchiveHandler.Routes(srv.Router, cfg)
	chatReplayHandler.Routes(srv.Router)

	if err := srv.Run(ctx); err != nil {
		logger.Error("server stopped", "err", err)
		os.Exit(1)
	}
}

func corpusWorkersExplicitlyDisabled() bool {
	v, ok := os.LookupEnv("CORPUS_WORKERS_ENABLED")
	if !ok {
		return false
	}
	return v == "0" || v == "false"
}
