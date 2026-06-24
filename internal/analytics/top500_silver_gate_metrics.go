package analytics

import "streamclone/internal/metrics"

// RecordSilverGateDecision increments gate decision metrics with low-cardinality labels.
func RecordSilverGateDecision(result SilverGateResult, lane, operation string) {
	resultLabel := "skip"
	if result.AllowEnqueue {
		resultLabel = "allow"
	}
	metrics.Top500SilverGateDecisionsTotal.WithLabelValues(
		resultLabel,
		string(result.Decision),
		lane,
		operation,
	).Inc()
}
