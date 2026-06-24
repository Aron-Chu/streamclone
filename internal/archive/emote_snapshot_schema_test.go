package archive

import (
	"encoding/json"
	"testing"
	"time"
)

func TestEmoteSnapshotSchema(t *testing.T) {
	doc := NewEmoteSnapshotDocument(EmoteSnapshotMeta{
		Login:      "xqc",
		Provider:   "7tv",
		Count:      1,
		ExportedAt: time.Now().UTC(),
		Strategy:   EmoteSnapshotStrategyWeekly,
	}, []EmoteSnapshotLine{{
		EmoteID:  "abc",
		Name:     "KEKW",
		Provider: "7tv",
	}})
	raw, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateEmoteSnapshotDocument(raw); err != nil {
		t.Fatalf("validate: %v", err)
	}
	if doc.SchemaVersion != EmoteSnapshotSchemaVersion {
		t.Fatalf("schema version mismatch")
	}
}

func TestEmoteDiffDetectsAdd(t *testing.T) {
	prev := []EmoteSnapshotLine{{EmoteID: "1", Name: "A", Provider: "7tv"}}
	next := []EmoteSnapshotLine{
		{EmoteID: "1", Name: "A", Provider: "7tv"},
		{EmoteID: "2", Name: "B", Provider: "7tv"},
	}
	adds, removes := diffEmoteSnapshots(prev, next)
	if len(adds) != 1 || adds[0].EmoteID != "2" {
		t.Fatalf("expected one add, got %+v", adds)
	}
	if len(removes) != 0 {
		t.Fatalf("expected no removes, got %+v", removes)
	}
}
