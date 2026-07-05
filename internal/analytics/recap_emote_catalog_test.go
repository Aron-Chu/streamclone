package analytics

import "testing"

func TestMergeRecapEmoteCatalogPrefersResolvedMetadata(t *testing.T) {
	rollupOnly := []TopEmote{{Name: "EZ", Provider: "seventv", Count: 24}}
	resolved := []TopEmote{{
		Name:     "EZ",
		Provider: "seventv",
		ID:       "ez-local",
		ImageURL: "https://cdn.7tv.app/emote/01EZ/4x.webp",
		Count:    24,
	}}
	merged := mergeRecapEmoteCatalogs(rollupOnly, resolved)
	if len(merged) != 1 {
		t.Fatalf("merged = %+v", merged)
	}
	if merged[0].ID != "ez-local" {
		t.Fatalf("id = %q", merged[0].ID)
	}
}

func TestRecapEmoteCodesDedupesCaseInsensitive(t *testing.T) {
	got := recapEmoteCodes([]string{" EZ ", "ez", "Clap"})
	if len(got) != 2 {
		t.Fatalf("got %#v", got)
	}
}

func TestTopEmoteFromRecapIdentityPrefersLocalID(t *testing.T) {
	item := topEmoteFromRecapIdentity("seventv", "01UPSTREAM", "KEKW", "local-uuid", 3)
	if item.ID != "local-uuid" {
		t.Fatalf("id = %q", item.ID)
	}
	if item.ImageURL == "" {
		t.Fatal("expected image url")
	}
}
