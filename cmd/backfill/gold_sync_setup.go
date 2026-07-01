package main

import (
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"streamclone/internal/analytics"
	"streamclone/internal/archive"
	"streamclone/internal/chat/enrich"
	"streamclone/internal/config"
)

func goldIVRConfigFromApp(cfg config.Config) analytics.GoldIVRConfig {
	return analytics.GoldIVRConfig{
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
}

func newBackfillRedis(cfg config.Config) (*redis.Client, error) {
	opt, err := redis.ParseURL(cfg.RedisURL)
	if err != nil {
		return nil, fmt.Errorf("redis url parse: %w", err)
	}
	return redis.NewClient(opt), nil
}

func newBackfillSyncService(
	cfg config.Config,
	pool *pgxpool.Pool,
	rdb *redis.Client,
	logger *slog.Logger,
	withArchive bool,
) (*analytics.SyncService, error) {
	store := analytics.NewStore(pool)
	helix := analytics.NewHelixClient(
		cfg.TwitchAPIURL,
		cfg.TwitchTokenURL,
		cfg.TwitchOAuthClientID,
		cfg.TwitchOAuthClientSecret,
		cfg.Upstream.UserAgent,
	)
	enricher := enrich.New(rdb, cfg.DeltaDebounceMS, logger)
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
	)
	if withArchive {
		blob, err := archive.NewBlobStore(cfg.ArchiveBlobStoreConfig())
		if err != nil {
			return nil, err
		}
		archiveWriter := archive.NewWriter(blob, archive.NewManifestStore(pool))
		pgxArchiveDB := archive.NewPgxAnalyticsDB(pool)
		emoteExporter := archive.NewEmoteExporter(archiveWriter, archive.NewPgxEmoteSnapshotDB(pool))
		vodChatDB := archive.NewVODChatDBWithProvenance(pgxArchiveDB, emoteExporter)
		archiveSyncExporter := archive.NewSyncExporter(archiveWriter, vodChatDB)
		syncService = syncService.WithArchiveExportOnSync(cfg.ArchiveExportOnSync, archiveSyncExporter)
	}
	goldIVR := analytics.NewGoldIVRService(goldIVRConfigFromApp(cfg), store, nil, logger)
	analytics.LogGoldIVREffectiveConfig(logger, goldIVRConfigFromApp(cfg), cfg.GoldIVREnabledChannelAllowlist)
	syncService = syncService.WithGoldIVR(goldIVR)
	enabled, maxSegments, retryMax, leaseTTL, owner := goldVODSegmentOptsFromConfig(cfg)
	syncService = syncService.WithGoldVODSegments(enabled, maxSegments, retryMax, leaseTTL, owner)
	return syncService, nil
}

func goldVODSegmentOptsFromConfig(cfg config.Config) (enabled bool, maxSegmentsPerVOD, retryMax, leaseTTLSeconds int, owner string) {
	return cfg.GoldVODSegmentsEnabled,
		cfg.GoldMaxSegmentsPerVOD,
		cfg.GoldRetryMax,
		cfg.GoldLeaseTTLSeconds,
		""
}

func goldArchiveRequired(cfg config.Config, args []string) bool {
	if cliFlag(args, "--no-archive") {
		return false
	}
	return cfg.GoldArchiveRequired
}

func newArchiveBlobStore(cfg config.Config) (archive.BlobStore, error) {
	if !cfg.ArchiveEnabled || cfg.ArchiveStorageProvider != "azure" {
		return nil, fmt.Errorf("archive azure export must be enabled (ARCHIVE_ENABLED=true)")
	}
	return archive.NewBlobStore(cfg.ArchiveBlobStoreConfig())
}

func cliFlag(args []string, name string) bool {
	for _, arg := range args {
		if arg == name {
			return true
		}
	}
	return false
}
