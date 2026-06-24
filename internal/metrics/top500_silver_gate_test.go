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

func TestTop500SilverGateMetricsAllDecisionReasonLabels(t *testing.T) {
	reasons := []string{
		"allow_enqueue",
		"skip_not_candidate",
		"skip_metadata_stale",
		"skip_missing_stream_id",
		"skip_already_done",
		"skip_duplicate_job",
		"skip_channel_cooldown",
		"skip_recent_failure",
		"skip_global_backoff",
		"skip_daily_budget",
		"skip_running_limit",
		"skip_queue_full",
		"skip_disk_guard",
		"skip_backup_guard",
		"skip_archive_guard",
		"skip_alerting_guard",
		"skip_corpus_unhealthy",
		"skip_hosted_unhealthy",
		"skip_counter_unavailable",
		"skip_counter_stale",
	}
	for _, reason := range reasons {
		result := "skip"
		if reason == "allow_enqueue" {
			result = "allow"
		}
		Top500SilverGateDecisionsTotal.WithLabelValues(result, reason, "top500_selective", "evaluate").Inc()
	}
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
