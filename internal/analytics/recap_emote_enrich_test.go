package analytics

import (
	"context"
	"strings"
	"testing"

	pulserecap "streamclone/internal/analytics/recap"
)

func TestRecapEmoteEnrichCaseInsensitiveJoin(t *testing.T) {
	catalog := []TopEmote{{
		Name:     "lol",
		ID:       "emote-lol",
		Provider: "seventv",
		ImageURL: "/emotes/emote-lol/1x.webp",
		Count:    99,
	}}
	recapEmotes := []pulserecap.Emote{{Code: "LOL", Count: 12, Provider: "seventv"}}
	out := enrichRecapEmotes(recapEmotes, catalog)
	if out[0].Code != "LOL" || out[0].Count != 12 {
		t.Fatalf("recap identity changed: %+v", out[0])
	}
	if out[0].ID != "emote-lol" {
		t.Fatalf("id = %q", out[0].ID)
	}
	if out[0].ImageURL != "/emotes/emote-lol/1x.webp" {
		t.Fatalf("imageUrl = %q", out[0].ImageURL)
	}
}

func TestRecapEmoteEnrichHostedURLRewrite(t *testing.T) {
	localID := "75f49395-d5fc-41da-998c-880c6d8fddcb"
	rollups := []MinuteRollup{{
		Emotes: map[string]int{
			"seventv:" + localID + ":KEKW": 10,
		},
	}}
	h := &Handler{
		pulseHosted:   PulseHostedConfig{Hosted: true},
		cdnPublicBase: "https://api.streampulse.stream/emotes",
	}
	recap := &pulserecap.StreamRecap{
		TopEmotes: []pulserecap.Emote{{Code: "KEKW", Count: 10, Provider: "seventv"}},
	}
	h.enrichRecapTopEmotes(context.Background(), recap, rollups, "stream-1", "")
	got := recap.TopEmotes[0].ImageURL
	if !strings.HasPrefix(got, "https://api.streampulse.stream/emotes/") {
		t.Fatalf("hosted imageUrl = %q", got)
	}
	if recap.EmoteEnrichmentStatus != "complete" {
		t.Fatalf("status = %q, want complete", recap.EmoteEnrichmentStatus)
	}
}

func TestRecapEmoteEnrichMissingImageFallback(t *testing.T) {
	recapEmotes := []pulserecap.Emote{
		{Code: "KEKW", Count: 10, Provider: "seventv"},
		{Code: "UNKNOWN", Count: 3, Provider: "seventv"},
	}
	out := enrichRecapEmotes(recapEmotes, nil)
	if out[0].ID != "" || out[0].ImageURL != "" {
		t.Fatalf("unmatched should stay code-only: %+v", out[0])
	}
	if out[1].Code != "UNKNOWN" || out[1].Count != 3 {
		t.Fatalf("fallback row changed: %+v", out[1])
	}
	if got := computeEmoteEnrichmentStatus(out); got != "missing" {
		t.Fatalf("status = %q, want missing", got)
	}

	partial := enrichRecapEmotes(recapEmotes, []TopEmote{{
		Name:     "KEKW",
		ID:       "kekw-id",
		Provider: "seventv",
		ImageURL: "/emotes/kekw-id/1x.webp",
		Count:    1,
	}})
	if got := computeEmoteEnrichmentStatus(partial); got != "partial" {
		t.Fatalf("status = %q, want partial", got)
	}
}

func TestRecapEmoteEnrichSevenTVFilter(t *testing.T) {
	in := []TopEmote{
		{Name: "Kappa", ID: "304894101", Provider: "twitch", ImageURL: "https://static-cdn.jtvnw.net/emoticons/v2/304894101/default/dark/2.0", Count: 50},
		{Name: "KEKW", ID: "kekw-id", Provider: "seventv", ImageURL: "/emotes/kekw-id/1x.webp", Count: 10},
	}
	filtered := filterTopEmotesSevenTV(in)
	if len(filtered) != 1 || filtered[0].Name != "KEKW" {
		t.Fatalf("filtered = %+v", filtered)
	}

	recapEmotes := []pulserecap.Emote{{Code: "Kappa", Count: 50, Provider: "twitch"}}
	out := enrichRecapEmotes(recapEmotes, filtered)
	if out[0].ID != "" || out[0].ImageURL != "" {
		t.Fatalf("non-7tv catalog should not join: %+v", out[0])
	}
}

func TestRecapEmoteEnrichCountAuthority(t *testing.T) {
	out := enrichRecapEmotes([]pulserecap.Emote{{Code: "KEKW", Count: 42, Provider: "seventv"}}, []TopEmote{{
		Name:     "KEKW",
		ID:       "kekw-id",
		Provider: "seventv",
		ImageURL: "/emotes/kekw-id/1x.webp",
		Count:    999,
	}})
	if out[0].Count != 42 {
		t.Fatalf("count = %d, want recap authority 42", out[0].Count)
	}
}

func TestRecapEmoteEnrichSevenTVOutsideGlobalTopN(t *testing.T) {
	rollups := []MinuteRollup{{
		Emotes: map[string]int{
			"twitch:1:Kappa":         500,
			"twitch:2:LUL":           400,
			"twitch:3:OMEGALUL":      350,
			"twitch:4:PogChamp":      300,
			"twitch:5:monkaS":        280,
			"twitch:6:KEKW":          260,
			"twitch:7:PepeHands":     240,
			"twitch:8:TriHard":       220,
			"twitch:9:4Head":         200,
			"twitch:10:BabyRage":     180,
			"seventv:clap-id:Clap":   112,
			"seventv:ez-id:EZ":       24,
			"seventv:gayge-id:Gayge": 6,
		},
	}}
	h := &Handler{
		pulseHosted:   PulseHostedConfig{Hosted: true},
		cdnPublicBase: "https://api.streampulse.stream/emotes",
	}
	recap := &pulserecap.StreamRecap{
		TopEmotes: []pulserecap.Emote{
			{Code: "Clap", Count: 112, Provider: "seventv"},
			{Code: "EZ", Count: 24, Provider: "seventv"},
			{Code: "Gayge", Count: 6, Provider: "seventv"},
		},
	}
	h.enrichRecapTopEmotes(context.Background(), recap, rollups, "stream-1", "")
	if recap.TopEmotes[1].ID != "ez-id" {
		t.Fatalf("EZ id = %q, want ez-id", recap.TopEmotes[1].ID)
	}
	if recap.TopEmotes[2].ID != "gayge-id" {
		t.Fatalf("Gayge id = %q, want gayge-id", recap.TopEmotes[2].ID)
	}
	if recap.EmoteEnrichmentStatus != "complete" {
		t.Fatalf("status = %q, want complete", recap.EmoteEnrichmentStatus)
	}
}

func TestRecapEmoteEnrichMomentTopEmotes(t *testing.T) {
	h := &Handler{
		pulseHosted:   PulseHostedConfig{Hosted: true},
		cdnPublicBase: "https://api.streampulse.stream/emotes",
	}
	rollups := []MinuteRollup{{
		Emotes: map[string]int{
			"seventv:kekw-id:KEKW": 10,
		},
	}}
	recap := &pulserecap.StreamRecap{
		TopEmotes: []pulserecap.Emote{{Code: "KEKW", Count: 10, Provider: "seventv"}},
		TopMoments: []pulserecap.Moment{{
			OffsetSeconds: 120,
			Score:         80,
			Reasons:       []string{"emote_spike"},
			TopEmotes:     []pulserecap.Emote{{Code: "KEKW", Count: 8, Provider: "seventv"}},
		}},
	}
	h.enrichRecapTopEmotes(context.Background(), recap, rollups, "stream-1", "")
	if recap.TopMoments[0].TopEmotes[0].ID != "kekw-id" {
		t.Fatalf("moment emote id = %q, want kekw-id", recap.TopMoments[0].TopEmotes[0].ID)
	}
	if strings.TrimSpace(recap.TopMoments[0].TopEmotes[0].ImageURL) == "" {
		t.Fatalf("moment imageUrl empty")
	}
}

func TestRecapEmoteEnrichCompleteStatus(t *testing.T) {
	emotes := []pulserecap.Emote{
		{Code: "A", Count: 1, Provider: "seventv", ID: "a", ImageURL: "/emotes/a/1x.webp"},
		{Code: "B", Count: 2, Provider: "seventv", ID: "b"},
	}
	if got := computeEmoteEnrichmentStatus(emotes); got != "complete" {
		t.Fatalf("status = %q, want complete", got)
	}
}
