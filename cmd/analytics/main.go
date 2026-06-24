package main

import (
	"context"
	"os"
	"strconv"
	"strings"
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
	if cfg.PulseHostedMode && strings.TrimSpace(cfg.PulseBetaKeys) == "" {
		logger.Error("PULSE_HOSTED_MODE=true but PULSE_BETA_KEYS is empty — hosted Pulse write/read routes fail closed until keys are configured")
	}
	pulseMaxAlwaysTrackedPerPrincipal := cfg.PulseMaxChannelsPerPrincipal
	if raw := strings.TrimSpace(os.Getenv("PULSE_MAX_ALWAYS_TRACKED_PER_PRINCIPAL")); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n >= 0 {
			pulseMaxAlwaysTrackedPerPrincipal = n
		}
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
	maxTracked := cfg.MaxConcurrentTrackedChannels
	if cfg.PulseMaxActiveChannels > 0 {
		maxTracked = cfg.PulseMaxActiveChannels
	}
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
		maxTracked,
		cfg.AnalyticsPollInterval,
		time.Duration(cfg.AnalyticsRetentionDays)*24*time.Hour,
		cfg.AnalyticsTopEmotesPerMinute,
	).WithAlwaysTracked(allAlways).WithLiveEmoteEnsurer(
		analytics.NewLiveEmoteEnsurer(analytics.LiveEmoteEnsurerConfig{
			EmoteURL:       cfg.EmoteServiceURL,
			Enricher:       enricher,
			Resolver:       helix,
			Redis:          rdb,
			Logger:         logger,
			EventAPIActive: strings.EqualFold(strings.TrimSpace(os.Getenv("SEVENTV_EVENTAPI_ENABLED")), "true"),
		}),
	)
	collector.Start(ctx)
	defer collector.Stop()

	var archiveExporter analytics.SyncArchiveExporter
	var archiveSyncExporter *archive.SyncExporter
	var archiveWriter *archive.Writer
	var archiveBlob archive.BlobStore
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
		archiveBlob = blob
		manifest := archive.NewManifestStore(pool)
		archiveWriter = archive.NewWriter(blob, manifest)
		pgxArchiveDB := archive.NewPgxAnalyticsDB(pool)
		emoteExporter := archive.NewEmoteExporter(archiveWriter, archive.NewPgxEmoteSnapshotDB(pool))
		vodChatDB := archive.NewVODChatDBWithProvenance(pgxArchiveDB, emoteExporter)
		archiveSyncExporter = archive.NewSyncExporter(archiveWriter, vodChatDB)
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
		cfg.AnalyticsTTScrapeBackoffEnabled,
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

	if cfg.ArchiveEnabled || cfg.Tier0Enabled || cfg.BronzeEnabled || cfg.BackfillEnabled {
		metrics.StartArchiveMetricsRefresh(ctx, pool, cfg.ArchiveMetricsRefreshInterval)
		logger.Info("archive metrics refresh started", "interval", cfg.ArchiveMetricsRefreshInterval.String())
	}

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
	top500SamplerCfg := analytics.Top500SamplerConfigFromApp(cfg)
	analytics.InitTop500MetadataSamplerMetrics(top500SamplerCfg)
	if cfg.Top500MetadataEnabled {
		top500Provider := analytics.NewHelixTop500MetadataProvider(helix)
		top500Sampler := analytics.NewTop500MetadataSampler(top500SamplerCfg, store, top500Provider, store)
		analytics.StartTop500MetadataSampler(ctx, top500Sampler, cfg.Top500MetadataLiveInterval, logger)
		logger.Info("top500 metadata sampler started",
			"dry_run", cfg.Top500MetadataDryRun,
			"write_enabled", cfg.Top500MetadataWriteEnabled,
			"top_n", top500SamplerCfg.TopN,
			"live_interval", cfg.Top500MetadataLiveInterval.String(),
		)
	} else {
		logger.Info("top500 metadata sampler disabled", "flag", "TOP500_METADATA_ENABLED")
	}
	if cfg.BackfillEnabled {
		staleLease := cfg.BackfillStaleRunningAfter
		heartbeat := cfg.BackfillHeartbeatInterval
		workerOpts := func(name string, tiers []string) analytics.BackfillWorkerOptions {
			return analytics.BackfillWorkerOptions{
				Name:              name,
				TierFilter:        tiers,
				StaleRunningAfter: staleLease,
				HeartbeatInterval: heartbeat,
			}
		}
		silverWorker := analytics.NewBackfillWorker(pool, syncService, archiveExporter, cfg.BackfillWorkerInterval).
			WithWorkerOptions(workerOpts("silver", []string{"silver"}))
		analytics.StartBackfillWorker(ctx, silverWorker, logger)
		if cfg.GoldBackfillEnabled && cfg.BackfillGoldWorkerEnabled {
			goldWorker := analytics.NewBackfillWorker(pool, syncService, archiveExporter, cfg.BackfillWorkerInterval).
				WithWorkerOptions(workerOpts("gold", []string{"gold", "gold_full", "gold_lite"})).
				WithGoldSyncTimeout(time.Duration(cfg.GoldSyncTimeoutMS) * time.Millisecond)
			if archiveSyncExporter != nil {
				goldWorker = goldWorker.WithVODChatExporter(archiveSyncExporter)
			}
			analytics.StartBackfillWorker(ctx, goldWorker, logger)
		}
		analytics.StartStaleBackfillReclaimer(ctx, pool, staleLease, 5*time.Minute, logger)
		logger.Info("backfill workers started",
			"interval", cfg.BackfillWorkerInterval.String(),
			"gold_worker", cfg.GoldBackfillEnabled && cfg.BackfillGoldWorkerEnabled,
			"stale_after", staleLease.String(),
		)
		if cfg.SilverAutoEnqueueEnabled && archiveBlob != nil {
			silverEnqueuer := analytics.NewSilverEnqueuer(
				pool,
				analytics.NewArchiveVODCatalog(archiveBlob),
				analytics.SilverEnqueuerConfig{
					SinceDays: cfg.SilverEnqueueSinceDays,
					TopN:      cfg.SilverEnqueueTopN,
					MaxPerRun: cfg.SilverEnqueueMaxPerRun,
					Interval:  cfg.SilverEnqueueInterval,
				},
			)
			analytics.StartSilverEnqueuer(ctx, silverEnqueuer, logger)
			logger.Info("silver auto-enqueuer started",
				"since_days", cfg.SilverEnqueueSinceDays,
				"top_n", cfg.SilverEnqueueTopN,
				"max_per_run", cfg.SilverEnqueueMaxPerRun,
				"interval", cfg.SilverEnqueueInterval.String(),
			)
		} else if cfg.SilverAutoEnqueueEnabled && archiveBlob == nil {
			logger.Warn("SILVER_AUTO_ENQUEUE_ENABLED is true but archive blob store is not configured")
		}
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

	heatmapCache := heatmap.NewCache(rdb, logger)
	collector.WithPulseCacheInvalidator(func(ctx context.Context, login, streamID string, includeHeatmap bool) {
		analytics.InvalidatePulseBFFCache(ctx, rdb, login, logger)
		if includeHeatmap {
			analytics.InvalidatePulseHeatmapCache(ctx, heatmapCache, streamID, logger)
		}
	})
	pulseRuntime := analytics.PulseRuntimeConfigFromEnv()
	pulseBackfill := analytics.NewPulseBackfillManager(syncService, store, helix, rdb, heatmapCache).WithRuntime(pulseRuntime)
	if cfg.PulseMaxBackfills > 0 {
		pulseBackfill.WithMaxConcurrent(cfg.PulseMaxBackfills)
	}
	handler := analytics.NewHandler(store, collector, helix, syncService)
	pulseHosted := analytics.PulseHostedConfig{
		Hosted:                  cfg.PulseHostedMode,
		BetaKeys:                analytics.ParsePulseBetaKeys(cfg.PulseBetaKeys),
		MaxActiveChannels:       cfg.PulseMaxActiveChannels,
		MaxChannelsPerPrincipal: pulseMaxAlwaysTrackedPerPrincipal,
		WatchRatePerMin:         cfg.PulseWatchRatePerMin,
		BackfillRatePerHour:     cfg.PulseBackfillRatePerHour,
	}
	if raw := strings.TrimSpace(os.Getenv("PULSE_IDLE_TTL")); raw != "" {
		if d, err := time.ParseDuration(raw); err == nil && d > 0 {
			pulseHosted.IdleTTL = d
		}
	}
	if pulseHosted.IdleTTL == 0 {
		pulseHosted.IdleTTL = 15 * time.Minute
	}
	handler.WithHeatmapCache(heatmapCache).WithTimeseries(tsWriter).WithRedis(rdb).WithPulseBackfill(pulseBackfill).WithPulseHosted(pulseHosted).WithPulseRuntime(pulseRuntime)
	handler.WithRateLimiter(analytics.NewPulseRateLimiter(rdb, pulseHosted.WatchRatePerMin, pulseHosted.BackfillRatePerHour))
	analytics.StartProtectedGoLivePoller(ctx, analytics.NewProtectedGoLivePoller(store, helix, collector, pulseRuntime, logger), logger)
	logger.Info("pulse runtime config",
		"hosted", pulseHosted.Hosted,
		"active_cap", pulseHosted.MaxActiveChannels,
		"backfill_cap", cfg.PulseMaxBackfills,
		"helix_live", pulseRuntime.HelixLiveEnabled,
		"helix_vod", pulseRuntime.HelixVodEnabled,
		"helix_metadata", pulseRuntime.HelixMetadataEnabled,
		"helix_golive", pulseRuntime.HelixGoLiveEnabled,
		"gql_comments", pulseRuntime.GQLCommentsEnabled,
		"pulse_backfill", pulseRuntime.BackfillEnabled,
		"read_only", pulseRuntime.ReadOnlyMode,
	)
	handler.StartPublicCacheRefresh(ctx)
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
	handler.AdminPulseRoutes(srv.Router, cfg)
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
