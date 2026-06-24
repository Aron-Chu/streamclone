package analytics

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestSilverGateProductionFilesHaveNoForbiddenSideEffects(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	dir := filepath.Dir(file)
	forbidden := []string{
		"insertSilverBackfillJob",
		"INSERT INTO backfill_jobs",
		"insertGold",
		"TwitchTracker",
		"Camoufox",
		"VideoComments",
		"fetchVODComments",
		"WatchForPrincipal",
		"WatchWithPriority",
		"TouchForPrincipal",
		"PulseBackfillManager",
		"GQL",
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasPrefix(name, "top500_silver_gate") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		body := string(raw)
		for _, pattern := range forbidden {
			if strings.Contains(body, pattern) {
				t.Fatalf("%s contains forbidden side-effect pattern %q", name, pattern)
			}
		}
	}
}
