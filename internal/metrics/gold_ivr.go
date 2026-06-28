package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	GoldIVRJobsTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "gold_ivr_jobs_total",
		Help: "Total Gold IVR Lite accelerator attempts.",
	})
	GoldIVRJobsSuccessTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "gold_ivr_jobs_success_total",
		Help: "Successful Gold IVR Lite imports.",
	})
	GoldIVRJobsFailedTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "gold_ivr_jobs_failed_total",
		Help: "Failed Gold IVR Lite imports.",
	})
	GoldIVRJobsFallbackGQLTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "gold_ivr_jobs_fallback_gql_total",
		Help: "Gold jobs that fell back to GQL after IVR miss or failure.",
	})
	GoldIVRMessagesImportedTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "gold_ivr_messages_imported_total",
		Help: "IVR messages imported into provisional rollups.",
	})
	GoldIVRBytesImportedTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "gold_ivr_bytes_imported_total",
		Help: "IVR NDJSON bytes downloaded for import.",
	})
	GoldIVRParserErrorsTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "gold_ivr_parser_errors_total",
		Help: "IVR NDJSON parser errors during import.",
	})
	GoldIVRRollupMinutesWrittenTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "gold_ivr_rollup_minutes_written_total",
		Help: "Minute rollups written from IVR provisional import.",
	})
	GoldIVRPreflightTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "gold_ivr_preflight_total",
		Help: "IVR /list preflight checks.",
	})
	GoldIVRPreflightHitTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "gold_ivr_preflight_hit_total",
		Help: "IVR preflight coverage hits.",
	})
	GoldIVRPreflightMissTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "gold_ivr_preflight_miss_total",
		Help: "IVR preflight coverage misses.",
	})
	GoldIVRPreflightErrorTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "gold_ivr_preflight_error_total",
		Help: "IVR preflight errors.",
	})
	GoldIVRCoverageCacheHitTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "gold_ivr_coverage_cache_hit_total",
		Help: "IVR preflight cache hits.",
	})
	GoldIVRQualityFailTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "gold_ivr_quality_fail_total",
		Help: "IVR imports rejected by quality checks.",
	})
	GoldSourceStateTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "gold_source_state_total",
		Help: "Stream chat source state transitions.",
	}, []string{"state"})
	GoldChatSourceTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "gold_chat_source_total",
		Help: "Stream chat source assignments.",
	}, []string{"source"})
	GoldIVRShadowJobsTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "gold_ivr_shadow_jobs_total",
		Help: "IVR shadow-mode dry runs (no rollup writes).",
	})
	GoldIVRShadowSuccessTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "gold_ivr_shadow_success_total",
		Help: "Successful IVR shadow runs.",
	})
	GoldIVRShadowFailedTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "gold_ivr_shadow_failed_total",
		Help: "Failed IVR shadow runs.",
	})
	GoldIVRShadowMessagesTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "gold_ivr_shadow_messages_total",
		Help: "Messages parsed during IVR shadow runs.",
	})
	GoldIVRShadowDuplicateAdjustedScore = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "gold_ivr_shadow_duplicate_adjusted_score",
		Help:    "Shadow compare score vs existing rollups (0-100).",
		Buckets: []float64{50, 70, 85, 90, 95, 99, 100},
	})
	GoldIVRShadowRecommendationTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "gold_ivr_shadow_recommendation_total",
		Help: "IVR shadow recommendation outcomes.",
	}, []string{"recommendation"})
)
