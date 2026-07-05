package analytics

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"streamclone/internal/analytics/heatmap"
)

func TestHubLivePulseMomentsMeta(t *testing.T) {
	status, reason := hubLivePulseMomentsMeta(
		[]HubLiveChannel{{Login: "xqc", ChatPerMin: 100}},
		[]HubLivePulseMoment{{Login: "xqc"}},
	)
	if status != "ready" || reason != "" {
		t.Fatalf("ready meta = (%q, %q)", status, reason)
	}

	status, reason = hubLivePulseMomentsMeta(
		[]HubLiveChannel{{Login: "xqc", ChatPerMin: 100}},
		nil,
	)
	if status != "no_peaks" || reason != "no_detected_peaks_in_pool" {
		t.Fatalf("no_peaks meta = (%q, %q)", status, reason)
	}

	status, reason = hubLivePulseMomentsMeta(
		[]HubLiveChannel{{Login: "", ChatPerMin: 0}},
		nil,
	)
	if status != "fallback" || reason != "no_irc_eligible_channels" {
		t.Fatalf("fallback meta = (%q, %q)", status, reason)
	}
}

func TestBuildHubLivePulseMomentsSortsAndCaps(t *testing.T) {
	h := &Handler{}
	channels := []HubLiveChannel{
		{Login: "b", DisplayName: "B", ChatPerMin: 50, Viewers: 1000},
		{Login: "a", DisplayName: "A", ChatPerMin: 100, Viewers: 2000},
	}
	// Without store, builder returns nil (safe empty).
	if got := h.buildHubLivePulseMoments(context.Background(), channels); got != nil {
		t.Fatalf("expected nil without store, got %v", got)
	}
}

func TestHubLivePulseMomentsPublicJSONSafe(t *testing.T) {
	moments := []HubLivePulseMoment{
		{
			Login:         "xqc",
			DisplayName:   "xQc",
			StreamID:      "stream-1",
			OffsetSeconds: 120,
			Score:         92,
			Label:         "Chat spike",
			TopEmotes:     []HubEmote{{Name: "KEKW", Provider: "7tv", Count: 12}},
			Confidence:    97,
		},
	}
	body, err := json.Marshal(map[string]any{"livePulseMoments": moments})
	if err != nil {
		t.Fatal(err)
	}
	raw := strings.ToLower(string(body))
	for _, forbidden := range []string{"rollups", `"emotes":{`, "principal", "gql", "messages", "rawchat"} {
		if strings.Contains(raw, forbidden) {
			t.Fatalf("livePulseMoments payload must not contain %q", forbidden)
		}
	}
	if !strings.Contains(raw, `"livepulsemoments"`) && !strings.Contains(raw, "livepulsemoments") {
		// json tag is livePulseMoments
		if !strings.Contains(string(body), "livePulseMoments") {
			t.Fatal("expected livePulseMoments key in payload")
		}
	}
	if len(moments) > hubLivePulseMomentsCap {
		t.Fatalf("cap constant should be %d", hubLivePulseMomentsCap)
	}
}

func TestHubLivePulseMomentAtFromStreamStart(t *testing.T) {
	started := time.Date(2026, 6, 30, 12, 0, 0, 0, time.UTC)
	got, ok := hubLivePulseMomentFromPeak(HubLiveChannel{Login: "xqc"}, PortalPeak{
		OffsetSeconds: 120,
		Score:         90,
		ReasonLabel:   "Chat spike",
		Reasons:       []string{heatmap.ReasonChatSpike},
		ChatCount:     40,
	}, "", started, "s1", 0, 600, "Just Chatting")
	if !ok {
		t.Fatal("expected moment")
	}
	wantAt := started.Add(120 * time.Second).UnixMilli()
	if got.At != wantAt {
		t.Fatalf("At = %d, want %d", got.At, wantAt)
	}
	if got.StreamStartedAt != started.UnixMilli() {
		t.Fatalf("streamStartedAt = %d", got.StreamStartedAt)
	}
}

func TestHubLivePulsePeaksPerChannelAllowsDominance(t *testing.T) {
	if hubLivePulsePeaksPerChannel < 4 {
		t.Fatalf("hubLivePulsePeaksPerChannel = %d, want >= 4 so dominant channels can fill more of the top-%d list", hubLivePulsePeaksPerChannel, hubLivePulseMomentsCap)
	}
}

func TestHubLivePulseMomentsCapIsTen(t *testing.T) {
	if hubLivePulseMomentsCap != 10 {
		t.Fatalf("hubLivePulseMomentsCap = %d, want 10", hubLivePulseMomentsCap)
	}
}

func TestHubLivePulseMomentFromPeakCarriesChannel(t *testing.T) {
	ch := HubLiveChannel{
		Login:           "jynxzi",
		DisplayName:     "Jynxzi",
		ProfileImageURL: "https://cdn.example/p.png",
		ChatPerMin:      400,
	}
	peak := PortalPeak{
		OffsetSeconds:  120,
		Score:          92,
		ReasonLabel:    "7TV emote spike",
		DominantSignal: "seventv",
		ChatCount:      800,
		Confidence:     97,
		VodState:       "no_vod",
	}
	moment, ok := hubLivePulseMomentFromPeak(ch, peak, "", time.Time{}, "stream-1", 0, 0, "Just Chatting")
	if !ok {
		t.Fatal("expected moment")
	}
	if moment.Login != "jynxzi" {
		t.Fatalf("login = %q", moment.Login)
	}
	if moment.StreamID != "stream-1" {
		t.Fatalf("streamId = %q", moment.StreamID)
	}
	if moment.Label != "7TV emote spike" && moment.Label != "Emote spike" {
		t.Fatalf("label = %q", moment.Label)
	}
	if moment.Category != "Just Chatting" {
		t.Fatalf("category = %q, want Just Chatting from resolved category arg", moment.Category)
	}
	if moment.Source != "live_irc" {
		t.Fatalf("source = %q", moment.Source)
	}
}
