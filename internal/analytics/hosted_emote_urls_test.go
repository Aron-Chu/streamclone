package analytics

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"streamclone/internal/analytics/heatmap"
	"streamclone/internal/emoteimage"
)

func TestRewriteHostedExtensionEmoteURLs(t *testing.T) {
	localID := "75f49395-d5fc-41da-998c-880c6d8fddcb"
	providerID := "62a3bf572b964d6cc2766004"
	base := "https://api.streampulse.stream/emotes"
	in := []ExtensionEmote{{
		ID:       localID,
		Provider: "seventv",
		ImageURL: emoteimage.URL("seventv", localID, "1x"),
	}}
	out := rewriteHostedExtensionEmoteURLs(in, map[string]string{localID: providerID}, base)
	if len(out) != 1 {
		t.Fatalf("len=%d", len(out))
	}
	want := "https://cdn.7tv.app/emote/" + providerID + "/4x.webp"
	if out[0].ImageURL != want {
		t.Fatalf("7tv lookup: got %q want %q", out[0].ImageURL, want)
	}
	ffz := []ExtensionEmote{{
		ID:       localID,
		Provider: "ffz",
		ImageURL: emoteimage.URL("ffz", localID, "1x"),
	}}
	outFFZ := rewriteHostedExtensionEmoteURLs(ffz, map[string]string{localID: "12345"}, base)
	if outFFZ[0].ImageURL != "https://cdn.frankerfacez.com/emoticon/12345/4" {
		t.Fatalf("ffz hosted: got %q", outFFZ[0].ImageURL)
	}
}

func TestHostedEmoteCDNBaseRequiresHosted(t *testing.T) {
	h := &Handler{cdnPublicBase: "https://api.streampulse.stream/emotes"}
	if h.hostedEmoteCDNBase() != "" {
		t.Fatal("expected empty when not hosted")
	}
	h.pulseHosted.Hosted = true
	if h.hostedEmoteCDNBase() != "https://api.streampulse.stream/emotes" {
		t.Fatalf("got %q", h.hostedEmoteCDNBase())
	}
}

func TestRewritePortalTopEmotesWithoutHostedMode(t *testing.T) {
	localID := "75f49395-d5fc-41da-998c-880c6d8fddcb"
	providerID := "62a3bf572b964d6cc2766004"
	in := []TopEmote{{
		ID:       localID,
		Provider: "seventv",
		Name:     "KEKW",
		ImageURL: emoteimage.URL("seventv", localID, "1x"),
		Count:    10,
	}}
	out := rewriteHostedTopEmoteURLs(in, map[string]string{localID: providerID}, nil, "", true)
	want := "https://cdn.7tv.app/emote/" + providerID + "/4x.webp"
	if out[0].ImageURL != want {
		t.Fatalf("got %q want %q", out[0].ImageURL, want)
	}
}

func TestRewriteHostedTopEmotesWithoutCDNBase(t *testing.T) {
	localID := "75f49395-d5fc-41da-998c-880c6d8fddcb"
	providerID := "62a3bf572b964d6cc2766004"
	in := []TopEmote{{
		ID:       localID,
		Provider: "seventv",
		ImageURL: emoteimage.URL("seventv", localID, "1x"),
	}}
	out := rewriteHostedTopEmoteURLs(in, map[string]string{localID: providerID}, nil, "", true)
	want := "https://cdn.7tv.app/emote/" + providerID + "/4x.webp"
	if out[0].ImageURL != want {
		t.Fatalf("got %q want %q", out[0].ImageURL, want)
	}
}

func TestRewriteHostedTopEmotesWithCDNBase(t *testing.T) {
	localID := "75f49395-d5fc-41da-998c-880c6d8fddcb"
	providerID := "62a3bf572b964d6cc2766004"
	base := "https://api.streampulse.stream/emotes"
	in := []TopEmote{{
		ID:       localID,
		Provider: "ffz",
		ImageURL: emoteimage.URL("ffz", localID, "1x"),
	}}
	out := rewriteHostedTopEmoteURLs(in, map[string]string{localID: "12345"}, nil, base, true)
	if out[0].ImageURL != "https://cdn.frankerfacez.com/emoticon/12345/4" {
		t.Fatalf("ffz hosted: got %q", out[0].ImageURL)
	}
	_ = providerID
}

func TestDecoratePortalChannelEmotesTwitchProviderID(t *testing.T) {
	h := &Handler{
		pulseHosted:   PulseHostedConfig{Hosted: true},
		cdnPublicBase: "https://api.streampulse.stream/emotes",
	}
	in := []PortalChannelEmote{{
		Provider:        "twitch",
		ProviderEmoteID: "1035663",
		Name:            "xqcL",
	}}
	out := h.decoratePortalChannelEmotes(t.Context(), in)
	want := "https://static-cdn.jtvnw.net/emoticons/v2/1035663/default/dark/2.0"
	if len(out) != 1 || out[0].ImageURL != want {
		t.Fatalf("got %#v want imageUrl %q", out, want)
	}
}

func TestDecoratePortalPeaksTwitchProviderID(t *testing.T) {
	h := &Handler{
		pulseHosted:   PulseHostedConfig{Hosted: true},
		cdnPublicBase: "https://cdn.streampulse.stream/emotes",
	}
	twitchUUID := "a1b2c3d4-e5f6-7890-abcd-ef1234567890"
	peaks := []PortalPeak{{
		TopEmotes: []ExtensionEmote{{
			ID:       twitchUUID,
			Name:     "xqcL",
			Provider: "twitch",
			ImageURL: emoteimage.LocalPath(twitchUUID, "1x"),
			Count:    5,
		}, {
			ID:       "1035663",
			Name:     "xqcL",
			Provider: "twitch",
			ImageURL: fmt.Sprintf("https://static-cdn.jtvnw.net/emoticons/v2/%s/default/dark/2.0", twitchUUID),
			Count:    3,
		}},
	}}
	out := h.decoratePortalPeaks(t.Context(), peaks)
	if out[0].TopEmotes[0].ImageURL != "" {
		t.Fatalf("twitch uuid-only emote should omit imageUrl, got %q", out[0].TopEmotes[0].ImageURL)
	}
	want := "https://static-cdn.jtvnw.net/emoticons/v2/1035663/default/dark/2.0"
	if out[0].TopEmotes[1].ImageURL != want {
		t.Fatalf("got %q want %q", out[0].TopEmotes[1].ImageURL, want)
	}
	moment, ok := hubLivePulseMomentFromPeak(HubLiveChannel{Login: "xqc"}, out[0], "", time.Time{}, "s1", 0, 0, "")
	if !ok {
		t.Fatal("expected moment")
	}
	if len(moment.TopEmotes) != 2 || moment.TopEmotes[1].ImageURL != want {
		t.Fatalf("moment top emotes=%#v", moment.TopEmotes)
	}
}

func TestDecoratePortalChannelEmotesSyncedSevenTVUsesCDNBase(t *testing.T) {
	localID := "75f49395-d5fc-41da-998c-880c6d8fddcb"
	h := &Handler{
		pulseHosted:   PulseHostedConfig{Hosted: true},
		cdnPublicBase: "https://api.streampulse.stream/emotes",
	}
	in := []PortalChannelEmote{{
		Provider:        "seventv",
		ProviderEmoteID: localID,
		Name:            "KEKW",
	}}
	out := h.decoratePortalChannelEmotes(t.Context(), in)
	want := "https://api.streampulse.stream/emotes/" + localID + "/1x.webp"
	if len(out) != 1 || out[0].ImageURL != want {
		t.Fatalf("got %#v want imageUrl %q", out, want)
	}
}

func TestDecorateExtensionVodPulseEmotesUsesCDNBase(t *testing.T) {
	localID := "75f49395-d5fc-41da-998c-880c6d8fddcb"
	h := &Handler{
		pulseHosted:   PulseHostedConfig{Hosted: true},
		cdnPublicBase: "https://cdn.streampulse.stream/emotes",
	}
	localEmote := func(count int) ExtensionEmote {
		return ExtensionEmote{
			ID:       localID,
			Name:     "KEKW",
			Provider: "seventv",
			ImageURL: emoteimage.LocalPath(localID, "1x"),
			Count:    count,
		}
	}
	payload := ExtensionVodPulseResponse{
		TopEmotes: []ExtensionEmote{localEmote(10)},
		Timeline: &ExtensionVodTimeline{Points: []ExtensionVodTimelinePoint{{
			OffsetSeconds: 60,
			TopEmotes:     []ExtensionEmote{localEmote(7)},
		}}},
		TopMoments: []ExtensionVodMoment{{
			OffsetSeconds: 120,
			TopEmotes:     []ExtensionEmote{localEmote(5)},
		}},
		BestClipCandidate: &ExtensionVodClipCandidate{
			OffsetSeconds: 120,
			TopEmotes:     []ExtensionEmote{localEmote(5)},
		},
	}

	h.decorateExtensionVodPulseEmotes(t.Context(), &payload)

	assertNoRelativeExtensionImageURL(t, "topEmotes", payload.TopEmotes)
	assertNoRelativeExtensionImageURL(t, "timeline", payload.Timeline.Points[0].TopEmotes)
	assertNoRelativeExtensionImageURL(t, "topMoments", payload.TopMoments[0].TopEmotes)
	assertNoRelativeExtensionImageURL(t, "bestClipCandidate", payload.BestClipCandidate.TopEmotes)
	want := "https://cdn.streampulse.stream/emotes/" + localID + "/1x.webp"
	if got := payload.TopEmotes[0].ImageURL; got != want {
		t.Fatalf("topEmotes imageUrl = %q, want %q", got, want)
	}
}

func TestDecorateHeatmapResponseEmotesUsesCDNBase(t *testing.T) {
	localID := "75f49395-d5fc-41da-998c-880c6d8fddcb"
	h := &Handler{
		pulseHosted:   PulseHostedConfig{Hosted: true},
		cdnPublicBase: "https://cdn.streampulse.stream/emotes",
	}
	resp := heatmap.HeatmapResponse{
		Points: []heatmap.ReplayHeatmapPoint{{
			TopEmotes: []heatmap.HeatmapEmote{{
				ID:       localID,
				Name:     "KEKW",
				Provider: "seventv",
				ImageURL: emoteimage.LocalPath(localID, "1x"),
				Count:    10,
			}},
		}},
	}
	detail := heatmap.HeatmapDetailResponse{
		Points: []heatmap.ReplayHeatmapDetailPoint{{
			ReplayHeatmapPoint: heatmap.ReplayHeatmapPoint{
				TopEmotes: []heatmap.HeatmapEmote{{
					ID:       localID,
					Name:     "KEKW",
					Provider: "seventv",
					ImageURL: emoteimage.LocalPath(localID, "1x"),
					Count:    10,
				}},
			},
		}},
	}

	h.decorateHeatmapResponseEmotes(&resp)
	h.decorateHeatmapDetailResponseEmotes(&detail)

	assertNoRelativeHeatmapImageURL(t, "compact", resp.Points[0].TopEmotes)
	assertNoRelativeHeatmapImageURL(t, "detail", detail.Points[0].TopEmotes)
	want := "https://cdn.streampulse.stream/emotes/" + localID + "/1x.webp"
	if got := resp.Points[0].TopEmotes[0].ImageURL; got != want {
		t.Fatalf("compact heatmap imageUrl = %q, want %q", got, want)
	}
	if got := detail.Points[0].TopEmotes[0].ImageURL; got != want {
		t.Fatalf("detail heatmap imageUrl = %q, want %q", got, want)
	}
}

func assertNoRelativeExtensionImageURL(t *testing.T, label string, emotes []ExtensionEmote) {
	t.Helper()
	for i, emote := range emotes {
		if strings.HasPrefix(emote.ImageURL, "/") {
			t.Fatalf("%s[%d].imageUrl starts with /: %q", label, i, emote.ImageURL)
		}
	}
}

func assertNoRelativeHeatmapImageURL(t *testing.T, label string, emotes []heatmap.HeatmapEmote) {
	t.Helper()
	for i, emote := range emotes {
		if strings.HasPrefix(emote.ImageURL, "/") {
			t.Fatalf("%s[%d].imageUrl starts with /: %q", label, i, emote.ImageURL)
		}
	}
}
