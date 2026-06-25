package archive

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// TombstoneLine records a VOD id that disappeared from the Helix catalog snapshot.
type TombstoneLine struct {
	SchemaVersion string    `json:"schemaVersion"`
	Login         string    `json:"login"`
	VodID         string    `json:"vodId"`
	DetectedAt    time.Time `json:"detectedAt"`
	Reason        string    `json:"reason,omitempty"`
}

// DiffVODCatalog returns vod ids present in previous but absent in current.
func DiffVODCatalog(previous, current []string) []string {
	prev := make(map[string]struct{}, len(previous))
	for _, id := range previous {
		if id = strings.TrimSpace(id); id != "" {
			prev[id] = struct{}{}
		}
	}
	for _, id := range current {
		if id = strings.TrimSpace(id); id != "" {
			delete(prev, id)
		}
	}
	out := make([]string, 0, len(prev))
	for id := range prev {
		out = append(out, id)
	}
	return out
}

// VodCatalogState tracks last-seen VOD ids per login for tombstone detection.
type VodCatalogState struct {
	db *pgxpool.Pool
}

func NewVodCatalogState(db *pgxpool.Pool) *VodCatalogState {
	return &VodCatalogState{db: db}
}

func (s *VodCatalogState) LoadVodIDs(ctx context.Context, login string) ([]string, error) {
	if s == nil || s.db == nil {
		return nil, nil
	}
	login = strings.ToLower(strings.TrimSpace(login))
	if login == "" {
		return nil, nil
	}
	rows, err := s.db.Query(ctx, `SELECT vod_id FROM vod_catalog_state WHERE login = $1`, login)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var vodID string
		if err := rows.Scan(&vodID); err != nil {
			return nil, err
		}
		out = append(out, vodID)
	}
	return out, rows.Err()
}

func (s *VodCatalogState) ReplaceLoginCatalog(ctx context.Context, login string, vodIDs []string) (removed []string, err error) {
	if s == nil || s.db == nil {
		return nil, nil
	}
	login = strings.ToLower(strings.TrimSpace(login))
	if login == "" {
		return nil, fmt.Errorf("vod catalog state: login required")
	}
	previous, err := s.LoadVodIDs(ctx, login)
	if err != nil {
		return nil, err
	}
	removed = DiffVODCatalog(previous, vodIDs)
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `DELETE FROM vod_catalog_state WHERE login = $1`, login); err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	for _, vodID := range vodIDs {
		vodID = strings.TrimSpace(vodID)
		if vodID == "" {
			continue
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO vod_catalog_state (login, vod_id, last_seen_at)
			VALUES ($1, $2, $3)`, login, vodID, now); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return removed, nil
}

func TombstoneBlobKey(vodID, date string) string {
	vodID = strings.TrimSpace(vodID)
	date = strings.TrimSpace(date)
	return fmt.Sprintf("streams/tombstones/vod_id=%s/date=%s/tombstone.json.gz", vodID, date)
}

func BronzeCoverageBlobKey(date string) string {
	date = strings.TrimSpace(date)
	return fmt.Sprintf("coverage/bronze/date=%s/summary.json.gz", date)
}

func (b *BronzeExporter) ExportTombstone(ctx context.Context, login, vodID string) error {
	if b == nil || b.writer == nil {
		return fmt.Errorf("bronze exporter not configured")
	}
	login = strings.ToLower(strings.TrimSpace(login))
	vodID = strings.TrimSpace(vodID)
	if login == "" || vodID == "" {
		return fmt.Errorf("tombstone export: login and vod id required")
	}
	date := time.Now().UTC().Format("2006-01-02")
	line := TombstoneLine{
		SchemaVersion: "vod_tombstone/v1",
		Login:         login,
		VodID:         vodID,
		DetectedAt:    time.Now().UTC(),
		Reason:        "missing_from_catalog",
	}
	raw, err := json.Marshal(line)
	if err != nil {
		return err
	}
	res, err := b.writer.putGzip(ctx, TombstoneBlobKey(vodID, date), raw)
	if err != nil {
		return err
	}
	res.RowCount = 1
	naturalKey := fmt.Sprintf("%s:%s:%s", login, vodID, date)
	rec := ExportRecord{
		ArtifactType:          ArtifactVODTombstone,
		NaturalKey:            naturalKey,
		GCSURI:                res.URI,
		ETag:                  res.ETag,
		RowCount:              1,
		ByteSize:              res.ByteSize,
		Status:                StatusConfirmed,
		ExportedAt:            time.Now().UTC(),
		Tier:                  "bronze",
		ChannelLogin:          login,
		VodID:                 vodID,
		ContentSHA256:         res.ContentSHA256,
		UncompressedSizeBytes: res.UncompressedSizeBytes,
	}
	if b.writer.manifest == nil {
		return nil
	}
	return b.writer.manifest.Upsert(ctx, rec)
}

func (b *BronzeExporter) ExportRosterTier0(ctx context.Context, payload []byte) error {
	if b == nil || b.writer == nil {
		return fmt.Errorf("bronze exporter not configured")
	}
	date := time.Now().UTC().Format("2006-01-02")
	res, err := b.writer.putGzip(ctx, RosterTier0BlobKey(date), payload)
	if err != nil {
		return err
	}
	res.RowCount = 1
	rec := ExportRecord{
		ArtifactType:          ArtifactBronzeRoster,
		NaturalKey:            BronzeRosterKey(date),
		GCSURI:                res.URI,
		ETag:                  res.ETag,
		RowCount:              1,
		ByteSize:              res.ByteSize,
		Status:                StatusConfirmed,
		ExportedAt:            time.Now().UTC(),
		Tier:                  "bronze",
		ContentSHA256:         res.ContentSHA256,
		UncompressedSizeBytes: res.UncompressedSizeBytes,
	}
	if b.writer.manifest == nil {
		return nil
	}
	return b.writer.manifest.Upsert(ctx, rec)
}

// BronzeCoverageSummary is the daily bronze coverage blob (TASK-009).
type BronzeCoverageSummary struct {
	SchemaVersion string    `json:"schemaVersion"`
	Date          string    `json:"date"`
	GeneratedAt   time.Time `json:"generatedAt"`
	TopN          int       `json:"topN"`
	ChannelsTotal int       `json:"channelsTotal"`
	WithVODIndex  int       `json:"withVodIndex"`
	WithSummary   int       `json:"withSummary"`
	Errors        int       `json:"errors"`
}

func (b *BronzeExporter) ExportBronzeCoverage(ctx context.Context, summary BronzeCoverageSummary) error {
	if b == nil || b.writer == nil {
		return fmt.Errorf("bronze exporter not configured")
	}
	if summary.Date == "" {
		summary.Date = time.Now().UTC().Format("2006-01-02")
	}
	summary.SchemaVersion = "bronze_coverage/v1"
	summary.GeneratedAt = time.Now().UTC()
	raw, err := json.Marshal(summary)
	if err != nil {
		return err
	}
	res, err := b.writer.putGzip(ctx, BronzeCoverageBlobKey(summary.Date), raw)
	if err != nil {
		return err
	}
	res.RowCount = 1
	naturalKey := fmt.Sprintf("bronze:%s", summary.Date)
	rec := ExportRecord{
		ArtifactType:          ArtifactBronzeCoverage,
		NaturalKey:            naturalKey,
		GCSURI:                res.URI,
		ETag:                  res.ETag,
		RowCount:              1,
		ByteSize:              res.ByteSize,
		Status:                StatusConfirmed,
		ExportedAt:            time.Now().UTC(),
		Tier:                  "bronze",
		ContentSHA256:         res.ContentSHA256,
		UncompressedSizeBytes: res.UncompressedSizeBytes,
	}
	if b.writer.manifest == nil {
		return nil
	}
	return b.writer.manifest.Upsert(ctx, rec)
}
