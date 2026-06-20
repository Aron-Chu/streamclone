package archive

import (
	"context"
	"sync"
	"testing"
)

type mockBlob struct {
	mu   sync.Mutex
	data map[string][]byte
}

func newMockBlob() *mockBlob {
	return &mockBlob{data: map[string][]byte{}}
}

func (m *mockBlob) Put(_ context.Context, key string, data []byte, _ string) (BlobPutResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.data[key] = append([]byte(nil), data...)
	return BlobPutResult{
		URI:      "https://ststreamclone3lf6tt.blob.core.windows.net/streamclone-archive/streamclone/" + key,
		ETag:     "etag-1",
		ByteSize: int64(len(data)),
	}, nil
}

func (m *mockBlob) Get(_ context.Context, key string) ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	raw, ok := m.data[key]
	if !ok {
		return nil, context.Canceled
	}
	return append([]byte(nil), raw...), nil
}

func (m *mockBlob) BlobURI(key string) string {
	return "https://ststreamclone3lf6tt.blob.core.windows.net/streamclone-archive/streamclone/" + key
}

type mockAnalyticsDB struct {
	stream  *StreamExportData
	rollups []RollupExportLine
}

func (m *mockAnalyticsDB) ExportStreamRow(_ context.Context, streamID string) (*StreamExportData, error) {
	if m.stream == nil {
		return &StreamExportData{StreamID: streamID}, nil
	}
	return m.stream, nil
}

func (m *mockAnalyticsDB) ExportRollups(_ context.Context, _ string) ([]RollupExportLine, error) {
	return m.rollups, nil
}

func TestGzipRoundTrip(t *testing.T) {
	raw := []byte(`{"minuteTs":"2026-06-20T12:00:00Z","viewerAvg":100}`)
	gz, err := Gzip(raw)
	if err != nil {
		t.Fatal(err)
	}
	out, err := Gunzip(gz)
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != string(raw) {
		t.Fatalf("round trip mismatch: %q", out)
	}
}

func TestWriterExportStream(t *testing.T) {
	blob := newMockBlob()
	manifest := newMockManifest()
	writer := NewWriter(blob, manifest)
	db := &mockAnalyticsDB{
		stream: &StreamExportData{StreamID: "317014684259", Login: "ohnepixel"},
		rollups: []RollupExportLine{
			{ViewerAvg: 100, ViewerMax: 100, ViewerLatest: 100, ViewerSamples: 1},
		},
	}
	if err := writer.ExportStream(context.Background(), "317014684259", db); err != nil {
		t.Fatal(err)
	}
	if _, ok := blob.data[RollupsBlobKey("317014684259")]; !ok {
		t.Fatal("expected rollups blob key")
	}
	if _, ok := blob.data[StreamSessionBlobKey("317014684259")]; !ok {
		t.Fatal("expected session blob key")
	}
	if _, ok := blob.data[StreamChannelBlobKey("ohnepixel", "317014684259")]; !ok {
		t.Fatal("expected channel index blob key")
	}
	if len(manifest.records) < 3 {
		t.Fatalf("expected at least 3 manifest upserts, got %d", len(manifest.records))
	}
	for _, rec := range manifest.records {
		if rec.Status != StatusConfirmed {
			t.Fatalf("manifest status = %q, want confirmed", rec.Status)
		}
		if rec.GCSURI == "" {
			t.Fatal("manifest gcs_uri is empty")
		}
	}
}

func TestWriterExportTTDetail(t *testing.T) {
	blob := newMockBlob()
	manifest := newMockManifest()
	writer := NewWriter(blob, manifest)
	html := []byte("<html><meta id='ecs' /></html>")
	if err := writer.ExportTTDetail(context.Background(), "ohnepixel", "317014684259", html); err != nil {
		t.Fatal(err)
	}
	key := TTDetailBlobKey("ohnepixel", "317014684259")
	if _, ok := blob.data[key]; !ok {
		t.Fatalf("expected tt-detail blob at %q", key)
	}
	if len(manifest.records) != 1 {
		t.Fatalf("expected 1 manifest upsert, got %d", len(manifest.records))
	}
	if manifest.records[0].ArtifactType != ArtifactTTDetailHTML {
		t.Fatalf("artifact type = %q", manifest.records[0].ArtifactType)
	}
}

func TestWriterExportTTDetailSkipsEmptyHTML(t *testing.T) {
	blob := newMockBlob()
	writer := NewWriter(blob, nil)
	if err := writer.ExportTTDetail(context.Background(), "ohnepixel", "317014684259", nil); err != nil {
		t.Fatal(err)
	}
	if len(blob.data) != 0 {
		t.Fatal("expected no blobs for empty html")
	}
}

type mockManifest struct {
	records []ExportRecord
}

func newMockManifest() *mockManifest {
	return &mockManifest{}
}

func (m *mockManifest) Upsert(_ context.Context, rec ExportRecord) error {
	m.records = append(m.records, rec)
	return nil
}

func TestLoadConnectionStringFromValue(t *testing.T) {
	cs, err := LoadConnectionString(AzureConfig{ConnectionString: "AccountName=ststreamclone3lf6tt;..."})
	if err != nil {
		t.Fatal(err)
	}
	if cs == "" {
		t.Fatal("expected connection string")
	}
}

func TestRollupsBlobKeyLayout(t *testing.T) {
	key := RollupsBlobKey("319181844960")
	want := "rollups/stream_id=319181844960/part-000.jsonl.gz"
	if key != want {
		t.Fatalf("key = %q, want %q", key, want)
	}
}

func TestTTDetailBlobKeyLayout(t *testing.T) {
	key := TTDetailBlobKey("ohnepixel", "319181844960")
	want := "tt-detail/ohnepixel/319181844960/page.html.gz"
	if key != want {
		t.Fatalf("key = %q, want %q", key, want)
	}
}

func TestBronzeBlobKeyLayout(t *testing.T) {
	if got := Top500BlobKey(); got != "channels/top500.json.gz" {
		t.Fatalf("top500 key = %q", got)
	}
	if got := ChannelSummaryBlobKey("OhnePixel"); got != "channels/summary/ohnepixel.json" {
		t.Fatalf("summary key = %q", got)
	}
	if got := VODIndexBlobKey("xqc"); got != "channels/vod_index/xqc.jsonl.gz" {
		t.Fatalf("vod index key = %q", got)
	}
	if got := VODChatBlobKey("319181844960"); got != "vod_chat/stream_id=319181844960/messages.jsonl.gz" {
		t.Fatalf("vod chat key = %q", got)
	}
}

func TestWriterBronzeExports(t *testing.T) {
	blob := newMockBlob()
	manifest := newMockManifest()
	writer := NewWriter(blob, manifest)

	topPayload := []byte(`{"topN":500,"logins":["a"]}`)
	if err := writer.ExportTop500(context.Background(), topPayload); err != nil {
		t.Fatal(err)
	}
	if _, ok := blob.data[Top500BlobKey()]; !ok {
		t.Fatal("expected top500 blob")
	}

	summary := []byte(`{"rank":1}`)
	if err := writer.ExportChannelSummary(context.Background(), "ohnepixel", summary); err != nil {
		t.Fatal(err)
	}
	if _, ok := blob.data[ChannelSummaryBlobKey("ohnepixel")]; !ok {
		t.Fatal("expected summary blob")
	}

	vodLines := []byte(`{"streamId":"1"}` + "\n")
	if err := writer.ExportVODIndex(context.Background(), "ohnepixel", vodLines); err != nil {
		t.Fatal(err)
	}
	if _, ok := blob.data[VODIndexBlobKey("ohnepixel")]; !ok {
		t.Fatal("expected vod index blob")
	}
	if len(manifest.records) < 3 {
		t.Fatalf("expected manifest upserts, got %d", len(manifest.records))
	}
}

type mockVODChatDB struct {
	messages []VODChatExportLine
}

func (m *mockVODChatDB) ExportVODChatMessages(_ context.Context, streamID string) ([]VODChatExportLine, error) {
	if len(m.messages) == 0 {
		return []VODChatExportLine{
			{
				ID:            1,
				StreamID:      streamID,
				MessageID:     "m1",
				DisplayName:   "viewer",
				SenderHash:    "hash",
				Text:          "hello",
				OffsetSeconds: 10,
			},
		}, nil
	}
	return m.messages, nil
}

func TestWriterExportVODChat(t *testing.T) {
	blob := newMockBlob()
	manifest := newMockManifest()
	writer := NewWriter(blob, manifest)
	chatDB := &mockVODChatDB{}
	if err := writer.ExportVODChat(context.Background(), "319181844960", chatDB); err != nil {
		t.Fatal(err)
	}
	key := VODChatBlobKey("319181844960")
	if _, ok := blob.data[key]; !ok {
		t.Fatalf("expected vod chat blob at %q", key)
	}
	if len(manifest.records) != 1 {
		t.Fatalf("expected 1 manifest upsert, got %d", len(manifest.records))
	}
	rec := manifest.records[0]
	if rec.ArtifactType != ArtifactVODChatMessage {
		t.Fatalf("artifact type = %q", rec.ArtifactType)
	}
	if rec.NaturalKey != "vod_chat:319181844960" {
		t.Fatalf("natural key = %q", rec.NaturalKey)
	}
	if rec.RowCount != 1 {
		t.Fatalf("row count = %d", rec.RowCount)
	}
}

func TestVODChatBlobKeyLayout(t *testing.T) {
	key := VODChatBlobKey("319181844960")
	want := "vod_chat/stream_id=319181844960/messages.jsonl.gz"
	if key != want {
		t.Fatalf("key = %q, want %q", key, want)
	}
}
