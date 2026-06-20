package seeder

import (
	"sort"
	"testing"

	"streamclone/internal/emote/store"
)

func mkEmote(id, name string) sevenTVEmote {
	return sevenTVEmote{ID: id, Name: name}
}

func TestDiffSevenTVSet_AddRemoveRename(t *testing.T) {
	remote := []sevenTVEmote{
		mkEmote("a", "Keep"),     // unchanged
		mkEmote("b", "NewName"),  // renamed (local name "OldName")
		mkEmote("c", "Brandnew"), // newly added remotely
	}
	local := []store.SetProviderEmote{
		{EmoteID: "uuid-a", ProviderEmoteID: "a", Name: "Keep"},
		{EmoteID: "uuid-b", ProviderEmoteID: "b", Name: "OldName"},
		{EmoteID: "uuid-d", ProviderEmoteID: "d", Name: "Stale"}, // removed remotely
	}

	diff := diffSevenTVSet(remote, local)

	if got := diff.RemoveEmoteIDs; len(got) != 1 || got[0] != "uuid-d" {
		t.Fatalf("expected to remove uuid-d, got %v", got)
	}
	if len(diff.Renames) != 1 || diff.Renames[0].EmoteID != "uuid-b" || diff.Renames[0].NewName != "NewName" {
		t.Fatalf("expected rename uuid-b -> NewName, got %#v", diff.Renames)
	}
	if got := diff.AddProviderIDs; len(got) != 1 || got[0] != "c" {
		t.Fatalf("expected to add provider id c, got %v", got)
	}
}

func TestDiffSevenTVSet_NoChange(t *testing.T) {
	remote := []sevenTVEmote{mkEmote("a", "One"), mkEmote("b", "Two")}
	local := []store.SetProviderEmote{
		{EmoteID: "uuid-a", ProviderEmoteID: "a", Name: "One"},
		{EmoteID: "uuid-b", ProviderEmoteID: "b", Name: "Two"},
	}
	diff := diffSevenTVSet(remote, local)
	if len(diff.RemoveEmoteIDs) != 0 || len(diff.Renames) != 0 || len(diff.AddProviderIDs) != 0 {
		t.Fatalf("expected empty diff, got %#v", diff)
	}
}

func TestDiffSevenTVSet_EmptyRemotePrunesAll(t *testing.T) {
	local := []store.SetProviderEmote{
		{EmoteID: "uuid-a", ProviderEmoteID: "a", Name: "One"},
		{EmoteID: "uuid-b", ProviderEmoteID: "b", Name: "Two"},
	}
	diff := diffSevenTVSet(nil, local)
	got := append([]string(nil), diff.RemoveEmoteIDs...)
	sort.Strings(got)
	if len(got) != 2 || got[0] != "uuid-a" || got[1] != "uuid-b" {
		t.Fatalf("expected both locals pruned, got %v", got)
	}
	if len(diff.AddProviderIDs) != 0 || len(diff.Renames) != 0 {
		t.Fatalf("expected no adds/renames, got %#v", diff)
	}
}

func TestDiffSevenTVSet_IgnoresBlankRemoteName(t *testing.T) {
	// A remote emote with an empty name should not trigger a rename.
	remote := []sevenTVEmote{mkEmote("a", "")}
	local := []store.SetProviderEmote{{EmoteID: "uuid-a", ProviderEmoteID: "a", Name: "Keep"}}
	diff := diffSevenTVSet(remote, local)
	if len(diff.Renames) != 0 {
		t.Fatalf("expected no rename for blank remote name, got %#v", diff.Renames)
	}
}
