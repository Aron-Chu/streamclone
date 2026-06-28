package analytics

import (
	"context"
	"encoding/json"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
)

const (
	emoteHistoryStatusOK        = "ok"
	emoteHistoryStatusDegraded  = "degraded"
	emoteHistoryStatusUnhealthy = "unhealthy"

	emoteHistoryReadinessWindow = 24 * time.Hour
	emoteHistoryStaleAfter      = 48 * time.Hour
)

type EmoteHistoryReadinessResponse struct {
	Status         string                       `json:"status"`
	GeneratedAt    time.Time                    `json:"generatedAt"`
	Config         EmoteHistoryReadinessConfig  `json:"config"`
	Summary        EmoteHistoryReadinessSummary `json:"summary"`
	ProviderHealth []EmoteHistoryProviderHealth `json:"providerHealth"`
	EndpointSanity EmoteHistoryEndpointSanity   `json:"endpointSanity"`
	ReasonCodes    []string                     `json:"reasonCodes"`
	Sources        []SourceStatus               `json:"sources"`
}

type EmoteHistoryReadinessConfig struct {
	SnapshotEnabled          bool  `json:"snapshotEnabled"`
	SnapshotIntervalSeconds  int64 `json:"snapshotIntervalSeconds"`
	SnapshotBatchSize        int   `json:"snapshotBatchSize"`
	NormalizeEnabled         bool  `json:"normalizeEnabled"`
	NormalizeIntervalSeconds int64 `json:"normalizeIntervalSeconds"`
	NormalizeSinceSeconds    int64 `json:"normalizeSinceSeconds"`
	NormalizeBatchSize       int   `json:"normalizeBatchSize"`
	RecentWindowSeconds      int64 `json:"recentWindowSeconds"`
}

type EmoteHistoryReadinessSummary struct {
	MigrationPresent            bool       `json:"migrationPresent"`
	SnapshotRows                int64      `json:"snapshotRows"`
	ProviderHealthRows          int64      `json:"providerHealthRows"`
	ChannelsWithRecentSnapshots int64      `json:"channelsWithRecentSnapshots"`
	RecentSnapshotChannels      []string   `json:"recentSnapshotChannels"`
	LatestSnapshotAt            *time.Time `json:"latestSnapshotAt,omitempty"`
	LastSnapshotAgeSeconds      *int64     `json:"lastSnapshotAgeSeconds,omitempty"`
	NormalizedUsageRows         int64      `json:"normalizedUsageRows"`
	RecentNormalizedUsageRows   int64      `json:"recentNormalizedUsageRows"`
	LatestNormalizationAt       *time.Time `json:"latestNormalizationAt,omitempty"`
	LastNormalizationAgeSeconds *int64     `json:"lastNormalizationAgeSeconds,omitempty"`
	ProviderFailures            int64      `json:"providerFailures"`
	PublicTopEmotes             int        `json:"publicTopEmotes"`
	PublicHistoryBuckets        int        `json:"publicHistoryBuckets"`
	PublicCoveragePresent       bool       `json:"publicCoveragePresent"`
	PublicFreshnessPresent      bool       `json:"publicFreshnessPresent"`
}

type EmoteHistoryProviderHealth struct {
	Provider         string     `json:"provider"`
	Rows             int64      `json:"rows"`
	ReadyRows        int64      `json:"readyRows"`
	FailedRows       int64      `json:"failedRows"`
	LatestSnapshotAt *time.Time `json:"latestSnapshotAt,omitempty"`
	LatestFailureAt  *time.Time `json:"latestFailureAt,omitempty"`
	State            string     `json:"state"`
	LastError        string     `json:"lastError,omitempty"`
}

type EmoteHistoryEndpointSanity struct {
	Channel                  string `json:"channel"`
	Status                   string `json:"status"`
	TopEmotes                int    `json:"topEmotes"`
	HistoryBuckets           int    `json:"historyBuckets"`
	CoverageFreshnessPresent bool   `json:"coverageFreshnessPresent"`
	Sanitized                bool   `json:"sanitized"`
	Error                    string `json:"error,omitempty"`
}

func (h *Handler) EmoteHistoryRoutes(r chi.Router) {
	r.Get("/v1/internal/emote-history/readiness", h.getEmoteHistoryReadiness)
}

func (h *Handler) getEmoteHistoryReadiness(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	report, err := h.emoteHistoryReadiness(ctx, time.Now().UTC())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, EmoteHistoryReadinessResponse{
			Status:      emoteHistoryStatusUnhealthy,
			GeneratedAt: time.Now().UTC(),
			ReasonCodes: []string{"readiness_query_failed"},
			Sources:     []SourceStatus{{Source: "emote_history", State: "unavailable", Message: err.Error()}},
		})
		return
	}
	status := http.StatusOK
	if report.Status == emoteHistoryStatusUnhealthy {
		status = http.StatusServiceUnavailable
	}
	writeJSON(w, status, report)
}

func (h *Handler) emoteHistoryReadiness(ctx context.Context, now time.Time) (EmoteHistoryReadinessResponse, error) {
	cfg := h.emoteHistoryJobs
	report := EmoteHistoryReadinessResponse{
		Status:      emoteHistoryStatusOK,
		GeneratedAt: now,
		Config: EmoteHistoryReadinessConfig{
			SnapshotEnabled:          cfg.SnapshotEnabled,
			SnapshotIntervalSeconds:  int64(cfg.SnapshotInterval.Seconds()),
			SnapshotBatchSize:        cfg.SnapshotBatchSize,
			NormalizeEnabled:         cfg.NormalizeEnabled,
			NormalizeIntervalSeconds: int64(cfg.NormalizeInterval.Seconds()),
			NormalizeSinceSeconds:    int64(cfg.NormalizeSince.Seconds()),
			NormalizeBatchSize:       cfg.NormalizeBatchSize,
			RecentWindowSeconds:      int64(emoteHistoryReadinessWindow.Seconds()),
		},
		EndpointSanity: EmoteHistoryEndpointSanity{Channel: "xqc", Status: "unknown"},
		Sources:        []SourceStatus{{Source: "emote_history", State: "ready"}},
	}
	if h == nil || h.store == nil || h.store.db == nil {
		report.Status = emoteHistoryStatusUnhealthy
		report.ReasonCodes = []string{"store_unavailable"}
		report.Sources = []SourceStatus{{Source: "emote_history", State: "unavailable", Message: "store unavailable"}}
		return report, nil
	}
	if !cfg.SnapshotEnabled {
		report.addReason("snapshot_job_disabled")
	}
	if !cfg.NormalizeEnabled {
		report.addReason("normalize_job_disabled")
	}
	data, err := h.store.emoteHistoryReadinessData(ctx, now, emoteHistoryReadinessWindow)
	if err != nil {
		return report, err
	}
	report.Summary = data.Summary
	report.ProviderHealth = data.ProviderHealth
	if !report.Summary.MigrationPresent {
		report.addReason("migration_missing")
	}
	if report.Summary.ProviderHealthRows == 0 {
		report.addReason("no_provider_health_rows")
	}
	if report.Summary.SnapshotRows == 0 {
		report.addReason("no_recent_snapshots")
	}
	if report.Summary.ChannelsWithRecentSnapshots == 0 {
		report.addReason("no_channels_covered")
	}
	if report.Summary.NormalizedUsageRows == 0 || report.Summary.RecentNormalizedUsageRows == 0 {
		report.addReason("no_recent_normalized_usage")
	}
	if report.Summary.ProviderFailures > 0 {
		report.addReason("provider_failures")
	}
	if report.Summary.LastSnapshotAgeSeconds != nil && *report.Summary.LastSnapshotAgeSeconds > int64(emoteHistoryStaleAfter.Seconds()) {
		report.addReason("stale_data")
	}
	if report.Summary.LastNormalizationAgeSeconds != nil && *report.Summary.LastNormalizationAgeSeconds > int64(emoteHistoryStaleAfter.Seconds()) {
		report.addReason("stale_data")
	}
	report.EndpointSanity = h.emoteHistoryEndpointSanity(ctx)
	report.Summary.PublicTopEmotes = report.EndpointSanity.TopEmotes
	report.Summary.PublicHistoryBuckets = report.EndpointSanity.HistoryBuckets
	report.Summary.PublicCoveragePresent = report.EndpointSanity.CoverageFreshnessPresent
	report.Summary.PublicFreshnessPresent = report.EndpointSanity.CoverageFreshnessPresent
	if report.EndpointSanity.Status != "ok" || !report.EndpointSanity.Sanitized || report.EndpointSanity.TopEmotes == 0 || report.EndpointSanity.HistoryBuckets == 0 || !report.EndpointSanity.CoverageFreshnessPresent {
		report.addReason("endpoint_unhealthy")
	}
	report.finalizeStatus()
	return report, nil
}

type emoteHistoryReadinessData struct {
	Summary        EmoteHistoryReadinessSummary
	ProviderHealth []EmoteHistoryProviderHealth
}

func (s *Store) emoteHistoryReadinessData(ctx context.Context, now time.Time, window time.Duration) (emoteHistoryReadinessData, error) {
	out := emoteHistoryReadinessData{}
	windowStart := now.Add(-window)
	if err := s.db.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_schema='public' AND table_name='channel_emote_set_snapshots')
		   AND EXISTS (SELECT 1 FROM information_schema.tables WHERE table_schema='public' AND table_name='emote_usage_minute_rollups')
		   AND EXISTS (SELECT 1 FROM information_schema.tables WHERE table_schema='public' AND table_name='emote_usage_stream_rollups')
		   AND EXISTS (SELECT 1 FROM information_schema.columns WHERE table_schema='public' AND table_name='channel_emote_providers' AND column_name='last_snapshot_at')`).Scan(&out.Summary.MigrationPresent); err != nil {
		return out, err
	}
	if !out.Summary.MigrationPresent {
		return out, nil
	}
	if err := s.db.QueryRow(ctx, `
		WITH snapshots AS (
			SELECT COUNT(*)::bigint AS snapshot_rows, MAX(fetched_at) AS latest_snapshot_at
			FROM channel_emote_set_snapshots
		), recent_channels AS (
			SELECT COALESCE(array_agg(login ORDER BY login), ARRAY[]::text[]) AS channels, COUNT(*)::bigint AS channel_count
			FROM (
				SELECT DISTINCT lower(login)::text AS login
				FROM channel_emote_set_snapshots
				WHERE fetched_at >= $1 AND login <> ''
				ORDER BY lower(login)::text
				LIMIT 25
			) c
		), providers AS (
			SELECT COUNT(*)::bigint AS provider_rows,
			       COUNT(*) FILTER (WHERE COALESCE(snapshot_error,'') <> '' OR COALESCE(snapshot_state,state) IN ('failed','partial'))::bigint AS provider_failures,
			       MAX(last_snapshot_at) AS latest_provider_snapshot
			FROM channel_emote_providers
		), usage AS (
			SELECT COUNT(*)::bigint AS normalized_rows,
			       COUNT(*) FILTER (WHERE updated_at >= $1)::bigint AS recent_normalized_rows,
			       MAX(updated_at) AS latest_normalization
			FROM emote_usage_minute_rollups
		)
		SELECT snapshots.snapshot_rows, COALESCE(snapshots.latest_snapshot_at, providers.latest_provider_snapshot), recent_channels.channel_count, recent_channels.channels,
		       providers.provider_rows, providers.provider_failures, usage.normalized_rows, usage.recent_normalized_rows, usage.latest_normalization
		FROM snapshots CROSS JOIN recent_channels CROSS JOIN providers CROSS JOIN usage`, windowStart).Scan(
		&out.Summary.SnapshotRows,
		&out.Summary.LatestSnapshotAt,
		&out.Summary.ChannelsWithRecentSnapshots,
		&out.Summary.RecentSnapshotChannels,
		&out.Summary.ProviderHealthRows,
		&out.Summary.ProviderFailures,
		&out.Summary.NormalizedUsageRows,
		&out.Summary.RecentNormalizedUsageRows,
		&out.Summary.LatestNormalizationAt,
	); err != nil {
		return out, err
	}
	if out.Summary.LatestSnapshotAt != nil {
		age := int64(now.Sub(*out.Summary.LatestSnapshotAt).Seconds())
		out.Summary.LastSnapshotAgeSeconds = &age
	}
	if out.Summary.LatestNormalizationAt != nil {
		age := int64(now.Sub(*out.Summary.LatestNormalizationAt).Seconds())
		out.Summary.LastNormalizationAgeSeconds = &age
	}
	rows, err := s.db.Query(ctx, `
		SELECT provider,
		       COUNT(*)::bigint,
		       COUNT(*) FILTER (WHERE COALESCE(snapshot_state,state)='ready')::bigint,
		       COUNT(*) FILTER (WHERE COALESCE(snapshot_error,'') <> '' OR COALESCE(snapshot_state,state) IN ('failed','partial'))::bigint,
		       MAX(last_snapshot_at),
		       MAX(updated_at) FILTER (WHERE COALESCE(snapshot_error,'') <> '' OR COALESCE(snapshot_state,state) IN ('failed','partial')),
		       COALESCE((array_agg(COALESCE(snapshot_state,state) ORDER BY updated_at DESC NULLS LAST))[1], 'unknown') AS state,
		       COALESCE((array_agg(snapshot_error ORDER BY updated_at DESC NULLS LAST) FILTER (WHERE COALESCE(snapshot_error,'') <> ''))[1], '') AS last_error
		FROM channel_emote_providers
		GROUP BY provider
		ORDER BY provider`)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var item EmoteHistoryProviderHealth
		if err := rows.Scan(&item.Provider, &item.Rows, &item.ReadyRows, &item.FailedRows, &item.LatestSnapshotAt, &item.LatestFailureAt, &item.State, &item.LastError); err != nil {
			return out, err
		}
		out.ProviderHealth = append(out.ProviderHealth, item)
	}
	return out, rows.Err()
}

func (h *Handler) emoteHistoryEndpointSanity(ctx context.Context) EmoteHistoryEndpointSanity {
	out := EmoteHistoryEndpointSanity{Channel: "xqc", Status: "unknown"}
	resp, err := h.store.PortalChannelEmotes(ctx, "xqc", 30*24*time.Hour)
	if err != nil {
		out.Status = "failed"
		out.Error = err.Error()
		return out
	}
	body, err := json.Marshal(resp)
	if err != nil {
		out.Status = "failed"
		out.Error = err.Error()
		return out
	}
	out.TopEmotes = len(resp.TopEmotes)
	out.HistoryBuckets = len(resp.History)
	out.CoverageFreshnessPresent = resp.Freshness.ProviderState != "" && resp.Range != ""
	out.Sanitized = emoteHistoryReadinessSanitized(body)
	if out.CoverageFreshnessPresent && out.Sanitized {
		out.Status = "ok"
	} else {
		out.Status = "failed"
	}
	return out
}

func (r *EmoteHistoryReadinessResponse) addReason(code string) {
	code = strings.TrimSpace(code)
	if code == "" {
		return
	}
	for _, existing := range r.ReasonCodes {
		if existing == code {
			return
		}
	}
	r.ReasonCodes = append(r.ReasonCodes, code)
}

func (r *EmoteHistoryReadinessResponse) finalizeStatus() {
	sort.Strings(r.ReasonCodes)
	status := emoteHistoryStatusOK
	for _, code := range r.ReasonCodes {
		switch code {
		case "migration_missing", "store_unavailable", "readiness_query_failed":
			status = emoteHistoryStatusUnhealthy
		}
	}
	if status != emoteHistoryStatusUnhealthy && len(r.ReasonCodes) > 0 {
		status = emoteHistoryStatusDegraded
	}
	r.Status = status
	if status == emoteHistoryStatusOK {
		r.Sources = []SourceStatus{{Source: "emote_history", State: "ready"}}
		return
	}
	state := "limited"
	if status == emoteHistoryStatusUnhealthy {
		state = "unavailable"
	}
	r.Sources = []SourceStatus{{Source: "emote_history", State: state, Message: strings.Join(r.ReasonCodes, ",")}}
}

func emoteHistoryReadinessSanitized(body []byte) bool {
	needle := strings.ToLower(string(body))
	for _, term := range []string{"rawchat", "chattext", "fragments", "chatter", "username", "userlogin", "userid", "leaderboard", "operator", "gql", "corpus"} {
		if strings.Contains(needle, term) {
			return false
		}
	}
	return json.Valid(body)
}
