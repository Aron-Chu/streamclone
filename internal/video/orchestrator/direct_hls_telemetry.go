package orchestrator

import (
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"streamclone/internal/metrics"
)

type directHLSTelemetry struct {
	mu sync.RWMutex
}

var directHLS directHLSTelemetry

func recordDirectHLSFetch(sourceURL string, duration time.Duration) {
	ms := duration.Milliseconds()
	if ms < 0 {
		ms = 0
	}
	kind := "playlist"
	lower := strings.ToLower(sourceURL)
	if strings.Contains(lower, ".ts") || strings.Contains(lower, ".m4s") {
		kind = "segment"
	}
	metrics.DirectHLSFetchDuration.WithLabelValues(kind).Observe(float64(ms) / 1000)
}

// activeTransport reports the relay's current playback transport for diagnostics.
// "webrtc" is reserved for a future WebRTC relay path; today the relay only emits HLS.
func activeTransport(probe hlsProbeResp) string {
	if llHlsEnabled() {
		return "ll-hls"
	}
	if probe.PartTarget != "" || strings.Contains(probe.PlaylistSummary, "EXT-X-PART") {
		return "ll-hls"
	}
	return "hls-mpegts"
}

func computeEndToEndLiveDelaySec(liveEdge int, probe hlsProbeResp) *float64 {
	if liveEdge <= 0 {
		return nil
	}
	segmentDuration := parseHlsTargetDuration(probe.TargetDuration)
	delay := float64(liveEdge) * segmentDuration
	if probe.PartTarget != "" {
		if partTarget, err := strconv.ParseFloat(strings.TrimSuffix(probe.PartTarget, "s"), 64); err == nil && partTarget > 0 {
			delay += partTarget
		}
	}
	out := delay
	return &out
}

func parseHlsTargetDuration(raw string) float64 {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 2
	}
	if v, err := strconv.ParseFloat(strings.TrimSuffix(raw, "s"), 64); err == nil && v > 0 {
		return v
	}
	return 2
}

func llHlsEnabled() bool {
	return strings.EqualFold(strings.TrimSpace(os.Getenv("HLS_LOW_LATENCY_ENABLED")), "true")
}
