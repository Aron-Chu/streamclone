package analytics

import "testing"

func TestNormalizeEmoteSnapshotItemsStable(t *testing.T) {
	rows := []EmoteSnapshotItem{
		{Provider: "7TV", ProviderEmoteID: "2", Alias: "OMEGALUL"},
		{Provider: "seventv", ProviderEmoteID: "1", Alias: "KEKW"},
		{Provider: "seventv", ProviderEmoteID: "1", Alias: "KEKW"},
		{Provider: "", ProviderEmoteID: "", Alias: "skip"},
	}
	got := NormalizeEmoteSnapshotItems("seventv", rows)
	if len(got) != 2 {
		t.Fatalf("normalized item count = %d, want 2", len(got))
	}
	if got[0].Provider != "seventv" || got[0].ProviderEmoteID != "1" || got[0].Alias != "KEKW" {
		t.Fatalf("first normalized item = %+v", got[0])
	}
	left := StableEmoteSnapshotHash(rows)
	right := StableEmoteSnapshotHash([]EmoteSnapshotItem{rows[1], rows[0]})
	if left != right {
		t.Fatalf("stable hash changed with input order: %s != %s", left, right)
	}
}

func TestSnapshotShouldCreateHistoryOnlyForChangedSuccessfulFetch(t *testing.T) {
	hash := StableEmoteSnapshotHash([]EmoteSnapshotItem{{Provider: "seventv", ProviderEmoteID: "1", Alias: "KEKW"}})
	if SnapshotShouldCreateHistory(hash, hash, true) {
		t.Fatal("unchanged snapshot should not create fake history")
	}
	if SnapshotShouldCreateHistory(hash, "different", false) {
		t.Fatal("failed provider fetch should not create history")
	}
	if !SnapshotShouldCreateHistory(hash, "different", true) {
		t.Fatal("changed successful snapshot should create history")
	}
}

func TestDiffEmoteSnapshotsAddRemoveReaddAndAlias(t *testing.T) {
	previous := []EmoteSnapshotItem{
		{Provider: "seventv", ProviderEmoteID: "1", Alias: "KEKW"},
		{Provider: "seventv", ProviderEmoteID: "2", Alias: "OMEGALUL"},
		{Provider: "seventv", ProviderEmoteID: "3", Alias: "Old"},
	}
	current := []EmoteSnapshotItem{
		{Provider: "seventv", ProviderEmoteID: "1", Alias: "KEKW"},
		{Provider: "seventv", ProviderEmoteID: "3", Alias: "New"},
		{Provider: "seventv", ProviderEmoteID: "4", Alias: "Fresh"},
		{Provider: "seventv", ProviderEmoteID: "5", Alias: "Back"},
	}
	diff := DiffEmoteSnapshots(previous, current, map[string]struct{}{"seventv:5": {}})
	if len(diff.Added) != 1 || diff.Added[0].ProviderEmoteID != "4" {
		t.Fatalf("added = %+v", diff.Added)
	}
	if len(diff.Readded) != 1 || diff.Readded[0].ProviderEmoteID != "5" {
		t.Fatalf("readded = %+v", diff.Readded)
	}
	if len(diff.Removed) != 1 || diff.Removed[0].ProviderEmoteID != "2" {
		t.Fatalf("removed = %+v", diff.Removed)
	}
	if len(diff.AliasChanges) != 1 || diff.AliasChanges[0].FromAlias != "Old" || diff.AliasChanges[0].ToAlias != "New" {
		t.Fatalf("alias changes = %+v", diff.AliasChanges)
	}
}

func TestParseEmoteRollupKeyMalformed(t *testing.T) {
	parsed := ParseEmoteRollupKey("bad:key")
	if parsed.Provider != "" || parsed.ID != "" || parsed.Name != "bad:key" {
		t.Fatalf("parsed malformed key = %+v", parsed)
	}
	parsed = ParseEmoteRollupKey("seventv:123:KEKW")
	if parsed.Provider != "seventv" || parsed.ID != "123" || parsed.Name != "KEKW" {
		t.Fatalf("parsed provider key = %+v", parsed)
	}
	parsed = ParseEmoteRollupKey("7tv:456:catJAM")
	if parsed.Provider != "seventv" || parsed.ID != "456" || parsed.Name != "catJAM" {
		t.Fatalf("parsed 7tv provider key = %+v", parsed)
	}
	parsed = ParseEmoteRollupKey("7tv:OMEGALUL")
	if parsed.Provider != "seventv" || parsed.ID != "" || parsed.Name != "OMEGALUL" {
		t.Fatalf("parsed provider/name fallback key = %+v", parsed)
	}
	parsed = ParseEmoteRollupKey("ffz/monkaS")
	if parsed.Provider != "ffz" || parsed.ID != "" || parsed.Name != "monkaS" {
		t.Fatalf("parsed provider slash fallback key = %+v", parsed)
	}
}

func TestResolveEmoteIdentityFallbackStates(t *testing.T) {
	provider := ResolveEmoteIdentityAt(ParseEmoteRollupKey("seventv:123:KEKW"), nil)
	if provider.Resolution != EmoteIdentityProviderID || provider.Confidence != 1 {
		t.Fatalf("provider id resolution = %+v", provider)
	}
	fallback := ResolveEmoteIdentityAt(ParseEmoteRollupKey("KEKW"), []EmoteIdentityCandidate{{Provider: "seventv", ProviderEmoteID: "123", Name: "KEKW"}})
	if fallback.Resolution != EmoteIdentityAliasFallback || fallback.ProviderEmoteID != "123" || fallback.Confidence >= 1 {
		t.Fatalf("alias fallback = %+v", fallback)
	}
	providerFallback := ResolveEmoteIdentityAt(ParseEmoteRollupKey("7tv:KEKW"), []EmoteIdentityCandidate{
		{Provider: "seventv", ProviderEmoteID: "123", Name: "KEKW"},
		{Provider: "bttv", ProviderEmoteID: "456", Name: "KEKW"},
	})
	if providerFallback.Resolution != EmoteIdentityAliasFallback || providerFallback.Provider != "seventv" || providerFallback.ProviderEmoteID != "123" || providerFallback.Confidence >= 1 {
		t.Fatalf("provider-scoped alias fallback = %+v", providerFallback)
	}
	ambiguous := ResolveEmoteIdentityAt(ParseEmoteRollupKey("KEKW"), []EmoteIdentityCandidate{
		{Provider: "seventv", ProviderEmoteID: "123", Name: "KEKW"},
		{Provider: "bttv", ProviderEmoteID: "456", Name: "KEKW"},
	})
	if ambiguous.Resolution != EmoteIdentityAmbiguous {
		t.Fatalf("ambiguous = %+v", ambiguous)
	}
	unresolved := ResolveEmoteIdentityAt(ParseEmoteRollupKey("Nope"), nil)
	if unresolved.Resolution != EmoteIdentityUnresolved {
		t.Fatalf("unresolved = %+v", unresolved)
	}
}
