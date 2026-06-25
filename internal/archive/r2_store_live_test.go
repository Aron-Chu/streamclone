package archive

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"strings"
	"testing"
)

func TestR2BlobStoreLiveSampleRead(t *testing.T) {
	if strings.TrimSpace(os.Getenv("ARCHIVE_R2_LIVE_TEST")) != "1" {
		t.Skip("set ARCHIVE_R2_LIVE_TEST=1 to run live R2 read smoke")
	}
	cfg := R2Config{
		AccountID:           strings.TrimSpace(os.Getenv("ARCHIVE_R2_ACCOUNT_ID")),
		Bucket:              envOr("ARCHIVE_R2_BUCKET", "streampulse-artifacts-staging"),
		Prefix:              envOr("ARCHIVE_R2_PREFIX", "archive"),
		Endpoint:            strings.TrimSpace(os.Getenv("ARCHIVE_R2_ENDPOINT")),
		AccessKeyIDFile:     strings.TrimSpace(os.Getenv("ARCHIVE_R2_ACCESS_KEY_ID_FILE")),
		SecretAccessKeyFile: strings.TrimSpace(os.Getenv("ARCHIVE_R2_SECRET_ACCESS_KEY_FILE")),
	}
	if cfg.AccountID == "" {
		cfg.AccountID = "51dd8007b22ac92482388d8b6cdbb6e3"
	}
	store, err := NewR2BlobStore(cfg)
	if err != nil {
		t.Fatal(err)
	}
	key := "rollups/stream_id=316787476195/part-000.jsonl.gz"
	data, err := store.Get(context.Background(), key)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) != 131 {
		t.Fatalf("byte size = %d, want 131", len(data))
	}
	sum := sha256.Sum256(data)
	got := hex.EncodeToString(sum[:])
	want := "8c0fd0d6a814325beb752a0a5caa20905cedf5f8078c20df9d2e2c83b1519056"
	if got != want {
		t.Fatalf("sha256 = %s, want %s", got, want)
	}
	if err := func() error {
		_, err := Gunzip(data)
		return err
	}(); err != nil {
		t.Fatalf("gzip: %v", err)
	}
}

func envOr(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}
