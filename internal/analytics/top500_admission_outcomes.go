package analytics

import (
	"strings"
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
