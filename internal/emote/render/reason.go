package render

// Reason describes why a local emote render job was enqueued.
type Reason string

const (
	ReasonEnsure         Reason = "ensure"
	ReasonChatObserved   Reason = "chat_observed"
	ReasonUIRequest      Reason = "ui_request"
	ReasonCustomUpload   Reason = "custom_upload"
	ReasonManualBackfill Reason = "manual_backfill"
	ReasonRetry          Reason = "retry"
	ReasonLegacyBackfill Reason = "legacy_backfill"
)

func (r Reason) String() string {
	return string(r)
}

func (r Reason) priority() int {
	switch r {
	case ReasonCustomUpload, ReasonUIRequest:
		return 3
	case ReasonChatObserved, ReasonRetry:
		return 2
	case ReasonEnsure:
		return 1
	case ReasonManualBackfill, ReasonLegacyBackfill:
		return 0
	default:
		return 1
	}
}
