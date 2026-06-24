package archive

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	defaultDirectoryExportInterval = time.Hour
	defaultEmoteSnapshotInterval   = 7 * 24 * time.Hour
	defaultModEventExportInterval  = time.Hour
)

// StartDirectorySampleExportWorker exports the previous UTC hour on each tick.
func StartDirectorySampleExportWorker(ctx context.Context, writer *Writer, pool *pgxpool.Pool, interval time.Duration, log interface {
	Info(string, ...any)
	Warn(string, ...any)
}) {
	if writer == nil || pool == nil {
		return
	}
	if interval <= 0 {
		interval = defaultDirectoryExportInterval
	}
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		run := func(label string) {
			prev := time.Now().UTC().Add(-time.Hour)
			date := prev.Format("2006-01-02")
			hour := prev.Format("15")
			if err := writer.ExportDirectorySamples(ctx, pool, date, hour); err != nil && log != nil {
				log.Warn("archive directory sample export failed", "trigger", label, "date", date, "hour", hour, "err", err)
				return
			}
			if log != nil {
				log.Info("archive directory sample export tick", "trigger", label, "date", date, "hour", hour)
			}
		}
		run("startup")
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				run("interval")
			}
		}
	}()
}

// StartEmoteSnapshotWorker runs weekly 7TV snapshot exports for the roster.
func StartEmoteSnapshotWorker(ctx context.Context, exporter *EmoteExporter, interval time.Duration, globalEnabled bool, sevenTVAPIURL string, log interface {
	Info(string, ...any)
	Warn(string, ...any)
}) {
	if exporter == nil {
		return
	}
	if interval <= 0 {
		interval = defaultEmoteSnapshotInterval
	}
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		run := func(label string) {
			if globalEnabled {
				if err := exporter.ExportGlobalSevenTVSnapshot(ctx, sevenTVAPIURL); err != nil && log != nil {
					log.Warn("archive global 7tv snapshot failed", "trigger", label, "err", err)
				}
			}
			exported, skipped, err := exporter.ExportWeeklySnapshots(ctx, false)
			if err != nil && log != nil {
				log.Warn("archive emote snapshot export failed", "trigger", label, "err", err)
				return
			}
			if (exported > 0 || skipped > 0) && log != nil {
				log.Info("archive emote snapshot export tick", "trigger", label, "exported", exported, "skipped", skipped)
			}
		}
		run("startup")
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				run("interval")
			}
		}
	}()
}

// StartModEventExportWorker exports mod events approaching retention cutoff.
func StartModEventExportWorker(ctx context.Context, writer *Writer, pool *pgxpool.Pool, retentionDays int, interval time.Duration, log interface {
	Info(string, ...any)
	Warn(string, ...any)
}) {
	if writer == nil || pool == nil {
		return
	}
	if retentionDays <= 0 {
		retentionDays = 14
	}
	if interval <= 0 {
		interval = defaultModEventExportInterval
	}
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		run := func(label string) {
			cutoff := time.Now().UTC().Add(-time.Duration(retentionDays) * 24 * time.Hour)
			if err := writer.ExportModEventsBeforePurge(ctx, pool, cutoff); err != nil && log != nil {
				log.Warn("archive mod event export failed", "trigger", label, "err", err)
				return
			}
			if log != nil {
				log.Info("archive mod event export tick", "trigger", label, "cutoff", cutoff)
			}
		}
		run("startup")
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				run("interval")
			}
		}
	}()
}

// VODChatDBWithProvenance wraps analytics DB and supplies Gold provenance metadata.
type VODChatDBWithProvenance struct {
	*PgxAnalyticsDB
	emotes *EmoteExporter
}

func NewVODChatDBWithProvenance(db *PgxAnalyticsDB, emotes *EmoteExporter) *VODChatDBWithProvenance {
	return &VODChatDBWithProvenance{PgxAnalyticsDB: db, emotes: emotes}
}

func (d *VODChatDBWithProvenance) BuildVODChatProvenance(ctx context.Context, streamID, login string) VODChatProvenance {
	if d == nil || d.emotes == nil {
		return VODChatProvenance{
			StreamID:              streamID,
			Login:                 login,
			EmoteSnapshotStrategy: defaultEmoteSnapshotStrategy,
			ExportedAt:            time.Now().UTC(),
		}
	}
	return d.emotes.BuildVODChatProvenance(ctx, streamID, login)
}

// EventAPIChangelogAdapter records 7TV EventAPI deltas into cold storage.
type EventAPIChangelogAdapter struct {
	Exporter *EmoteExporter
}

func (a *EventAPIChangelogAdapter) RecordSetUpdate(ctx context.Context, login, setID string, raw json.RawMessage) error {
	if a == nil || a.Exporter == nil {
		return nil
	}
	return a.Exporter.AppendChangelog(ctx, EmoteChangelogLine{
		Provider:      "7tv",
		Login:         login,
		EventType:     "set_update",
		ProviderSetID: setID,
		Payload:       raw,
		RecordedAt:    time.Now().UTC(),
	})
}
