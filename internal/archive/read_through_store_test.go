package archive

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type countingBlob struct {
	name  string
	inner *mockBlob
	gets  int
	puts  int
}

func newCountingBlob(name string) *countingBlob {
	return &countingBlob{name: name, inner: newMockBlob()}
}

func (c *countingBlob) Put(ctx context.Context, key string, data []byte, contentType string) (BlobPutResult, error) {
	c.puts++
	return c.inner.Put(ctx, key, data, contentType)
}

func (c *countingBlob) Get(ctx context.Context, key string) ([]byte, error) {
	c.gets++
	raw, err := c.inner.Get(ctx, key)
	if err != nil {
		return nil, err
	}
	return raw, nil
}

func (c *countingBlob) BlobURI(key string) string {
	return "https://example.test/" + c.name + "/" + key
}

type missBlob struct {
	inner BlobStore
}

func (m *missBlob) Put(ctx context.Context, key string, data []byte, contentType string) (BlobPutResult, error) {
	if m.inner == nil {
		return BlobPutResult{}, errors.New("miss blob not configured")
	}
	return m.inner.Put(ctx, key, data, contentType)
}

func (m *missBlob) Get(context.Context, string) ([]byte, error) {
	return nil, ErrNotFound
}

func (m *missBlob) BlobURI(key string) string {
	return "https://example.test/miss/" + key
}

func TestR2DefaultEndpoint(t *testing.T) {
	got := R2DefaultEndpoint("51dd8007b22ac92482388d8b6cdbb6e3")
	want := "https://51dd8007b22ac92482388d8b6cdbb6e3.r2.cloudflarestorage.com"
	if got != want {
		t.Fatalf("endpoint = %q, want %q", got, want)
	}
}

func TestR2BlobStoreFullKeyNormalization(t *testing.T) {
	store := &R2BlobStore{prefix: "archive"}
	key := R2BlobStoreFullKey(store, "/rollups/stream_id=1/part-000.jsonl.gz")
	want := "archive/rollups/stream_id=1/part-000.jsonl.gz"
	if key != want {
		t.Fatalf("full key = %q, want %q", key, want)
	}
}

func TestR2BlobStoreBlobURI(t *testing.T) {
	store := &R2BlobStore{
		bucket:    "streampulse-artifacts-staging",
		prefix:    "archive",
		accountID: "51dd8007b22ac92482388d8b6cdbb6e3",
		endpoint:  R2DefaultEndpoint("51dd8007b22ac92482388d8b6cdbb6e3"),
	}
	uri := store.BlobURI("rollups/stream_id=316787476195/part-000.jsonl.gz")
	if uri == "" || !strings.Contains(uri, "streampulse-artifacts-staging") || !strings.Contains(uri, "archive/rollups") {
		t.Fatalf("unexpected uri: %q", uri)
	}
}

func TestNewR2BlobStoreRequiresCredentials(t *testing.T) {
	_, err := NewR2BlobStore(R2Config{Bucket: "streampulse-artifacts-staging"})
	if err == nil {
		t.Fatal("expected error for missing credentials")
	}
}

func TestReadThroughStoreUsesR2OnHit(t *testing.T) {
	azure := newCountingBlob("azure")
	r2 := newCountingBlob("r2")
	r2.inner.data["rollups/k"] = []byte("from-r2")

	store, err := NewReadThroughStore(azure, r2, ReadThroughOptions{ReadThrough: true})
	if err != nil {
		t.Fatal(err)
	}
	data, err := store.Get(context.Background(), "rollups/k")
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "from-r2" {
		t.Fatalf("data = %q", data)
	}
	if r2.gets != 1 {
		t.Fatalf("r2 gets = %d, want 1", r2.gets)
	}
	if azure.gets != 0 {
		t.Fatalf("azure gets = %d, want 0", azure.gets)
	}
}

func TestReadThroughStoreFallsBackToAzure(t *testing.T) {
	azure := newCountingBlob("azure")
	azure.inner.data["rollups/k"] = []byte("from-azure")
	r2 := &missBlob{}

	store, err := NewReadThroughStore(azure, r2, ReadThroughOptions{ReadThrough: true})
	if err != nil {
		t.Fatal(err)
	}
	data, err := store.Get(context.Background(), "rollups/k")
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "from-azure" {
		t.Fatalf("data = %q", data)
	}
	if azure.gets != 1 {
		t.Fatalf("azure gets = %d, want 1", azure.gets)
	}
}

func TestReadThroughStorePutUsesAzureByDefault(t *testing.T) {
	azure := newCountingBlob("azure")
	r2 := newCountingBlob("r2")
	store, err := NewReadThroughStore(azure, r2, ReadThroughOptions{ReadThrough: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Put(context.Background(), "rollups/k", []byte("x"), "application/gzip"); err != nil {
		t.Fatal(err)
	}
	if azure.puts != 1 {
		t.Fatalf("azure puts = %d, want 1", azure.puts)
	}
	if r2.puts != 0 {
		t.Fatalf("r2 puts = %d, want 0", r2.puts)
	}
	if store.BlobURI("rollups/k") != azure.BlobURI("rollups/k") {
		t.Fatal("expected azure uri for default puts")
	}
}

func TestReadThroughStoreDualWrite(t *testing.T) {
	azure := newCountingBlob("azure")
	r2 := newCountingBlob("r2")
	store, err := NewReadThroughStore(azure, r2, ReadThroughOptions{DualWrite: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Put(context.Background(), "rollups/k", []byte("payload"), "application/gzip"); err != nil {
		t.Fatal(err)
	}
	if azure.puts != 1 || r2.puts != 1 {
		t.Fatalf("puts azure=%d r2=%d, want 1 each", azure.puts, r2.puts)
	}
}

func TestNewBlobStoreRequiresR2WhenReadThrough(t *testing.T) {
	cfg := StoreConfig{
		Azure:       AzureConfig{ConnectionString: "AccountName=test;AccountKey=dGVzdA==;DefaultEndpointsProtocol=https;EndpointSuffix=core.windows.net"},
		ReadThrough: true,
	}
	_, err := NewBlobStore(cfg)
	if err == nil {
		t.Fatal("expected error when read-through enabled without r2 config")
	}
}

func TestNewBlobStoreAzureOnlyPath(t *testing.T) {
	cfg := StoreConfig{
		Azure: AzureConfig{ConnectionString: "AccountName=test;AccountKey=dGVzdA==;DefaultEndpointsProtocol=https;EndpointSuffix=core.windows.net"},
	}
	blob, err := NewBlobStore(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := blob.(*AzureBlobStore); !ok {
		t.Fatalf("expected *AzureBlobStore, got %T", blob)
	}
}

func TestStoreConfigNeedsR2Flags(t *testing.T) {
	cases := []struct {
		name string
		cfg  StoreConfig
		want bool
	}{
		{"default", StoreConfig{}, false},
		{"read-through", StoreConfig{ReadThrough: true}, true},
		{"dual-write", StoreConfig{DualWrite: true}, true},
		{"primary-r2", StoreConfig{PrimaryProvider: "r2"}, true},
	}
	for _, tc := range cases {
		if got := tc.cfg.needsR2(); got != tc.want {
			t.Fatalf("%s: needsR2 = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestNormalizePrimaryProvider(t *testing.T) {
	if got := NormalizePrimaryProvider("R2"); got != "r2" {
		t.Fatalf("got %q", got)
	}
	if got := NormalizePrimaryProvider(""); got != "azure" {
		t.Fatalf("got %q", got)
	}
}

// Ensure mockBlob still satisfies BlobStore.
var _ BlobStore = (*mockBlob)(nil)
