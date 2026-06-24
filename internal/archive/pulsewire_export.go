package archive

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// PulseWireExportLine is one social item exported to cold storage (TASK-032).
type PulseWireExportLine struct {
	ID           string          `json:"id"`
	Source       string          `json:"source"`
	Title        string          `json:"title,omitempty"`
	URL          string          `json:"url,omitempty"`
	Score        int             `json:"score,omitempty"`
	CommentCount int             `json:"commentCount,omitempty"`
	CapturedAt   time.Time       `json:"capturedAt"`
	Metrics      json.RawMessage `json:"metrics,omitempty"`
}

func PulseWireRawBlobKey(source, date string) string {
	source = strings.TrimSpace(source)
	date = strings.TrimSpace(date)
	return fmt.Sprintf("pulsewire/raw/source=%s/date=%s/part-000.jsonl.gz", source, date)
}

type pulsewireMetrics struct {
	Score    int `json:"score"`
	Comments int `json:"comments"`
}

// ExportPulseWireSource exports recent social_items rows for one source.
func (w *Writer) ExportPulseWireSource(ctx context.Context, db *pgxpool.Pool, source string, limit int) error {
	if w == nil || w.blob == nil || db == nil {
		return fmt.Errorf("pulsewire export: not configured")
	}
	source = strings.ToLower(strings.TrimSpace(source))
	if source == "" {
		return fmt.Errorf("pulsewire export: source required")
	}
	if limit <= 0 {
		limit = 500
	}
	rows, err := db.Query(ctx, `
		SELECT id::text, source, COALESCE(url,''), COALESCE(text,''), metrics, ingested_at
		FROM social_items
		WHERE source = $1
		ORDER BY ingested_at DESC
		LIMIT $2`, source, limit)
	if err != nil {
		return err
	}
	defer rows.Close()
	var buf strings.Builder
	var count int
	for rows.Next() {
		var line PulseWireExportLine
		var metricsRaw []byte
		if err := rows.Scan(&line.ID, &line.Source, &line.URL, &line.Title, &metricsRaw, &line.CapturedAt); err != nil {
			return err
		}
		if len(metricsRaw) > 0 {
			line.Metrics = metricsRaw
			var m pulsewireMetrics
			if json.Unmarshal(metricsRaw, &m) == nil {
				line.Score = m.Score
				line.CommentCount = m.Comments
			}
		}
		b, err := json.Marshal(line)
		if err != nil {
			return err
		}
		buf.Write(b)
		buf.WriteByte('\n')
		count++
	}
	if count == 0 {
		return nil
	}
	date := time.Now().UTC().Format("2006-01-02")
	res, err := w.putGzip(ctx, PulseWireRawBlobKey(source, date), []byte(buf.String()))
	if err != nil {
		return err
	}
	res.RowCount = int64(count)
	naturalKey := PulseWireRawKey(source, date, 0)
	rec := ExportRecord{
		ArtifactType:          ArtifactSocialItem,
		NaturalKey:            naturalKey,
		GCSURI:                res.URI,
		ETag:                  res.ETag,
		RowCount:              res.RowCount,
		ByteSize:              res.ByteSize,
		Status:                StatusConfirmed,
		ExportedAt:            time.Now().UTC(),
		Tier:                  "pulsewire",
		Provider:              source,
		ContentSHA256:         res.ContentSHA256,
		UncompressedSizeBytes: res.UncompressedSizeBytes,
	}
	if w.manifest == nil {
		return nil
	}
	return w.manifest.Upsert(ctx, rec)
}
