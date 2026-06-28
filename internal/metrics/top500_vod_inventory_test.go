package metrics

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestTop500VODInventoryMetricsUseAllowedLabels(t *testing.T) {
	Top500VODInventoryTotal.WithLabelValues("queued", "eligible").Set(2)
	Top500VODInventoryRankBucketTotal.WithLabelValues("rank_001_050", "done", "loaded").Set(1)
	Top500VODInventoryOldestQueuedAgeSeconds.Set(60)
	Top500VODInventoryArchiveConfirmedTotal.Set(1)
	Top500VODInventoryTerminalTotal.WithLabelValues("expired").Set(1)

	if got := testutil.ToFloat64(Top500VODInventoryArchiveConfirmedTotal); got != 1 {
		t.Fatalf("archive confirmed gauge = %v, want 1", got)
	}
	assertTop500VODInventoryLabelsAllowed(t)
}

func assertTop500VODInventoryLabelsAllowed(t *testing.T) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("top500_vod_inventory.go"))
	if err != nil {
		t.Fatalf("read metrics file: %v", err)
	}
	allowed := map[string]bool{
		"gold_status":        true,
		"availability_state": true,
		"rank_bucket":        true,
		"state":              true,
	}
	re := regexp.MustCompile(`\[\]string\{([^}]*)\}`)
	for _, match := range re.FindAllStringSubmatch(string(raw), -1) {
		for _, label := range strings.Split(match[1], ",") {
			label = strings.Trim(label, " \t\r\n\"")
			if label == "" {
				continue
			}
			if !allowed[label] {
				t.Fatalf("top500 vod inventory metric uses forbidden label %q", label)
			}
		}
	}
}
