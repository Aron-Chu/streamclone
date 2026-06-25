package archive

import (
	"fmt"
	"strings"
	"time"
)

// Natural key helpers — see docs/scraping-archive/artifact-natural-keys.md.

func BronzeVODCatalogKey(login, date string) string {
	login = strings.ToLower(strings.TrimSpace(login))
	date = strings.TrimSpace(date)
	return fmt.Sprintf("%s:%s", login, date)
}

func ChannelIdentityKey(channelID, date string) string {
	channelID = strings.TrimSpace(channelID)
	date = strings.TrimSpace(date)
	return fmt.Sprintf("%s:%s", channelID, date)
}

func ProviderCrosswalkKey(login, date string) string {
	login = strings.ToLower(strings.TrimSpace(login))
	date = strings.TrimSpace(date)
	return fmt.Sprintf("%s:%s", login, date)
}

func BronzeRosterKey(date string) string {
	return fmt.Sprintf("roster:%s", strings.TrimSpace(date))
}

// LegacyVODIndexKey is the pre-v2 natural key for bronze VOD index blobs.
func LegacyVODIndexKey(login string) string {
	return fmt.Sprintf("vod_index:%s", strings.ToLower(strings.TrimSpace(login)))
}

func ViewerRollupKey(streamID string) string {
	streamID = strings.TrimSpace(streamID)
	return fmt.Sprintf("%s:twitchtracker", streamID)
}

// LegacyRollupsKey is the pre-canonical rollup natural key.
func LegacyRollupsKey(streamID string) string {
	return fmt.Sprintf("rollups:%s", strings.TrimSpace(streamID))
}

func TTDetailKey(streamID string) string {
	streamID = strings.TrimSpace(streamID)
	return fmt.Sprintf("%s:twitchtracker", streamID)
}

func TTChartJSONKey(streamID string, fetchedAt time.Time) string {
	streamID = strings.TrimSpace(streamID)
	return fmt.Sprintf("%s:%d", streamID, fetchedAt.UTC().Unix())
}

func EmoteSnapshotKey(provider, login, date string) string {
	provider = emoteProviderSlug(provider)
	login = strings.ToLower(strings.TrimSpace(login))
	date = strings.TrimSpace(date)
	return fmt.Sprintf("%s:%s:%s", provider, login, date)
}

func EmoteSnapshotGlobalKey(date string) string {
	return fmt.Sprintf("7tv:global:%s", strings.TrimSpace(date))
}

func EmoteChangelogKey(provider, login, eventType string, recordedAt time.Time) string {
	provider = emoteProviderSlug(provider)
	login = strings.ToLower(strings.TrimSpace(login))
	eventType = strings.TrimSpace(eventType)
	return fmt.Sprintf("%s:%s:%s:%d", provider, login, eventType, recordedAt.UTC().Unix())
}

func GoldLiteKey(streamID string) string {
	return strings.TrimSpace(streamID)
}

func GoldFullPartKey(streamID string, partNo int) string {
	return fmt.Sprintf("%s:part:%d", strings.TrimSpace(streamID), partNo)
}

func PulseWireRawKey(source, date string, partNo int) string {
	return fmt.Sprintf("%s:%s:%d", strings.TrimSpace(source), strings.TrimSpace(date), partNo)
}

func emoteProviderSlug(provider string) string {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "seventv", "7tv":
		return "7tv"
	case "ffz":
		return "ffz"
	case "bttv":
		return "bttv"
	default:
		return strings.ToLower(strings.TrimSpace(provider))
	}
}

// MapLegacyNaturalKey returns canonical artifact type and key when input uses legacy patterns.
func MapLegacyNaturalKey(artifactType, naturalKey string) (string, string) {
	artifactType = strings.TrimSpace(artifactType)
	naturalKey = strings.TrimSpace(naturalKey)
	if strings.HasPrefix(naturalKey, "vod_index:") && artifactType == ArtifactBronzeVODIndex {
		login := strings.TrimPrefix(naturalKey, "vod_index:")
		date := time.Now().UTC().Format("2006-01-02")
		return "bronze_vod_catalog", BronzeVODCatalogKey(login, date)
	}
	if strings.HasPrefix(naturalKey, "rollups:") && artifactType == ArtifactAnalyticsRollups {
		streamID := strings.TrimPrefix(naturalKey, "rollups:")
		return ArtifactAnalyticsRollups, ViewerRollupKey(streamID)
	}
	return artifactType, naturalKey
}
