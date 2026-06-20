package archive

import "testing"

func TestRollupsNaturalKey(t *testing.T) {
	got := rollupsNaturalKey("319181844960")
	want := "rollups:319181844960"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestIncrementalExportWorkerNilSafe(t *testing.T) {
	var worker *IncrementalExportWorker
	exported, failed, err := worker.RunOnce(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if exported != 0 || failed != 0 {
		t.Fatalf("expected zero counts, got exported=%d failed=%d", exported, failed)
	}
}

func TestIncrementalExportWorkerExportStreams(t *testing.T) {
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
	worker := &IncrementalExportWorker{exporter: exporter}
	exported, failed := worker.exportStreams(t.Context(), []pendingStreamExport{
		{StreamID: "319181844960", Login: "ohnepixel"},
	})
	if exported != 1 || failed != 0 {
		t.Fatalf("exported=%d failed=%d", exported, failed)
	}
	if len(manifest.records) == 0 {
		t.Fatal("expected manifest upserts")
	}
}
