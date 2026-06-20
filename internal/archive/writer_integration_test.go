//go:build integration

package archive

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

func TestAzureBlobStoreSmokeUpload(t *testing.T) {
	file := strings.TrimSpace(os.Getenv("ARCHIVE_AZURE_CONNECTION_STRING_FILE"))
	if file == "" {
		file = os.Getenv("HOME") + "/.streamclone/azure-archive-connection-string"
	}
	if _, err := os.Stat(file); err != nil {
		t.Skipf("connection string file missing: %s", file)
	}
	account := strings.TrimSpace(os.Getenv("ARCHIVE_AZURE_STORAGE_ACCOUNT"))
	if account == "" {
		account = "ststreamclone3lf6tt"
	}
	blob, err := NewAzureBlobStore(AzureConfig{
		StorageAccount:       account,
		Container:            "streamclone-archive",
		Prefix:               "streamclone",
		ConnectionStringFile: file,
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	key := "smoke-tests/agent-a-" + time.Now().UTC().Format("20060102T150405Z") + ".txt"
	res, err := blob.Put(ctx, key, []byte("streamclone archive writer smoke"), "text/plain")
	if err != nil {
		t.Fatal(err)
	}
	if res.URI == "" || res.ETag == "" {
		t.Fatalf("unexpected put result: %+v", res)
	}
}
