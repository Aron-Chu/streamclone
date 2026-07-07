package analytics

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"streamclone/internal/config"
)

const (
	CorpusStatusHealthy  = "healthy"
	CorpusStatusDegraded = "degraded"
	CorpusStatusCritical = "critical"
	CorpusStatusDisabled = "disabled"
)

type CorpusRuntimeConfig struct {
	TargetTopN             int  `json:"targetTopN"`
	MetadataEnabled        bool `json:"metadataEnabled"`
	MetadataWriteEnabled   bool `json:"metadataWriteEnabled"`
	MetadataDryRun         bool `json:"metadataDryRun"`
	LiveAdmissionEnabled   bool `json:"liveAdmissionEnabled"`
	LiveAdmissionTopN      int  `json:"liveAdmissionTopN"`
	MaxActiveIRCChannels   int  `json:"maxActiveIrcChannels"`
	CorpusWorkersEnabled   bool `json:"corpusWorkersEnabled"`
	SilverEnabled          bool `json:"silverEnabled"`
	GoldEnabled            bool `json:"goldEnabled"`
	GoldWorkerEnabled      bool `json:"goldWorkerEnabled"`
	GoldWorkerCount        int  `json:"goldWorkerCount"`
	ArchiveEnabled         bool `json:"archiveEnabled"`
	BackfillWorkersEnabled bool `json:"backfillWorkersEnabled"`
}

type CorpusReadinessIssue struct {
	Code     string `json:"code"`
	Severity string `json:"severity"`
	Message  string `json:"message"`
}

type CorpusReadinessComponent struct {
	Status      string   `json:"status"`
	Message     string   `json:"message,omitempty"`
	ReasonCodes []string `json:"reasonCodes,omitempty"`
}

type CorpusReadinessComponents struct {
	API            CorpusReadinessComponent `json:"api"`
	Database       CorpusReadinessComponent `json:"database"`
	Metadata       CorpusReadinessComponent `json:"metadata"`
	LiveAdmission  CorpusReadinessComponent `json:"liveAdmission"`
	IRCCollector   CorpusReadinessComponent `json:"ircCollector"`
	SilverQueue    CorpusReadinessComponent `json:"silverQueue"`
	GoldQueue      CorpusReadinessComponent `json:"goldQueue"`
	EmoteHistory   CorpusReadinessComponent `json:"emoteHistory"`
	ArchiveStorage CorpusReadinessComponent `json:"archiveStorage"`
	WorkerCapacity CorpusReadinessComponent `json:"workerCapacity"`
	RateLimits     CorpusReadinessComponent `json:"rateLimits"`
}

type CorpusDesiredState struct {
	Mode                      string `json:"mode"`
	TargetTopN                int    `json:"targetTopN"`
	MetadataTrackerEnabled    bool   `json:"metadataTrackerEnabled"`
	MetadataWritesEnabled     bool   `json:"metadataWritesEnabled"`
	LiveAdmissionEnabled      bool   `json:"liveAdmissionEnabled"`
	LiveAdmissionTopN         int    `json:"liveAdmissionTopN"`
	MaxActiveIRCChannels      int    `json:"maxActiveIrcChannels"`
	SilverJobsEnabled         bool   `json:"silverJobsEnabled"`
	GoldJobsEnabled           bool   `json:"goldJobsEnabled"`
	GoldWorkersEnabled        bool   `json:"goldWorkersEnabled"`
	GoldWorkerCount           int    `json:"goldWorkerCount"`
	EmoteSnapshotsEnabled     bool   `json:"emoteSnapshotsEnabled"`
	EmoteNormalizationEnabled bool   `json:"emoteNormalizationEnabled"`
	ArchiveStorageEnabled     bool   `json:"archiveStorageEnabled"`
	BackfillWorkersEnabled    bool   `json:"backfillWorkersEnabled"`
}

type CorpusActualState struct {
	MetadataLiveRows            int        `json:"metadataLiveRows"`
	MetadataStaleRows           int        `json:"metadataStaleRows"`
	ViewerOnlyRows              int        `json:"viewerOnlyRows"`
	CollectorRows               int        `json:"collectorRows"`
	ExpectedCollectorRows       int        `json:"expectedCollectorRows"`
	CollectorDeficitRows        int        `json:"collectorDeficitRows"`
	ActiveCollectors            int        `json:"activeCollectors"`
	RollupSignalRows            int        `json:"rollupSignalRows"`
	RollupCollapseRows          int        `json:"rollupCollapseRows"`
	SilverPending               int        `json:"silverPending"`
	SilverRunning               int        `json:"silverRunning"`
	SilverDone                  int        `json:"silverDone"`
	SilverEligible              int        `json:"silverEligible"`
	GoldPending                 int        `json:"goldPending"`
	GoldRunning                 int        `json:"goldRunning"`
	GoldDone                    int        `json:"goldDone"`
	GoldEligible                int        `json:"goldEligible"`
	GoldSegmentsQueued          int        `json:"goldSegmentsQueued"`
	GoldSegmentsRunning         int        `json:"goldSegmentsRunning"`
	GoldSegmentsDone            int        `json:"goldSegmentsDone"`
	GoldSegmentsFailed          int        `json:"goldSegmentsFailed"`
	GoldSegmentsDeadLetter      int        `json:"goldSegmentsDeadLetter"`
	GQLRateLimitedBuckets       int        `json:"gqlRateLimitedBuckets"`
	EmoteSnapshotRows           int64      `json:"emoteSnapshotRows"`
	ChannelsWithRecentSnapshots int64      `json:"channelsWithRecentSnapshots"`
	EmoteNormalizedUsageRows    int64      `json:"emoteNormalizedUsageRows"`
	RecentEmoteNormalizedRows   int64      `json:"recentEmoteNormalizedRows"`
	LatestEmoteSnapshotAt       *time.Time `json:"latestEmoteSnapshotAt,omitempty"`
	LatestEmoteNormalizationAt  *time.Time `json:"latestEmoteNormalizationAt,omitempty"`
}

type CorpusEmoteHistoryState struct {
	Status         string                       `json:"status"`
	ReasonCodes    []string                     `json:"reasonCodes,omitempty"`
	Summary        EmoteHistoryReadinessSummary `json:"summary"`
	EndpointSanity EmoteHistoryEndpointSanity   `json:"endpointSanity"`
}

type CorpusGoldSegmentSummary struct {
	Queued             int `json:"queued"`
	Running            int `json:"running"`
	Done               int `json:"done"`
	Failed             int `json:"failed"`
	DeadLetter         int `json:"deadLetter"`
	Skipped            int `json:"skipped"`
	Total              int `json:"total"`
	RateLimitedBuckets int `json:"rateLimitedBuckets"`
}

type CorpusBackfillTierSummary struct {
	Queued              int  `json:"queued"`
	Running             int  `json:"running"`
	Done                int  `json:"done"`
	Skipped             int  `json:"skipped"`
	Failed              int  `json:"failed"`
	Total               int  `json:"total"`
	Eligible            int  `json:"eligible"`
	OldestQueuedSeconds *int `json:"oldestQueuedSeconds,omitempty"`
}

type CorpusReadinessTopRoster struct {
	TopN                     int        `json:"topN"`
	LiveRows                 int        `json:"liveRows"`
	CollectorTrackingRows    int        `json:"collectorTrackingRows"`
	ExpectedCollectorRows    int        `json:"expectedCollectorRows"`
	LiveCollectorDeficitRows int        `json:"liveCollectorDeficitRows"`
	MetadataOnlyRows         int        `json:"metadataOnlyRows"`
	MetadataStaleRows        int        `json:"metadataStaleRows"`
	AdmissionDisabledRows    int        `json:"admissionDisabledRows"`
	CapacityBlockedRows      int        `json:"capacityBlockedRows"`
	WarmingRows              int        `json:"warmingRows"`
	CollectingRows           int        `json:"collectingRows"`
	ViewerOnlyRows           int        `json:"viewerOnlyRows"`
	ZeroChatAfterAgeRows     int        `json:"zeroChatAfterAgeRows"`
	LatestMetadataSampledAt  *time.Time `json:"latestMetadataSampledAt,omitempty"`
	OldestMetadataSampledAt  *time.Time `json:"oldestMetadataSampledAt,omitempty"`
}

type CorpusReadinessReport struct {
	GeneratedAt  time.Time                 `json:"generatedAt"`
	Status       string                    `json:"status"`
	Config       CorpusRuntimeConfig       `json:"config"`
	Desired      CorpusDesiredState        `json:"desired"`
	Actual       CorpusActualState         `json:"actual"`
	Components   CorpusReadinessComponents `json:"components"`
	TopRoster    CorpusReadinessTopRoster  `json:"topRoster"`
	Silver       CorpusBackfillTierSummary `json:"silver"`
	Gold         CorpusBackfillTierSummary `json:"gold"`
	GoldSegments CorpusGoldSegmentSummary  `json:"goldSegments"`
	EmoteHistory CorpusEmoteHistoryState   `json:"emoteHistory"`
	Issues       []CorpusReadinessIssue    `json:"issues,omitempty"`
}

func DefaultCorpusRuntimeConfig() CorpusRuntimeConfig {
	return CorpusRuntimeConfig{
		TargetTopN:        DefaultTop500MetadataTopN,
		LiveAdmissionTopN: DefaultTop500MetadataTopN,
		GoldWorkerCount:   1,
	}
}

func CorpusRuntimeConfigFromApp(cfg config.Config) CorpusRuntimeConfig {
	targetTopN := normalizeCorpusTopN(cfg.CorpusTargetTopN)
	if targetTopN == 0 {
		targetTopN = normalizeCorpusTopN(cfg.Top500MetadataTopN)
	}
	if targetTopN == 0 {
		targetTopN = DefaultTop500MetadataTopN
	}
	maxActiveIRC := cfg.PulseMaxActiveChannels
	if maxActiveIRC <= 0 {
		maxActiveIRC = cfg.MaxConcurrentTrackedChannels
	}
	admissionTopN := 0
	admissionMaxIRC := 0
	if cfg.PulseMaxActiveChannels > 0 {
		admissionMaxIRC = cfg.PulseMaxActiveChannels
	}
	if cfg.PulseLiveAdmissionTopN > 0 {
		admissionTopN = config.ClampLiveAdmissionTopN(cfg.PulseLiveAdmissionTopN, admissionMaxIRC)
	} else {
		admissionTopN = targetTopN
	}
	return CorpusRuntimeConfig{
		TargetTopN:             targetTopN,
		MetadataEnabled:        cfg.Top500MetadataEnabled,
		MetadataWriteEnabled:   cfg.Top500MetadataWriteEnabled,
		MetadataDryRun:         cfg.Top500MetadataDryRun,
		LiveAdmissionEnabled:   cfg.PulseLiveAdmissionEnabled,
		LiveAdmissionTopN:      admissionTopN,
		MaxActiveIRCChannels:   maxActiveIRC,
		CorpusWorkersEnabled:   cfg.CorpusWorkersEnabled,
		SilverEnabled:          cfg.SilverAutoEnqueueEnabled,
		GoldEnabled:            cfg.GoldBackfillEnabled,
		GoldWorkerEnabled:      cfg.BackfillGoldWorkerEnabled,
		GoldWorkerCount:        cfg.BackfillGoldWorkerCount,
		ArchiveEnabled:         cfg.ArchiveEnabled,
		BackfillWorkersEnabled: cfg.BackfillEnabled,
	}.withDefaults()
}

func (c CorpusRuntimeConfig) withDefaults() CorpusRuntimeConfig {
	if c.TargetTopN <= 0 {
		c.TargetTopN = DefaultTop500MetadataTopN
	}
	if c.TargetTopN > MaxTop500MetadataTopN {
		c.TargetTopN = MaxTop500MetadataTopN
	}
	if c.LiveAdmissionTopN <= 0 {
		c.LiveAdmissionTopN = c.TargetTopN
	}
	admissionMaxIRC := 0
	if c.MaxActiveIRCChannels > 0 {
		admissionMaxIRC = c.MaxActiveIRCChannels
	}
	c.LiveAdmissionTopN = config.ClampLiveAdmissionTopN(c.LiveAdmissionTopN, admissionMaxIRC)
	if c.GoldWorkerCount <= 0 {
		c.GoldWorkerCount = 1
	}
	return c
}

func normalizeCorpusTopN(n int) int {
	if n <= 0 {
		return 0
	}
	if n > MaxTop500MetadataTopN {
		return MaxTop500MetadataTopN
	}
	return n
}

func (h *Handler) CorpusRoutes(r chi.Router) {
	r.Get("/v1/corpus/readiness", h.getCorpusReadiness)
	r.Route("/v1/internal/corpus", func(ir chi.Router) {
		if h != nil && h.pulseHosted.Hosted {
			ir.Use(func(next http.Handler) http.Handler {
				return AdminArchiveAuthMiddleware(h.appConfig, next)
			})
		}
		ir.Get("/readiness", h.getCorpusReadiness)
		ir.Get("/gaps", h.getCorpusGoldGaps)
		ir.Post("/gaps/requeue", h.postCorpusGoldGapsRequeue)
		ir.Get("/workers", h.getCorpusGoldWorkers)
		ir.Post("/inventory/{vod_id}/sync-gold-status", h.postSyncTop500GoldStatus)
	})
}

func (h *Handler) getCorpusReadiness(w http.ResponseWriter, r *http.Request) {
	report := h.buildCorpusReadinessReport(r.Context())
	status := http.StatusOK
	if report.Status == CorpusStatusCritical {
		status = http.StatusServiceUnavailable
	}
	writeJSON(w, status, report)
}

func (h *Handler) buildCorpusReadinessReport(ctx context.Context) CorpusReadinessReport {
	now := time.Now().UTC()
	cfg := h.corpusRuntimeConfig()
	report := CorpusReadinessReport{
		GeneratedAt: now,
		Status:      CorpusStatusHealthy,
		Config:      cfg,
		Desired:     corpusDesiredState(cfg, h.emoteHistoryJobs),
		Components: CorpusReadinessComponents{
			API:        component(CorpusStatusHealthy, "analytics API route is serving"),
			RateLimits: component(CorpusStatusHealthy, "No active rate-limit evidence in readiness state"),
		},
	}

	if h.store == nil {
		report.Components.Database = component(CorpusStatusCritical, "Postgres store is unavailable", "store_unavailable")
		report.Components.Metadata = component(CorpusStatusCritical, "metadata cannot be checked without Postgres", "store_unavailable")
		report.Components.LiveAdmission = component(CorpusStatusCritical, "admission cannot be checked without Postgres", "store_unavailable")
		report.Components.IRCCollector = component(CorpusStatusCritical, "collector cannot be checked without roster state", "store_unavailable")
		addIssue(&report, CorpusStatusCritical, "store_unavailable", "Postgres store is unavailable")
		finalizeCorpusReadiness(&report)
		return report
	}

	if err := h.store.Ping(ctx); err != nil {
		report.Components.Database = component(CorpusStatusCritical, "Postgres ping failed", "db_unavailable")
		addIssue(&report, CorpusStatusCritical, "db_unavailable", err.Error())
	} else {
		report.Components.Database = component(CorpusStatusHealthy, "Postgres is reachable")
	}

	topRoster, err := h.buildTop100ReadinessReport(ctx, cfg.TargetTopN, cfg.LiveAdmissionEnabled, ReadinessReportOptions{})
	if err != nil {
		report.Components.Metadata = component(CorpusStatusCritical, "Top-N metadata query failed", "metadata_query_failed")
		report.Components.LiveAdmission = component(CorpusStatusCritical, "live admission cannot be evaluated", "metadata_query_failed")
		report.Components.IRCCollector = component(CorpusStatusCritical, "collector cannot be evaluated", "metadata_query_failed")
		addIssue(&report, CorpusStatusCritical, "metadata_query_failed", err.Error())
	} else {
		report.TopRoster = corpusTopRosterFromReadiness(topRoster)
		report.Components.Metadata = metadataReadinessComponent(cfg, topRoster, &report)
		report.Components.LiveAdmission = liveAdmissionReadinessComponent(cfg, topRoster, &report)
		report.Components.IRCCollector = ircCollectorReadinessComponent(topRoster, &report)
	}

	if h.store != nil {
		if counts, err := h.store.BackfillTierCounts(ctx); err == nil {
			for _, c := range counts {
				switch strings.ToLower(strings.TrimSpace(c.Tier)) {
				case "silver":
					applyCorpusTierCount(&report.Silver, c.Status, c.Count)
				case "gold", "gold_full", "gold_lite":
					applyCorpusTierCount(&report.Gold, c.Status, c.Count)
				}
			}
		}
		if eligible, err := h.store.CorpusSilverEligibleCount(ctx, cfg.TargetTopN); err == nil {
			report.Silver.Eligible = eligible
		}
		if eligible, err := h.store.CorpusGoldEligibleCount(ctx); err == nil {
			report.Gold.Eligible = eligible
		}
		if age, err := h.store.BackfillOldestQueuedAgeSeconds(ctx, "silver"); err == nil {
			report.Silver.OldestQueuedSeconds = age
		}
		if age, err := h.store.BackfillOldestQueuedAgeSeconds(ctx, "gold", "gold_full", "gold_lite"); err == nil {
			report.Gold.OldestQueuedSeconds = age
		}
		if summary, err := h.store.CorpusGoldSegmentSummary(ctx); err == nil {
			report.GoldSegments = summary
		}
	}
	report.Components.SilverQueue = queueReadinessComponent("Silver", cfg.CorpusWorkersEnabled && cfg.BackfillWorkersEnabled && cfg.SilverEnabled, "silver", report.Silver, &report)
	report.Components.GoldQueue = queueReadinessComponent("Gold", cfg.CorpusWorkersEnabled && cfg.BackfillWorkersEnabled && cfg.GoldEnabled, "gold", report.Gold, &report)
	if emoteReport, err := h.emoteHistoryReadiness(ctx, now); err != nil {
		report.Components.EmoteHistory = component(CorpusStatusCritical, "Emote history readiness query failed", "emote_history_unhealthy")
		addIssue(&report, CorpusStatusCritical, "emote_history_unhealthy", err.Error())
	} else {
		report.EmoteHistory = corpusEmoteHistoryState(emoteReport)
		report.Components.EmoteHistory = emoteHistoryCorpusComponent(h.emoteHistoryJobs, emoteReport, &report)
	}
	report.Components.ArchiveStorage = archiveReadinessComponent(cfg)
	report.Components.WorkerCapacity = workerCapacityReadinessComponent(cfg, report.TopRoster)
	report.Components.RateLimits = rateLimitReadinessComponent(report.GoldSegments, &report)
	report.Actual = corpusActualState(report)

	finalizeCorpusReadiness(&report)
	return report
}

func corpusDesiredState(cfg CorpusRuntimeConfig, emoteJobs EmoteHistoryJobConfig) CorpusDesiredState {
	mode := "api_only"
	if cfg.MetadataEnabled || cfg.LiveAdmissionEnabled || cfg.CorpusWorkersEnabled || emoteJobs.SnapshotEnabled || emoteJobs.NormalizeEnabled || cfg.ArchiveEnabled {
		mode = "corpus_control_plane"
	}
	return CorpusDesiredState{
		Mode:                      mode,
		TargetTopN:                cfg.TargetTopN,
		MetadataTrackerEnabled:    cfg.MetadataEnabled,
		MetadataWritesEnabled:     cfg.MetadataWriteEnabled && !cfg.MetadataDryRun,
		LiveAdmissionEnabled:      cfg.LiveAdmissionEnabled,
		LiveAdmissionTopN:         cfg.LiveAdmissionTopN,
		MaxActiveIRCChannels:      cfg.MaxActiveIRCChannels,
		SilverJobsEnabled:         cfg.CorpusWorkersEnabled && cfg.BackfillWorkersEnabled && cfg.SilverEnabled,
		GoldJobsEnabled:           cfg.CorpusWorkersEnabled && cfg.BackfillWorkersEnabled && cfg.GoldEnabled,
		GoldWorkersEnabled:        cfg.CorpusWorkersEnabled && cfg.BackfillWorkersEnabled && cfg.GoldWorkerEnabled,
		GoldWorkerCount:           cfg.GoldWorkerCount,
		EmoteSnapshotsEnabled:     emoteJobs.SnapshotEnabled,
		EmoteNormalizationEnabled: emoteJobs.NormalizeEnabled,
		ArchiveStorageEnabled:     cfg.ArchiveEnabled,
		BackfillWorkersEnabled:    cfg.BackfillWorkersEnabled,
	}
}

func corpusActualState(report CorpusReadinessReport) CorpusActualState {
	return CorpusActualState{
		MetadataLiveRows:            report.TopRoster.LiveRows,
		MetadataStaleRows:           report.TopRoster.MetadataStaleRows,
		ViewerOnlyRows:              report.TopRoster.ViewerOnlyRows,
		CollectorRows:               report.TopRoster.CollectorTrackingRows,
		ExpectedCollectorRows:       report.TopRoster.ExpectedCollectorRows,
		CollectorDeficitRows:        report.TopRoster.LiveCollectorDeficitRows,
		ActiveCollectors:            report.TopRoster.CollectorTrackingRows,
		RollupSignalRows:            report.TopRoster.CollectingRows + report.TopRoster.ViewerOnlyRows,
		RollupCollapseRows:          report.TopRoster.ZeroChatAfterAgeRows,
		SilverPending:               report.Silver.Queued,
		SilverRunning:               report.Silver.Running,
		SilverDone:                  report.Silver.Done,
		SilverEligible:              report.Silver.Eligible,
		GoldPending:                 report.Gold.Queued,
		GoldRunning:                 report.Gold.Running,
		GoldDone:                    report.Gold.Done,
		GoldEligible:                report.Gold.Eligible,
		GoldSegmentsQueued:          report.GoldSegments.Queued,
		GoldSegmentsRunning:         report.GoldSegments.Running,
		GoldSegmentsDone:            report.GoldSegments.Done,
		GoldSegmentsFailed:          report.GoldSegments.Failed,
		GoldSegmentsDeadLetter:      report.GoldSegments.DeadLetter,
		GQLRateLimitedBuckets:       report.GoldSegments.RateLimitedBuckets,
		EmoteSnapshotRows:           report.EmoteHistory.Summary.SnapshotRows,
		ChannelsWithRecentSnapshots: report.EmoteHistory.Summary.ChannelsWithRecentSnapshots,
		EmoteNormalizedUsageRows:    report.EmoteHistory.Summary.NormalizedUsageRows,
		RecentEmoteNormalizedRows:   report.EmoteHistory.Summary.RecentNormalizedUsageRows,
		LatestEmoteSnapshotAt:       report.EmoteHistory.Summary.LatestSnapshotAt,
		LatestEmoteNormalizationAt:  report.EmoteHistory.Summary.LatestNormalizationAt,
	}
}

func corpusEmoteHistoryState(report EmoteHistoryReadinessResponse) CorpusEmoteHistoryState {
	return CorpusEmoteHistoryState{
		Status:         report.Status,
		ReasonCodes:    compactReasonCodes(report.ReasonCodes),
		Summary:        report.Summary,
		EndpointSanity: report.EndpointSanity,
	}
}

func corpusTopRosterFromReadiness(report Top100ReadinessReport) CorpusReadinessTopRoster {
	out := CorpusReadinessTopRoster{
		TopN:                     report.TopN,
		LiveRows:                 report.Summary.LiveRows,
		CollectorTrackingRows:    report.Summary.CollectorTrackingRows,
		ExpectedCollectorRows:    report.Summary.ExpectedCollectorRows,
		LiveCollectorDeficitRows: report.Summary.LiveCollectorDeficitRows,
		MetadataOnlyRows:         report.Summary.MetadataOnlyRows,
		MetadataStaleRows:        report.Summary.MetadataStaleRows,
		AdmissionDisabledRows:    report.Summary.AdmissionDisabledRows,
		CapacityBlockedRows:      report.Summary.CapacityBlockedRows,
		WarmingRows:              report.Summary.WarmingRows,
		CollectingRows:           report.Summary.CollectingRows,
		ViewerOnlyRows:           report.Summary.ViewerOnlyRows,
		ZeroChatAfterAgeRows:     report.Summary.ZeroChatAfterAgeRows,
	}
	for _, row := range report.Rows {
		if row.MetadataSampledAt.IsZero() {
			continue
		}
		sampled := row.MetadataSampledAt
		if out.LatestMetadataSampledAt == nil || sampled.After(*out.LatestMetadataSampledAt) {
			value := sampled
			out.LatestMetadataSampledAt = &value
		}
		if out.OldestMetadataSampledAt == nil || sampled.Before(*out.OldestMetadataSampledAt) {
			value := sampled
			out.OldestMetadataSampledAt = &value
		}
	}
	return out
}

// corpusMetadataStaleCriticalFraction reserves critical for broad sampler
// failure. A handful of stale roster rows (offline channels awaiting sampler
// catch-up, transient Helix hiccups) degrades instead of flipping the whole
// public hub to critical.
const corpusMetadataStaleCriticalFraction = 0.2

func metadataStaleSeverity(staleRows, liveRows int) string {
	if staleRows <= 0 {
		return CorpusStatusHealthy
	}
	if liveRows > 0 && float64(staleRows) >= corpusMetadataStaleCriticalFraction*float64(liveRows) {
		return CorpusStatusCritical
	}
	return CorpusStatusDegraded
}

func corpusPipelineStateFromReadiness(cfg CorpusRuntimeConfig, report Top100ReadinessReport) string {
	if !cfg.MetadataEnabled || !cfg.MetadataWriteEnabled || cfg.MetadataDryRun {
		return CorpusStatusCritical
	}
	if report.Summary.LiveRows == 0 {
		return CorpusStatusDegraded
	}
	metadataStale := metadataStaleSeverity(report.Summary.MetadataStaleRows, report.Summary.LiveRows)
	if metadataStale == CorpusStatusCritical {
		return CorpusStatusCritical
	}
	if report.Summary.AdmissionDisabledRows > 0 || (!cfg.LiveAdmissionEnabled && report.Summary.LiveRows > 0) {
		return CorpusStatusCritical
	}
	if report.CollectorMax <= 0 || report.Summary.CollectorTrackingRows == 0 {
		return CorpusStatusCritical
	}
	if metadataStale == CorpusStatusDegraded ||
		report.Summary.LiveCollectorDeficitRows > 0 || report.Summary.CapacityBlockedRows > 0 || report.Summary.ZeroChatAfterAgeRows > 0 {
		return CorpusStatusDegraded
	}
	return CorpusStatusHealthy
}

func metadataReadinessComponent(cfg CorpusRuntimeConfig, report Top100ReadinessReport, readiness *CorpusReadinessReport) CorpusReadinessComponent {
	if !cfg.MetadataEnabled {
		addIssue(readiness, CorpusStatusCritical, "metadata_disabled", "Top-N metadata sampling is disabled")
		return component(CorpusStatusCritical, "Top-N metadata sampling is disabled", "metadata_disabled", "profile_disabled")
	}
	if !cfg.MetadataWriteEnabled || cfg.MetadataDryRun {
		addIssue(readiness, CorpusStatusCritical, "metadata_write_disabled", "Top-N metadata is not writing current rows")
		return component(CorpusStatusCritical, "Top-N metadata writes are disabled", "write_disabled")
	}
	if report.Summary.LiveRows == 0 {
		return component(CorpusStatusDegraded, "No live Top-N metadata rows are currently visible", "no_live_rows")
	}
	switch metadataStaleSeverity(report.Summary.MetadataStaleRows, report.Summary.LiveRows) {
	case CorpusStatusCritical:
		addIssue(readiness, CorpusStatusCritical, "metadata_stale", "Top-N metadata rows are stale")
		return component(CorpusStatusCritical, "Top-N metadata rows are stale", "metadata_stale")
	case CorpusStatusDegraded:
		addIssue(readiness, CorpusStatusDegraded, "metadata_stale_partial", "Some Top-N metadata rows are stale")
		return component(CorpusStatusDegraded, "Some Top-N metadata rows are stale", "metadata_stale_partial")
	}
	return component(CorpusStatusHealthy, "Top-N metadata is fresh")
}

func liveAdmissionReadinessComponent(cfg CorpusRuntimeConfig, report Top100ReadinessReport, readiness *CorpusReadinessReport) CorpusReadinessComponent {
	if report.Summary.LiveRows == 0 {
		return component(CorpusStatusDegraded, "No live metadata rows to admit", "no_live_rows")
	}
	if !cfg.LiveAdmissionEnabled {
		addIssue(readiness, CorpusStatusCritical, "live_admission_disabled", "Live Top-N admission is disabled while live metadata rows exist")
		return component(CorpusStatusCritical, "Live Top-N admission is disabled while live metadata rows exist", "admission_disabled", "profile_disabled")
	}
	if report.Summary.LiveCollectorDeficitRows > 0 {
		addIssue(readiness, CorpusStatusDegraded, "collector_deficit", "Collector tracking is below expected live Top-N coverage")
		return component(CorpusStatusDegraded, "Collector tracking is below expected live Top-N coverage", "collector_count_low", "collector_deficit")
	}
	return component(CorpusStatusHealthy, "Live admission is tracking expected rows")
}

func ircCollectorReadinessComponent(report Top100ReadinessReport, readiness *CorpusReadinessReport) CorpusReadinessComponent {
	if report.Summary.LiveRows == 0 {
		return component(CorpusStatusDegraded, "No live roster rows are available", "no_live_rows")
	}
	if report.CollectorMax <= 0 {
		addIssue(readiness, CorpusStatusCritical, "collector_capacity_zero", "IRC collector capacity is zero")
		return component(CorpusStatusCritical, "IRC collector capacity is zero", "capacity_zero")
	}
	if report.Summary.CollectorTrackingRows == 0 {
		addIssue(readiness, CorpusStatusCritical, "collector_empty", "No live Top-N rows are in the IRC collector")
		return component(CorpusStatusCritical, "No live Top-N rows are in the IRC collector", "collector_empty")
	}
	var degradedReasons []string
	if report.Summary.ZeroChatAfterAgeRows > 0 {
		addIssue(readiness, CorpusStatusDegraded, "zero_chat_after_age", "Live Top-N rows are aged without chat rollups")
		degradedReasons = append(degradedReasons, "rollup_collapse", "zero_chat_after_age")
	}
	if report.Summary.LiveCollectorDeficitRows > 0 {
		degradedReasons = append(degradedReasons, "collector_count_low", "collector_deficit")
	}
	if len(degradedReasons) > 0 {
		return component(CorpusStatusDegraded, "IRC collector is below desired live Top-N coverage or chat rollups are missing", degradedReasons...)
	}
	return component(CorpusStatusHealthy, "IRC collector has expected live coverage")
}

func queueReadinessComponent(label string, enabled bool, tier string, summary CorpusBackfillTierSummary, readiness *CorpusReadinessReport) CorpusReadinessComponent {
	if !enabled {
		return component(CorpusStatusDisabled, label+" queue is intentionally disabled for this profile", tier+"_disabled", "profile_disabled")
	}
	if summary.Eligible > 0 && summary.Queued+summary.Running == 0 {
		code := tier + "_eligible_but_queue_empty"
		addIssue(readiness, CorpusStatusDegraded, code, label+" queue has eligible corpus work but no queued or running jobs")
		return component(CorpusStatusDegraded, label+" queue has eligible corpus work but no queued or running jobs", corpusQueueEmptyReasonCode(tier), code)
	}
	if summary.Failed > 0 && summary.Queued+summary.Running == 0 && summary.Done == 0 {
		code := tier + "_failed_without_active_recovery"
		addIssue(readiness, CorpusStatusDegraded, code, label+" queue has failures and no active recovery jobs")
		return component(CorpusStatusDegraded, label+" queue has failures and no active recovery jobs", code)
	}
	return component(CorpusStatusHealthy, label+" queue is enabled")
}

func corpusQueueEmptyReasonCode(tier string) string {
	switch strings.ToLower(strings.TrimSpace(tier)) {
	case "silver":
		return "silver_queue_empty_with_eligible_streams"
	case "gold":
		return "gold_queue_empty_with_eligible_vods"
	default:
		return tier + "_queue_empty_with_eligible_work"
	}
}

func emoteHistoryCorpusComponent(cfg EmoteHistoryJobConfig, emoteReport EmoteHistoryReadinessResponse, readiness *CorpusReadinessReport) CorpusReadinessComponent {
	if !cfg.SnapshotEnabled && !cfg.NormalizeEnabled {
		return component(CorpusStatusDisabled, "Emote history jobs are intentionally disabled for this profile", "emote_history_disabled", "profile_disabled")
	}
	if emoteReport.Status == emoteHistoryStatusUnhealthy {
		addIssue(readiness, CorpusStatusCritical, "emote_history_unhealthy", "Emote history readiness is unhealthy")
		return component(CorpusStatusCritical, "Emote history readiness is unhealthy", "emote_history_unhealthy")
	}
	reasons := mapEmoteHistoryCorpusReasons(emoteReport.ReasonCodes)
	if len(reasons) > 0 {
		addIssue(readiness, CorpusStatusDegraded, reasons[0], "Emote history is behind desired corpus state")
		return component(CorpusStatusDegraded, "Emote history is behind desired corpus state", reasons...)
	}
	return component(CorpusStatusHealthy, "Emote history snapshots and normalized usage are ready")
}

func mapEmoteHistoryCorpusReasons(codes []string) []string {
	reasons := make([]string, 0, len(codes))
	for _, code := range codes {
		switch code {
		case "stale_data", "no_recent_snapshots", "no_channels_covered", "no_recent_normalized_usage":
			reasons = append(reasons, "emote_history_stale", code)
		case "endpoint_unhealthy":
			reasons = append(reasons, "emote_history_endpoint_unhealthy")
		case "snapshot_job_disabled", "normalize_job_disabled":
			reasons = append(reasons, "emote_history_partial_disabled", code)
		case "provider_failures":
			reasons = append(reasons, "emote_history_provider_failures")
		}
	}
	return compactReasonCodes(reasons)
}

func archiveReadinessComponent(cfg CorpusRuntimeConfig) CorpusReadinessComponent {
	if !cfg.ArchiveEnabled {
		return component(CorpusStatusDisabled, "Archive storage is intentionally disabled for this profile", "archive_disabled", "profile_disabled")
	}
	return component(CorpusStatusHealthy, "Archive storage is enabled")
}

func workerCapacityReadinessComponent(cfg CorpusRuntimeConfig, roster CorpusReadinessTopRoster) CorpusReadinessComponent {
	if cfg.MaxActiveIRCChannels <= 0 {
		return component(CorpusStatusCritical, "IRC worker capacity is zero", "capacity_zero")
	}
	if roster.LiveRows > 0 && roster.LiveCollectorDeficitRows > 0 {
		return component(CorpusStatusDegraded, "Worker capacity or admission is below current live demand", "collector_count_low", "collector_deficit")
	}
	return component(CorpusStatusHealthy, "Worker capacity is configured")
}

func rateLimitReadinessComponent(summary CorpusGoldSegmentSummary, readiness *CorpusReadinessReport) CorpusReadinessComponent {
	if summary.RateLimitedBuckets > 0 {
		addIssue(readiness, CorpusStatusDegraded, "gql_rate_limited", "Gold VOD GQL rate-limit evidence is active")
		return component(CorpusStatusDegraded, "Gold VOD GQL rate-limit evidence is active", "gql_rate_limited")
	}
	return component(CorpusStatusHealthy, "No active rate-limit evidence in readiness state")
}

func applyCorpusTierCount(counts *CorpusBackfillTierSummary, status string, n int) {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "queued":
		counts.Queued += n
	case "running":
		counts.Running += n
	case "done":
		counts.Done += n
	case "skipped":
		counts.Skipped += n
	case "failed":
		counts.Failed += n
	}
	counts.Total += n
}

func component(status, message string, reasonCodes ...string) CorpusReadinessComponent {
	return CorpusReadinessComponent{Status: status, Message: message, ReasonCodes: compactReasonCodes(reasonCodes)}
}

func compactReasonCodes(codes []string) []string {
	out := make([]string, 0, len(codes))
	seen := map[string]bool{}
	for _, code := range codes {
		code = strings.TrimSpace(code)
		if code == "" || seen[code] {
			continue
		}
		seen[code] = true
		out = append(out, code)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func addIssue(report *CorpusReadinessReport, severity, code, message string) {
	if report == nil {
		return
	}
	report.Issues = append(report.Issues, CorpusReadinessIssue{Code: code, Severity: severity, Message: message})
}

func finalizeCorpusReadiness(report *CorpusReadinessReport) {
	if report == nil {
		return
	}
	status := CorpusStatusHealthy
	for _, component := range []CorpusReadinessComponent{
		report.Components.API,
		report.Components.Database,
		report.Components.Metadata,
		report.Components.LiveAdmission,
		report.Components.IRCCollector,
		report.Components.SilverQueue,
		report.Components.GoldQueue,
		report.Components.EmoteHistory,
		report.Components.ArchiveStorage,
		report.Components.WorkerCapacity,
		report.Components.RateLimits,
	} {
		status = worseCorpusStatus(status, component.Status)
	}
	for _, issue := range report.Issues {
		status = worseCorpusStatus(status, issue.Severity)
	}
	report.Status = status
}

func worseCorpusStatus(current, next string) string {
	if corpusStatusRank(next) > corpusStatusRank(current) {
		return next
	}
	return current
}

func corpusStatusRank(status string) int {
	switch status {
	case CorpusStatusCritical:
		return 3
	case CorpusStatusDegraded:
		return 2
	case CorpusStatusHealthy, CorpusStatusDisabled:
		return 1
	default:
		return 0
	}
}
