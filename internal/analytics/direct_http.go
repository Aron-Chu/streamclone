package analytics

import (
	"sync"
	"time"
)

const (
	directHTTPWindowSize     = 100
	directHTTPMinSuccessRate = 0.10
	directHTTPDisableFor     = time.Hour
)

type directHTTPTelemetry struct {
	mu            sync.Mutex
	attempts      []bool
	disabledUntil time.Time
}

func (t *directHTTPTelemetry) record(success bool) {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.attempts = append(t.attempts, success)
	if len(t.attempts) > directHTTPWindowSize {
		t.attempts = t.attempts[len(t.attempts)-directHTTPWindowSize:]
	}
	if len(t.attempts) < directHTTPWindowSize {
		return
	}
	successes := 0
	for _, ok := range t.attempts {
		if ok {
			successes++
		}
	}
	if float64(successes)/float64(len(t.attempts)) < directHTTPMinSuccessRate {
		t.disabledUntil = time.Now().Add(directHTTPDisableFor)
		t.attempts = nil
	}
}

func (t *directHTTPTelemetry) allowed() bool {
	if t == nil {
		return true
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.disabledUntil.IsZero() || time.Now().After(t.disabledUntil)
}
