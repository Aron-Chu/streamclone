package orchestrator

import (
	"os"
	"strconv"
	"strings"
	"time"
)

func parseLatencyMode(raw string) (mode string, liveEdge int) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "instant":
		return "instant", 1
	case "fast":
		return "fast", 2
	default:
		return "stable", 3
	}
}

func latencyModeLabel(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "instant":
		return "instant"
	case "fast":
		return "fast"
	default:
		return "stable"
	}
}

func hlsFastStart(latencyMode string) bool {
	if strings.EqualFold(strings.TrimSpace(latencyMode), "stable") {
		return false
	}
	if v := strings.TrimSpace(os.Getenv("HLS_FAST_START_SEGMENT_COUNT")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return true
		}
	}
	return strings.EqualFold(strings.TrimSpace(latencyMode), "instant") ||
		strings.EqualFold(strings.TrimSpace(latencyMode), "fast")
}

func hlsProbeTuning(latencyMode string) (stabilityWindow time.Duration, skipVariant bool) {
	if !hlsFastStart(latencyMode) {
		return hlsStabilityWindow, false
	}
	if v := strings.TrimSpace(os.Getenv("HLS_FAST_START_SEGMENT_COUNT")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return 0, true
		}
	}
	return 0, false
}
