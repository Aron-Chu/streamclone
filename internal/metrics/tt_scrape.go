package metrics

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// AnalyticsTTScrapeFailures counts TwitchTracker scrape errors by reason and egress path.
	AnalyticsTTScrapeFailures = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "analytics_tt_scrape_failures_total",
		Help: "TwitchTracker scrape failures by reason and path (direct_http or browser).",
	}, []string{"source", "reason", "path"})

	// AnalyticsTTScrapeSuccess counts successful TwitchTracker scrapes by egress path.
	AnalyticsTTScrapeSuccess = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "analytics_tt_scrape_success_total",
		Help: "Successful TwitchTracker scrapes by path and cache_hit.",
	}, []string{"source", "path", "cache_hit"})

	// AnalyticsTTScrapeDuration records end-to-end TT scrape attempt duration.
	AnalyticsTTScrapeDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "analytics_tt_scrape_duration_seconds",
		Help:    "TwitchTracker scrape attempt duration in seconds.",
		Buckets: []float64{0.5, 1, 2, 5, 10, 20, 40, 60, 90, 120, 180},
	}, []string{"source", "path", "outcome"})
)

// RecordTTScrapeAttempt records taxonomy metrics for one TT scrape attempt.
// Keeps legacy analytics_scraper_requests_total in sync for dashboard compatibility.
func RecordTTScrapeAttempt(source, path, reason string, cacheHit bool, duration time.Duration) {
	outcome := "success"
	if reason != "ok" {
		outcome = "error"
		AnalyticsTTScrapeFailures.WithLabelValues(source, reason, path).Inc()
		AnalyticsScraperRequests.WithLabelValues(source, "error").Inc()
	} else {
		cacheLabel := "false"
		if cacheHit {
			cacheLabel = "true"
		}
		AnalyticsTTScrapeSuccess.WithLabelValues(source, path, cacheLabel).Inc()
		AnalyticsScraperRequests.WithLabelValues(source, "success").Inc()
	}
	AnalyticsTTScrapeDuration.WithLabelValues(source, path, outcome).Observe(duration.Seconds())
}
