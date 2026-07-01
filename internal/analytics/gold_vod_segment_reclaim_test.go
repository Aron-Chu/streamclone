package analytics

import (
	"testing"
	"time"
)

func TestReclaimStaleGoldVODSegmentsRequiresDB(t *testing.T) {
	if _, _, err := ReclaimStaleGoldVODSegments(t.Context(), nil, time.Minute); err == nil {
		t.Fatal("expected error when db is nil")
	}
}
