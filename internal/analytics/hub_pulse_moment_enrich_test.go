package analytics

import (
	"testing"

	"streamclone/internal/analytics/heatmap"
)

func TestEnrichHubPulseMomentJustWentLive(t *testing.T) {
	label, kind, tag, skip := enrichHubPulseMoment(PortalPeak{
		OffsetSeconds:  60,
		Reasons:        []string{heatmap.ReasonViewerSpike},
		ReasonLabel:    "Viewer spike",
		DominantSignal: "viewers",
		ChatCount:      5,
	}, 120, 600)
	if skip {
		t.Fatal("expected peak to remain")
	}
	if label != "Just went live" || kind != "stream_opening" {
		t.Fatalf("got (%q, %q, %q)", label, kind, tag)
	}
}

func TestEnrichHubPulseMomentSkipsWarmup(t *testing.T) {
	_, _, _, skip := enrichHubPulseMoment(PortalPeak{
		OffsetSeconds: 30,
		Reasons:       []string{heatmap.ReasonChatSpike},
		ReasonLabel:   "Chat spike",
		ChatCount:     3,
	}, 180, 600)
	if !skip {
		t.Fatal("expected warmup peak to be skipped")
	}
}

func TestEnrichHubPulseMomentGameChange(t *testing.T) {
	label, kind, _, skip := enrichHubPulseMoment(PortalPeak{
		OffsetSeconds: 600,
		Reasons:       []string{heatmap.ReasonGameChange},
		ReasonLabel:   "game change",
		ChatCount:     40,
	}, 0, 3600)
	if skip || label != "Game change" || kind != "game_change" {
		t.Fatalf("got (%q, %q, skip=%v)", label, kind, skip)
	}
}

func TestEnrichHubPulseMomentEarlyStreamTag(t *testing.T) {
	_, _, tag, skip := enrichHubPulseMoment(PortalPeak{
		OffsetSeconds:  240,
		Reasons:        []string{heatmap.ReasonSevenTVSpike},
		ReasonLabel:    "7TV emote spike",
		DominantSignal: "seventv",
		ChatCount:      80,
		EmoteCount:     40,
	}, 0, 600)
	if skip || tag != "early_stream" {
		t.Fatalf("tag = %q, skip = %v", tag, skip)
	}
}
