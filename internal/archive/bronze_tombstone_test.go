package archive

import "testing"

func TestDiffVODCatalog(t *testing.T) {
	removed := DiffVODCatalog([]string{"a", "b", "c"}, []string{"b", "d"})
	if len(removed) != 2 {
		t.Fatalf("expected 2 removed, got %d: %v", len(removed), removed)
	}
	got := map[string]bool{}
	for _, id := range removed {
		got[id] = true
	}
	if !got["a"] || !got["c"] {
		t.Fatalf("expected a and c removed, got %v", removed)
	}
	if len(DiffVODCatalog(nil, []string{"x"})) != 0 {
		t.Fatal("expected no tombstones from empty previous")
	}
}
