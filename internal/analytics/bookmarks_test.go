package analytics

import "testing"

func TestNormalizeBookmarkSource(t *testing.T) {
	if got := normalizeBookmarkSource(""); got != "web" {
		t.Fatalf("empty source = %q, want web", got)
	}
	if got := normalizeBookmarkSource("extension"); got != "extension" {
		t.Fatalf("extension source = %q", got)
	}
	if got := normalizeBookmarkSource("clipper"); got != "" {
		t.Fatalf("invalid source = %q, want empty", got)
	}
}

func TestNormalizeBookmarkPatch(t *testing.T) {
	label := "  funny team wipe  "
	notes := "  maybe clip later  "
	gotLabel, gotNotes, err := normalizeBookmarkPatch(UpdatePulseBookmarkRequest{
		Label: &label,
		Notes: &notes,
	})
	if err != nil {
		t.Fatal(err)
	}
	if gotLabel == nil || *gotLabel != "funny team wipe" {
		t.Fatalf("label = %#v", gotLabel)
	}
	if gotNotes == nil || *gotNotes != "maybe clip later" {
		t.Fatalf("notes = %#v", gotNotes)
	}
}

func TestNormalizeBookmarkPatchRejectsEmpty(t *testing.T) {
	if _, _, err := normalizeBookmarkPatch(UpdatePulseBookmarkRequest{}); err == nil {
		t.Fatal("expected empty patch error")
	}
	label := " "
	if _, _, err := normalizeBookmarkPatch(UpdatePulseBookmarkRequest{Label: &label}); err == nil {
		t.Fatal("expected blank label error")
	}
}
