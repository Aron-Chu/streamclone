package analytics

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestGQLPriorityFromIVRRollups(t *testing.T) {
	if got := gqlPriorityFromIVRRollups(nil, 0); got != "normal" {
		t.Fatalf("empty: got %q", got)
	}
	low := []MinuteRollup{{MinuteTS: time.Now(), ChatCount: 10}}
	if got := gqlPriorityFromIVRRollups(low, 10); got != "normal" {
		t.Fatalf("low: got %q", got)
	}
	high := []MinuteRollup{{MinuteTS: time.Now(), ChatCount: 90}, {MinuteTS: time.Now().Add(time.Minute), ChatCount: 20}}
	if got := gqlPriorityFromIVRRollups(high, 110); got != "high" {
		t.Fatalf("high: got %q", got)
	}
	urgent := []MinuteRollup{{MinuteTS: time.Now(), ChatCount: 220}}
	if got := gqlPriorityFromIVRRollups(urgent, 220); got != "urgent" {
		t.Fatalf("urgent: got %q", got)
	}
}

func TestPeakOverlapFromCounts(t *testing.T) {
	left := map[int64]int{1: 100, 2: 50, 3: 10}
	right := map[int64]int{1: 90, 2: 55, 4: 5}
	if got := peakOverlapFromCounts(left, right, 3); got != 50.0 {
		t.Fatalf("top3 overlap: got %f", got)
	}
}

func TestWriteGoldIVRShadowArtifact(t *testing.T) {
	dir := t.TempDir()
	artifact := GoldIVRShadowArtifact{
		StreamID:                  "stream-1",
		Login:                     "ludwig",
		ChannelID:                 "40934651",
		IVRMessageCount:           95,
		Recommendation:            "shadow_only_or_peaks_only",
		GQLPriorityRecommendation: "high",
		WroteRollups:              false,
		UpdatedStreamMetadata:     false,
		ShadowOnly:                true,
		CreatedAt:                 time.Date(2026, 6, 28, 12, 0, 0, 0, time.UTC),
	}
	path, err := writeGoldIVRShadowArtifact(dir, artifact)
	if err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), `"wrote_rollups": false`) || !strings.Contains(string(body), `"gql_priority_recommendation": "high"`) {
		t.Fatalf("artifact missing fields: %s", string(body))
	}
	if filepath.Dir(path) != dir {
		t.Fatalf("unexpected dir %q", filepath.Dir(path))
	}
}

func TestReconciliationRecommendation(t *testing.T) {
	if got := reconciliationRecommendation(96, 100); got != "shadow_reconciled_peaks_only" {
		t.Fatalf("peaks only: got %q", got)
	}
	if got := reconciliationRecommendation(70, 80); got != "shadow_reconciled_hold" {
		t.Fatalf("hold: got %q", got)
	}
	if got := reconciliationRecommendation(50, 20); got != "shadow_reconciled_reject" {
		t.Fatalf("reject: got %q", got)
	}
}
