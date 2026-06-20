package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

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
			default:
				fmt.Println("Usage: backfill bronze status | backfill bronze run-once")
				os.Exit(2)
			}
		case "sessions":
			if len(os.Args) < 3 || os.Args[2] != "cleanup" {
				fmt.Println("Usage: backfill sessions cleanup [--login=ohnepixel]")
				os.Exit(2)
			}
			explicit := sessionCleanupLoginsFromArgs(os.Args[3:])
			logins, err := store.ResolveSessionCleanupLogins(ctx, explicit, cfg.AlwaysTrackedChannels)
			if err != nil {
				logger.Error("resolve cleanup logins failed", "err", err)
				os.Exit(1)
			}
			report, err := store.CleanupSessionStubs(ctx, logins)
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
		case "gold":
			if len(os.Args) < 3 {
				fmt.Println("Usage: backfill gold enqueue --stream-id=<id> [--login=] | backfill gold eval --stream-id=<id>")
				os.Exit(2)
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
			default:
				fmt.Println("Usage: backfill gold enqueue --stream-id=<id> [--login=] | backfill gold eval --stream-id=<id>")
				os.Exit(2)
			}
		}
	}

	fmt.Println("backfill worker runs inside analytics when BACKFILL_ENABLED=true")
	fmt.Println("gold enqueuer runs inside analytics when GOLD_BACKFILL_ENABLED=true")
	fmt.Println("bronze indexer runs inside analytics when BRONZE_ENABLED=true")
	fmt.Println("Use: backfill status | backfill gold enqueue|eval | backfill bronze status | backfill bronze run-once | backfill sessions cleanup [--login=...]")
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

func sessionCleanupLoginsFromArgs(args []string) []string {
	var out []string
	for _, arg := range args {
		if strings.HasPrefix(arg, "--login=") {
			out = append(out, strings.TrimSpace(strings.TrimPrefix(arg, "--login=")))
		}
	}
	return out
}

func newBronzeIndexer(ctx context.Context, cfg config.Config, pool *pgxpool.Pool, logger interface {
	Warn(string, ...any)
}) (*analytics.BronzeIndexer, error) {
	if !cfg.ArchiveEnabled || cfg.ArchiveStorageProvider != "azure" {
		return nil, fmt.Errorf("archive azure export must be enabled (ARCHIVE_ENABLED=true)")
	}
	blob, err := archive.NewAzureBlobStore(archive.AzureConfig{
		StorageAccount:       cfg.ArchiveAzureStorageAccount,
		Container:            cfg.ArchiveAzureContainer,
		Prefix:               cfg.ArchiveAzurePrefix,
		ConnectionStringFile: cfg.ArchiveAzureConnectionStringFile,
	})
	if err != nil {
		return nil, err
	}
	writer := archive.NewWriter(blob, archive.NewManifestStore(pool))
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
	).WithWriter(writer), nil
}
