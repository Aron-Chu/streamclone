package main

import (
	"context"
	"fmt"
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
	"streamclone/internal/chat/batch"
	"streamclone/internal/chat/enrich"
	"streamclone/internal/chat/ircconn"
	"streamclone/internal/config"
	"streamclone/internal/emote/render"
	"streamclone/internal/emote/seeder"
	"streamclone/internal/httpx"
	"streamclone/internal/log"
	"streamclone/internal/metrics"
	"streamclone/internal/timeseries"
)

type sevenTVSnapshotProvider struct {
	seed *seeder.Seeder
}

func (p sevenTVSnapshotProvider) SnapshotChannelEmotes(ctx context.Context, twitchID string) (analytics.EmoteProviderSnapshot, error) {
	detail, err := p.seed.SevenTVSnapshotDetail(ctx, twitchID)
	if err != nil {
		return analytics.EmoteProviderSnapshot{Provider: string(seeder.ProviderSevenTV)}, err
	}
	now := time.Now().UTC()
	items := make([]analytics.EmoteSnapshotItem, 0, len(detail.Items))
	for _, item := range detail.Items {
		items = append(items, analytics.EmoteSnapshotItem{
			Provider:        detail.Provider,
			ProviderEmoteID: item.ProviderEmoteID,
			ProviderSetID:   item.ProviderSetID,
			Alias:           item.Alias,
			CanonicalName:   item.CanonicalName,
			SourceURL:       item.SourceURL,
			Flags:           item.Flags,
			Animated:        item.Animated,
			ZeroWidth:       item.ZeroWidth,
		})
	}
	return analytics.EmoteProviderSnapshot{
		Provider:      detail.Provider,
		ProviderSetID: detail.SetID,
		Items:         items,
		FetchedAt:     now,
		EffectiveAt:   now,
		Complete:      true,
		HTTPStatus:    200,
		Source:        "seventv_snapshot_poll",
	}, nil
}

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
		logger.Info("CORPUS_WORKERS_ENABLED=false — corpus plane off for this process")
	}
	if cfg.PulseHostedMode && strings.TrimSpace(cfg.PulseBetaKeys) == "" {
		logger.Info("PULSE_HOSTED_MODE=true without PULSE_BETA_KEYS — extension routes use guest principals")
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
	if cfg.EmoteRenderOnChatObserved {
		observePub := render.NewObservePublisher(rdb, logger)
		enricher.SetEmoteObserver(func(channel string, frag batch.Fragment) {
			observePub.TryPublish(render.ObservePayload{
				EmoteID:      frag.ID,
				Provider:     frag.Provider,
				ChannelLogin: channel,
				Scale:        "1x",
			})
		})
	}
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
		blob, blobErr := archive.NewBlobStore(cfg.ArchiveBlobStoreConfig())
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
		cfg.AnalyticsTTUseProxy,
	).WithArchiveExportOnSync(cfg.ArchiveExportOnSync, archiveExporter)

	ivrCfg := analytics.GoldIVRConfig{
		Enabled:                     cfg.GoldIVREnabled,
		LiteEnabled:                 cfg.GoldIVRLiteEnabled,
		CanonicalReplace:            cfg.GoldIVRCanonicalReplace,
		ShadowMode:                  cfg.GoldIVRShadowMode,
		ShadowArtifactDir:           cfg.GoldIVRShadowArtifactDir,
		ShadowArtifactRetentionDays: cfg.GoldIVRShadowArtifactRetentionDays,
		ShadowArtifactMaxFiles:      cfg.GoldIVRShadowArtifactMaxFiles,
		PeaksOnlyEnabled:            cfg.GoldIVRPeaksOnlyEnabled,
		PeaksOnlyMaxMinutes:         cfg.GoldIVRPeaksOnlyMaxMinutes,
		PeaksOnlyMinChatCount:       cfg.GoldIVRPeaksOnlyMinChatCount,
		BaseURL:                     cfg.GoldIVRBaseURL,
		MaxBytesPerJob:              cfg.GoldIVRMaxBytesPerJob,
		MaxMessagesPerJob:           cfg.GoldIVRMaxMessagesPerJob,
		MaxDurationMinutes:          cfg.GoldIVRMaxDurationMinutes,
		HTTPTimeout:                 time.Duration(cfg.GoldIVRHTTPTimeoutSeconds) * time.Second,
		MaxRetries:                  cfg.GoldIVRMaxRetries,
		Allowlist:                   analytics.ParseGoldIVRAllowlist(cfg.GoldIVREnabledChannelAllowlist),
	}
	goldIVR := analytics.NewGoldIVRService(ivrCfg, store, nil, logger)
	syncService = syncService.WithGoldIVR(goldIVR)
	syncService = syncService.WithGoldVODSegments(
		cfg.GoldVODSegmentsEnabled,
		cfg.GoldMaxSegmentsPerVOD,
		cfg.GoldRetryMax,
		cfg.GoldLeaseTTLSeconds,
		"",
	).WithGoldGQLRateLimits(cfg.GoldGlobalGQLRPM, cfg.GoldPerVODGQLRPM)
	analytics.LogGoldIVREffectiveConfig(logger, ivrCfg, cfg.GoldIVREnabledChannelAllowlist)

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
		top500Provider := analytics.NewTop500MetadataProvider(cfg.Top500MetadataFixtureProvider, helix)
		top500Sampler := analytics.NewTop500MetadataSampler(top500SamplerCfg, store, top500Provider, store)
		analytics.StartTop500MetadataSampler(ctx, top500Sampler, cfg.Top500MetadataLiveInterval, logger)
		if cfg.Top500MetadataFixtureProvider {
			logger.Info("top500 metadata sampler started",
				"provider", "fixture",
				"dry_run", cfg.Top500MetadataDryRun,
				"write_enabled", cfg.Top500MetadataWriteEnabled,
				"top_n", top500SamplerCfg.TopN,
				"live_interval", cfg.Top500MetadataLiveInterval.String(),
			)
		} else {
			logger.Info("top500 metadata sampler started",
				"dry_run", cfg.Top500MetadataDryRun,
				"write_enabled", cfg.Top500MetadataWriteEnabled,
				"top_n", top500SamplerCfg.TopN,
				"live_interval", cfg.Top500MetadataLiveInterval.String(),
			)
		}
	} else {
		logger.Info("top500 metadata sampler disabled", "flag", "TOP500_METADATA_ENABLED")
	}
	silverGateCfg := analytics.SilverGateConfigFromApp(cfg)
	analytics.InitTop500SilverGateMetrics(silverGateCfg)
	if cfg.Top500SilverGateEnabled {
		silverGate := analytics.NewTop500SilverGateFromApp(cfg, logger, store, rdb)
		analytics.LogSilverGateStartup(logger, silverGateCfg, cfg.Top500SilverGateFixtureCandidates, silverGate.UsesRealCounterReader())
		analytics.StartTop500SilverGate(ctx, silverGate)
	} else {
		logger.Info("top500 silver gate disabled", "flag", "TOP500_SILVER_GATE_ENABLED")
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
		for i := 0; i < cfg.BackfillSilverWorkerCount; i++ {
			name := "silver"
			if cfg.BackfillSilverWorkerCount > 1 {
				name = fmt.Sprintf("silver-%d", i+1)
			}
			silverWorker := analytics.NewBackfillWorker(pool, syncService, archiveExporter, cfg.BackfillWorkerInterval).
				WithWorkerOptions(workerOpts(name, []string{"silver"}))
			analytics.StartBackfillWorker(ctx, silverWorker, logger)
		}
		if cfg.GoldBackfillEnabled && cfg.BackfillGoldWorkerEnabled {
			for i := 0; i < cfg.BackfillGoldWorkerCount; i++ {
				name := "gold"
				if cfg.BackfillGoldWorkerCount > 1 {
					name = fmt.Sprintf("gold-%d", i+1)
				}
				goldWorker := analytics.NewBackfillWorker(pool, syncService, archiveExporter, cfg.BackfillWorkerInterval).
					WithWorkerOptions(workerOpts(name, []string{"gold", "gold_full", "gold_lite"})).
					WithGoldSyncTimeout(time.Duration(cfg.GoldSyncTimeoutMS) * time.Millisecond)
				if archiveSyncExporter != nil {
					goldWorker = goldWorker.
						WithVODChatExporter(archiveSyncExporter).
						WithGoldLiteExporter(archiveSyncExporter, cfg.GoldLiteRequireRollups)
				}
				analytics.StartBackfillWorker(ctx, goldWorker, logger)
			}
		}
		analytics.StartStaleBackfillReclaimer(ctx, pool, staleLease, 5*time.Minute, logger)
		if cfg.GoldVODSegmentsEnabled {
			analytics.StartStaleGoldVODSegmentReclaimer(ctx, pool, 0, 0, logger)
		}
		if cfg.BackfillQueueMaintenanceEnabled {
			analytics.StartBackfillQueueMaintainer(ctx, pool, cfg.BackfillQueueMaintenanceInterval, analytics.BackfillQueueMaintenanceOptions{
				StaleRunningAfter: staleLease,
				RequeueFailedMax:  cfg.BackfillRequeueFailedMaxPerRun,
				RepairSessionsMax: cfg.BackfillRepairSessionsMaxPerRun,
			}, logger)
			logger.Info("backfill queue maintainer started",
				"interval", cfg.BackfillQueueMaintenanceInterval.String(),
				"requeue_max", cfg.BackfillRequeueFailedMaxPerRun,
			)
		}
		logger.Info("backfill workers started",
			"interval", cfg.BackfillWorkerInterval.String(),
			"silver_worker_count", cfg.BackfillSilverWorkerCount,
			"gold_worker", cfg.GoldBackfillEnabled && cfg.BackfillGoldWorkerEnabled,
			"gold_worker_count", cfg.BackfillGoldWorkerCount,
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
		if cfg.Top500GoldVODInventoryEnabled && archiveBlob != nil {
			directGoldEnqueue := cfg.Top500GoldVODInventoryDirectEnqueue && cfg.GoldFullEnabled && !cfg.GoldFullOperatorOnly
			inventory := analytics.NewTop500GoldVODInventory(
				pool,
				analytics.NewArchiveVODCatalog(archiveBlob),
				analytics.Top500GoldVODInventoryConfig{
					SinceDays:     cfg.Top500GoldVODInventorySinceDays,
					TopN:          cfg.Top500GoldVODInventoryTopN,
					MaxPerRun:     cfg.Top500GoldVODInventoryMaxPerRun,
					DirectEnqueue: directGoldEnqueue,
					Interval:      cfg.Top500GoldVODInventoryInterval,
				},
			)
			analytics.StartTop500GoldVODInventory(ctx, inventory, logger)
			logger.Info("top500 gold vod inventory started",
				"since_days", cfg.Top500GoldVODInventorySinceDays,
				"top_n", cfg.Top500GoldVODInventoryTopN,
				"max_per_run", cfg.Top500GoldVODInventoryMaxPerRun,
				"direct_enqueue", directGoldEnqueue,
				"interval", cfg.Top500GoldVODInventoryInterval.String(),
			)
		} else if cfg.Top500GoldVODInventoryEnabled && archiveBlob == nil {
			logger.Warn("TOP500_GOLD_VOD_INVENTORY_ENABLED is true but archive blob store is not configured")
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

	var emoteSnapshotProvider analytics.EmoteSnapshotProvider
	if cfg.EmoteHistorySnapshotEnabled {
		emoteSnapshotProvider = sevenTVSnapshotProvider{seed: seeder.NewWithImportConcurrency(nil, nil, nil, logger, cfg.Upstream.SevenTVAPIURL, cfg.Upstream.SevenTVCDNURL, cfg.Upstream.FFZAPIURL, cfg.Upstream.BTTVAPIURL, nil, cfg.EmoteImportConcurrency)}
	}
	emoteHistoryJobConfig := analytics.EmoteHistoryJobConfig{
		SnapshotEnabled:    cfg.EmoteHistorySnapshotEnabled,
		SnapshotInterval:   cfg.EmoteHistorySnapshotInterval,
		SnapshotBatchSize:  cfg.EmoteHistorySnapshotBatchSize,
		NormalizeEnabled:   cfg.EmoteHistoryNormalizeEnabled,
		NormalizeInterval:  cfg.EmoteHistoryNormalizeInterval,
		NormalizeSince:     cfg.EmoteHistoryNormalizeSince,
		NormalizeBatchSize: cfg.EmoteHistoryNormalizeBatchSize,
	}
	analytics.StartEmoteHistoryJobs(ctx, store, emoteSnapshotProvider, emoteHistoryJobConfig, logger)
	providerRefreshWorker := analytics.NewPublicEmoteProviderRefreshWorker(store, analytics.PublicEmoteProviderRefreshConfig{
		Enabled:  cfg.PublicEmoteProviderRefreshEnabled,
		Interval: cfg.PublicEmoteProviderRefreshInterval,
		Range:    "24h",
	}, logger)
	if providerRefreshWorker.Enabled() {
		providerRefreshWorker.Start(ctx)
		logger.Info("public emote provider refresh worker started", "interval", cfg.PublicEmoteProviderRefreshInterval.String(), "range", "24h")
	} else {
		logger.Info("public emote provider refresh worker disabled", "flag", "PUBLIC_EMOTE_PROVIDER_REFRESH_ENABLED")
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
	if cfg.PulseAutoBackfillEnabled {
		pulseAutoBackfill := analytics.NewPulseAutoBackfillEnqueuer(store, pulseBackfill, pulseRuntime, analytics.PulseAutoBackfillOptions{
			Interval:  cfg.PulseAutoBackfillInterval,
			Cooldown:  cfg.PulseAutoBackfillCooldown,
			Since:     cfg.PulseAutoBackfillSince,
			MaxPerRun: cfg.PulseAutoBackfillMaxPerRun,
			ScanLimit: cfg.PulseAutoBackfillScanLimit,
		})
		analytics.StartPulseAutoBackfillEnqueuer(ctx, pulseAutoBackfill, logger)
		logger.Info("pulse auto-backfill enqueuer started",
			"interval", cfg.PulseAutoBackfillInterval.String(),
			"cooldown", cfg.PulseAutoBackfillCooldown.String(),
			"since", cfg.PulseAutoBackfillSince.String(),
			"max_per_run", cfg.PulseAutoBackfillMaxPerRun,
			"scan_limit", cfg.PulseAutoBackfillScanLimit,
		)
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
	handler.WithHeatmapCache(heatmapCache).WithTimeseries(tsWriter).WithRedis(rdb).WithPulseBackfill(pulseBackfill).WithPulseHosted(pulseHosted).WithPulseRuntime(pulseRuntime).WithCorpusRuntime(analytics.CorpusRuntimeConfigFromApp(cfg)).WithEmoteHistoryJobs(emoteHistoryJobConfig).WithCDNPublicBase(cfg.CDNPublicBase).WithAppConfig(cfg)
	handler.WithRateLimiter(analytics.NewPulseRateLimiter(rdb, pulseHosted.WatchRatePerMin, pulseHosted.BackfillRatePerHour))
	analytics.StartProtectedGoLivePoller(ctx, analytics.NewProtectedGoLivePoller(store, helix, collector, pulseRuntime, logger), logger)
	admissionSource := analytics.NewLiveAdmissionSource(cfg, store, helix, logger)
	analytics.StartLiveAdmissionPoller(ctx, analytics.NewLiveAdmissionPoller(admissionSource, collector, cfg, logger), logger)
	if cfg.PulseLiveAdmissionEnabled {
		logger.Info("top500 priority watch poller enabled",
			"top_n", cfg.PulseLiveAdmissionTopN,
			"interval", cfg.PulseLiveAdmissionInterval.String(),
			"source", cfg.PulseLiveAdmissionSource,
		)
	}
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
	if shouldStartPublicCacheRefreshForThisProcess() {
		handler.StartPublicCacheRefresh(ctx)
	} else {
		logger.Info("public cache refresh disabled: corpus worker process")
	}
	if cfg.HostedRetentionPruneEnabled && shouldStartPublicCacheRefreshForThisProcess() {
		analytics.StartHostedRetentionMaintainer(ctx, store, cfg.HostedBackfillJobRetentionDays, logger)
		logger.Info("hosted retention maintainer started",
			"backfill_job_retention_days", cfg.HostedBackfillJobRetentionDays,
		)
	}
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
	v = strings.ToLower(strings.TrimSpace(v))
	return v == "0" || v == "false"
}

func corpusWorkersEnabledForThisProcess() bool {
	v, ok := os.LookupEnv("CORPUS_WORKERS_ENABLED")
	if !ok {
		return false
	}
	v = strings.ToLower(strings.TrimSpace(v))
	return v == "1" || v == "true"
}

func shouldStartPublicCacheRefreshForThisProcess() bool {
	return !corpusWorkersEnabledForThisProcess()
}
