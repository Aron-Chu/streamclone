package analytics

import (
	"context"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
)

type OpsReadinessResponse struct {
	Top100ReadinessReport
}

type OpsLaunchSnapshotResponse struct {
	GeneratedAt time.Time                `json:"generatedAt"`
	Version     string                   `json:"version,omitempty"`
	Health      AdminPulseHealthResponse `json:"health"`
	Readiness   Top100ReadinessSummary   `json:"readiness"`
	Admission   OpsReadinessAdmission    `json:"admission"`
	Redis       OpsRedisSnapshot         `json:"redis"`
	Postgres    HostedDatabaseFootprint  `json:"postgres"`
	Retention   HostedRetentionConfig    `json:"retention"`
}

type OpsReadinessAdmission struct {
	Enabled         bool `json:"enabled"`
	CollectorActive int  `json:"collectorActive"`
	CollectorMax    int  `json:"collectorMax"`
	TopN            int  `json:"topN"`
}

type OpsRedisSnapshot struct {
	Available           bool   `json:"available"`
	UsedMemoryHuman     string `json:"usedMemoryHuman,omitempty"`
	MaxMemoryHuman      string `json:"maxMemoryHuman,omitempty"`
	MaxMemoryPolicy     string `json:"maxMemoryPolicy,omitempty"`
	ConnectedClients    int64  `json:"connectedClients,omitempty"`
	RejectedConnections int64  `json:"rejectedConnections,omitempty"`
	TotalKeys           int64  `json:"totalKeys,omitempty"`
	ExpiringKeys        int64  `json:"expiringKeys,omitempty"`
	Error               string `json:"error,omitempty"`
}

func (h *Handler) OpsRoutes(r chi.Router) {
	r.Route("/v1/internal/ops", func(r chi.Router) {
		r.Use(OpsProbeAuthMiddleware(h.appConfig, h.pulseHosted.Hosted))
		r.Get("/readiness", h.opsReadiness)
		r.Get("/launch-snapshot", h.opsLaunchSnapshot)
		r.Get("/retention/report", h.opsRetentionReport)
	})
}

func (h *Handler) opsReadiness(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "store_unavailable"})
		return
	}
	topN := DefaultTop500MetadataTopN
	if raw := r.URL.Query().Get("topN"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			topN = parsed
		}
	}
	includeRows := strings.EqualFold(r.URL.Query().Get("includeRows"), "true")
	opts := ReadinessReportOptions{SkipRollups: !includeRows}
	admissionEnabled := h.corpusRuntimeConfig().LiveAdmissionEnabled
	report, err := h.buildTop100ReadinessReport(r.Context(), topN, admissionEnabled, opts)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if !includeRows {
		report.Rows = nil
		report.RecentAdmissions = nil
	}
	writeJSON(w, http.StatusOK, OpsReadinessResponse{Top100ReadinessReport: report})
}

func (h *Handler) opsLaunchSnapshot(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	topN := DefaultTop500MetadataTopN
	if raw := r.URL.Query().Get("topN"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			topN = parsed
		}
	}
	admissionEnabled := h.corpusRuntimeConfig().LiveAdmissionEnabled
	summary := Top100ReadinessSummary{}
	collectorActive, collectorMax := 0, 0
	if h.store != nil {
		report, err := h.buildTop100ReadinessReport(ctx, topN, admissionEnabled, ReadinessReportOptions{SkipRollups: true})
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		summary = report.Summary
		collectorActive = report.CollectorActive
		collectorMax = report.CollectorMax
	}
	footprint := HostedDatabaseFootprint{}
	if h.store != nil {
		if fp, err := h.store.HostedDatabaseFootprint(ctx); err == nil {
			footprint = fp
		} else {
			footprint.Error = err.Error()
		}
	}
	writeJSON(w, http.StatusOK, OpsLaunchSnapshotResponse{
		GeneratedAt: time.Now().UTC(),
		Version:     opsLaunchVersion(),
		Health:      h.adminPulseHealthPayload(),
		Readiness:   summary,
		Admission: OpsReadinessAdmission{
			Enabled:         admissionEnabled,
			CollectorActive: collectorActive,
			CollectorMax:    collectorMax,
			TopN:            topN,
		},
		Redis:     h.opsRedisSnapshot(ctx),
		Postgres:  footprint,
		Retention: h.hostedRetentionConfig(),
	})
}

func (h *Handler) opsRetentionReport(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	report := HostedRetentionReport{
		GeneratedAt: time.Now().UTC(),
		Config:      h.hostedRetentionConfig(),
	}
	if h.store == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "store_unavailable"})
		return
	}
	footprint, err := h.store.HostedDatabaseFootprint(ctx)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	report.Footprint = footprint
	writeJSON(w, http.StatusOK, report)
}

func opsLaunchVersion() string {
	for _, key := range []string{"STREAMCLONE_VERSION", "VERSION"} {
		if v := strings.TrimSpace(os.Getenv(key)); v != "" {
			return v
		}
	}
	return ""
}

func (h *Handler) opsRedisSnapshot(ctx context.Context) OpsRedisSnapshot {
	if h == nil || h.rdb == nil {
		return OpsRedisSnapshot{Available: false, Error: "redis_unavailable"}
	}
	snap := OpsRedisSnapshot{Available: true}
	info, err := h.rdb.Info(ctx, "memory", "stats", "keyspace").Result()
	if err != nil {
		snap.Error = err.Error()
		return snap
	}
	for _, line := range strings.Split(info, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		key, val := parts[0], strings.TrimSpace(parts[1])
		switch key {
		case "used_memory_human":
			snap.UsedMemoryHuman = val
		case "maxmemory_human":
			snap.MaxMemoryHuman = val
		case "maxmemory_policy":
			snap.MaxMemoryPolicy = val
		case "connected_clients":
			snap.ConnectedClients, _ = strconv.ParseInt(val, 10, 64)
		case "rejected_connections":
			snap.RejectedConnections, _ = strconv.ParseInt(val, 10, 64)
		case "db0":
			// db0:keys=13555,expires=245,avg_ttl=12345
			for _, segment := range strings.Split(val, ",") {
				segment = strings.TrimSpace(segment)
				if strings.HasPrefix(segment, "keys=") {
					snap.TotalKeys, _ = strconv.ParseInt(strings.TrimPrefix(segment, "keys="), 10, 64)
				}
				if strings.HasPrefix(segment, "expires=") {
					snap.ExpiringKeys, _ = strconv.ParseInt(strings.TrimPrefix(segment, "expires="), 10, 64)
				}
			}
		}
	}
	return snap
}
