package metrics

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestTop500SilverGateMetricsRegisterAndUseAllowedLabels(t *testing.T) {
	Top500SilverGateEnabled.Set(0)
	Top500SilverGateDryRun.Set(1)
	Top500SilverGateWriteEnabled.Set(0)
	Top500SilverGateDecisionsTotal.WithLabelValues("skip", "skip_daily_budget", "top500_selective", "evaluate").Inc()
	Top500SilverGateCandidatesTotal.WithLabelValues("top500_selective", "tick").Inc()
	Top500SilverGateEnqueueAttemptsTotal.WithLabelValues("skip", "top500_selective", "enqueue").Inc()
	Top500SilverGateEnqueueErrorsTotal.WithLabelValues("write_disabled", "top500_selective", "enqueue").Inc()
	Top500SilverGateIdempotencySkipsTotal.WithLabelValues("skip_duplicate_job", "top500_selective").Inc()
	Top500SilverGateDurationSeconds.WithLabelValues("evaluate", "top500_selective").Observe(0.01)

	if got := testutil.ToFloat64(Top500SilverGateDryRun); got != 1 {
		t.Fatalf("dry run gauge = %v, want 1", got)
	}
	assertTop500SilverGateLabelsAllowed(t)
}

func assertTop500SilverGateLabelsAllowed(t *testing.T) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("top500_silver_gate.go"))
	if err != nil {
		t.Fatalf("read metrics file: %v", err)
	}
	allowed := map[string]bool{
		"result": true, "reason": true, "lane": true, "operation": true,
	}
	forbidden := map[string]bool{
		"login": true, "channel_id": true, "stream_id": true, "vod_id": true,
		"title": true, "rank": true, "viewer_count": true, "user": true,
	}
	re := regexp.MustCompile(`\[\]string\{([^}]*)\}`)
	for _, match := range re.FindAllStringSubmatch(string(raw), -1) {
		for _, label := range strings.Split(match[1], ",") {
			label = strings.Trim(label, " \t\r\n\"")
			if label == "" {
				continue
			}
			if forbidden[label] {
				t.Fatalf("silver gate metric uses forbidden label %q", label)
			}
			if !allowed[label] {
				t.Fatalf("silver gate metric uses unexpected label %q", label)
			}
		}
	}
}
