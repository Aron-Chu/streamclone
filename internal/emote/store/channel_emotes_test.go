package store

import (
	"math/rand"
	"reflect"
	"testing"
)

func winners(rows []ChannelEmote) map[string]ChannelEmote {
	out := make(map[string]ChannelEmote, len(rows))
	for _, row := range rows {
		out[row.Name] = row
	}
	return out
}

func TestResolveChannelEmotesChannelBeatsGlobal(t *testing.T) {
	rows := []ChannelEmote{
		// A 7TV global, once from the channel's set and once from the
		// all-globals branch of the query.
		{Name: "Clap", EmoteID: "7tv-global-clap", Provider: "seventv", IsGlobal: true},
		{Name: "Clap", EmoteID: "7tv-global-clap", Provider: "seventv", IsGlobal: true},
		// The channel's own BTTV emote with the same name.
		{Name: "Clap", EmoteID: "bttv-channel-clap", Provider: "bttv"},
		// A Twitch global and the channel's FFZ emote.
		{Name: "Kappa", EmoteID: "twitch-global-kappa", Provider: "twitch", IsGlobal: true},
		{Name: "Kappa", EmoteID: "ffz-channel-kappa", Provider: "ffz"},
	}
	got := resolveChannelEmotes(rows)
	if len(got) != 2 {
		t.Fatalf("want one row per name, got %d: %+v", len(got), got)
	}
	w := winners(got)
	if w["Clap"].EmoteID != "bttv-channel-clap" {
		t.Fatalf("Clap = %+v, want the channel's BTTV emote", w["Clap"])
	}
	if w["Kappa"].EmoteID != "ffz-channel-kappa" {
		t.Fatalf("Kappa = %+v, want the channel's FFZ emote", w["Kappa"])
	}
}

func TestResolveChannelEmotesProviderOrderWithinScope(t *testing.T) {
	rows := []ChannelEmote{
		// Channel scope: custom, then 7TV, BTTV, FFZ, Twitch.
		{Name: "Wave", EmoteID: "twitch-wave", Provider: "twitch"},
		{Name: "Wave", EmoteID: "ffz-wave", Provider: "ffz"},
		{Name: "Wave", EmoteID: "bttv-wave", Provider: "bttv"},
		{Name: "Wave", EmoteID: "7tv-wave", Provider: "seventv"},
		{Name: "LUL", EmoteID: "7tv-lul", Provider: "seventv"},
		{Name: "LUL", EmoteID: "custom-lul", Provider: "custom"},
		{Name: "Jam", EmoteID: "twitch-jam", Provider: "twitch"},
		{Name: "Jam", EmoteID: "ffz-jam", Provider: "ffz"},
		// Global scope uses the same provider order.
		{Name: "Dance", EmoteID: "twitch-dance", Provider: "twitch", IsGlobal: true},
		{Name: "Dance", EmoteID: "ffz-dance", Provider: "ffz", IsGlobal: true},
		{Name: "Dance", EmoteID: "bttv-dance", Provider: "bttv", IsGlobal: true},
	}
	w := winners(resolveChannelEmotes(rows))
	want := map[string]string{"Wave": "7tv-wave", "LUL": "custom-lul", "Jam": "ffz-jam", "Dance": "bttv-dance"}
	for name, id := range want {
		if w[name].EmoteID != id {
			t.Fatalf("%s = %+v, want %s", name, w[name], id)
		}
	}
}

// Two different emotes with the same name, provider and scope: the lowest
// emote id wins, so every rebuild picks the same one.
func TestResolveChannelEmotesTieIsDeterministic(t *testing.T) {
	rows := []ChannelEmote{
		{Name: "Same", EmoteID: "b", Provider: "seventv"},
		{Name: "Same", EmoteID: "a", Provider: "seventv"},
	}
	if got := winners(resolveChannelEmotes(rows))["Same"].EmoteID; got != "a" {
		t.Fatalf("Same = %q, want a", got)
	}
}

func TestResolveChannelEmotesIgnoresInputOrder(t *testing.T) {
	rows := []ChannelEmote{
		{Name: "Clap", EmoteID: "7tv-global-clap", Provider: "seventv", IsGlobal: true},
		{Name: "Clap", EmoteID: "bttv-channel-clap", Provider: "bttv"},
		{Name: "Clap", EmoteID: "twitch-channel-clap", Provider: "twitch"},
		{Name: "Wave", EmoteID: "ffz-wave", Provider: "ffz"},
		{Name: "Wave", EmoteID: "7tv-wave", Provider: "seventv"},
		{Name: "Solo", EmoteID: "solo", Provider: "bttv", IsGlobal: true},
	}
	want := winners(resolveChannelEmotes(rows))
	r := rand.New(rand.NewSource(1))
	for i := 0; i < 20; i++ {
		shuffled := append([]ChannelEmote(nil), rows...)
		r.Shuffle(len(shuffled), func(a, b int) { shuffled[a], shuffled[b] = shuffled[b], shuffled[a] })
		if got := winners(resolveChannelEmotes(shuffled)); !reflect.DeepEqual(got, want) {
			t.Fatalf("shuffle %d: got %+v, want %+v", i, got, want)
		}
	}
}

// The winners keep the order the query returned them in, so the list API's
// order is unchanged apart from the rows that lost.
func TestResolveChannelEmotesKeepsQueryOrder(t *testing.T) {
	rows := []ChannelEmote{
		{Name: "A", EmoteID: "twitch-a", Provider: "twitch"},
		{Name: "B", EmoteID: "7tv-b-global", Provider: "seventv", IsGlobal: true},
		{Name: "C", EmoteID: "7tv-c", Provider: "seventv"},
		{Name: "B", EmoteID: "bttv-b", Provider: "bttv"},
		{Name: "D", EmoteID: "ffz-d", Provider: "ffz"},
	}
	got := resolveChannelEmotes(rows)
	var ids []string
	for _, row := range got {
		ids = append(ids, row.EmoteID)
	}
	want := []string{"twitch-a", "7tv-c", "bttv-b", "ffz-d"}
	if !reflect.DeepEqual(ids, want) {
		t.Fatalf("order = %v, want %v", ids, want)
	}
}
