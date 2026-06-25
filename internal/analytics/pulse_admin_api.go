package analytics

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/redis/go-redis/v9"

	"streamclone/internal/config"
)

type AdminPulseRegistryEntry struct {
	Login           string    `json:"login"`
	PrincipalRefs   int       `json:"principalRefs"`
	PoolAlwaysTrack bool      `json:"poolAlwaysTrack"`
	WatchPriority   int       `json:"watchPriority"`
	LastViewedAt    time.Time `json:"lastViewedAt,omitempty"`
	AddedAt         time.Time `json:"addedAt,omitempty"`
	CurrentStreamID string    `json:"currentStreamId,omitempty"`
}

type AdminPulseRegistryResponse struct {
	Active          int                       `json:"active"`
	Max             int                       `json:"max"`
	TrackedChannels []AdminPulseRegistryEntry `json:"trackedChannels"`
	AlwaysTracked   []string                  `json:"alwaysTracked"`
}

type AdminPulseJobsResponse struct {
	Active int                `json:"active"`
	Max    int                `json:"max"`
	Jobs   []PulseBackfillJob `json:"jobs"`
}

type AdminPulseAbuseResponse struct {
	WatchRateLimitPerMin     int `json:"watchRateLimitPerMin"`
	BackfillRateLimitPerHour int `json:"backfillRateLimitPerHour"`
	SummaryRateLimitPerMin   int `json:"summaryRateLimitPerMin"`
	RateLimitKeysSampled     int `json:"rateLimitKeysSampled,omitempty"`
	Note                     string `json:"note,omitempty"`
}

func (h *Handler) AdminPulseRoutes(r chi.Router, cfg config.Config) {
	r.Route("/v1/admin/pulse", func(r chi.Router) {
		r.Use(func(next http.Handler) http.Handler {
			return PulseAdminAuthMiddleware(cfg, h.pulseHosted.Hosted, next)
		})
		r.Get("/health", h.adminPulseHealth)
		r.Get("/registry", h.adminPulseRegistry)
		r.Get("/jobs", h.adminPulseJobs)
		r.Get("/abuse", h.adminPulseAbuse)
		r.Post("/tracking/{login}/evict", h.adminPulseEvictChannel)
		r.Post("/jobs/{jobId}/cancel", h.adminPulseCancelJob)
		r.Post("/vod-resolution/{streamID}/retry", h.adminPulseVodRetry)
	})
}

func (h *Handler) adminPulseRegistry(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, h.adminPulseRegistryPayload())
}

func (h *Handler) adminPulseRegistryPayload() AdminPulseRegistryResponse {
	if h.collector == nil {
		return AdminPulseRegistryResponse{}
	}
	snap := h.collector.AdminTrackingRegistry()
	return AdminPulseRegistryResponse{
		Active:          snap.Active,
		Max:             snap.Max,
		TrackedChannels: snap.Entries,
		AlwaysTracked:   snap.AlwaysTracked,
	}
}

func (h *Handler) adminPulseJobs(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, h.adminPulseJobsPayload())
}

func (h *Handler) adminPulseJobsPayload() AdminPulseJobsResponse {
	if h.pulseBackfill == nil {
		return AdminPulseJobsResponse{}
	}
	jobs := h.pulseBackfill.ListJobs(false)
	snap := h.pulseBackfill.Snapshot()
	return AdminPulseJobsResponse{
		Active: snap.Active,
		Max:    snap.Max,
		Jobs:   jobs,
	}
}

func (h *Handler) adminPulseAbuse(w http.ResponseWriter, r *http.Request) {
	resp := AdminPulseAbuseResponse{
		WatchRateLimitPerMin:     h.pulseHosted.WatchRatePerMin,
		BackfillRateLimitPerHour: h.pulseHosted.BackfillRatePerHour,
		SummaryRateLimitPerMin:   pulseSummaryRateLimitPerMin,
	}
	if h.rdb != nil {
		if n, err := countRedisKeys(r.Context(), h.rdb, "sp:rl:*"); err == nil {
			resp.RateLimitKeysSampled = n
		} else {
			resp.Note = "redis rate-limit key scan unavailable"
		}
	} else {
		resp.Note = "redis not configured — rate-limit counters unavailable"
	}
	writeJSON(w, http.StatusOK, resp)
}

func countRedisKeys(ctx context.Context, rdb *redis.Client, pattern string) (int, error) {
	var n int
	var cursor uint64
	for {
		keys, next, err := rdb.Scan(ctx, cursor, pattern, 100).Result()
		if err != nil {
			return n, err
		}
		n += len(keys)
		cursor = next
		if cursor == 0 || n >= 500 {
			break
		}
	}
	return n, nil
}

func (h *Handler) adminPulseEvictChannel(w http.ResponseWriter, r *http.Request) {
	login, ok := validLogin(chi.URLParam(r, "login"))
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_channel"})
		return
	}
	if h.collector == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "collector_unavailable"})
		return
	}
	op, _ := pulseOperatorFromContext(r.Context())
	active, evicted := h.collector.EvictChannel(login)
	if evicted {
		slog.Info("admin pulse channel evicted",
			"login", login,
			"operator", op.Email,
			"operator_source", op.Source,
			"active_after", active,
		)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"login":   login,
		"evicted": evicted,
		"active":  active,
		"max":     h.collector.TrackingSnapshot().Max,
	})
}

func (h *Handler) adminPulseCancelJob(w http.ResponseWriter, r *http.Request) {
	jobID := strings.TrimSpace(chi.URLParam(r, "jobId"))
	if jobID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_job"})
		return
	}
	if h.pulseBackfill == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "backfill_unavailable"})
		return
	}
	op, _ := pulseOperatorFromContext(r.Context())
	job, err := h.pulseBackfill.CancelJob(jobID)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}
	slog.Info("admin pulse backfill cancelled",
		"job_id", jobID,
		"login", job.Login,
		"operator", op.Email,
		"operator_source", op.Source,
	)
	writeJSON(w, http.StatusOK, job)
}

// AdminTrackingRegistrySnapshot is operator-safe tracking state for admin registry.
type AdminTrackingRegistrySnapshot struct {
	Active        int
	Max           int
	AlwaysTracked []string
	Entries       []AdminPulseRegistryEntry
}

func (c *Collector) AdminTrackingRegistry() AdminTrackingRegistrySnapshot {
	if c == nil {
		return AdminTrackingRegistrySnapshot{}
	}
	now := time.Now().UTC()
	c.mu.Lock()
	defer c.mu.Unlock()
	entries := make([]AdminPulseRegistryEntry, 0, len(c.tracked))
	for login, tc := range c.tracked {
		refs := 0
		for _, n := range tc.refCounts {
			refs += n
		}
		entries = append(entries, AdminPulseRegistryEntry{
			Login:           login,
			PrincipalRefs:   refs,
			PoolAlwaysTrack: tc.poolAlwaysTrack,
			WatchPriority:   tc.watchPriority,
			LastViewedAt:    tc.lastViewedAt,
			AddedAt:         tc.addedAt,
			CurrentStreamID: tc.currentStreamID,
		})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Login < entries[j].Login })
	always := make([]string, 0, len(c.alwaysTracked))
	for login := range c.alwaysTracked {
		always = append(always, login)
	}
	sort.Strings(always)
	_ = now
	return AdminTrackingRegistrySnapshot{
		Active:        len(c.tracked),
		Max:           c.maxTracked,
		AlwaysTracked: always,
		Entries:       entries,
	}
}

// EvictChannel removes a channel from the tracking pool (operator action).
func (c *Collector) EvictChannel(login string) (active int, evicted bool) {
	if c == nil {
		return 0, false
	}
	login = normalizeLogin(login)
	c.mu.Lock()
	_, ok := c.tracked[login]
	if !ok {
		active = len(c.tracked)
		c.mu.Unlock()
		return active, false
	}
	delete(c.tracked, login)
	delete(c.alwaysTracked, login)
	active = len(c.tracked)
	c.mu.Unlock()
	if c.irc != nil {
		c.irc.Part(context.Background(), login)
	}
	return active, true
}

func (m *PulseBackfillManager) ListJobs(includeTerminal bool) []PulseBackfillJob {
	if m == nil {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]PulseBackfillJob, 0, len(m.jobs))
	for _, job := range m.jobs {
		if job == nil {
			continue
		}
		if !includeTerminal && isPulseBackfillTerminal(job.Status) {
			continue
		}
		copy := *job
		out = append(out, copy)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].UpdatedAt.After(out[j].UpdatedAt)
	})
	return out
}

func (m *PulseBackfillManager) CancelJob(jobID string) (*PulseBackfillJob, error) {
	if m == nil {
		return nil, fmt.Errorf("backfill_unavailable")
	}
	jobID = strings.TrimSpace(jobID)
	m.mu.Lock()
	job, ok := m.jobs[jobID]
	if !ok || job == nil {
		m.mu.Unlock()
		return nil, fmt.Errorf("job_not_found")
	}
	if isPulseBackfillTerminal(job.Status) {
		m.mu.Unlock()
		return nil, fmt.Errorf("job_already_terminal")
	}
	job.Status = PulseBackfillCancelled
	job.Message = "cancelled by operator"
	job.UpdatedAt = time.Now().UTC()
	streamID := job.StreamID
	copy := *job
	m.mu.Unlock()
	m.finishJob(streamID, jobID)
	return &copy, nil
}
