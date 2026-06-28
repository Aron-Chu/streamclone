package archive

import "testing"

func TestStreamExportRowsNilStore(t *testing.T) {
	var store *ManifestStore
	rows, err := store.StreamExportRows(t.Context(), "123")
	if err != nil {
		t.Fatal(err)
	}
	if rows != nil {
		t.Fatalf("expected nil rows, got %v", rows)
	}
}

func TestStreamExportRowsEmptyStreamID(t *testing.T) {
	store := &ManifestStore{}
	rows, err := store.StreamExportRows(t.Context(), "  ")
	if err != nil {
		t.Fatal(err)
	}
	if rows != nil {
		t.Fatalf("expected nil rows, got %v", rows)
	}
}
