package archive

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// BronzeCatalogLine is one VOD catalog JSONL row (vod_catalog/v1).
type BronzeCatalogLine struct {
	SchemaVersion string          `json:"schemaVersion"`
	Login         string          `json:"login"`
	ChannelID     string          `json:"channelId,omitempty"`
	VodID         string          `json:"vodId"`
	Title         string          `json:"title,omitempty"`
	CreatedAt     time.Time       `json:"createdAt"`
	Duration      string          `json:"duration,omitempty"`
	Availability  string          `json:"availability,omitempty"`
	RawHelix      json.RawMessage `json:"rawHelix,omitempty"`
	ExportedAt    time.Time       `json:"exportedAt"`
}

// ChannelIdentityBlob is Helix + provider identity snapshot.
type ChannelIdentityBlob struct {
	SchemaVersion string          `json:"schemaVersion"`
	Login         string          `json:"login"`
	ChannelID     string          `json:"channelId"`
	DisplayName   string          `json:"displayName,omitempty"`
	RawHelix      json.RawMessage `json:"rawHelix,omitempty"`
	SevenTVUserID string          `json:"sevenTvUserId,omitempty"`
	ExportedAt    time.Time       `json:"exportedAt"`
}

// ProviderCrosswalkBlob maps Twitch login to provider ids.
type ProviderCrosswalkBlob struct {
	SchemaVersion string            `json:"schemaVersion"`
	Login         string            `json:"login"`
	ChannelID     string            `json:"channelId,omitempty"`
	Providers     map[string]string `json:"providers"`
	ExportedAt    time.Time         `json:"exportedAt"`
}

// BronzeExporter writes bronze-tier corpus blobs.
type BronzeExporter struct {
	writer *Writer
}

func NewBronzeExporter(writer *Writer) *BronzeExporter {
	return &BronzeExporter{writer: writer}
}

func (b *BronzeExporter) ExportVODCatalog(ctx context.Context, login, channelID string, lines []BronzeCatalogLine) error {
	if b == nil || b.writer == nil {
		return fmt.Errorf("bronze exporter not configured")
	}
	login = strings.ToLower(strings.TrimSpace(login))
	date := time.Now().UTC().Format("2006-01-02")
	var buf strings.Builder
	for _, line := range lines {
		line.SchemaVersion = "vod_catalog/v1"
		line.Login = login
		line.ExportedAt = time.Now().UTC()
		raw, err := json.Marshal(line)
		if err != nil {
			return err
		}
		buf.Write(raw)
		buf.WriteByte('\n')
	}
	res, err := b.writer.putGzip(ctx, BronzeVODCatalogBlobKey(login, date), []byte(buf.String()))
	if err != nil {
		return err
	}
	res.RowCount = int64(len(lines))
	catalogKey := BronzeVODCatalogKey(login, date)
	rec := ExportRecord{
		ArtifactType: ArtifactBronzeVODCatalog,
		NaturalKey:   catalogKey,
		GCSURI:       res.URI,
		ETag:         res.ETag,
		RowCount:     res.RowCount,
		ByteSize:     res.ByteSize,
		Status:       StatusConfirmed,
		ExportedAt:   time.Now().UTC(),
		Tier:         "bronze",
		ChannelLogin: login,
		ChannelID:    channelID,
		ContentSHA256: res.ContentSHA256,
		UncompressedSizeBytes: res.UncompressedSizeBytes,
	}
	if b.writer.manifest != nil {
		if err := b.writer.manifest.Upsert(ctx, rec); err != nil {
			return err
		}
	}
	// Dual-write legacy key one release.
	legacyRes := res
	if err := b.writer.confirmManifest(ctx, ArtifactBronzeVODIndex, LegacyVODIndexKey(login), legacyRes); err != nil {
		return err
	}
	return nil
}

func (b *BronzeExporter) ExportIdentity(ctx context.Context, identity ChannelIdentityBlob) error {
	if b == nil || b.writer == nil {
		return fmt.Errorf("bronze exporter not configured")
	}
	identity.SchemaVersion = "channel_identity/v1"
	identity.Login = strings.ToLower(strings.TrimSpace(identity.Login))
	date := time.Now().UTC().Format("2006-01-02")
	identity.ExportedAt = time.Now().UTC()
	raw, err := json.Marshal(identity)
	if err != nil {
		return err
	}
	res, err := b.writer.putGzip(ctx, ChannelIdentityBlobKey(identity.Login, date), raw)
	if err != nil {
		return err
	}
	res.RowCount = 1
	rec := ExportRecord{
		ArtifactType: ArtifactChannelIdentity,
		NaturalKey:   ChannelIdentityKey(identity.ChannelID, date),
		GCSURI:       res.URI,
		ETag:         res.ETag,
		RowCount:     1,
		ByteSize:     res.ByteSize,
		Status:       StatusConfirmed,
		ExportedAt:   time.Now().UTC(),
		Tier:         "bronze",
		ChannelLogin: identity.Login,
		ChannelID:    identity.ChannelID,
		ContentSHA256: res.ContentSHA256,
		UncompressedSizeBytes: res.UncompressedSizeBytes,
	}
	if b.writer.manifest == nil {
		return nil
	}
	return b.writer.manifest.Upsert(ctx, rec)
}

func (b *BronzeExporter) ExportCrosswalk(ctx context.Context, crosswalk ProviderCrosswalkBlob) error {
	if b == nil || b.writer == nil {
		return fmt.Errorf("bronze exporter not configured")
	}
	crosswalk.SchemaVersion = "provider_crosswalk/v1"
	crosswalk.Login = strings.ToLower(strings.TrimSpace(crosswalk.Login))
	date := time.Now().UTC().Format("2006-01-02")
	crosswalk.ExportedAt = time.Now().UTC()
	raw, err := json.Marshal(crosswalk)
	if err != nil {
		return err
	}
	res, err := b.writer.putGzip(ctx, ProviderCrosswalkBlobKey(crosswalk.Login, date), raw)
	if err != nil {
		return err
	}
	res.RowCount = 1
	rec := ExportRecord{
		ArtifactType: ArtifactProviderCrosswalk,
		NaturalKey:   ProviderCrosswalkKey(crosswalk.Login, date),
		GCSURI:       res.URI,
		ETag:         res.ETag,
		RowCount:     1,
		ByteSize:     res.ByteSize,
		Status:       StatusConfirmed,
		ExportedAt:   time.Now().UTC(),
		Tier:         "bronze",
		ChannelLogin: crosswalk.Login,
		ChannelID:    crosswalk.ChannelID,
		ContentSHA256: res.ContentSHA256,
		UncompressedSizeBytes: res.UncompressedSizeBytes,
	}
	if b.writer.manifest == nil {
		return nil
	}
	return b.writer.manifest.Upsert(ctx, rec)
}
