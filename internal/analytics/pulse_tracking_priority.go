package analytics

// Tracking priority tiers for the analytics collector pool (higher = more important).
const (
	TrackPriorityIdleNoRef            = 0
	TrackPriorityHelixTopLiveFill     = 9
	TrackPriorityTopRoster            = 10
	TrackPriorityCorpusRosterLive     = 11
	TrackPriorityManualWatch          = 30
	TrackPriorityPrincipalAlwaysTrack = 60
	TrackPriorityGlobalProtected      = 80
)

func trackingPriorityCanPreempt(incoming, victim int) bool {
	if incoming <= victim {
		return false
	}
	if incoming <= TrackPriorityTopRoster {
		return false
	}
	if incoming == TrackPriorityManualWatch && victim > TrackPriorityTopRoster {
		return false
	}
	return true
}
