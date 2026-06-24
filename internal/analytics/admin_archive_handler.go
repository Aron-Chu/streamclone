package analytics

import (
	"context"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"streamclone/internal/archive/jobtracker"
	"streamclone/internal/config"
)

type AdminArchiveHandler struct {
	pool    *pgxpool.Pool
	jobsCLI *JobsCLI
	tracker *jobtracker.Tracker
}

func NewAdminArchiveHandler(pool *pgxpool.Pool, cfg config.Config) *AdminArchiveHandler {
	tracker := jobtracker.NewTracker(pool, cfg.ArchiveJobHeartbeatInterval, cfg.ArchiveJobStaleAfter, cfg.ArchiveJobEventLogEnabled)
	return &AdminArchiveHandler{
		pool:    pool,
		jobsCLI: NewJobsCLI(pool, cfg.ArchiveJobHeartbeatInterval, cfg.ArchiveJobStaleAfter, cfg.ArchiveJobEventLogEnabled),
		tracker: tracker,
	}
}

func (h *AdminArchiveHandler) Routes(r chi.Router, cfg config.Config) {
	r.Route("/v1/admin/archive", func(r chi.Router) {
		r.Use(func(next http.Handler) http.Handler {
			return AdminArchiveAuthMiddleware(cfg, next)
		})
		r.Get("/jobs", h.listJobs)
		r.Get("/jobs/{jobID}", h.getJob)
		r.Post("/jobs/{jobID}/retry-failed", h.retryFailed)
		r.Post("/jobs/{jobID}/resume", h.resumeJob)
		r.Post("/jobs/{jobID}/cancel", h.cancelJob)
		r.Get("/coverage/summary", h.coverageSummary)
	})
}

func (h *AdminArchiveHandler) listJobs(w http.ResponseWriter, r *http.Request) {
	status := r.URL.Query().Get("status")
	rows, err := h.jobsCLI.List(r.Context(), status, 100)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"jobs": rows})
}

func (h *AdminArchiveHandler) getJob(w http.ResponseWriter, r *http.Request) {
	jobID := chi.URLParam(r, "jobID")
	resp, err := h.jobsCLI.Show(r.Context(), jobID)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not_found"})
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *AdminArchiveHandler) retryFailed(w http.ResponseWriter, r *http.Request) {
	jobID := chi.URLParam(r, "jobID")
	if err := h.jobsCLI.RetryFailed(r.Context(), jobID); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeAuditEvent(r.Context(), h.tracker, jobID, "retry_failed", "operator retry-failed")
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *AdminArchiveHandler) resumeJob(w http.ResponseWriter, r *http.Request) {
	jobID := chi.URLParam(r, "jobID")
	if err := h.jobsCLI.Resume(r.Context(), jobID); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeAuditEvent(r.Context(), h.tracker, jobID, "job_resumed", "operator resume")
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *AdminArchiveHandler) cancelJob(w http.ResponseWriter, r *http.Request) {
	jobID := chi.URLParam(r, "jobID")
	if err := h.jobsCLI.Cancel(r.Context(), jobID); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeAuditEvent(r.Context(), h.tracker, jobID, "job_cancelled", "operator cancel")
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *AdminArchiveHandler) coverageSummary(w http.ResponseWriter, r *http.Request) {
	since, err := ParseCoverageSince(r.URL.Query().Get("since"), time.Now().UTC())
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_since"})
		return
	}
	// Tier0 roster size default for summary.
	report, err := BuildCoverageReport(r.Context(), h.pool, nil, since, 200)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, report)
}

func writeAuditEvent(ctx context.Context, t *jobtracker.Tracker, jobID, eventType, message string) {
	if t == nil {
		return
	}
	_ = t.LogOperatorEvent(ctx, jobID, eventType, message)
}
