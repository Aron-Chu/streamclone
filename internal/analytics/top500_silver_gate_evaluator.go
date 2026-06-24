package analytics

import (
	"strings"
	"time"
)

// EvaluateSilverGate applies TOP500-022 ordered checks without side effects.
func EvaluateSilverGate(candidate SilverGateCandidate, budget SilverBudgetSnapshot, _ SilverGateConfig) SilverGateResult {
	login := normalizeLogin(candidate.Login)
	channelID := strings.TrimSpace(candidate.ChannelID)
	if login == "" || channelID == "" {
		return deny(SilverGateSkipNotCandidate)
	}
	if !budget.Available {
		return deny(SilverGateSkipCounterUnavailable)
	}
	if budget.Stale {
		return deny(SilverGateSkipCounterStale)
	}
	if candidate.MetadataStale(time.Now().UTC()) {
		return deny(SilverGateSkipMetadataStale)
	}
	if strings.TrimSpace(candidate.StreamID) == "" {
		return deny(SilverGateSkipMissingStreamID)
	}
	if budget.AlreadyDone {
		return deny(SilverGateSkipAlreadyDone)
	}
	if budget.DuplicateQueuedOrRunning {
		return deny(SilverGateSkipDuplicateJob)
	}
	if budget.InChannelCooldown {
		return deny(SilverGateSkipChannelCooldown)
	}
	if budget.InFailureBackoff {
		return deny(SilverGateSkipRecentFailure)
	}
	if budget.GlobalTTBackoffActive {
		return deny(SilverGateSkipGlobalBackoff)
	}
	if budget.SilverEnqueuedToday >= SilverGateGlobalMaxEnqueuePerDay {
		return deny(SilverGateSkipDailyBudget)
	}
	if budget.SilverRunningNow >= SilverGateGlobalMaxRunning {
		return deny(SilverGateSkipRunningLimit)
	}
	if budget.SilverQueueDepth >= SilverGateGlobalMaxQueueDepth {
		return deny(SilverGateSkipQueueFull)
	}
	if budget.DiskGuardActive {
		return deny(SilverGateSkipDiskGuard)
	}
	if budget.BackupGuardActive {
		return deny(SilverGateSkipBackupGuard)
	}
	if budget.ArchiveGuardActive {
		return deny(SilverGateSkipArchiveGuard)
	}
	if budget.AlertingGuardActive {
		return deny(SilverGateSkipAlertingGuard)
	}
	if budget.CorpusUnhealthy {
		return deny(SilverGateSkipCorpusUnhealthy)
	}
	if budget.HostedUnhealthy {
		return deny(SilverGateSkipHostedUnhealthy)
	}

	return SilverGateResult{
		Decision:     SilverGateAllowEnqueue,
		AllowEnqueue: true,
	}
}

func deny(reason SilverGateDecisionReason) SilverGateResult {
	return SilverGateResult{Decision: reason, AllowEnqueue: false}
}

// MetadataStale reports whether the candidate metadata is outside freshness SLO.
func (c SilverGateCandidate) MetadataStale(now time.Time) bool {
	if c.SampledAt.IsZero() {
		return true
	}
	if !c.StaleAfter.IsZero() && now.After(c.StaleAfter) {
		return true
	}
	return false
}

func normalizeSilverGateConfig(cfg SilverGateConfig) SilverGateConfig {
	if cfg.Interval <= 0 {
		cfg.Interval = DefaultTop500SilverGateInterval
	}
	if cfg.MaxCandidates <= 0 || cfg.MaxCandidates > MaxTop500SilverGateMaxCandidates {
		cfg.MaxCandidates = DefaultTop500SilverGateMaxCandidates
	}
	if cfg.MaxEnqueuePerRun <= 0 || cfg.MaxEnqueuePerRun > MaxTop500SilverGateMaxEnqueuePerRun {
		cfg.MaxEnqueuePerRun = DefaultTop500SilverGateMaxEnqueuePerRun
	}
	return cfg
}
