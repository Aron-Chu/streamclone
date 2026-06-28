package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"streamclone/internal/analytics"
	"streamclone/internal/archive"
	"streamclone/internal/config"
	"streamclone/internal/log"
)

func main() {
	cfg, err := config.Load()
	logger := log.New("backfill", cfg.LogLevel)
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
	store := analytics.NewStore(pool)

	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "status":
			jobs, err := analytics.ListBackfillJobs(ctx, pool, 100)
			if err != nil {
				logger.Error("list jobs failed", "err", err)
				os.Exit(1)
			}
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			_ = enc.Encode(jobs)
			return
		case "bronze":
			if len(os.Args) < 3 {
				fmt.Println("Usage: backfill bronze status | backfill bronze run-once")
				os.Exit(2)
			}
			switch os.Args[2] {
			case "status":
				rows, err := analytics.ListBronzeIndexState(ctx, pool, 100)
				if err != nil {
					logger.Error("list bronze state failed", "err", err)
					os.Exit(1)
				}
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				_ = enc.Encode(rows)
				return
			case "run-once":
				indexer, err := newBronzeIndexer(ctx, cfg, pool, logger)
				if err != nil {
					logger.Error("bronze indexer init failed", "err", err)
					os.Exit(1)
				}
				if err := indexer.RunOnce(ctx); err != nil {
					logger.Error("bronze run-once failed", "err", err)
					os.Exit(1)
				}
				logger.Info("bronze run-once completed")
				return
			case "vod-range":
				login := bronzeLoginFromArgs(os.Args[3:])
				if login == "" {
					fmt.Println("Usage: backfill bronze vod-range --login=<channel>")
					os.Exit(2)
				}
				out, err := bronzeVODRange(ctx, cfg, pool, login)
				if err != nil {
					logger.Error("bronze vod-range failed", "err", err)
					os.Exit(1)
				}
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				_ = enc.Encode(out)
				return
			default:
				fmt.Println("Usage: backfill bronze status | backfill bronze run-once | backfill bronze vod-range --login=<channel>")
				os.Exit(2)
			}
		case "sessions":
			if len(os.Args) < 3 {
				fmt.Println("Usage: backfill sessions audit|cleanup [--dry-run] [--login=ohnepixel]")
				os.Exit(2)
			}
			switch os.Args[2] {
			case "audit":
				explicit, _ := sessionCleanupArgsFromCLI(os.Args[3:])
				logins, err := store.ResolveSessionCleanupLogins(ctx, explicit, cfg.AlwaysTrackedChannels)
				if err != nil {
					logger.Error("resolve audit logins failed", "err", err)
					os.Exit(1)
				}
				report, err := store.AuditSessionIntegrity(ctx, logins)
				if err != nil {
					logger.Error("session audit failed", "err", err)
					os.Exit(1)
				}
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				_ = enc.Encode(report)
				return
			case "cleanup":
				explicit, dryRun := sessionCleanupArgsFromCLI(os.Args[3:])
				logins, err := store.ResolveSessionCleanupLogins(ctx, explicit, cfg.AlwaysTrackedChannels)
				if err != nil {
					logger.Error("resolve cleanup logins failed", "err", err)
					os.Exit(1)
				}
				report, err := store.CleanupSessionStubsWithOptions(ctx, logins, analytics.SessionCleanupOptions{DryRun: dryRun})
				if err != nil {
					logger.Error("session cleanup failed", "err", err)
					os.Exit(1)
				}
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				_ = enc.Encode(report)
				if len(report.Errors) > 0 {
					os.Exit(2)
				}
				return
			default:
				fmt.Println("Usage: backfill sessions audit|cleanup [--dry-run] [--login=ohnepixel]")
				os.Exit(2)
			}
		case "coverage":
			if len(os.Args) < 3 {
				fmt.Println("Usage: backfill coverage report")
				os.Exit(2)
			}
			switch os.Args[2] {
			case "report":
				sinceRaw, outPath := coverageArgsFromCLI(os.Args[3:])
				since, err := analytics.ParseCoverageSince(sinceRaw, time.Now().UTC())
				if err != nil {
					logger.Error("invalid --since", "err", err)
					os.Exit(2)
				}
				report, err := analytics.BuildCoverageReport(ctx, pool, nil, since, cfg.Tier0RosterTopN)
				if err != nil {
					logger.Error("coverage report failed", "err", err)
					os.Exit(1)
				}
				fmt.Println(analytics.FormatCoverageSummary(report))
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				if err := enc.Encode(report); err != nil {
					logger.Error("encode coverage report failed", "err", err)
					os.Exit(1)
				}
				if outPath != "" {
					if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
						logger.Error("create output dir failed", "err", err, "path", outPath)
						os.Exit(1)
					}
					f, err := os.Create(outPath)
					if err != nil {
						logger.Error("create output file failed", "err", err, "path", outPath)
						os.Exit(1)
					}
					defer f.Close()
					fileEnc := json.NewEncoder(f)
					fileEnc.SetIndent("", "  ")
					if err := fileEnc.Encode(report); err != nil {
						logger.Error("write output file failed", "err", err, "path", outPath)
						os.Exit(1)
					}
					logger.Info("coverage report written", "path", outPath)
				}
				return
			default:
				fmt.Println("Usage: backfill coverage report")
				os.Exit(2)
			}
		case "jobs":
			jobsCLI := analytics.NewJobsCLI(pool, cfg.ArchiveJobHeartbeatInterval, cfg.ArchiveJobStaleAfter, cfg.ArchiveJobEventLogEnabled)
			if len(os.Args) < 3 {
				fmt.Println("Usage: backfill jobs list|show|retry-failed|resume|cancel|reclaim-stale|cleanup [--apply]")
				os.Exit(2)
			}
			switch os.Args[2] {
			case "list":
				rows, err := jobsCLI.List(ctx, analytics.JobsStatusFromArgs(os.Args[3:]), analytics.JobsLimitFromArgs(os.Args[3:]))
				if err != nil {
					logger.Error("jobs list failed", "err", err)
					os.Exit(1)
				}
				_ = analytics.PrintJobsJSON(rows)
				return
			case "show":
				jobID := analytics.JobsJobIDFromArgs(os.Args[3:])
				if jobID == "" {
					fmt.Println("Usage: backfill jobs show --job-id=<uuid>")
					os.Exit(2)
				}
				resp, err := jobsCLI.Show(ctx, jobID)
				if err != nil {
					logger.Error("jobs show failed", "err", err)
					os.Exit(1)
				}
				_ = analytics.PrintJobsJSON(resp)
				return
			case "retry-failed":
				jobID := analytics.JobsJobIDFromArgs(os.Args[3:])
				if jobID == "" {
					fmt.Println("Usage: backfill jobs retry-failed --job-id=<uuid>")
					os.Exit(2)
				}
				if err := jobsCLI.RetryFailed(ctx, jobID); err != nil {
					logger.Error("jobs retry-failed", "err", err)
					os.Exit(1)
				}
				fmt.Println("retry-failed ok")
				return
			case "resume":
				jobID := analytics.JobsJobIDFromArgs(os.Args[3:])
				if jobID == "" {
					fmt.Println("Usage: backfill jobs resume --job-id=<uuid>")
					os.Exit(2)
				}
				if err := jobsCLI.Resume(ctx, jobID); err != nil {
					logger.Error("jobs resume", "err", err)
					os.Exit(1)
				}
				fmt.Println("resume ok")
				return
			case "cancel":
				jobID := analytics.JobsJobIDFromArgs(os.Args[3:])
				if jobID == "" {
					fmt.Println("Usage: backfill jobs cancel --job-id=<uuid>")
					os.Exit(2)
				}
				if err := jobsCLI.Cancel(ctx, jobID); err != nil {
					logger.Error("jobs cancel", "err", err)
					os.Exit(1)
				}
				fmt.Println("cancel ok")
				return
			case "reclaim-stale":
				failed, requeued, err := analytics.ReclaimStaleRunningJobs(ctx, pool, cfg.BackfillStaleRunningAfter)
				if err != nil {
					logger.Error("reclaim-stale failed", "err", err)
					os.Exit(1)
				}
				fmt.Printf("reclaim-stale ok failed=%d requeued=%d stale_after=%s\n", failed, requeued, cfg.BackfillStaleRunningAfter)
				return
			case "cleanup":
				report, err := analytics.CleanupBackfillQueue(ctx, pool, analytics.BackfillCleanupOptions{
					DryRun:            dryRunFromArgs(os.Args[3:]),
					StaleRunningAfter: cfg.BackfillStaleRunningAfter,
				})
				if err != nil {
					logger.Error("jobs cleanup failed", "err", err)
					os.Exit(1)
				}
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				_ = enc.Encode(report)
				return
			case "maintenance":
				report, err := analytics.RunBackfillQueueMaintenance(ctx, pool, analytics.BackfillQueueMaintenanceOptions{
					StaleRunningAfter: cfg.BackfillStaleRunningAfter,
					RequeueFailedMax:  cfg.BackfillRequeueFailedMaxPerRun,
					RepairSessionsMax: cfg.BackfillRepairSessionsMaxPerRun,
				})
				if err != nil {
					logger.Error("jobs maintenance failed", "err", err)
					os.Exit(1)
				}
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				_ = enc.Encode(report)
				return
			default:
				fmt.Println("Usage: backfill jobs list|show|retry-failed|resume|cancel|reclaim-stale|cleanup|maintenance [--apply]")
				os.Exit(2)
			}
		case "silver":
			if len(os.Args) < 3 {
				fmt.Println("Usage: backfill silver enqueue-from-bronze")
				os.Exit(2)
			}
			switch os.Args[2] {
			case "enqueue-from-bronze":
				blob, err := newArchiveBlobStore(cfg)
				if err != nil {
					logger.Error("archive blob init failed", "err", err)
					os.Exit(1)
				}
				enqueuer := analytics.NewSilverEnqueuer(
					pool,
					analytics.NewArchiveVODCatalog(blob),
					analytics.SilverEnqueuerConfig{
						SinceDays: cfg.SilverEnqueueSinceDays,
						TopN:      cfg.SilverEnqueueTopN,
						MaxPerRun: cfg.SilverEnqueueMaxPerRun,
						Interval:  cfg.SilverEnqueueInterval,
					},
				)
				result, err := enqueuer.RunOnce(ctx)
				if err != nil {
					logger.Error("silver enqueue-from-bronze failed", "err", err)
					os.Exit(1)
				}
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				_ = enc.Encode(result)
				return
			default:
				fmt.Println("Usage: backfill silver enqueue-from-bronze")
				os.Exit(2)
			}
		case "gold":
			if len(os.Args) < 3 {
				fmt.Println("Usage: backfill gold enqueue --stream-id=<id> [--login=] | backfill gold eval --stream-id=<id> | backfill gold run-once [--no-archive] | backfill gold shadow-reconcile run-once|fixture | backfill gold inventory run-once|coverage")
				os.Exit(2)
			}
			if os.Args[2] == "shadow-reconcile" {
				if len(os.Args) < 4 {
					fmt.Println("Usage: backfill gold shadow-reconcile run-once --stream-id=<id> --login=<login> [--no-archive]")
					fmt.Println("       backfill gold shadow-reconcile fixture --channel=<login> --vod=<vod_id> [--broadcaster-id=] [--no-archive]")
					os.Exit(2)
				}
				switch os.Args[3] {
				case "run-once":
					streamID, login, noArchive := shadowReconcileArgsFromCLI(os.Args[4:])
					result, err := runGoldShadowReconcileOnce(ctx, cfg, pool, logger, streamID, login, noArchive)
					printGoldShadowReconcileResult(result)
					if err != nil {
						logger.Error("gold shadow-reconcile run-once failed", "err", err)
						os.Exit(1)
					}
					return
				case "fixture":
					channel, vodID, broadcasterID, startedAt, endedAt, noArchive := shadowReconcileFixtureArgsFromCLI(os.Args[4:])
					result, err := runGoldShadowReconcileFixture(ctx, cfg, pool, logger, channel, vodID, broadcasterID, startedAt, endedAt, noArchive)
					printGoldShadowReconcileResult(result)
					if err != nil {
						logger.Error("gold shadow-reconcile fixture failed", "err", err)
						os.Exit(1)
					}
					return
				default:
					fmt.Println("Usage: backfill gold shadow-reconcile run-once|fixture ...")
					os.Exit(2)
				}
			}
			if os.Args[2] == "inventory" {
				if len(os.Args) < 4 {
					fmt.Println("Usage: backfill gold inventory run-once|coverage [--enqueue]")
					os.Exit(2)
				}
				switch os.Args[3] {
				case "run-once":
					result, err := runGoldInventoryOnce(ctx, cfg, pool, directEnqueueFromArgs(os.Args[4:]))
					if err != nil {
						logger.Error("gold inventory run-once failed", "err", err)
						os.Exit(1)
					}
					enc := json.NewEncoder(os.Stdout)
					enc.SetIndent("", "  ")
					_ = enc.Encode(result)
					return
				case "coverage":
					result, err := analytics.BuildTop500GoldVODInventoryCoverage(ctx, pool)
					if err != nil {
						logger.Error("gold inventory coverage failed", "err", err)
						os.Exit(1)
					}
					enc := json.NewEncoder(os.Stdout)
					enc.SetIndent("", "  ")
					_ = enc.Encode(result)
					return
				default:
					fmt.Println("Usage: backfill gold inventory run-once|coverage [--enqueue]")
					os.Exit(2)
				}
			}
			dbAlways, err := store.GetAlwaysTracked(ctx)
			if err != nil {
				logger.Error("load always tracked failed", "err", err)
				os.Exit(1)
			}
			allAlways := append(cfg.AlwaysTrackedChannels, dbAlways...)
			rules := analytics.NewGoldRulesEngine(allAlways, cfg.GoldMinPeakViewers, cfg.GoldMinDurationMinutes)
			enqueuer := analytics.NewGoldEnqueuer(pool, rules, cfg.GoldEnqueuerInterval)
			streamID, login := goldArgsFromCLI(os.Args[3:])
			switch os.Args[2] {
			case "enqueue":
				if streamID == "" {
					fmt.Println("Usage: backfill gold enqueue --stream-id=<id> [--login=]")
					os.Exit(2)
				}
				ok, err := enqueuer.EnqueueForce(ctx, streamID, login)
				if err != nil {
					logger.Error("gold enqueue failed", "err", err)
					os.Exit(1)
				}
				if !ok {
					fmt.Println("gold job already queued, running, or done")
					return
				}
				fmt.Println("gold job enqueued")
				return
			case "eval":
				if streamID == "" {
					fmt.Println("Usage: backfill gold eval --stream-id=<id>")
					os.Exit(2)
				}
				result, err := enqueuer.EvalRules(ctx, streamID)
				if err != nil {
					logger.Error("gold eval failed", "err", err)
					os.Exit(1)
				}
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				_ = enc.Encode(result)
				return
			case "run-once":
				noArchive := !goldArchiveRequired(cfg, os.Args[3:])
				if err := runGoldWorkerOnce(ctx, cfg, pool, logger, noArchive); err != nil {
					logger.Error("gold run-once failed", "err", err)
					os.Exit(1)
				}
				fmt.Println("gold run-once completed")
				return
			default:
				fmt.Println("Usage: backfill gold enqueue --stream-id=<id> [--login=] | backfill gold eval --stream-id=<id> | backfill gold run-once | backfill gold inventory run-once|coverage")
				os.Exit(2)
			}
		}
	}

	fmt.Println("backfill worker runs inside analytics when BACKFILL_ENABLED=true")
	fmt.Println("gold enqueuer runs inside analytics when GOLD_BACKFILL_ENABLED=true")
	fmt.Println("bronze indexer runs inside analytics when BRONZE_ENABLED=true")
	fmt.Println("Use: backfill status | ... | backfill gold enqueue|eval|run-once|shadow-reconcile|inventory | ...")
}

func runGoldWorkerOnce(ctx context.Context, cfg config.Config, pool *pgxpool.Pool, logger *slog.Logger, noArchive bool) error {
	if !noArchive && (!cfg.ArchiveEnabled || cfg.ArchiveStorageProvider != "azure") {
		return fmt.Errorf("archive azure export must be enabled (ARCHIVE_ENABLED=true) or pass --no-archive / set GOLD_ARCHIVE_REQUIRED=false")
	}
	rdb, err := newBackfillRedis(cfg)
	if err != nil {
		return err
	}
	defer rdb.Close()

	syncService, err := newBackfillSyncService(cfg, pool, rdb, logger, !noArchive)
	if err != nil {
		return err
	}

	var archiveSyncExporter *archive.SyncExporter
	if !noArchive {
		blob, err := archive.NewBlobStore(cfg.ArchiveBlobStoreConfig())
		if err != nil {
			return err
		}
		archiveWriter := archive.NewWriter(blob, archive.NewManifestStore(pool))
		pgxArchiveDB := archive.NewPgxAnalyticsDB(pool)
		emoteExporter := archive.NewEmoteExporter(archiveWriter, archive.NewPgxEmoteSnapshotDB(pool))
		vodChatDB := archive.NewVODChatDBWithProvenance(pgxArchiveDB, emoteExporter)
		archiveSyncExporter = archive.NewSyncExporter(archiveWriter, vodChatDB)
	}

	worker := analytics.NewBackfillWorker(pool, syncService, archiveSyncExporter, 0).
		WithWorkerOptions(analytics.BackfillWorkerOptions{
			Name:              "gold-run-once",
			TierFilter:        []string{"gold", "gold_full", "gold_lite"},
			StaleRunningAfter: cfg.BackfillStaleRunningAfter,
			HeartbeatInterval: cfg.BackfillHeartbeatInterval,
		}).
		WithGoldSyncTimeout(time.Duration(cfg.GoldSyncTimeoutMS)*time.Millisecond).
		WithVODChatExporter(archiveSyncExporter).
		WithGoldLiteExporter(archiveSyncExporter, cfg.GoldLiteRequireRollups)
	return worker.RunOnce(ctx)
}

type bronzeVODRangeResult struct {
	Login    string    `json:"login"`
	RowCount int       `json:"rowCount"`
	NewestAt time.Time `json:"newestAt,omitempty"`
	OldestAt time.Time `json:"oldestAt,omitempty"`
	SpanDays float64   `json:"spanDays,omitempty"`
	BlobKey  string    `json:"blobKey,omitempty"`
}

func bronzeLoginFromArgs(args []string) string {
	for _, arg := range args {
		if strings.HasPrefix(arg, "--login=") {
			return strings.TrimSpace(strings.TrimPrefix(arg, "--login="))
		}
	}
	return ""
}

func bronzeVODRange(ctx context.Context, cfg config.Config, pool *pgxpool.Pool, login string) (bronzeVODRangeResult, error) {
	login = strings.ToLower(strings.TrimSpace(login))
	out := bronzeVODRangeResult{Login: login, BlobKey: archive.VODIndexBlobKey(login)}
	if login == "" {
		return out, fmt.Errorf("login required")
	}
	if !cfg.ArchiveEnabled || cfg.ArchiveStorageProvider != "azure" {
		return out, fmt.Errorf("archive azure export must be enabled")
	}
	blob, err := newArchiveBlobStore(cfg)
	if err != nil {
		return out, err
	}
	raw, err := blob.Get(ctx, out.BlobKey)
	if err != nil {
		return out, err
	}
	data, err := archive.Gunzip(raw)
	if err != nil {
		return out, err
	}
	var newest, oldest time.Time
	for line := range strings.SplitSeq(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var vod analytics.ArchivedVOD
		if err := json.Unmarshal([]byte(line), &vod); err != nil {
			continue
		}
		if vod.StartedAt.IsZero() {
			continue
		}
		out.RowCount++
		if newest.IsZero() || vod.StartedAt.After(newest) {
			newest = vod.StartedAt
		}
		if oldest.IsZero() || vod.StartedAt.Before(oldest) {
			oldest = vod.StartedAt
		}
	}
	if out.RowCount == 0 {
		return out, fmt.Errorf("no VOD rows in blob")
	}
	out.NewestAt = newest.UTC()
	out.OldestAt = oldest.UTC()
	out.SpanDays = newest.Sub(oldest).Hours() / 24
	_ = pool // reserved for future manifest lookup
	return out, nil
}

func coverageArgsFromCLI(args []string) (sinceRaw, outPath string) {
	for _, arg := range args {
		if strings.HasPrefix(arg, "--since=") {
			sinceRaw = strings.TrimSpace(strings.TrimPrefix(arg, "--since="))
		}
		if strings.HasPrefix(arg, "--out=") {
			outPath = strings.TrimSpace(strings.TrimPrefix(arg, "--out="))
		}
	}
	return sinceRaw, outPath
}

func goldArgsFromCLI(args []string) (streamID, login string) {
	for _, arg := range args {
		if strings.HasPrefix(arg, "--stream-id=") {
			streamID = strings.TrimSpace(strings.TrimPrefix(arg, "--stream-id="))
		}
		if strings.HasPrefix(arg, "--login=") {
			login = strings.TrimSpace(strings.TrimPrefix(arg, "--login="))
		}
	}
	return streamID, login
}

func dryRunFromArgs(args []string) bool {
	for _, arg := range args {
		if arg == "--apply" {
			return false
		}
	}
	return true
}

func directEnqueueFromArgs(args []string) bool {
	for _, arg := range args {
		if arg == "--enqueue" {
			return true
		}
	}
	return false
}

func runGoldInventoryOnce(ctx context.Context, cfg config.Config, pool *pgxpool.Pool, directEnqueue bool) (analytics.Top500GoldVODInventoryResult, error) {
	blob, err := newArchiveBlobStore(cfg)
	if err != nil {
		return analytics.Top500GoldVODInventoryResult{}, err
	}
	if !directEnqueue {
		directEnqueue = cfg.Top500GoldVODInventoryDirectEnqueue
	}
	if directEnqueue && !cfg.GoldFullEnabled {
		return analytics.Top500GoldVODInventoryResult{}, fmt.Errorf("direct Gold enqueue requires GOLD_FULL_ENABLED=true")
	}
	builder := analytics.NewTop500GoldVODInventory(
		pool,
		analytics.NewArchiveVODCatalog(blob),
		analytics.Top500GoldVODInventoryConfig{
			SinceDays:     cfg.Top500GoldVODInventorySinceDays,
			TopN:          cfg.Top500GoldVODInventoryTopN,
			MaxPerRun:     cfg.Top500GoldVODInventoryMaxPerRun,
			DirectEnqueue: directEnqueue,
			Interval:      cfg.Top500GoldVODInventoryInterval,
		},
	)
	return builder.RunOnce(ctx)
}

func sessionCleanupArgsFromCLI(args []string) ([]string, bool) {
	var out []string
	var dryRun bool
	for _, arg := range args {
		if strings.HasPrefix(arg, "--login=") {
			out = append(out, strings.TrimSpace(strings.TrimPrefix(arg, "--login=")))
		}
		if arg == "--dry-run" {
			dryRun = true
		}
	}
	return out, dryRun
}

func newBronzeIndexer(ctx context.Context, cfg config.Config, pool *pgxpool.Pool, logger interface {
	Warn(string, ...any)
}) (*analytics.BronzeIndexer, error) {
	if !cfg.ArchiveEnabled || cfg.ArchiveStorageProvider != "azure" {
		return nil, fmt.Errorf("archive azure export must be enabled (ARCHIVE_ENABLED=true)")
	}
	blob, err := archive.NewBlobStore(cfg.ArchiveBlobStoreConfig())
	if err != nil {
		return nil, err
	}
	writer := archive.NewWriter(blob, archive.NewManifestStore(pool)).WithOptions(archive.WriterOptions{
		ContentHashEnabled:   cfg.ArchiveContentHashEnabled,
		WriteSidecarManifest: cfg.ArchiveWriteSidecarManifest,
		ParserVersion:        cfg.ArchiveParserVersion,
	})
	helix := analytics.NewHelixClient(
		cfg.TwitchAPIURL,
		cfg.TwitchTokenURL,
		cfg.TwitchOAuthClientID,
		cfg.TwitchOAuthClientSecret,
		cfg.Upstream.UserAgent,
	)
	store := analytics.NewStore(pool)
	if err := store.EnsureAlwaysTrackedTable(ctx); err != nil {
		return nil, err
	}
	dbAlways, err := store.GetAlwaysTracked(ctx)
	if err != nil {
		return nil, err
	}
	allAlways := append(cfg.AlwaysTrackedChannels, dbAlways...)
	if !helix.Enabled() {
		logger.Warn("helix client credentials missing; vod index export will fail")
	}
	return analytics.NewBronzeIndexer(
		pool,
		helix,
		cfg.MetadataServiceURL,
		cfg.TwitchTrackerAPIURL,
		cfg.Upstream.UserAgent,
		cfg.BronzeTopN,
		allAlways,
		cfg.BronzeHelixConcurrency,
		cfg.BronzeTTSummaryConcurrency,
	).WithWriter(writer).WithBronzeCorpus(
		archive.NewBronzeExporter(writer),
		cfg.BronzeIdentityEnabled,
		cfg.BronzeCrosswalkEnabled,
		"https://7tv.io/v3",
	), nil
}
