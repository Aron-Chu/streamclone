package archive

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	StatusPending   = "pending"
	StatusConfirmed = "confirmed"
	StatusPartial   = "partial"
	StatusComplete  = "complete"
	StatusFailed    = "failed"

	ArtifactAnalyticsStream = "analytics_stream"
	ArtifactAnalyticsRollups = "analytics_rollups"
	ArtifactBronzeVODCatalog   = "bronze_vod_catalog"
	ArtifactChannelIdentity    = "channel_identity"
	ArtifactProviderCrosswalk  = "provider_crosswalk"
	ArtifactEmoteSnapshotGlobal = "emote_snapshot_global"
	ArtifactGoldLite           = "gold_lite"
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
	ArtifactEmoteSnapshot      = "emote_snapshot"
	ArtifactEmoteChangelog     = "emote_changelog"
	ArtifactTTChartJSON        = "tt_chart_json"
	ArtifactBronzeRoster       = "bronze_roster"
	ArtifactVODTombstone       = "vod_tombstone"
	ArtifactBronzeCoverage     = "bronze_coverage"
)

type ExportRecord struct {
	ArtifactType            string
	NaturalKey              string
	GCSURI                  string
	ObjectGeneration        string
	ETag                    string
	RowCount                int64
	ByteSize                int64
	Status                  string
	Error                   string
	ExportedAt              time.Time
	ArtifactID              string
	Tier                    string
	Provider                string
	ChannelLogin            string
	ChannelID               string
	StreamID                string
	VodID                   string
	SourceURL               string
	ContentSHA256           string
	UncompressedSizeBytes   int64
	FailureReason           string
	Metadata                map[string]any
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
	rec.ArtifactType, rec.NaturalKey = MapLegacyNaturalKey(rec.ArtifactType, rec.NaturalKey)
	metaJSON, err := marshalMetadata(rec.Metadata)
	if err != nil {
		return err
	}
	var artifactID any
	if strings.TrimSpace(rec.ArtifactID) != "" {
		artifactID = rec.ArtifactID
	}
	_, err = s.db.Exec(ctx, `
		INSERT INTO archive_exports (
			artifact_type, natural_key, gcs_uri, object_generation, etag, row_count,
			byte_size, export_status, error, exported_at, updated_at,
			artifact_id, tier, provider, channel_login, channel_id, stream_id, vod_id,
			source_url, content_sha256, uncompressed_size_bytes, failure_reason, metadata
		)
		VALUES (
			$1,$2,$3,NULLIF($4,''),NULLIF($5,''),NULLIF($6,0),NULLIF($7,0),$8,NULLIF($9,''),$10,now(),
			$11,NULLIF($12,''),NULLIF($13,''),NULLIF($14,''),NULLIF($15,''),NULLIF($16,''),NULLIF($17,''),
			NULLIF($18,''),NULLIF($19,''),NULLIF($20,0),NULLIF($21,''),$22
		)
		ON CONFLICT (artifact_type, natural_key) DO UPDATE SET
			gcs_uri = EXCLUDED.gcs_uri,
			object_generation = EXCLUDED.object_generation,
			etag = EXCLUDED.etag,
			row_count = EXCLUDED.row_count,
			byte_size = EXCLUDED.byte_size,
			export_status = EXCLUDED.export_status,
			error = EXCLUDED.error,
			exported_at = EXCLUDED.exported_at,
			artifact_id = COALESCE(EXCLUDED.artifact_id, archive_exports.artifact_id),
			tier = COALESCE(NULLIF(EXCLUDED.tier,''), archive_exports.tier),
			provider = COALESCE(NULLIF(EXCLUDED.provider,''), archive_exports.provider),
			channel_login = COALESCE(NULLIF(EXCLUDED.channel_login,''), archive_exports.channel_login),
			channel_id = COALESCE(NULLIF(EXCLUDED.channel_id,''), archive_exports.channel_id),
			stream_id = COALESCE(NULLIF(EXCLUDED.stream_id,''), archive_exports.stream_id),
			vod_id = COALESCE(NULLIF(EXCLUDED.vod_id,''), archive_exports.vod_id),
			source_url = COALESCE(NULLIF(EXCLUDED.source_url,''), archive_exports.source_url),
			content_sha256 = COALESCE(NULLIF(EXCLUDED.content_sha256,''), archive_exports.content_sha256),
			uncompressed_size_bytes = COALESCE(NULLIF(EXCLUDED.uncompressed_size_bytes,0), archive_exports.uncompressed_size_bytes),
			failure_reason = COALESCE(NULLIF(EXCLUDED.failure_reason,''), archive_exports.failure_reason),
			metadata = CASE WHEN EXCLUDED.metadata = '{}'::jsonb THEN archive_exports.metadata ELSE EXCLUDED.metadata END,
			updated_at = now()`,
		rec.ArtifactType, rec.NaturalKey, rec.GCSURI, rec.ObjectGeneration, rec.ETag,
		rec.RowCount, rec.ByteSize, rec.Status, rec.Error, exportedAt,
		artifactID, rec.Tier, rec.Provider, rec.ChannelLogin, rec.ChannelID, rec.StreamID, rec.VodID,
		rec.SourceURL, rec.ContentSHA256, rec.UncompressedSizeBytes, rec.FailureReason, metaJSON,
	)
	return err
}

func marshalMetadata(m map[string]any) ([]byte, error) {
	if len(m) == 0 {
		return []byte("{}"), nil
	}
	return json.Marshal(m)
}

func normalizeStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case StatusConfirmed, StatusComplete:
		return StatusConfirmed
	case StatusPartial:
		return StatusPartial
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

// PriorEmoteSnapshotKey returns the latest emote snapshot natural key before currentDate.
func (s *ManifestStore) PriorEmoteSnapshotKey(ctx context.Context, provider, login, currentDate string) (string, error) {
	if s == nil || s.db == nil {
		return "", errors.New("archive manifest: store unavailable")
	}
	pattern := emoteProviderSlug(provider) + ":" + strings.ToLower(strings.TrimSpace(login)) + ":%"
	currentKey := EmoteSnapshotKey(provider, login, currentDate)
	var prior string
	err := s.db.QueryRow(ctx, `
		SELECT natural_key FROM archive_exports
		WHERE artifact_type = $1 AND natural_key LIKE $2 AND natural_key <> $3
		ORDER BY exported_at DESC LIMIT 1`,
		ArtifactEmoteSnapshot, pattern, currentKey,
	).Scan(&prior)
	return prior, err
}
