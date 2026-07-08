package ingestcore

import "strings"

// IngestTier classifies live ingest coverage.
type IngestTier int

const (
	TierP0Always IngestTier = iota
	TierP1Hot
	TierP2Roster
	TierP3Archive
)

func (t IngestTier) String() string {
	switch t {
	case TierP0Always:
		return "P0_ALWAYS"
	case TierP1Hot:
		return "P1_HOT"
	case TierP2Roster:
		return "P2_ROSTER"
	default:
		return "P3_ARCHIVE"
	}
}

// TierLabel returns the prometheus/metrics tier label.
func (t IngestTier) Label() string {
	switch t {
	case TierP0Always:
		return "P0"
	case TierP1Hot:
		return "P1"
	case TierP2Roster:
		return "P2"
	default:
		return "P3"
	}
}

// AssignTier picks ingest tier from tracking priority and Helix rank when tiering is enabled.
func AssignTier(cfg Config, trackPriority int, helixRank int, tieringEnabled bool) IngestTier {
	if !tieringEnabled {
		if trackPriority >= 10 {
			return TierP1Hot
		}
		return TierP3Archive
	}
	// P0: protected / always-track (priority >= 60)
	if trackPriority >= 60 {
		return TierP0Always
	}
	// Manual watch counts as P1 hot set member
	if trackPriority >= 30 {
		return TierP1Hot
	}
	if helixRank > 0 && helixRank <= cfg.P1HotLimit {
		return TierP1Hot
	}
	if helixRank > 0 && helixRank <= cfg.HubRosterLimit {
		return TierP2Roster
	}
	return TierP3Archive
}

// WantsFullIRC returns true when the tier should occupy an IRC collector slot.
func (t IngestTier) WantsFullIRC() bool {
	return t == TierP0Always || t == TierP1Hot
}

// DesiredChannel is one channel the scheduler wants on IRC.
type DesiredChannel struct {
	Login         string
	StreamID      string
	Tier          IngestTier
	TrackPriority int
	HelixRank     int
}

func normalizeLogin(login string) string {
	login = strings.ToLower(strings.TrimSpace(login))
	login = strings.TrimPrefix(login, "#")
	return login
}
