package analytics

import (
	"strings"
	"time"

	"streamclone/internal/metrics"
)

const (
	TopRosterAdmissionModeDefault = "top_roster"

	TopRosterAdmissionAdmitted        = "admitted"
	TopRosterAdmissionAlreadyTracking = "already_tracking"
	TopRosterAdmissionCapacityFull    = "capacity_full"
	TopRosterAdmissionEmptyLogin      = "empty_login"
	TopRosterAdmissionEmptyStreamID   = "empty_stream_id"
	TopRosterAdmissionNotLive         = "skipped_not_live"
	TopRosterAdmissionDuplicateStream = "duplicate_stream"
	TopRosterAdmissionWatchError      = "watch_error"

	TopRosterAdmissionSkipDisabled           = "disabled"
	TopRosterAdmissionSkipRateLimited        = "rate_limited"
	TopRosterAdmissionSkipCollectorUnhealthy = "collector_unhealthy"
	TopRosterAdmissionSkipEnvMismatch        = "env_mismatch"
	TopRosterAdmissionSkipNoOAuth            = "no_oauth"
	TopRosterAdmissionSkipLeaseConflict      = "lease_conflict"
	TopRosterAdmissionSkipCapacityFull       = "capacity_full"
	TopRosterAdmissionSkipInvalidCandidate   = "invalid_candidate"
	TopRosterAdmissionSkipAlreadyTracking    = "already_tracking"
	TopRosterAdmissionSkipDuplicateStream    = "duplicate_stream"
	TopRosterAdmissionSkipWatchError         = "watch_error"
)

func classifyTopRosterWatchResponse(resp WatchResponse) string {
	if resp.Tracking {
		if strings.Contains(strings.ToLower(resp.Message), "already") {
			return TopRosterAdmissionAlreadyTracking
		}
		return TopRosterAdmissionAdmitted
	}
	if strings.Contains(strings.ToLower(resp.Message), "full") {
		return TopRosterAdmissionCapacityFull
	}
	return TopRosterAdmissionWatchError
}

func topRosterAdmissionSkipReason(outcome, message string) string {
	switch outcome {
	case TopRosterAdmissionCapacityFull:
		return TopRosterAdmissionSkipCapacityFull
	case TopRosterAdmissionAlreadyTracking:
		return TopRosterAdmissionSkipAlreadyTracking
	case TopRosterAdmissionDuplicateStream:
		return TopRosterAdmissionSkipDuplicateStream
	case TopRosterAdmissionEmptyLogin, TopRosterAdmissionEmptyStreamID, TopRosterAdmissionNotLive:
		return TopRosterAdmissionSkipInvalidCandidate
	case TopRosterAdmissionWatchError:
		lower := strings.ToLower(message)
		switch {
		case strings.Contains(lower, "rate") && strings.Contains(lower, "limit"):
			return TopRosterAdmissionSkipRateLimited
		case strings.Contains(lower, "oauth") || strings.Contains(lower, "token") || strings.Contains(lower, "auth"):
			return TopRosterAdmissionSkipNoOAuth
		case strings.Contains(lower, "lease"):
			return TopRosterAdmissionSkipLeaseConflict
		case strings.Contains(lower, "collector") || strings.Contains(lower, "irc"):
			return TopRosterAdmissionSkipCollectorUnhealthy
		default:
			return TopRosterAdmissionSkipWatchError
		}
	default:
		return ""
	}
}

func classifyTopRosterCandidate(c *Collector, tracked map[string]struct{}, row Top500Current) (outcome, message, streamID string) {
	login := normalizeLogin(row.Login)
	streamID = ""
	if row.StreamID != nil {
		streamID = strings.TrimSpace(*row.StreamID)
	}
	switch {
	case login == "":
		return TopRosterAdmissionEmptyLogin, "login required", streamID
	case !row.IsLive:
		return TopRosterAdmissionNotLive, "not live", streamID
	case streamID == "":
		return TopRosterAdmissionEmptyStreamID, "stream id required", streamID
	case c != nil && c.TrackedStreamID(login) == streamID:
		return TopRosterAdmissionDuplicateStream, "duplicate stream id", streamID
	}
	if tracked != nil {
		if _, ok := tracked[login]; ok {
			return TopRosterAdmissionAlreadyTracking, "already tracking", streamID
		}
	}
	return "", "", streamID
}

func buildTopRosterAdmissionAttempt(row Top500Current, streamID string, attemptedAt time.Time, outcome, message string, state admissionCycleState) TopRosterAdmissionAttempt {
	login := normalizeLogin(row.Login)
	_, tracking := state.trackedLogins[login]
	return TopRosterAdmissionAttempt{
		Login:             login,
		Rank:              row.Rank,
		StreamID:          streamID,
		SampledAt:         row.SampledAt,
		AttemptedAt:       attemptedAt,
		Outcome:           outcome,
		Message:           message,
		CollectorTracking: tracking,
		ActiveCollectors:  state.active,
		MaxCollectors:     state.max,
	}
}

func recordTopRosterAdmissionMetrics(mode, outcome string) {
	if outcome == "" {
		return
	}
	metrics.TopRosterAdmissionAttemptsTotal.WithLabelValues(outcome, mode).Inc()
}

func recordTopRosterAdmissionSkipForOutcome(mode, outcome, message string, skippedByReason map[string]int) {
	reason := topRosterAdmissionSkipReason(outcome, message)
	if reason == "" {
		return
	}
	recordTopRosterAdmissionSkip(mode, reason)
	if skippedByReason != nil {
		skippedByReason[reason]++
	}
}

func recordTopRosterAdmissionSkip(mode, reason string) {
	if reason == "" {
		return
	}
	metrics.TopRosterAdmissionSkippedTotal.WithLabelValues(reason, mode).Inc()
}
