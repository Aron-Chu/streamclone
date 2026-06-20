package archive

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	StatusPending   = "pending"
	StatusConfirmed = "confirmed"
	StatusFailed    = "failed"

	ArtifactAnalyticsStream = "analytics_stream"
	ArtifactAnalyticsRollups = "analytics_rollups"
	ArtifactTTDetailHTML    = "tt_detail_html"
	ArtifactVODChatMessage  = "vod_chat_message"
	ArtifactLiveChatMessage = "live_chat_message"
	ArtifactChatModEvent    = "chat_mod_event"
	ArtifactSocialItem      = "social_item"
	ArtifactDirectorySample    = "directory_sample"
	ArtifactFollowerSample     = "follower_sample"
	ArtifactBronzeTop500       = "bronze_top500"
	ArtifactBronzeVODIndex     = "bronze_vod_index"
	ArtifactBronzeChannelSummary = "bronze_channel_summary"
)

type ExportRecord struct {
	ArtifactType     string
	NaturalKey       string
	GCSURI           string
	ObjectGeneration string
	ETag             string
	RowCount         int64
	ByteSize         int64
	Status           string
	Error            string
	ExportedAt       time.Time
}

type ManifestStore struct {
	db *pgxpool.Pool
}

func NewManifestStore(db *pgxpool.Pool) *ManifestStore {
	return &ManifestStore{db: db}
}

func (s *ManifestStore) Upsert(ctx context.Context, rec ExportRecord) error {
	if s == nil || s.db == nil {
		return nil
	}
	rec.ArtifactType = strings.TrimSpace(rec.ArtifactType)
	rec.NaturalKey = strings.TrimSpace(rec.NaturalKey)
	rec.Status = normalizeStatus(rec.Status)
	if rec.ArtifactType == "" || rec.NaturalKey == "" {
		return errors.New("archive manifest: artifact type and natural key are required")
	}
	if strings.TrimSpace(rec.GCSURI) == "" {
		return errors.New("archive manifest: gcs uri is required")
	}
	var exportedAt any
	if !rec.ExportedAt.IsZero() {
		exportedAt = rec.ExportedAt.UTC()
	}
	_, err := s.db.Exec(ctx, `
		INSERT INTO archive_exports (
			artifact_type, natural_key, gcs_uri, object_generation, etag, row_count,
			byte_size, export_status, error, exported_at, updated_at
		)
		VALUES ($1,$2,$3,NULLIF($4,''),NULLIF($5,''),NULLIF($6,0),NULLIF($7,0),$8,NULLIF($9,''),$10,now())
		ON CONFLICT (artifact_type, natural_key) DO UPDATE SET
			gcs_uri = EXCLUDED.gcs_uri,
			object_generation = EXCLUDED.object_generation,
			etag = EXCLUDED.etag,
			row_count = EXCLUDED.row_count,
			byte_size = EXCLUDED.byte_size,
			export_status = EXCLUDED.export_status,
			error = EXCLUDED.error,
			exported_at = EXCLUDED.exported_at,
			updated_at = now()`,
		rec.ArtifactType, rec.NaturalKey, rec.GCSURI, rec.ObjectGeneration, rec.ETag,
		rec.RowCount, rec.ByteSize, rec.Status, rec.Error, exportedAt,
	)
	return err
}

func normalizeStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case StatusConfirmed:
		return StatusConfirmed
	case StatusFailed:
		return StatusFailed
	default:
		return StatusPending
	}
}

type RetentionBlockedError struct {
	ArtifactType string
	Missing      int64
}

func (e RetentionBlockedError) Error() string {
	return fmt.Sprintf("archive retention guard blocked purge: %d %s artifact(s) lack confirmed archive_exports rows", e.Missing, e.ArtifactType)
}

func BlockIfMissing(artifactType string, missing int64) error {
	if missing <= 0 {
		return nil
	}
	return RetentionBlockedError{ArtifactType: artifactType, Missing: missing}
}

func (s *ManifestStore) ExportStatus(ctx context.Context, artifactType, naturalKey string) (string, error) {
	if s == nil || s.db == nil {
		return "", errors.New("archive manifest: store unavailable")
	}
	var status string
	err := s.db.QueryRow(ctx, `
		SELECT export_status FROM archive_exports
		WHERE artifact_type = $1 AND natural_key = $2`,
		artifactType, naturalKey,
	).Scan(&status)
	return status, err
}
