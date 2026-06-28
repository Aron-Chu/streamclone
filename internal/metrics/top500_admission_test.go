package metrics

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestTopRosterAdmissionMetricsRegisterAndUseAllowedLabels(t *testing.T) {
	TopRosterAdmissionEnabled.Set(1)
	TopRosterAdmissionLiveConsidered.Set(12)
	TopRosterAdmissionActiveCollectors.Set(10)
	TopRosterAdmissionZeroChatLiveRows.Set(3)
	TopRosterAdmissionAttemptsTotal.WithLabelValues("admitted", "top_roster").Inc()
	TopRosterAdmissionAttemptsTotal.WithLabelValues("capacity_full", "top_roster").Inc()
	TopRosterAdmissionCapacityBlockedTotal.Inc()

	if got := testutil.ToFloat64(TopRosterAdmissionEnabled); got != 1 {
		t.Fatalf("admission enabled gauge = %v, want 1", got)
	}
	assertTopRosterAdmissionLabelsAllowed(t)
}

func assertTopRosterAdmissionLabelsAllowed(t *testing.T) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("top500_admission.go"))
	if err != nil {
		t.Fatalf("read metrics file: %v", err)
	}
	allowed := map[string]bool{
		"outcome": true,
		"mode":    true,
	}
	re := regexp.MustCompile(`\[\]string\{([^}]*)\}`)
	for _, match := range re.FindAllStringSubmatch(string(raw), -1) {
		for _, label := range strings.Split(match[1], ",") {
			label = strings.Trim(label, " \t\r\n\"")
			if label == "" {
				continue
			}
			if !allowed[label] {
				t.Fatalf("top roster admission metric uses forbidden label %q", label)
			}
		}
	}
}
