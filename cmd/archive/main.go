package main

import (
	"context"
	"fmt"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"

	"streamclone/internal/archive"
	"streamclone/internal/config"
	"streamclone/internal/log"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(2)
	}
	cfg, err := config.Load()
	logger := log.New("archive", cfg.LogLevel)
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

	blob, err := archive.NewBlobStore(cfg.ArchiveBlobStoreConfig())
	if err != nil {
		logger.Error("azure blob init failed", "err", err)
		os.Exit(1)
	}
	manifest := archive.NewManifestStore(pool)
	writer := archive.NewWriter(blob, manifest)
	restorer := archive.NewRestorer(blob, pool)
	exporterDB := archive.NewPgxAnalyticsDB(pool)

	switch os.Args[1] {
	case "export":
		if len(os.Args) < 3 {
			fmt.Fprintln(os.Stderr, "usage: archive export <stream-id>")
			os.Exit(2)
		}
		streamID := os.Args[2]
		if err := writer.ExportStream(ctx, streamID, exporterDB); err != nil {
			logger.Error("export failed", "stream_id", streamID, "err", err)
			os.Exit(1)
		}
		logger.Info("export completed", "stream_id", streamID)
	case "restore":
		if len(os.Args) < 3 {
			fmt.Fprintln(os.Stderr, "usage: archive restore --stream-id <id>")
			os.Exit(2)
		}
		streamID := flagValue(os.Args[2:], "--stream-id")
		if streamID == "" && len(os.Args) >= 3 {
			streamID = os.Args[2]
		}
		result, err := restorer.RestoreStream(ctx, streamID)
		if err != nil {
			logger.Error("restore failed", "stream_id", streamID, "err", err)
			os.Exit(1)
		}
		logger.Info("restore completed", "stream_id", result.StreamID, "rollups", result.RollupCount)
	case "verify":
		if len(os.Args) < 3 {
			fmt.Fprintln(os.Stderr, "usage: archive verify --stream-id <id>")
			os.Exit(2)
		}
		streamID := flagValue(os.Args[2:], "--stream-id")
		if streamID == "" {
			streamID = os.Args[2]
		}
		if err := archive.VerifyStream(ctx, manifest, blob, streamID); err != nil {
			logger.Error("verify failed", "stream_id", streamID, "err", err)
			os.Exit(1)
		}
		logger.Info("verify ok", "stream_id", streamID)
	default:
		printUsage()
		os.Exit(2)
	}
}

func flagValue(args []string, name string) string {
	for i := 0; i < len(args)-1; i++ {
		if args[i] == name {
			return args[i+1]
		}
	}
	return ""
}

func printUsage() {
	fmt.Println(`Streamclone archive CLI

Usage:
  archive export <stream-id>
  archive restore --stream-id <id>
  archive verify --stream-id <id>`)
}
