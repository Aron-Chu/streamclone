package analytics

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"streamclone/internal/analytics/heatmap"
	"streamclone/internal/archive"
)

func TestBuildStoredArtifactsSummary(t *testing.T) {
	now := time.Now().UTC()
	rows := []archive.StreamExportRow{
		{
			ArtifactType:  archive.ArtifactAnalyticsRollups,
			NaturalKey:    "123:twitchtracker",
			ExportStatus:  archive.StatusConfirmed,
			Provider:      "azure",
			ByteSize:      4096,
			ContentSHA256: "abcdef0123456789",
			UpdatedAt:     now,
		},
		{
			ArtifactType: archive.ArtifactVODChatMessage,
			NaturalKey:   "vod_chat:123",
			ExportStatus: archive.StatusPartial,
			Provider:     "azure",
			ByteSize:     8192,
			UpdatedAt:    now,
		},
	}
	got := buildStoredArtifactsSummary(rows)
	if got.MinuteSeries.State != storedArtifactReady {
		t.Fatalf("minuteSeries state=%q want ready", got.MinuteSeries.State)
	}
	if !got.MinuteSeries.CanRestore {
		t.Fatal("expected minute series CanRestore")
	}
	if got.ChatExport.State != storedArtifactPartial {
		t.Fatalf("chatExport state=%q want partial", got.ChatExport.State)
	}
	if !got.ChatExport.CanBackfill {
		t.Fatal("expected chat export CanBackfill")
	}
	if got.MinuteSeries.Provider != "azure" {
		t.Fatalf("provider=%q", got.MinuteSeries.Provider)
	}
	if got.MinuteSeries.Checksum != "abcdef0123456789" {
		t.Fatalf("checksum=%q", got.MinuteSeries.Checksum)
	}
}

func TestStoredArtifactsJSONOmitsForbiddenPortalKeys(t *testing.T) {
	summary := buildStoredArtifactsSummary([]archive.StreamExportRow{
		{ArtifactType: archive.ArtifactAnalyticsRollups, ExportStatus: archive.StatusConfirmed, Provider: "azure"},
		{ArtifactType: archive.ArtifactVODChatMessage, ExportStatus: archive.StatusConfirmed, Provider: "azure"},
	})
	detail := PortalStreamDetail{
		Channel:          "xqc",
		State:            "historical",
		StoredArtifacts:  &summary,
		DataSourceBadges: portalBadgesFromStored(summary, []SourceStatus{{Source: "analytics_db", State: "ready"}}),
	}
	body, err := json.Marshal(detail)
	if err != nil {
		t.Fatal(err)
	}
	raw := strings.ToLower(string(body))
	for _, forbidden := range []string{"rollups", "messages", "operator", "gql", "corpus", "archive", "gcs_uri", "blob.core"} {
		if strings.Contains(raw, forbidden) {
			t.Fatalf("stored artifacts payload must not contain %q", forbidden)
		}
	}
}

func TestPortalBadgesFromStoredDeduplicatesMergedSources(t *testing.T) {
	summary := buildStoredArtifactsSummary([]archive.StreamExportRow{
		{ArtifactType: archive.ArtifactAnalyticsRollups, ExportStatus: archive.StatusConfirmed, Provider: "azure"},
		{ArtifactType: archive.ArtifactAnalyticsStream, ExportStatus: archive.StatusConfirmed, Provider: "azure"},
	})
	sources := mergeStoredSources([]SourceStatus{{Source: "analytics_db", State: "ready"}}, summary)
	badges := portalBadgesFromStored(summary, sources)

	seen := make(map[string]bool, len(badges))
	for _, badge := range badges {
		if seen[badge.Source] {
			t.Fatalf("duplicate badge source %q in %+v", badge.Source, badges)
		}
		seen[badge.Source] = true
	}
	if !seen["stored_minute_series"] || !seen["stored_session"] {
		t.Fatalf("expected stored badges in %+v", badges)
	}
}

func TestEnrichCoverageWithStoredArtifacts(t *testing.T) {
	stored := buildStoredArtifactsSummary([]archive.StreamExportRow{
		{ArtifactType: archive.ArtifactVODChatMessage, ExportStatus: archive.StatusConfirmed, Provider: "azure"},
	})
	base := ExtensionCoverage{
		State:                      CoverageStateWaitingForVOD,
		CoverageStartOffsetSeconds: 600,
		HasGaps:                    true,
		Message:                    "VOD chat not available yet — archive publishes after the stream ends",
	}
	got := enrichCoverageWithStoredArtifacts(base, stored, "999", false)
	if !got.CanBackfill {
		t.Fatal("expected canBackfill when export ready and vod linked")
	}
	if got.BackfillReason != "stored_export_ready" {
		t.Fatalf("backfillReason=%q", got.BackfillReason)
	}
	if got.VODStatus != "available" {
		t.Fatalf("vodStatus=%q want available when vod linked", got.VODStatus)
	}

	waiting := enrichCoverageWithStoredArtifacts(base, stored, "", false)
	if waiting.VODStatus != "export_ready" {
		t.Fatalf("vodStatus=%q want export_ready without vod link", waiting.VODStatus)
	}
}

func TestStoredChatExportGatesLateAttachBackfillUntilVodLinked(t *testing.T) {
	streamStart := time.Date(2026, 6, 22, 19, 0, 0, 0, time.UTC)
	base := computePulseCoverage([]heatmap.MinuteRollup{
		{MinuteTS: streamStart.Add(120 * time.Minute), ChatCount: 3},
	}, streamStart, 7200, true, "", false, false)
	if base.State != CoverageStateWaitingForVOD || !base.HasGaps || len(base.MissingRanges) == 0 {
		t.Fatalf("late attach coverage = %+v, want waiting state with missing prefix", base)
	}

	stored := buildStoredArtifactsSummary([]archive.StreamExportRow{
		{ArtifactType: archive.ArtifactVODChatMessage, ExportStatus: archive.StatusConfirmed, Provider: "azure"},
	})
	live := enrichCoverageWithStoredArtifacts(base, stored, "", true)
	if live.CanBackfill {
		t.Fatalf("live export coverage = %+v, want backfill gated until VOD id is linked", live)
	}
	if live.VODStatus != "export_ready" {
		t.Fatalf("live VODStatus = %q, want export_ready", live.VODStatus)
	}

	linked := enrichCoverageWithStoredArtifacts(base, stored, "999", false)
	if !linked.CanBackfill || linked.BackfillReason != "stored_export_ready" {
		t.Fatalf("linked export coverage = %+v, want stored export backfill enabled", linked)
	}
	if linked.VODStatus != "available" {
		t.Fatalf("linked VODStatus = %q, want available", linked.VODStatus)
	}
}

func TestSanitizeStorageProvider(t *testing.T) {
	if got := sanitizeStorageProvider("https://account.blob.core.windows.net/x"); got != "azure" {
		t.Fatalf("azure provider=%q", got)
	}
	if got := sanitizeStorageProvider("r2"); got != "r2" {
		t.Fatalf("r2 provider=%q", got)
	}
}
