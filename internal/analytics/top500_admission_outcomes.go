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
