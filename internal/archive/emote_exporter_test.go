package archive

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

type stubEmoteDB struct {
	lines []EmoteSnapshotLine
	setID string
	hash  string
	count int
}

func (s *stubEmoteDB) ListProviderEmotes(_ context.Context, _, _ string) ([]EmoteSnapshotLine, error) {
	return s.lines, nil
}

func (s *stubEmoteDB) ProviderSetSnapshot(_ context.Context, _, _ string) (string, string, int, bool, error) {
	return s.setID, s.hash, s.count, s.count > 0, nil
}

func TestEmoteExporterExportSnapshot(t *testing.T) {
	blob := newMockBlob()
	manifest := newMockManifest()
	writer := NewWriter(blob, manifest)
	db := &stubEmoteDB{
		lines: []EmoteSnapshotLine{{EmoteID: "e1", Name: "Pepe", ProviderEmoteID: "p1"}},
		setID: "set-1",
		hash:  "abc123",
		count: 1,
	}
	exporter := NewEmoteExporter(writer, db)
	if err := exporter.ExportSnapshot(context.Background(), "seventv", "testchannel", EmoteSnapshotStrategyWeekly); err != nil {
		t.Fatal(err)
	}
	if len(manifest.records) == 0 {
		t.Fatal("expected manifest row for emote snapshot")
	}
}

func TestEventAPIChangelogAdapter(t *testing.T) {
	blob := newMockBlob()
	manifest := newMockManifest()
	writer := NewWriter(blob, manifest)
	exporter := NewEmoteExporter(writer, &stubEmoteDB{})
	adapter := &EventAPIChangelogAdapter{Exporter: exporter}
	raw, _ := json.Marshal(map[string]string{"login": "x"})
	if err := adapter.RecordSetUpdate(context.Background(), "x", "set-1", raw); err != nil {
		t.Fatal(err)
	}
	if len(manifest.records) == 0 {
		t.Fatal("expected changelog manifest row")
	}
}

func TestBuildVODChatProvenance(t *testing.T) {
	exporter := NewEmoteExporter(newMockBlobWriter(), &stubEmoteDB{setID: "s", hash: "h", count: 2})
	prov := exporter.BuildVODChatProvenance(context.Background(), "123", "testchannel")
	if prov.StreamID != "123" || prov.EmoteSnapshotStrategy == "" {
		t.Fatalf("unexpected provenance: %+v", prov)
	}
	if prov.ExportedAt.IsZero() {
		t.Fatal("expected exportedAt")
	}
	if time.Since(prov.ExportedAt) > time.Minute {
		t.Fatal("exportedAt should be recent")
	}
}

func newMockBlobWriter() *Writer {
	return NewWriter(newMockBlob(), newMockManifest())
}
