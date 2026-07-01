package analytics

import (
	"testing"

	"streamclone/internal/analytics/recap"
)

func TestHubBurstFromExtensionEmotePeakOffsetSeconds(t *testing.T) {
	burst := hubBurstFromExtensionEmote(ExtensionEmote{
		Name:     "OMEGALUL",
		Provider: "seventv",
		Count:    42,
	}, "02:00", 120, 55.5)

	if burst.PeakOffset != "02:00" {
		t.Fatalf("PeakOffset = %q, want 02:00", burst.PeakOffset)
	}
	if burst.PeakOffsetSeconds != 120 {
		t.Fatalf("PeakOffsetSeconds = %d, want 120", burst.PeakOffsetSeconds)
	}
	if burst.Code != "OMEGALUL" || burst.Count != 42 {
		t.Fatalf("unexpected burst payload: %+v", burst)
	}
}

func TestHubFeaturedBurstsFromRecapPeakOffsetSeconds(t *testing.T) {
	rec := recap.StreamRecap{
		FunniestEmoteBurst: &recap.EmoteBurst{
			OffsetSeconds: 90,
			Code:          "KEKW",
			Count:         12,
		},
	}
	peaks := []PortalPeak{{
		OffsetSeconds: 300,
		EmoteCount:    10,
		TopEmotes: []ExtensionEmote{{
			Name:     "LULW",
			Provider: "7tv",
			Count:    8,
		}},
	}}

	bursts := hubFeaturedBurstsFromRecap(rec, peaks)
	if len(bursts) < 2 {
		t.Fatalf("expected at least 2 bursts, got %d", len(bursts))
	}
	if bursts[0].Code != "KEKW" || bursts[0].PeakOffsetSeconds != 90 {
		t.Fatalf("funniest burst: %+v", bursts[0])
	}
	if bursts[1].Code != "LULW" || bursts[1].PeakOffsetSeconds != 300 {
		t.Fatalf("peak emote burst: %+v", bursts[1])
	}
}
