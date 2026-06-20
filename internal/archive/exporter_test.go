package archive

import (
	"context"
	"testing"
)

func TestSyncExporterExportSync(t *testing.T) {
	blob := newMockBlob()
	manifest := newMockManifest()
	writer := NewWriter(blob, manifest)
	db := &mockAnalyticsDB{
		stream: &StreamExportData{StreamID: "319181844960", Login: "ohnepixel"},
		rollups: []RollupExportLine{
			{ViewerAvg: 50, ViewerMax: 60, ViewerLatest: 55, ViewerSamples: 1},
		},
	}
	exporter := NewSyncExporter(writer, db)
	if err := exporter.ExportSync(context.Background(), "319181844960", "ohnepixel", "sync ok"); err != nil {
		t.Fatal(err)
	}
	if len(manifest.records) == 0 {
		t.Fatal("expected manifest upserts")
	}
}

func TestSyncExporterRequiresConfig(t *testing.T) {
	var exporter *SyncExporter
	if err := exporter.ExportSync(context.Background(), "1", "x", ""); err == nil {
		t.Fatal("expected error for nil exporter")
	}
}
