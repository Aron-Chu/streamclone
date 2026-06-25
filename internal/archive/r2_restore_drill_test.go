package archive

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"strings"
	"testing"
)

type drillSample struct {
	name   string
	key    string
	sha256 string
	minLen int
}

var phase2bDrillSamples = []drillSample{
	{
		name:   "analytics_rollups",
		key:    "rollups/stream_id=316787476195/part-000.jsonl.gz",
		sha256: "8c0fd0d6a814325beb752a0a5caa20905cedf5f8078c20df9d2e2c83b1519056",
		minLen: 1,
	},
	{
		name:   "analytics_stream",
		key:    "streams/stream_id=316070541810/session.json.gz",
		sha256: "179338198c3570a56e250f6a4ec27816f85e73f40650471f2b8575a89cea677b",
		minLen: 1,
	},
	{
		name:   "bronze_vod_catalog",
		key:    "channels/vod_index/gorizontradio.jsonl.gz",
		sha256: "30e6fa98fb48c2b132824d1ac5e2243c0be9e9082ff32598d34d7687ca7f6c7f",
		minLen: 1,
	},
}

func TestR2RestoreDrillLive(t *testing.T) {
	if strings.TrimSpace(os.Getenv("ARCHIVE_R2_LIVE_TEST")) != "1" {
		t.Skip("set ARCHIVE_R2_LIVE_TEST=1 to run STOR-R2-004 restore drill")
	}
	cfg := liveDrillStoreConfig(t)
	ctx := context.Background()

	r2, err := NewR2BlobStore(cfg.R2)
	if err != nil {
		t.Fatalf("r2 init: %v", err)
	}
	azure, err := NewAzureBlobStore(cfg.Azure)
	if err != nil {
		t.Fatalf("azure init: %v", err)
	}
	readThrough, err := NewBlobStore(StoreConfig{
		Azure:           cfg.Azure,
		R2:              cfg.R2,
		PrimaryProvider: "azure",
		ReadThrough:     true,
		DualWrite:       false,
	})
	if err != nil {
		t.Fatalf("read-through store: %v", err)
	}
	if _, ok := readThrough.(*ReadThroughStore); !ok {
		t.Fatalf("expected *ReadThroughStore, got %T", readThrough)
	}

	for _, sample := range phase2bDrillSamples {
		t.Run("direct_r2_"+sample.name, func(t *testing.T) {
			raw, err := r2.Get(ctx, sample.key)
			if err != nil {
				t.Fatal(err)
			}
			assertDrillBlob(t, sample, raw)
			assertDrillPayloadShape(t, sample.name, raw)
		})
		t.Run("read_through_r2_hit_"+sample.name, func(t *testing.T) {
			raw, err := readThrough.Get(ctx, sample.key)
			if err != nil {
				t.Fatal(err)
			}
			assertDrillBlob(t, sample, raw)
		})
	}

	fallbackKey := strings.TrimSpace(os.Getenv("ARCHIVE_DRILL_AZURE_FALLBACK_KEY"))
	if fallbackKey == "" {
		t.Skip("ARCHIVE_DRILL_AZURE_FALLBACK_KEY unset — azure fallback subtest skipped")
	}
	t.Run("azure_fallback", func(t *testing.T) {
		if _, err := r2.Get(ctx, fallbackKey); !IsNotFound(err) {
			t.Fatalf("expected r2 miss for %q, got err=%v", fallbackKey, err)
		}
		azureRaw, err := azure.Get(ctx, fallbackKey)
		if err != nil {
			t.Fatalf("azure get: %v", err)
		}
		if len(azureRaw) == 0 {
			t.Fatal("azure fallback blob is empty")
		}
		if _, err := Gunzip(azureRaw); err != nil {
			t.Fatalf("azure fallback gzip: %v", err)
		}
		fallbackRaw, err := readThrough.Get(ctx, fallbackKey)
		if err != nil {
			t.Fatalf("read-through fallback: %v", err)
		}
		if !bytes.Equal(azureRaw, fallbackRaw) {
			t.Fatal("read-through fallback bytes differ from azure")
		}
	})
}

func liveDrillStoreConfig(t *testing.T) StoreConfig {
	t.Helper()
	accountID := drillEnvOr(t, "ARCHIVE_R2_ACCOUNT_ID", "51dd8007b22ac92482388d8b6cdbb6e3")
	r2 := R2Config{
		AccountID:           accountID,
		Bucket:              drillEnvOr(t, "ARCHIVE_R2_BUCKET", "streampulse-artifacts-staging"),
		Prefix:              drillEnvOr(t, "ARCHIVE_R2_PREFIX", "archive"),
		Endpoint:            strings.TrimSpace(os.Getenv("ARCHIVE_R2_ENDPOINT")),
		AccessKeyIDFile:     strings.TrimSpace(os.Getenv("ARCHIVE_R2_ACCESS_KEY_ID_FILE")),
		SecretAccessKeyFile: strings.TrimSpace(os.Getenv("ARCHIVE_R2_SECRET_ACCESS_KEY_FILE")),
	}
	if r2.Endpoint == "" {
		r2.Endpoint = R2DefaultEndpoint(accountID)
	}
	azure := AzureConfig{
		StorageAccount:       drillEnvOr(t, "ARCHIVE_AZURE_STORAGE_ACCOUNT", "ststreamclone3lf6tt"),
		Container:            drillEnvOr(t, "ARCHIVE_AZURE_CONTAINER", "streamclone-archive"),
		Prefix:               drillEnvOr(t, "ARCHIVE_AZURE_PREFIX", "streamclone"),
		ConnectionStringFile: strings.TrimSpace(os.Getenv("ARCHIVE_AZURE_CONNECTION_STRING_FILE")),
	}
	if azure.ConnectionStringFile == "" {
		t.Fatal("ARCHIVE_AZURE_CONNECTION_STRING_FILE is required for restore drill")
	}
	return StoreConfig{
		Azure:           azure,
		R2:              r2,
		PrimaryProvider: "azure",
		ReadThrough:     true,
		DualWrite:       false,
	}
}

func drillEnvOr(t *testing.T, key, fallback string) string {
	t.Helper()
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

func assertDrillBlob(t *testing.T, sample drillSample, raw []byte) {
	t.Helper()
	if len(raw) < sample.minLen {
		t.Fatalf("%s: byte size = %d, want >= %d", sample.name, len(raw), sample.minLen)
	}
	sum := sha256.Sum256(raw)
	got := hex.EncodeToString(sum[:])
	if got != sample.sha256 {
		t.Fatalf("%s: sha256 = %s, want %s", sample.name, got, sample.sha256)
	}
	if _, err := Gunzip(raw); err != nil {
		t.Fatalf("%s: gzip: %v", sample.name, err)
	}
}

func assertDrillPayloadShape(t *testing.T, artifactType string, raw []byte) {
	t.Helper()
	plain, err := Gunzip(raw)
	if err != nil {
		t.Fatal(err)
	}
	trimmed := bytes.TrimSpace(plain)
	switch artifactType {
	case "analytics_rollups":
		if len(trimmed) == 0 {
			t.Fatal("rollups payload is empty")
		}
		if bytes.Equal(trimmed, []byte("[]")) {
			return
		}
		scanner := bufio.NewScanner(bytes.NewReader(plain))
		if !scanner.Scan() {
			t.Fatal("rollups jsonl is empty")
		}
		line := bytes.TrimSpace(scanner.Bytes())
		var rollup RollupExportLine
		if err := json.Unmarshal(line, &rollup); err == nil && !rollup.MinuteTS.IsZero() {
			return
		}
		if !json.Valid(line) {
			t.Fatalf("rollup line is not valid json: %q", line)
		}
	case "analytics_stream":
		var session StreamExportData
		if err := json.Unmarshal(plain, &session); err != nil {
			t.Fatalf("stream session parse: %v", err)
		}
		if strings.TrimSpace(session.StreamID) == "" {
			t.Fatal("stream_id missing")
		}
	case "bronze_vod_catalog":
		if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("[]")) {
			return
		}
		scanner := bufio.NewScanner(bytes.NewReader(plain))
		if !scanner.Scan() {
			if json.Valid(trimmed) {
				return
			}
			t.Fatal("vod index payload is not valid json")
		}
		var row map[string]json.RawMessage
		if err := json.Unmarshal(scanner.Bytes(), &row); err != nil {
			t.Fatalf("vod index line parse: %v", err)
		}
		if len(row) == 0 {
			t.Fatal("vod index row is empty")
		}
	default:
		t.Fatalf("unknown artifact type %q", artifactType)
	}
}
