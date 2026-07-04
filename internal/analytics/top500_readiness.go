package analytics

import (
	"context"
	"strings"
	"time"

	"streamclone/internal/metrics"
)

const (
	Top100ReadinessMetadataOnly    = "metadata_only"
	Top100ReadinessCapacityBlocked = "capacity_blocked"
	Top100ReadinessWarming         = "warming"
	Top100ReadinessCollecting      = "collecting"
	Top100ReadinessViewerOnly      = "viewer_only"

	top100ZeroChatAgeThreshold = 5 * time.Minute
)

type Top100ReadinessRow struct {
	Login                    string     `json:"login"`
	Rank                     int        `json:"rank"`
	StreamID                 string     `json:"streamId,omitempty"`
	IsLive                   bool       `json:"isLive"`
	MetadataSampledAt        time.Time  `json:"metadataSampledAt,omitempty"`
	MetadataFreshnessSeconds *int       `json:"metadataFreshnessSeconds,omitempty"`
	MetadataStale            bool       `json:"metadataStale"`
	ViewerCount              *int       `json:"viewerCount,omitempty"`
	CategoryName             string     `json:"categoryName,omitempty"`
	CollectorTracking        bool       `json:"collectorTracking"`
	AdmissionOutcome         string     `json:"admissionOutcome,omitempty"`
	AdmissionMessage         string     `json:"admissionMessage,omitempty"`
	AdmissionAttemptedAt     *time.Time `json:"admissionAttemptedAt,omitempty"`
	RollupCount              int        `json:"rollupCount"`
	FirstChatOffsetSeconds   *int       `json:"firstChatOffsetSeconds,omitempty"`
	LatestChatCount          int        `json:"latestChatCount"`
	LatestTotalEmoteCount    int        `json:"latestTotalEmoteCount"`
	LatestSevenTvCount       int        `json:"latestSevenTvCount"`
	ViewerOnlyRecent         bool       `json:"viewerOnlyRecent"`
	ReadinessState           string     `json:"readinessState"`
}

type Top100ReadinessSummary struct {
	LiveRows                 int `json:"liveRows"`
	CollectorTrackingRows    int `json:"collectorTrackingRows"`
	ExpectedCollectorRows    int `json:"expectedCollectorRows"`
	LiveCollectorDeficitRows int `json:"liveCollectorDeficitRows"`
	MetadataOnlyRows         int `json:"metadataOnlyRows"`
	MetadataStaleRows        int `json:"metadataStaleRows"`
	AdmissionDisabledRows    int `json:"admissionDisabledRows"`
	CapacityBlockedRows      int `json:"capacityBlockedRows"`
	WarmingRows              int `json:"warmingRows"`
	CollectingRows           int `json:"collectingRows"`
	ViewerOnlyRows           int `json:"viewerOnlyRows"`
	ZeroChatAfterAgeRows     int `json:"zeroChatAfterAgeRows"`
}

type Top100ReadinessReport struct {
	GeneratedAt      time.Time                   `json:"generatedAt"`
	TopN             int                         `json:"topN"`
	AdmissionEnabled bool                        `json:"admissionEnabled"`
	CollectorActive  int                         `json:"collectorActive"`
	CollectorMax     int                         `json:"collectorMax"`
	Summary          Top100ReadinessSummary      `json:"summary"`
	Rows             []Top100ReadinessRow        `json:"rows"`
	RecentAdmissions []TopRosterAdmissionAttempt `json:"recentAdmissions,omitempty"`
}

// ReadinessReportOptions tunes expensive readiness report work.
type ReadinessReportOptions struct {
	// SkipRollups avoids per-channel RollupsByStream loads (public hub summary only).
	SkipRollups bool
}

func (h *Handler) buildTop100ReadinessReport(ctx context.Context, topN int, admissionEnabled bool, opts ReadinessReportOptions) (Top100ReadinessReport, error) {
	report := Top100ReadinessReport{
		GeneratedAt:      time.Now().UTC(),
		TopN:             topN,
		AdmissionEnabled: admissionEnabled,
		RecentAdmissions: snapshotTopRosterAdmissionAttempts(),
	}
	if report.TopN <= 0 {
		report.TopN = DefaultTop500MetadataTopN
	}
	if h.collector != nil {
		snap := h.collector.TrackingSnapshot()
		report.CollectorActive = snap.Active
		report.CollectorMax = snap.Max
	}
	if h.store == nil {
		return report, nil
	}
	source := NewReadinessLiveAdmissionSource(h.store)
	if source == nil {
		return report, nil
	}
	live, err := source.ListLiveCandidates(ctx, report.TopN)
	if err != nil {
		return report, err
	}
	now := report.GeneratedAt
	rows := make([]Top100ReadinessRow, 0, len(live))
	for _, current := range live {
		row := buildTop100ReadinessRow(ctx, h, current, now, opts.SkipRollups)
		rows = append(rows, row)
		updateTop100ReadinessSummary(&report.Summary, row, current, now)
	}
	report.Rows = rows
	report.Summary.LiveRows = len(rows)
	report.Summary.ExpectedCollectorRows = expectedCollectorRows(report.Summary.LiveRows, report.CollectorMax)
	if !admissionEnabled && report.Summary.LiveRows > 0 {
		report.Summary.AdmissionDisabledRows = report.Summary.LiveRows
	}
	if deficit := report.Summary.ExpectedCollectorRows - report.Summary.CollectorTrackingRows; deficit > 0 {
		report.Summary.LiveCollectorDeficitRows = deficit
	}
	metrics.TopRosterAdmissionZeroChatLiveRows.Set(float64(report.Summary.ZeroChatAfterAgeRows))
	return report, nil
}

func buildTop100ReadinessRow(ctx context.Context, h *Handler, current Top500Current, now time.Time, skipRollups bool) Top100ReadinessRow {
	login := normalizeLogin(current.Login)
	streamID := ""
	if current.StreamID != nil {
		streamID = strings.TrimSpace(*current.StreamID)
	}
	row := Top100ReadinessRow{
		Login:             login,
		Rank:              current.Rank,
		StreamID:          streamID,
		IsLive:            current.IsLive,
		MetadataSampledAt: current.SampledAt,
		CategoryName:      current.CategoryName,
		ViewerCount:       current.ViewerCount,
		MetadataStale:     !current.SampledAt.IsZero() && now.After(current.StaleAfter),
	}
	if !current.SampledAt.IsZero() {
		seconds := int(now.Sub(current.SampledAt).Seconds())
		if seconds < 0 {
			seconds = 0
		}
		row.MetadataFreshnessSeconds = &seconds
	}
	if h.collector != nil {
		row.CollectorTracking = h.collector.IsTracking(login)
	}
	if attempt, ok := getTopRosterAdmissionAttempt(login); ok {
		row.AdmissionOutcome = attempt.Outcome
		row.AdmissionMessage = attempt.Message
		if !attempt.AttemptedAt.IsZero() {
			attemptedAt := attempt.AttemptedAt
			row.AdmissionAttemptedAt = &attemptedAt
		}
	}
	if !skipRollups && streamID != "" && h.store != nil {
		rollups := top100ReadinessRollups(ctx, h, login, streamID, now)
		if rollups != nil {
			row.RollupCount = len(rollups)
			firstChatOffset, hasChat := firstChatOffsetFromRollups(rollups, current.StartedAt)
			if hasChat {
				row.FirstChatOffsetSeconds = &firstChatOffset
			}
			if len(rollups) > 0 {
				last := rollups[len(rollups)-1]
				row.LatestChatCount = last.ChatCount
				row.LatestTotalEmoteCount = last.TotalEmoteCount
				row.LatestSevenTvCount = last.SevenTVEmoteCount
				row.ViewerOnlyRecent = rollupHasViewerSignal(last) && last.ChatCount == 0 && last.SevenTVEmoteCount == 0 && last.TotalEmoteCount == 0
			}
		}
	}
	row.ReadinessState = classifyTop100ReadinessState(row)
	return row
}

func top100ReadinessRollups(ctx context.Context, h *Handler, login, streamID string, now time.Time) []MinuteRollup {
	if h == nil || h.store == nil || strings.TrimSpace(streamID) == "" {
		return nil
	}
	rollups, err := h.store.RollupsByStream(ctx, streamID)
	if err != nil {
		return nil
	}
	if rollupsHaveChatOrEmoteSignal(rollups) {
		return rollups
	}
	if h.collector == nil || !h.collector.IsTracking(login) {
		return rollups
	}
	fallbackRec, fallbackRollups, fallbackErr := h.store.LatestLiveStreamWithRecentRollupsByLogin(ctx, login, now.Add(-top100ZeroChatAgeThreshold), 10)
	if fallbackErr != nil || fallbackRec == nil || fallbackRec.StreamID == streamID || !rollupsHaveChatOrEmoteSignal(fallbackRollups) {
		return rollups
	}
	if full, fullErr := h.store.RollupsByStream(ctx, fallbackRec.StreamID); fullErr == nil && len(full) > 0 {
		return full
	}
	return fallbackRollups
}

func rollupsHaveChatOrEmoteSignal(rollups []MinuteRollup) bool {
	for _, rollup := range rollups {
		if rollup.ChatCount > 0 || rollup.TotalEmoteCount > 0 || rollup.SevenTVEmoteCount > 0 {
			return true
		}
	}
	return false
}

func updateTop100ReadinessSummary(summary *Top100ReadinessSummary, row Top100ReadinessRow, current Top500Current, now time.Time) {
	switch row.ReadinessState {
	case Top100ReadinessMetadataOnly:
		summary.MetadataOnlyRows++
	case Top100ReadinessCapacityBlocked:
		summary.CapacityBlockedRows++
	case Top100ReadinessWarming:
		summary.WarmingRows++
	case Top100ReadinessCollecting:
		summary.CollectingRows++
	case Top100ReadinessViewerOnly:
		summary.ViewerOnlyRows++
	}
	if row.CollectorTracking {
		summary.CollectorTrackingRows++
	}
	if row.MetadataStale {
		summary.MetadataStaleRows++
	}
	if shouldCountZeroChatAfterAge(row, current, now) {
		summary.ZeroChatAfterAgeRows++
	}
}

func expectedCollectorRows(liveRows, collectorMax int) int {
	if liveRows <= 0 {
		return 0
	}
	if collectorMax <= 0 || collectorMax > liveRows {
		return liveRows
	}
	return collectorMax
}

func classifyTop100ReadinessState(row Top100ReadinessRow) string {
	if row.FirstChatOffsetSeconds != nil || row.LatestChatCount > 0 || row.LatestSevenTvCount > 0 {
		return Top100ReadinessCollecting
	}
	if row.CollectorTracking {
		return Top100ReadinessWarming
	}
	if row.AdmissionOutcome == TopRosterAdmissionCapacityFull {
		return Top100ReadinessCapacityBlocked
	}
	if row.RollupCount > 0 && row.ViewerOnlyRecent {
		return Top100ReadinessViewerOnly
	}
	if row.IsLive && !row.MetadataStale {
		return Top100ReadinessMetadataOnly
	}
	return Top100ReadinessMetadataOnly
}

func shouldCountZeroChatAfterAge(row Top100ReadinessRow, current Top500Current, now time.Time) bool {
	if !row.IsLive || row.MetadataStale {
		return false
	}
	if row.LatestChatCount > 0 || row.LatestSevenTvCount > 0 || row.FirstChatOffsetSeconds != nil {
		return false
	}
	ageAnchor := current.SampledAt
	if current.StartedAt != nil && current.StartedAt.After(ageAnchor) {
		ageAnchor = *current.StartedAt
	}
	if ageAnchor.IsZero() {
		return false
	}
	return now.Sub(ageAnchor) >= top100ZeroChatAgeThreshold
}

func firstChatOffsetFromRollups(rollups []MinuteRollup, startedAt *time.Time) (int, bool) {
	if len(rollups) == 0 || startedAt == nil || startedAt.IsZero() {
		for _, rollup := range rollups {
			if rollup.ChatCount > 0 || rollup.SevenTVEmoteCount > 0 || rollup.TotalEmoteCount > 0 {
				return 0, true
			}
		}
		return 0, false
	}
	start := startedAt.UTC()
	for _, rollup := range rollups {
		if rollup.ChatCount == 0 && rollup.SevenTVEmoteCount == 0 && rollup.TotalEmoteCount == 0 {
			continue
		}
		offset := int(rollup.MinuteTS.Sub(start).Seconds())
		if offset < 0 {
			offset = 0
		}
		return offset, true
	}
	return 0, false
}

func rollupHasViewerSignal(rollup MinuteRollup) bool {
	return rollup.ViewerAvg > 0 || rollup.ViewerMax > 0 || rollup.ViewerLatest > 0 || rollup.ViewerSamples > 0
}
