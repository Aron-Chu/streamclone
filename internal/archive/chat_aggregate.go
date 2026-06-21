package archive

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// GoldLiteMinuteLine is one minute chat/emote aggregate row (TASK-014).
type GoldLiteMinuteLine struct {
	MinuteTS          time.Time      `json:"minuteTs"`
	ChatCount         int            `json:"chatCount"`
	TotalEmoteCount   int            `json:"totalEmoteCount"`
	SevenTVEmoteCount int            `json:"seventvEmoteCount"`
	Emotes            map[string]int `json:"emotes,omitempty"`
}

// ExportGoldLite uploads minute chat/emote aggregates from existing Postgres rollups.
func (w *Writer) ExportGoldLite(ctx context.Context, streamID string, db AnalyticsDB, requireRollups bool) error {
	if w == nil || w.blob == nil || db == nil {
		return fmt.Errorf("gold-lite export: not configured")
	}
	streamID = strings.TrimSpace(streamID)
	if streamID == "" {
		return fmt.Errorf("gold-lite export: stream id required")
	}
	rollups, err := db.ExportRollups(ctx, streamID)
	if err != nil {
		return err
	}
	if len(rollups) == 0 {
		if requireRollups {
			return fmt.Errorf("gold-lite export: no rollups for stream %s (GOLD_LITE_REQUIRE_ROLLUPS=true)", streamID)
		}
		return nil
	}
	var buf strings.Builder
	for _, line := range rollups {
		if line.ChatCount == 0 && line.TotalEmoteCount == 0 && len(line.Emotes) == 0 {
			continue
		}
		out := GoldLiteMinuteLine{
			MinuteTS:          line.MinuteTS,
			ChatCount:         line.ChatCount,
			TotalEmoteCount:   line.TotalEmoteCount,
			SevenTVEmoteCount: line.SevenTVEmoteCount,
			Emotes:            line.Emotes,
		}
		raw, err := json.Marshal(out)
		if err != nil {
			return err
		}
		buf.Write(raw)
		buf.WriteByte('\n')
	}
	if buf.Len() == 0 {
		if requireRollups {
			return fmt.Errorf("gold-lite export: rollups exist but no chat/emote aggregates for %s", streamID)
		}
		return nil
	}
	res, err := w.putGzip(ctx, GoldLiteChatBlobKey(streamID), []byte(buf.String()))
	if err != nil {
		return fmt.Errorf("upload gold-lite: %w", err)
	}
	res.RowCount = int64(len(rollups))
	rec := ExportRecord{
		ArtifactType:          ArtifactGoldLite,
		NaturalKey:            GoldLiteKey(streamID),
		GCSURI:                res.URI,
		ETag:                  res.ETag,
		RowCount:              res.RowCount,
		ByteSize:              res.ByteSize,
		Status:                StatusConfirmed,
		ExportedAt:            time.Now().UTC(),
		Tier:                  "gold_lite",
		StreamID:              streamID,
		ContentSHA256:         res.ContentSHA256,
		UncompressedSizeBytes: res.UncompressedSizeBytes,
	}
	if w.manifest == nil {
		return nil
	}
	return w.manifest.Upsert(ctx, rec)
}

func (e *SyncExporter) ExportGoldLite(ctx context.Context, streamID string, requireRollups bool) error {
	if e == nil || e.writer == nil || e.db == nil {
		return fmt.Errorf("gold-lite exporter not configured")
	}
	return e.writer.ExportGoldLite(ctx, streamID, e.db, requireRollups)
}
