package ingest

import (
	"errors"
	"testing"
)

func TestHealthRecordsAndCopiesSourceDetails(t *testing.T) {
	h := NewHealth()
	h.RecordOK("reddit", 4)
	h.RecordDetailOK("reddit", "comments", 8)

	snap := h.Snapshot()
	comments := snap["reddit"].Details["comments"]
	if !comments.Healthy || comments.LastItems != 8 || comments.LastOKAt == nil {
		t.Fatalf("comments detail after ok = %+v", comments)
	}

	// Snapshot must be a deep copy so callers cannot mutate the tracker.
	snap["reddit"].Details["comments"] = SourceDetailStatus{LastItems: 99}
	next := h.Snapshot()["reddit"].Details["comments"]
	if next.LastItems != 8 {
		t.Fatalf("details snapshot was not copied, got last_items=%d", next.LastItems)
	}

	h.RecordDetailFail("reddit", "comments", errors.New("comments status 429"))
	failed := h.Snapshot()["reddit"].Details["comments"]
	if failed.Healthy || failed.LastError != "comments status 429" || failed.LastErrAt == nil {
		t.Fatalf("comments detail after fail = %+v", failed)
	}

	h.RecordDetailSkip("reddit", "comments", "comment fetcher unavailable")
	skipped := h.Snapshot()["reddit"].Details["comments"]
	if skipped.Healthy || skipped.LastError != "comment fetcher unavailable" || skipped.LastErrAt == nil {
		t.Fatalf("comments detail after skip = %+v", skipped)
	}

	parent := h.Snapshot()["reddit"]
	if !parent.Healthy || parent.LastItems != 4 {
		t.Fatalf("detail updates should not mutate parent source health: %+v", parent)
	}
}
