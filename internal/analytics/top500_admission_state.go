package analytics

import (
	"sort"
	"sync"
	"time"
)

const topRosterAdmissionHistoryLimit = 256

// TopRosterAdmissionAttempt captures the latest admission poll outcome for one login.
type TopRosterAdmissionAttempt struct {
	Login             string    `json:"login"`
	Rank              int       `json:"rank"`
	StreamID          string    `json:"streamId,omitempty"`
	SampledAt         time.Time `json:"sampledAt,omitempty"`
	AttemptedAt       time.Time `json:"attemptedAt"`
	Outcome           string    `json:"outcome"`
	Message           string    `json:"message,omitempty"`
	CollectorTracking bool      `json:"collectorTracking"`
	ActiveCollectors  int       `json:"activeCollectors"`
	MaxCollectors     int       `json:"maxCollectors"`
}

type topRosterAdmissionRegistry struct {
	mu      sync.RWMutex
	byLogin map[string]TopRosterAdmissionAttempt
	order   []string
}

var globalTopRosterAdmissionRegistry topRosterAdmissionRegistry

func recordTopRosterAdmissionAttempt(attempt TopRosterAdmissionAttempt) {
	if attempt.Login == "" {
		return
	}
	if attempt.AttemptedAt.IsZero() {
		attempt.AttemptedAt = time.Now().UTC()
	}
	globalTopRosterAdmissionRegistry.record(attempt)
}

func (r *topRosterAdmissionRegistry) record(attempt TopRosterAdmissionAttempt) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.byLogin == nil {
		r.byLogin = make(map[string]TopRosterAdmissionAttempt, topRosterAdmissionHistoryLimit)
	}
	if _, exists := r.byLogin[attempt.Login]; !exists {
		r.order = append(r.order, attempt.Login)
	}
	r.byLogin[attempt.Login] = attempt
	for len(r.order) > topRosterAdmissionHistoryLimit {
		oldest := r.order[0]
		r.order = r.order[1:]
		delete(r.byLogin, oldest)
	}
}

func getTopRosterAdmissionAttempt(login string) (TopRosterAdmissionAttempt, bool) {
	return globalTopRosterAdmissionRegistry.get(normalizeLogin(login))
}

func snapshotTopRosterAdmissionAttempts() []TopRosterAdmissionAttempt {
	return globalTopRosterAdmissionRegistry.snapshot()
}

func (r *topRosterAdmissionRegistry) get(login string) (TopRosterAdmissionAttempt, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	attempt, ok := r.byLogin[login]
	return attempt, ok
}

func (r *topRosterAdmissionRegistry) snapshot() []TopRosterAdmissionAttempt {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if len(r.order) == 0 {
		return nil
	}
	out := make([]TopRosterAdmissionAttempt, 0, len(r.order))
	for _, login := range r.order {
		if attempt, ok := r.byLogin[login]; ok {
			out = append(out, attempt)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].AttemptedAt.Equal(out[j].AttemptedAt) {
			return out[i].Rank < out[j].Rank
		}
		return out[i].AttemptedAt.After(out[j].AttemptedAt)
	})
	return out
}
