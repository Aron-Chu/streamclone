package enrich

import (
	"strings"
	"testing"

	"streamclone/internal/chat/parse"
	"streamclone/internal/chat/tokenize"
)

func TestTokenizeNativeEmoteBeforeDictionary(t *testing.T) {
	e := &Enricher{dicts: make(map[string]*tokenize.ChannelDict)}
	d := &tokenize.ChannelDict{}
	trie := tokenize.NewTrie()
	trie.Insert("Kappa", tokenize.Emote{ID: "dict-kappa", URL: "http://cdn/kappa.webp", Provider: "seventv"})
	d.Swap(trie)
	e.dicts["channel"] = d

	text := "hi Kappa there"
	native := []parse.EmoteRange{{ID: "25", Start: 3, End: 7}}
	frags := e.Tokenize("channel", text, native)

	got := ""
	for _, f := range frags {
		got += f.C
	}
	if got != text {
		t.Fatalf("round-trip mismatch: want %q got %q (%+v)", text, got, frags)
	}

	var emoteFrags int
	for _, f := range frags {
		if f.T == "emote" && f.C == "Kappa" {
			emoteFrags++
			if f.Provider != "twitch" || f.ID != "25" {
				t.Fatalf("native emote should use twitch provider from IRC tag: %+v", f)
			}
			if !strings.Contains(f.U, "/25/") {
				t.Fatalf("unexpected twitch CDN url: %q", f.U)
			}
		}
	}
	if emoteFrags != 1 {
		t.Fatalf("expected one native Kappa fragment, got %d (%+v)", emoteFrags, frags)
	}
}

func TestTokenizeNativeOnlyMessage(t *testing.T) {
	e := &Enricher{dicts: make(map[string]*tokenize.ChannelDict)}
	d := &tokenize.ChannelDict{}
	d.Swap(tokenize.NewTrie())
	e.dicts["channel"] = d

	frags := e.Tokenize("channel", "Kappa", []parse.EmoteRange{{ID: "25", Start: 0, End: 4}})
	if len(frags) != 1 {
		t.Fatalf("expected 1 fragment, got %d (%+v)", len(frags), frags)
	}
	if frags[0].T != "emote" || frags[0].C != "Kappa" || frags[0].Provider != "twitch" {
		t.Fatalf("unexpected fragment: %+v", frags[0])
	}
}

func TestTokenizeWithoutNativeDelegatesToDictionary(t *testing.T) {
	e := &Enricher{dicts: make(map[string]*tokenize.ChannelDict)}
	d := &tokenize.ChannelDict{}
	trie := tokenize.NewTrie()
	trie.Insert("Kappa", tokenize.Emote{URL: "http://cdn/kappa.webp"})
	d.Swap(trie)
	e.dicts["channel"] = d

	frags := e.Tokenize("channel", "Kappa", nil)
	if len(frags) != 1 || frags[0].T != "emote" || frags[0].Provider != "" {
		t.Fatalf("expected dictionary emote, got %+v", frags)
	}
}
