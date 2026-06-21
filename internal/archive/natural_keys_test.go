package archive_test

import (
	"fmt"
	"testing"
	"time"

	"streamclone/internal/archive"
)

func TestBronzeVODCatalogKey(t *testing.T) {
	got := archive.BronzeVODCatalogKey("Ninja", "2026-06-20")
	if got != "ninja:2026-06-20" {
		t.Fatalf("got %q", got)
	}
}

func TestViewerRollupKey(t *testing.T) {
	got := archive.ViewerRollupKey("318832886110")
	if got != "318832886110:twitchtracker" {
		t.Fatalf("got %q", got)
	}
}

func TestLegacyRollupsMapping(t *testing.T) {
	typ, key := archive.MapLegacyNaturalKey(archive.ArtifactAnalyticsRollups, "rollups:abc")
	if typ != archive.ArtifactAnalyticsRollups || key != "abc:twitchtracker" {
		t.Fatalf("got %s %s", typ, key)
	}
}

func TestLegacyVODIndexMapping(t *testing.T) {
	typ, key := archive.MapLegacyNaturalKey(archive.ArtifactBronzeVODIndex, "vod_index:ninja")
	if typ != "bronze_vod_catalog" {
		t.Fatalf("type %s", typ)
	}
	if !stringsHasPrefix(key, "ninja:") {
		t.Fatalf("key %s", key)
	}
}

func TestEmoteSnapshotGlobalKey(t *testing.T) {
	got := archive.EmoteSnapshotGlobalKey("2026-06-20")
	if got != "7tv:global:2026-06-20" {
		t.Fatalf("got %q", got)
	}
}

func TestTTChartJSONKey(t *testing.T) {
	ts := time.Date(2026, 6, 20, 12, 0, 0, 0, time.UTC)
	got := archive.TTChartJSONKey("123", ts)
	want := fmt.Sprintf("123:%d", ts.UTC().Unix())
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func stringsHasPrefix(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}
