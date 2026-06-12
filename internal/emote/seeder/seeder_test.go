package seeder

import "testing"

func TestSevenTVZeroWidthFromAPIFlags(t *testing.T) {
	em := sevenTVEmote{Name: "RainTime", Flags: 1}
	em.Data.Flags = 256
	if !sevenTVZeroWidth(em) {
		t.Fatalf("expected overlay emote to be zero width")
	}
	em = sevenTVEmote{Name: "Clap"}
	if sevenTVZeroWidth(em) {
		t.Fatalf("expected regular emote not to be zero width")
	}
}

func TestSortRemoteEmotesPrioritizesUsableStaticEmotes(t *testing.T) {
	emotes := []remoteEmote{
		{Name: "wide", ZeroWidth: true},
		{Name: "dance", Animated: true},
		{Name: "alpha"},
		{Name: "Bravo"},
	}

	sortRemoteEmotes(emotes)

	got := []string{emotes[0].Name, emotes[1].Name, emotes[2].Name, emotes[3].Name}
	want := []string{"alpha", "Bravo", "dance", "wide"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("unexpected order at %d: got %q want %q; full=%v", i, got[i], want[i], got)
		}
	}
}

func TestNormalizeImportConcurrency(t *testing.T) {
	tests := []struct {
		name string
		in   int
		want int
	}{
		{name: "minimum", in: 0, want: 1},
		{name: "keeps valid", in: 8, want: 8},
		{name: "caps large", in: 100, want: 32},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := normalizeImportConcurrency(tt.in); got != tt.want {
				t.Fatalf("got %d want %d", got, tt.want)
			}
		})
	}
}
