package analytics

import "time"

const (
	DefaultTop500SilverGateMaxCandidates    = 5
	DefaultTop500SilverGateMaxEnqueuePerRun = 1
	DefaultTop500SilverGateInterval         = 10 * time.Minute
	MaxTop500SilverGateMaxCandidates        = 100
	MaxTop500SilverGateMaxEnqueuePerRun     = 10

	SilverGateLaneTop500Selective = "top500_selective"

	SilverGateGlobalMaxEnqueuePerDay = 10
	SilverGateGlobalMaxRunning       = 1
	SilverGateGlobalMaxQueueDepth    = 25
)

// SilverGateConfig holds runtime settings for the Top500 selective silver gate.
type SilverGateConfig struct {
	Enabled         bool
	DryRun          bool
	WriteEnabled    bool
	MaxCandidates   int
	MaxEnqueuePerRun int
	Interval        time.Duration
}

// SilverGateCandidate is an in-memory candidate for selective silver enqueue.
type SilverGateCandidate struct {
	CandidateID     string
	Login           string
	ChannelID       string
	StreamID        string
	Rank            int
	ViewerCount     int
	IsLive          bool
	StartedAt       time.Time
	SampledAt       time.Time
	StaleAfter      time.Time
	CandidateSource string
	PriorityScore   int
	ReasonContext   string
}

// SilverBudgetSnapshot is a point-in-time read model for gate decisions.
type SilverBudgetSnapshot struct {
	Available bool
	Stale     bool

	SilverEnqueuedToday int
	SilverRunningNow    int
	SilverQueueDepth    int

	LastAttemptAt            time.Time
	AlreadyDone              bool
	DuplicateQueuedOrRunning bool
	InChannelCooldown        bool
	InFailureBackoff         bool
	GlobalTTBackoffActive    bool

	DiskGuardActive     bool
	BackupGuardActive   bool
	ArchiveGuardActive  bool
	AlertingGuardActive bool
	CorpusUnhealthy     bool
	HostedUnhealthy     bool
}

// SilverGateDecisionReason is the primary gate outcome for metrics and audit.
type SilverGateDecisionReason string

const (
	SilverGateAllowEnqueue            SilverGateDecisionReason = "allow_enqueue"
	SilverGateSkipNotCandidate        SilverGateDecisionReason = "skip_not_candidate"
	SilverGateSkipMetadataStale       SilverGateDecisionReason = "skip_metadata_stale"
	SilverGateSkipMissingStreamID     SilverGateDecisionReason = "skip_missing_stream_id"
	SilverGateSkipAlreadyDone         SilverGateDecisionReason = "skip_already_done"
	SilverGateSkipDuplicateJob        SilverGateDecisionReason = "skip_duplicate_job"
	SilverGateSkipChannelCooldown     SilverGateDecisionReason = "skip_channel_cooldown"
	SilverGateSkipRecentFailure       SilverGateDecisionReason = "skip_recent_failure"
	SilverGateSkipGlobalBackoff       SilverGateDecisionReason = "skip_global_backoff"
	SilverGateSkipDailyBudget           SilverGateDecisionReason = "skip_daily_budget"
	SilverGateSkipRunningLimit          SilverGateDecisionReason = "skip_running_limit"
	SilverGateSkipQueueFull             SilverGateDecisionReason = "skip_queue_full"
	SilverGateSkipDiskGuard             SilverGateDecisionReason = "skip_disk_guard"
	SilverGateSkipBackupGuard           SilverGateDecisionReason = "skip_backup_guard"
	SilverGateSkipArchiveGuard          SilverGateDecisionReason = "skip_archive_guard"
	SilverGateSkipAlertingGuard         SilverGateDecisionReason = "skip_alerting_guard"
	SilverGateSkipCorpusUnhealthy       SilverGateDecisionReason = "skip_corpus_unhealthy"
	SilverGateSkipHostedUnhealthy       SilverGateDecisionReason = "skip_hosted_unhealthy"
	SilverGateSkipCounterUnavailable    SilverGateDecisionReason = "skip_counter_unavailable"
	SilverGateSkipCounterStale          SilverGateDecisionReason = "skip_counter_stale"
)

// SilverGateResult is the pure evaluation outcome for one candidate.
type SilverGateResult struct {
	Decision     SilverGateDecisionReason
	AllowEnqueue bool
}

// SilverEnqueueRequest is the logical enqueue payload for future implementation.
type SilverEnqueueRequest struct {
	Tier           string
	Login          string
	ChannelID      string
	StreamID       string
	Source         string
	Priority       int
	Reason         string
	IdempotencyKey string
	CreatedBy      string
}
