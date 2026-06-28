package archive

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// StreamExportRow is a sanitized manifest row for product-facing availability.
type StreamExportRow struct {
	ArtifactType  string
	NaturalKey    string
	ExportStatus  string
	Provider      string
	ByteSize      int64
	ContentSHA256 string
	ExportedAt    *time.Time
	UpdatedAt     time.Time
}

// StreamExportRows returns manifest rows tied to a stream without blob URLs.
func (s *ManifestStore) StreamExportRows(ctx context.Context, streamID string) ([]StreamExportRow, error) {
	if s == nil || s.db == nil {
		return nil, nil
	}
	streamID = strings.TrimSpace(streamID)
	if streamID == "" {
		return nil, nil
	}
	rollupKey := ViewerRollupKey(streamID)
	legacyRollupKey := LegacyRollupsKey(streamID)
	vodChatKey := fmt.Sprintf("vod_chat:%s", streamID)

	rows, err := s.db.Query(ctx, `
		SELECT artifact_type, natural_key, export_status, provider, byte_size,
		       content_sha256, exported_at, updated_at
		FROM archive_exports
		WHERE stream_id = $1
		   OR natural_key IN ($2, $3, $4)
		   OR (artifact_type = $5 AND natural_key = $1)
	`, streamID, rollupKey, legacyRollupKey, vodChatKey, ArtifactAnalyticsStream)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []StreamExportRow
	for rows.Next() {
		var row StreamExportRow
		var provider, checksum *string
		var exportedAt *time.Time
		if err := rows.Scan(
			&row.ArtifactType,
			&row.NaturalKey,
			&row.ExportStatus,
			&provider,
			&row.ByteSize,
			&checksum,
			&exportedAt,
			&row.UpdatedAt,
		); err != nil {
			return nil, err
		}
		if provider != nil {
			row.Provider = strings.TrimSpace(*provider)
		}
		if checksum != nil {
			row.ContentSHA256 = strings.TrimSpace(*checksum)
		}
		row.ExportStatus = normalizeStatus(row.ExportStatus)
		row.ExportedAt = exportedAt
		out = append(out, row)
	}
	return out, rows.Err()
}
