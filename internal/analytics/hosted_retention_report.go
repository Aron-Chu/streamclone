package analytics

import (
	"context"
	"fmt"
	"time"
)

type HostedDatabaseTableFootprint struct {
	Relation string `json:"relation"`
	Bytes    int64  `json:"bytes"`
	Rows     int64  `json:"rows,omitempty"`
}

type HostedDatabaseFootprint struct {
	DatabaseBytes int64                          `json:"databaseBytes,omitempty"`
	Tables        []HostedDatabaseTableFootprint `json:"tables,omitempty"`
	Error         string                         `json:"error,omitempty"`
}

type HostedRetentionConfig struct {
	AnalyticsRetentionDays        int  `json:"analyticsRetentionDays"`
	AnalyticsVODChatRetentionDays int  `json:"analyticsVodChatRetentionDays"`
	BackfillJobRetentionDays      int  `json:"backfillJobRetentionDays"`
	PruneEnabled                  bool `json:"pruneEnabled"`
}

type BackfillJobPruneReport struct {
	RetentionDays int   `json:"retentionDays"`
	DryRun        bool  `json:"dryRun"`
	DeletedRows   int64 `json:"deletedRows"`
}

type HostedRetentionReport struct {
	GeneratedAt       time.Time               `json:"generatedAt"`
	Config            HostedRetentionConfig   `json:"config"`
	Footprint         HostedDatabaseFootprint `json:"footprint"`
	LastBackfillPrune *BackfillJobPruneReport `json:"lastBackfillPrune,omitempty"`
	Errors            []string                `json:"errors,omitempty"`
}

func (h *Handler) hostedRetentionConfig() HostedRetentionConfig {
	cfg := h.appConfig
	return HostedRetentionConfig{
		AnalyticsRetentionDays:        cfg.AnalyticsRetentionDays,
		AnalyticsVODChatRetentionDays: cfg.AnalyticsVODChatRetentionDays,
		BackfillJobRetentionDays:      cfg.HostedBackfillJobRetentionDays,
		PruneEnabled:                  cfg.HostedRetentionPruneEnabled,
	}
}

func (s *Store) HostedDatabaseFootprint(ctx context.Context) (HostedDatabaseFootprint, error) {
	report := HostedDatabaseFootprint{}
	if s == nil || s.db == nil {
		return report, fmt.Errorf("analytics store unavailable")
	}
	if err := s.db.QueryRow(ctx, `SELECT pg_database_size(current_database())`).Scan(&report.DatabaseBytes); err != nil {
		return report, err
	}
	relations := []string{
		"analytics_minute_rollups",
		"analytics_vod_chat_messages",
		"backfill_jobs",
		"emote_usage_minute_rollups",
	}
	for _, rel := range relations {
		var bytes int64
		err := s.db.QueryRow(ctx, `
			SELECT COALESCE(pg_total_relation_size(to_regclass($1)), 0)`, rel).Scan(&bytes)
		if err != nil {
			continue
		}
		row := HostedDatabaseTableFootprint{Relation: rel, Bytes: bytes}
		if count, err := s.hostedTableRowEstimate(ctx, rel); err == nil {
			row.Rows = count
		}
		report.Tables = append(report.Tables, row)
	}
	return report, nil
}

func (s *Store) hostedTableRowEstimate(ctx context.Context, relation string) (int64, error) {
	var count int64
	err := s.db.QueryRow(ctx, `
		SELECT COALESCE(reltuples, 0)::bigint
		FROM pg_class
		WHERE relname = $1`, relation).Scan(&count)
	return count, err
}

// StartHostedRetentionMaintainer prunes terminal backfill jobs on a daily interval when enabled.
func StartHostedRetentionMaintainer(ctx context.Context, store *Store, retentionDays int, log interface {
	Info(msg string, args ...any)
	Warn(msg string, args ...any)
}) {
	if store == nil || retentionDays <= 0 {
		return
	}
	go func() {
		ticker := time.NewTicker(24 * time.Hour)
		defer ticker.Stop()
		run := func() {
			report, err := store.PurgeTerminalBackfillJobs(ctx, retentionDays, false)
			if err != nil {
				if log != nil {
					log.Warn("hosted backfill retention prune failed", "err", err)
				}
				return
			}
			if log != nil && report.DeletedRows > 0 {
				log.Info("hosted backfill retention prune complete", "deleted", report.DeletedRows, "retention_days", retentionDays)
			}
		}
		run()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				run()
			}
		}
	}()
}

func (s *Store) PurgeTerminalBackfillJobs(ctx context.Context, retentionDays int, dryRun bool) (BackfillJobPruneReport, error) {
	report := BackfillJobPruneReport{RetentionDays: retentionDays, DryRun: dryRun}
	if s == nil || s.db == nil {
		return report, fmt.Errorf("analytics store unavailable")
	}
	if retentionDays <= 0 {
		retentionDays = 30
		report.RetentionDays = retentionDays
	}
	cutoff := time.Now().UTC().Add(-time.Duration(retentionDays) * 24 * time.Hour)
	if dryRun {
		err := s.db.QueryRow(ctx, `
			SELECT COUNT(*)
			FROM backfill_jobs
			WHERE status IN ('done','skipped','failed')
			  AND updated_at < $1`, cutoff).Scan(&report.DeletedRows)
		return report, err
	}
	tag, err := s.db.Exec(ctx, `
		DELETE FROM backfill_jobs
		WHERE status IN ('done','skipped','failed')
		  AND updated_at < $1`, cutoff)
	if err != nil {
		return report, err
	}
	report.DeletedRows = tag.RowsAffected()
	return report, nil
}
