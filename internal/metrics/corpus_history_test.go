package metrics

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestCorpusHistoryMetricsUseAllowedLabels(t *testing.T) {
	CorpusMinuteRollupsTotal.Set(10)
	CorpusMinuteRollupStreamsTotal.Set(2)
	CorpusVODChatMessagesTotal.Set(5)
	CorpusVODChatStreamsTotal.Set(1)
	CorpusArchiveExportsTotal.WithLabelValues("silver", "analytics_rollups", "confirmed").Set(3)
	CorpusArchiveExportRowsTotal.WithLabelValues("gold", "vod_chat_message", "confirmed").Set(50)

	if got := testutil.ToFloat64(CorpusMinuteRollupsTotal); got != 10 {
		t.Fatalf("minute rollups gauge = %v, want 10", got)
	}
	assertCorpusHistoryLabelsAllowed(t)
}

func assertCorpusHistoryLabelsAllowed(t *testing.T) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("corpus_history.go"))
	if err != nil {
		t.Fatalf("read metrics file: %v", err)
	}
	allowed := map[string]bool{
		"tier":          true,
		"artifact_type": true,
		"export_status": true,
	}
	re := regexp.MustCompile(`\[\]string\{([^}]*)\}`)
	for _, match := range re.FindAllStringSubmatch(string(raw), -1) {
		for _, label := range strings.Split(match[1], ",") {
			label = strings.Trim(label, " \t\r\n\"")
			if label == "" {
				continue
			}
			if !allowed[label] {
				t.Fatalf("corpus history metric uses forbidden label %q", label)
			}
		}
	}
}
