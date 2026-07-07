package analytics

import (
	"fmt"
	"strings"

	"streamclone/internal/analytics/recap"
)

const (
	ClipCandidateRenderabilityNotRenderable         = "not_renderable"
	ClipCandidateRenderabilityQueueable             = "queueable"
	ClipCandidateRenderabilityRenderQueued          = "render_queued"
	ClipCandidateRenderabilityWorkerReadyUnverified = "worker_ready_unverified"
	ClipCandidateRenderabilityRenderFailed          = "render_failed"

	ClipCandidateInboxMoment        = "moment_candidate"
	ClipCandidateInboxNeedsSource   = "needs_source"
	ClipCandidateInboxLowConfidence = "low_confidence"
	ClipCandidateInboxQueueable     = "queueable"

	ClipCandidatePickEmoteSpikeOnly = "emote_spike_only"

	ClipCandidateConfidenceHigh   = "high"
	ClipCandidateConfidenceMedium = "medium"
	ClipCandidateConfidenceLow    = "low"
)

func enrichClipCandidateInbox(candidate *ClipCandidate) {
	if candidate == nil {
		return
	}
	pick := clipCandidatePickReasonFromSignals(candidate.Reason, candidate.ChatCount, candidate.EmoteCount)
	candidate.PickReason = pick
	candidate.ConfidenceBand = clipCandidateConfidenceBand(candidate.Confidence, pick)
	candidate.RenderabilityStatus = clipCandidateRenderabilityStatus(candidate)
	candidate.InboxState = clipCandidateInboxState(candidate)
	candidate.StatusCopy = clipCandidateStatusCopy(candidate)
	if candidate.Signals == nil {
		candidate.Signals = map[string]interface{}{}
	}
	candidate.Signals["pickReason"] = pick
	candidate.Signals["confidenceBand"] = candidate.ConfidenceBand
	candidate.Signals["inboxState"] = candidate.InboxState
	candidate.Signals["renderabilityStatus"] = candidate.RenderabilityStatus
}

func clipCandidatePickReasonFromSignals(reason string, chatCount, emoteCount int) string {
	reason = strings.TrimSpace(strings.ToLower(reason))
	if reason == "" {
		reason = "manual"
	}
	if clipMomentIsEmoteSpikeOnly(reason, chatCount, emoteCount) {
		return ClipCandidatePickEmoteSpikeOnly
	}
	return reason
}

func clipCandidatePickReasonFromMoment(moment recap.Moment) string {
	return clipCandidatePickReasonFromSignals(firstClipReason(moment.Reasons), moment.ChatCount, moment.EmoteCount)
}

func clipMomentIsEmoteSpikeOnly(reason string, chatCount, emoteCount int) bool {
	reason = strings.TrimSpace(strings.ToLower(reason))
	if chatCount <= 0 && emoteCount > 0 {
		return true
	}
	if reason == "emote_spike" && chatCount > 0 && emoteCount >= chatCount*2 {
		return true
	}
	if emoteCount >= 80 && chatCount > 0 && chatCount < 40 && emoteCount >= chatCount*3 {
		return true
	}
	return false
}

func clipCandidateConfidenceBand(confidence float64, pickReason string) string {
	if pickReason == ClipCandidatePickEmoteSpikeOnly {
		return ClipCandidateConfidenceLow
	}
	switch {
	case confidence >= 0.75:
		return ClipCandidateConfidenceHigh
	case confidence >= 0.5:
		return ClipCandidateConfidenceMedium
	default:
		return ClipCandidateConfidenceLow
	}
}

func clipCandidateRenderabilityStatus(candidate *ClipCandidate) string {
	if candidate.Job != nil {
		switch candidate.Job.Status {
		case ClipCandidateJobQueued:
			return ClipCandidateRenderabilityRenderQueued
		case ClipCandidateJobReady:
			return ClipCandidateRenderabilityWorkerReadyUnverified
		case ClipCandidateJobFailed:
			return ClipCandidateRenderabilityRenderFailed
		case ClipCandidateJobSourceUnavailable:
			return ClipCandidateRenderabilityNotRenderable
		}
	}
	switch candidate.SourceStatus {
	case ClipCandidateSourceMissing, ClipCandidateSourceUnknown:
		return ClipCandidateRenderabilityNotRenderable
	case ClipCandidateSourceRestricted:
		return ClipCandidateRenderabilityNotRenderable
	default:
		return ClipCandidateRenderabilityQueueable
	}
}

func clipCandidateInboxState(candidate *ClipCandidate) string {
	if candidate.SourceStatus == ClipCandidateSourceMissing ||
		candidate.SourceStatus == ClipCandidateSourceRestricted ||
		candidate.SourceStatus == ClipCandidateSourceUnknown {
		return ClipCandidateInboxNeedsSource
	}
	if candidate.PickReason == ClipCandidatePickEmoteSpikeOnly ||
		candidate.ConfidenceBand == ClipCandidateConfidenceLow {
		return ClipCandidateInboxLowConfidence
	}
	if candidate.RenderabilityStatus == ClipCandidateRenderabilityQueueable ||
		candidate.RenderabilityStatus == ClipCandidateRenderabilityRenderQueued ||
		candidate.RenderabilityStatus == ClipCandidateRenderabilityWorkerReadyUnverified {
		return ClipCandidateInboxQueueable
	}
	return ClipCandidateInboxMoment
}

func clipCandidateStatusCopy(candidate *ClipCandidate) string {
	if candidate.Job != nil {
		switch candidate.Job.Status {
		case ClipCandidateJobQueued:
			return "ReplayForge render is queued. Private playback is not verified yet."
		case ClipCandidateJobReady:
			return "ReplayForge worker reported ready, but durable private playback is not verified in the portal."
		case ClipCandidateJobFailed:
			if msg := strings.TrimSpace(candidate.Job.ErrorMessage); msg != "" {
				return fmt.Sprintf("ReplayForge render failed: %s", clampText(msg, 160))
			}
			return "ReplayForge render failed. Review source availability and retry."
		case ClipCandidateJobSourceUnavailable:
			return "Source video is unavailable, so this candidate cannot be rendered."
		}
	}
	switch candidate.SourceStatus {
	case ClipCandidateSourceMissing:
		return "High-scoring moment, but no VOD source is linked for rendering."
	case ClipCandidateSourceRestricted:
		return "Source video is restricted or unavailable for private rendering."
	case ClipCandidateSourceUnknown:
		return "Source availability is unknown. Confirm VOD linkage before rendering."
	}
	if candidate.PickReason == ClipCandidatePickEmoteSpikeOnly {
		return "Emote-heavy minute with weak chat hook. Treat as a lower-confidence editorial pick."
	}
	if candidate.ConfidenceBand == ClipCandidateConfidenceLow {
		return fmt.Sprintf("Lower-confidence pick (%s). Review before sending to ReplayForge.", clipCandidateReasonLabel(candidate.Reason))
	}
	return fmt.Sprintf("Deterministic recap pick (%s). Source available; renderability is not verified until ReplayForge completes.", clipCandidateReasonLabel(candidate.Reason))
}

func clipCandidateReasonLabel(reason string) string {
	switch strings.TrimSpace(strings.ToLower(reason)) {
	case "chat_spike":
		return "chat spike"
	case "emote_spike":
		return "emote spike"
	case "viewer_spike":
		return "viewer spike"
	case "emote_spike_only":
		return "emote spike only"
	default:
		if reason == "" {
			return "moment"
		}
		return strings.ReplaceAll(reason, "_", " ")
	}
}
