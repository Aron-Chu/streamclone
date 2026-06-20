package reddit

import (
	"context"
	"strings"
	"time"

	"streamclone/internal/metadata/model"
)

func sourceWithProvider(sourceName, provider, state, message string) model.SourceStatus {
	return model.SourceStatus{Source: sourceName, Provider: provider, State: state, Message: message}
}

func redditLSFRequestContext(parent context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(parent), 12*time.Minute)
}

func redditLSFStatusInterrupted(status model.SourceStatus) bool {
	msg := strings.ToLower(status.Message)
	return strings.Contains(msg, "context canceled") || strings.Contains(msg, "context deadline exceeded")
}

func normalizeRedditLSFStatus(status model.SourceStatus) model.SourceStatus {
	if status.State == "error" && redditLSFStatusInterrupted(status) {
		status.State = "unavailable"
		status.Message = "fetching from Reddit; first load may take a couple of minutes"
	}
	return status
}

func redditStatusContainsProvider(statuses []model.SourceStatus, provider string) bool {
	for _, s := range statuses {
		if s.Provider == provider {
			return true
		}
	}
	return false
}

func redditTime(period string) string {
	switch period {
	case "24h":
		return "day"
	case "30d":
		return "month"
	case "365d":
		return "year"
	case "all":
		return "all"
	default:
		return "week"
	}
}

func normalizeSort(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "hot", "new":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return "top"
	}
}

// WarmingStatus returns the LSF warming source status.
func WarmingStatus() model.SourceStatus {
	return sourceWithProvider("reddit_lsf", "warmup", "unavailable", "fetching from Reddit; first load may take a couple of minutes")
}

// PendingStatus returns the LSF pending source status.
func PendingStatus() model.SourceStatus {
	return sourceWithProvider("reddit_lsf", "pending", "unavailable", "ready to search Reddit when Analytics is idle")
}
