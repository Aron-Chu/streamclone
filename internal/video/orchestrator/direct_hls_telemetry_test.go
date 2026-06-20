package orchestrator

import "testing"

func TestParseHlsTargetDuration(t *testing.T) {
	cases := map[string]float64{
		"2":      2,
		"2s":     2,
		"1":      1,
		"0.5":    0.5,
		"":       2, // empty falls back to the mpegts default window
		"bogus":  2,
		"0":      2, // non-positive falls back to default
		"6.000":  6,
	}
	for raw, want := range cases {
		if got := parseHlsTargetDuration(raw); got != want {
			t.Fatalf("parseHlsTargetDuration(%q) = %v, want %v", raw, got, want)
		}
	}
}

func TestActiveTransport(t *testing.T) {
	t.Run("mpegts when no parts and flag off", func(t *testing.T) {
		t.Setenv("HLS_LOW_LATENCY_ENABLED", "false")
		if got := activeTransport(hlsProbeResp{TargetDuration: "2"}); got != "hls-mpegts" {
			t.Fatalf("activeTransport = %q, want hls-mpegts", got)
		}
	})
	t.Run("ll-hls when playlist advertises parts", func(t *testing.T) {
		t.Setenv("HLS_LOW_LATENCY_ENABLED", "false")
		if got := activeTransport(hlsProbeResp{PartTarget: "0.2"}); got != "ll-hls" {
			t.Fatalf("activeTransport(part target) = %q, want ll-hls", got)
		}
		if got := activeTransport(hlsProbeResp{PlaylistSummary: "#EXT-X-PART:DURATION=0.2"}); got != "ll-hls" {
			t.Fatalf("activeTransport(part summary) = %q, want ll-hls", got)
		}
	})
	t.Run("ll-hls when flag forces it", func(t *testing.T) {
		t.Setenv("HLS_LOW_LATENCY_ENABLED", "true")
		if got := activeTransport(hlsProbeResp{TargetDuration: "2"}); got != "ll-hls" {
			t.Fatalf("activeTransport(flag) = %q, want ll-hls", got)
		}
	})
}

func TestComputeEndToEndLiveDelaySec(t *testing.T) {
	if got := computeEndToEndLiveDelaySec(0, hlsProbeResp{TargetDuration: "2"}); got != nil {
		t.Fatalf("liveEdge<=0 should yield nil, got %v", *got)
	}

	got := computeEndToEndLiveDelaySec(3, hlsProbeResp{TargetDuration: "2"})
	if got == nil || *got != 6 {
		t.Fatalf("liveEdge=3 x 2s want 6, got %v", got)
	}

	withPart := computeEndToEndLiveDelaySec(3, hlsProbeResp{TargetDuration: "2", PartTarget: "0.2s"})
	if withPart == nil || *withPart < 6.19 || *withPart > 6.21 {
		t.Fatalf("expected 6 + 0.2 part target ~= 6.2, got %v", withPart)
	}
}
