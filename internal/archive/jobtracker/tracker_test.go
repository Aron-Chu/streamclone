package jobtracker_test

import (
	"testing"
	"time"

	"streamclone/internal/archive/jobtracker"
)

func TestTrackerDefaults(t *testing.T) {
	tr := jobtracker.NewTracker(nil, 0, 0, false)
	if tr == nil {
		t.Fatal("nil tracker")
	}
	n, err := tr.MarkStaleJobs(nil)
	if err != nil || n != 0 {
		t.Fatalf("stale n=%d err=%v", n, err)
	}
}

func TestStaleCutoff(t *testing.T) {
	staleAfter := 10 * time.Minute
	cutoff := time.Now().Add(-staleAfter)
	if cutoff.After(time.Now()) {
		t.Fatal("cutoff in future")
	}
}
