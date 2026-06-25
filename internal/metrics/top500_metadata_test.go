package metrics

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestTop500MetadataMetricsRegisterAndUseAllowedLabels(t *testing.T) {
	Top500MetadataSamplerEnabled.Set(1)
	Top500MetadataDryRun.Set(1)
	Top500MetadataWriteEnabled.Set(0)
	Top500MetadataTopNConfigured.Set(100)
	Top500MetadataRosterSize.Set(42)
	Top500MetadataFreshnessSeconds.Set(30)
	Top500MetadataChannelsPlannedTotal.WithLabelValues("planned", "dry_run").Inc()
	Top500MetadataChannelsSampledTotal.WithLabelValues("success", "dry_run").Inc()
	Top500MetadataProviderCallsTotal.WithLabelValues("fetch_streams", "success", "helix").Inc()
	Top500MetadataProviderErrorsTotal.WithLabelValues("fetch_streams", "helix_auth_missing", "helix").Inc()
	Top500MetadataProviderRateLimitsTotal.WithLabelValues("fetch_streams", "helix").Inc()
	Top500MetadataSnapshotWritesTotal.WithLabelValues("success", "write_enabled").Inc()
	Top500MetadataCurrentUpsertsTotal.WithLabelValues("success", "write_enabled").Inc()
	Top500MetadataWriteBatchSize.WithLabelValues("success", "write_enabled", "write_samples").Set(2)
	Top500MetadataWriteLatencySeconds.WithLabelValues("success", "write_enabled", "write_samples").Set(0.01)
	Top500MetadataSamplesDegradedTotal.WithLabelValues("dry_run", "dry_run").Inc()
	Top500MetadataRollbackState.WithLabelValues("dry_run", "dry_run").Set(1)
	Top500MetadataLockUnavailableTotal.WithLabelValues("lock_unavailable", "write_enabled").Inc()

	if got := testutil.ToFloat64(Top500MetadataSamplerEnabled); got != 1 {
		t.Fatalf("sampler enabled gauge = %v, want 1", got)
	}
	assertTop500MetadataLabelsAllowed(t)
}

func assertTop500MetadataLabelsAllowed(t *testing.T) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("top500_metadata.go"))
	if err != nil {
		t.Fatalf("read metrics file: %v", err)
	}
	allowed := map[string]bool{
		"result": true, "reason": true, "source": true, "mode": true, "lane": true, "operation": true,
	}
	re := regexp.MustCompile(`\[\]string\{([^}]*)\}`)
	for _, match := range re.FindAllStringSubmatch(string(raw), -1) {
		for _, label := range strings.Split(match[1], ",") {
			label = strings.Trim(label, " \t\r\n\"")
			if label == "" {
				continue
			}
			if !allowed[label] {
				t.Fatalf("top500 metadata metric uses forbidden label %q", label)
			}
		}
	}
}
