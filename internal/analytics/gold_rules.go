package analytics

import (
	"strings"
	"time"
)

// GoldRulesEngine decides whether a completed silver stream qualifies for gold-tier GQL chat.
type GoldRulesEngine struct {
	alwaysTracked      map[string]struct{}
	minPeakViewers     int
	minDurationMinutes int
}

// NewGoldRulesEngine builds a rules engine from config thresholds and always-tracked logins.
func NewGoldRulesEngine(alwaysTracked []string, minPeakViewers, minDurationMinutes int) *GoldRulesEngine {
	tracked := make(map[string]struct{}, len(alwaysTracked))
	for _, login := range alwaysTracked {
		if login = normalizeLogin(login); login != "" {
			tracked[login] = struct{}{}
		}
	}
	return &GoldRulesEngine{
		alwaysTracked:      tracked,
		minPeakViewers:     minPeakViewers,
		minDurationMinutes: minDurationMinutes,
	}
}

// GoldEval describes why a stream matched or missed gold rules.
type GoldEval struct {
	StreamID        string `json:"streamId"`
	Login           string `json:"login"`
	PeakViewers     int    `json:"peakViewers"`
	DurationMinutes int    `json:"durationMinutes"`
	Matched         bool   `json:"matched"`
	Reasons         []string `json:"reasons,omitempty"`
}

// Match returns true when login is always-tracked or peak/duration thresholds are met.
func (e *GoldRulesEngine) Match(login string, peakViewers, durationMinutes int) bool {
	if e == nil {
		return false
	}
	if _, ok := e.alwaysTracked[normalizeLogin(login)]; ok {
		return true
	}
	if e.minPeakViewers > 0 && peakViewers >= e.minPeakViewers {
		return true
	}
	if e.minDurationMinutes > 0 && durationMinutes >= e.minDurationMinutes {
		return true
	}
	return false
}

// Eval returns a structured dry-run result for operator tooling.
func (e *GoldRulesEngine) Eval(streamID, login string, peakViewers, durationMinutes int) GoldEval {
	out := GoldEval{
		StreamID:        strings.TrimSpace(streamID),
		Login:           normalizeLogin(login),
		PeakViewers:     peakViewers,
		DurationMinutes: durationMinutes,
	}
	if e == nil {
		return out
	}
	if _, ok := e.alwaysTracked[out.Login]; ok {
		out.Matched = true
		out.Reasons = append(out.Reasons, "always_tracked")
	}
	if e.minPeakViewers > 0 && peakViewers >= e.minPeakViewers {
		out.Matched = true
		out.Reasons = append(out.Reasons, "peak_viewers")
	}
	if e.minDurationMinutes > 0 && durationMinutes >= e.minDurationMinutes {
		out.Matched = true
		out.Reasons = append(out.Reasons, "duration_minutes")
	}
	return out
}

func streamDurationMinutes(startedAt time.Time, endedAt *time.Time) int {
	if endedAt == nil || !endedAt.After(startedAt) {
		return 0
	}
	return int(endedAt.Sub(startedAt).Minutes())
}
