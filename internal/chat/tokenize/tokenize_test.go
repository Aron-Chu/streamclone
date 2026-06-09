package tokenize

import "testing"

func buildTrie() *Trie {
	t := NewTrie()
	t.Insert("Kappa", Emote{URL: "http://cdn/kappa.webp"})
	t.Insert("KEKW", Emote{ID: "base-kekw", URL: "http://cdn/kekw.webp", Provider: "seventv"})
	t.Insert("LO", Emote{URL: "http://cdn/lo.webp"})
	t.Insert("Alert", Emote{ID: "zw-alert", URL: "http://cdn/alert.webp", Zw: true, Provider: "seventv"})
	t.Insert("RainTime", Emote{ID: "zw-rain", URL: "http://cdn/rain.webp", Zw: true, Provider: "seventv"})
	t.Insert("SnowTime", Emote{ID: "zw-snow", URL: "http://cdn/snow.webp", Zw: true, Provider: "seventv"})
	t.Insert("PogChamp", Emote{URL: "http://cdn/pog.webp"})
	t.Insert("widepeepoHappy", Emote{URL: "http://cdn/wph.webp", Zw: true})
	return t
}

func TestRoundTrip(t *testing.T) {
	in := "hey  Kappa nice PogChamp!! Kap end"
	frags := buildTrie().Tokenize(in)
	got := ""
	for _, f := range frags {
		got += f.C
	}
	if got != in {
		t.Fatalf("round-trip mismatch:\n want %q\n got  %q", in, got)
	}
}

func TestEmoteAndText(t *testing.T) {
	frags := buildTrie().Tokenize("a Kappa b")
	if len(frags) != 3 {
		t.Fatalf("expected 3 fragments, got %d (%+v)", len(frags), frags)
	}
	if frags[0].T != "text" || frags[0].C != "a " {
		t.Fatalf("frag0 = %+v", frags[0])
	}
	if frags[1].T != "emote" || frags[1].C != "Kappa" || frags[1].U == "" {
		t.Fatalf("frag1 = %+v", frags[1])
	}
	if frags[2].T != "text" || frags[2].C != " b" {
		t.Fatalf("frag2 = %+v", frags[2])
	}
}

func TestZeroWidthFlag(t *testing.T) {
	frags := buildTrie().Tokenize("widepeepoHappy")
	if len(frags) != 1 || !frags[0].Zw {
		t.Fatalf("expected single zero-width emote fragment, got %+v", frags)
	}
}

func TestWholeWordOnly(t *testing.T) {
	frags := buildTrie().Tokenize("Kappab Kap")
	for _, f := range frags {
		if f.T == "emote" {
			t.Fatalf("substring should not match as emote: %+v", frags)
		}
	}
}

func TestMentionFormatting(t *testing.T) {
	frags := buildTrie().Tokenize("hey @ninja there")
	if len(frags) != 3 {
		t.Fatalf("expected 3 fragments, got %d (%+v)", len(frags), frags)
	}
	if frags[0].T != "text" || frags[0].C != "hey " {
		t.Fatalf("frag0 = %+v", frags[0])
	}
	if frags[1].T != "mention" || frags[1].C != "@ninja" {
		t.Fatalf("frag1 = %+v", frags[1])
	}
	if frags[2].T != "text" || frags[2].C != " there" {
		t.Fatalf("frag2 = %+v", frags[2])
	}
}

func TestMentionKeepsTrailingPunctuation(t *testing.T) {
	frags := buildTrie().Tokenize("yo @ninja, hi")
	if len(frags) != 3 {
		t.Fatalf("expected 3 fragments, got %d (%+v)", len(frags), frags)
	}
	if frags[1].T != "mention" || frags[1].C != "@ninja" {
		t.Fatalf("mention frag = %+v", frags[1])
	}
	if frags[2].T != "text" || frags[2].C != ", hi" {
		t.Fatalf("tail frag = %+v", frags[2])
	}
}

func TestZeroWidthOverlayConsumesSpacer(t *testing.T) {
	frags := buildTrie().Tokenize("LO Alert")
	if len(frags) != 2 {
		t.Fatalf("expected 2 fragments, got %d (%+v)", len(frags), frags)
	}
	if frags[0].T != "emote" || frags[0].C != "LO" {
		t.Fatalf("base frag = %+v", frags[0])
	}
	if frags[1].T != "emote" || frags[1].C != "Alert" || !frags[1].Zw {
		t.Fatalf("overlay frag = %+v", frags[1])
	}
}

func TestZeroWidthSuffixOverlayWithoutSpace(t *testing.T) {
	frags := buildTrie().Tokenize("KEKWRainTime")
	if len(frags) != 2 {
		t.Fatalf("expected 2 fragments, got %d (%+v)", len(frags), frags)
	}
	if frags[0].T != "emote" || frags[0].C != "KEKW" || frags[0].ID != "base-kekw" || frags[0].Provider != "seventv" {
		t.Fatalf("base frag = %+v", frags[0])
	}
	if frags[1].T != "emote" || frags[1].C != "RainTime" || !frags[1].Zw || frags[1].ID != "zw-rain" {
		t.Fatalf("overlay frag = %+v", frags[1])
	}
}

func TestChainedZeroWidthSuffixes(t *testing.T) {
	frags := buildTrie().Tokenize("KEKWRainTimeSnowTime")
	if len(frags) != 3 {
		t.Fatalf("expected 3 fragments, got %d (%+v)", len(frags), frags)
	}
	if frags[0].C != "KEKW" || frags[1].C != "RainTime" || frags[2].C != "SnowTime" {
		t.Fatalf("unexpected fragments = %+v", frags)
	}
}

func TestZeroWidthSuffixDoesNotSplitRegularWords(t *testing.T) {
	frags := buildTrie().Tokenize("sneakyRainTime")
	if len(frags) != 1 || frags[0].T != "text" || frags[0].C != "sneakyRainTime" {
		t.Fatalf("expected regular text, got %+v", frags)
	}
}

func TestNilDictFallback(t *testing.T) {
	var d ChannelDict
	frags := d.Tokenize("no dict here")
	if len(frags) != 1 || frags[0].T != "text" || frags[0].C != "no dict here" {
		t.Fatalf("expected single text fallback, got %+v", frags)
	}
}
