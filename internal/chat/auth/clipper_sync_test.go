package auth

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteClipperAuthSyncFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "clipper-twitch.env")

	if err := writeClipperAuthSyncFile(path, "client-id", "access-token", "refresh-token"); err != nil {
		t.Fatalf("writeClipperAuthSyncFile: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read sync file: %v", err)
	}
	text := string(data)
	for _, want := range []string{
		"CLIPPER_TWITCH_CLIENT_ID=client-id",
		"CLIPPER_TWITCH_USER_ACCESS_TOKEN=access-token",
		"CLIPPER_TWITCH_REFRESH_TOKEN=refresh-token",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q in %q", want, text)
		}
	}

	if err := writeClipperAuthSyncFile("", "client-id", "access-token", ""); err != nil {
		t.Fatalf("empty path should no-op: %v", err)
	}
}
