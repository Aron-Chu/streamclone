package analytics

import (
	"crypto/sha1"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"streamclone/internal/analytics/recap"
)

const (
	ClipCandidateSourceRecap      = "recap"
	ClipCandidateSourceAvailable  = "available"
	ClipCandidateSourceMissing    = "missing"
	ClipCandidateSourceRestricted = "restricted"
	ClipCandidateSourceUnknown    = "unknown"

	ClipCandidateStatusNew       = "new"
	ClipCandidateStatusSaved     = "saved"
	ClipCandidateStatusDismissed = "dismissed"

	defaultClipCandidateLimit  = 5
	maxClipCandidateLimit      = 100
	defaultClipSeedStreamLimit = 5
	clipCandidateCursorPrefix  = "v1:"
)

type ClipCandidateBuildOptions struct {
	MaxCandidates              int
	MinScore                   int
	MinConfidence              float64
	MinChatCount               int
	MaxChatCount               int
	MinEmoteCount              int
	MinProviderEmoteCount      int
	ProviderEmoteProvider      string
	MinNonMissingRollupMinutes int
	RequireSourceAvailable     bool
	DuplicateRadiusSeconds     int
	MaxCandidatesPerHour       int
	MissingWindows             []ClipCandidateWindow
	ViewerOnlyAllowed          bool
	MinViewerCount             int
	RequireSignalForUnknown    bool
}

type ClipCandidateWindow struct {
	StartSeconds int
	EndSeconds   int
}

type ClipCandidateEmote struct {
	Provider string `json:"provider,omitempty"`
	ID       string `json:"id,omitempty"`
	Name     string `json:"name"`
	Count    int    `json:"count"`
	ImageURL string `json:"imageUrl,omitempty"`
}

type ClipCandidate struct {
	ID                  string                 `json:"id"`
	Login               string                 `json:"login"`
	StreamID            string                 `json:"streamId"`
	VodID               *string                `json:"vodId,omitempty"`
	StreamTitle         string                 `json:"streamTitle,omitempty"`
	StreamCategory      string                 `json:"streamCategory,omitempty"`
	StartedAt           *time.Time             `json:"startedAt,omitempty"`
	MinuteTS            *time.Time             `json:"minuteTs,omitempty"`
	OffsetSeconds       int                    `json:"offsetSeconds"`
	StartSeconds        int                    `json:"startSeconds"`
	EndSeconds          int                    `json:"endSeconds"`
	Score               int                    `json:"score"`
	Confidence          float64                `json:"confidence,omitempty"`
	Reason              string                 `json:"reason"`
	PickReason          string                 `json:"pickReason,omitempty"`
	ConfidenceBand      string                 `json:"confidenceBand,omitempty"`
	InboxState          string                 `json:"inboxState,omitempty"`
	RenderabilityStatus string                 `json:"renderabilityStatus,omitempty"`
	StatusCopy          string                 `json:"statusCopy,omitempty"`
	ChatCount           int                    `json:"chatCount,omitempty"`
	EmoteCount          int                    `json:"emoteCount,omitempty"`
	ViewerCount         int                    `json:"viewerCount,omitempty"`
	TopEmotes           []ClipCandidateEmote   `json:"topEmotes,omitempty"`
	SourceKind          string                 `json:"sourceKind"`
	CoverageState       string                 `json:"coverageState,omitempty"`
	SourceStatus        string                 `json:"sourceStatus"`
	SourceCheckedAt     *time.Time             `json:"sourceCheckedAt,omitempty"`
	Signals             map[string]interface{} `json:"signals,omitempty"`
	State               *ClipCandidateState    `json:"state,omitempty"`
	Job                 *ClipCandidateJob      `json:"job,omitempty"`
	CreatedAt           time.Time              `json:"createdAt,omitempty"`
	UpdatedAt           time.Time              `json:"updatedAt,omitempty"`
}

type ClipCandidateState struct {
	ID                   string    `json:"id"`
	CandidateID          string    `json:"candidateId"`
	PrincipalID          string    `json:"-"`
	PrincipalKind        string    `json:"-"`
	Status               string    `json:"status"`
	TitleOverride        *string   `json:"titleOverride,omitempty"`
	StartSecondsOverride *int      `json:"startSecondsOverride,omitempty"`
	EndSecondsOverride   *int      `json:"endSecondsOverride,omitempty"`
	Notes                string    `json:"notes,omitempty"`
	CreatedAt            time.Time `json:"createdAt,omitempty"`
	UpdatedAt            time.Time `json:"updatedAt,omitempty"`
}

type ClipCandidateListResponse struct {
	Items      []ClipCandidate `json:"items"`
	NextCursor string          `json:"nextCursor,omitempty"`
}

type ClipCandidatePreviewResponse struct {
	Items     []ClipCandidate                `json:"items"`
	Controls  ClipCandidatePreviewControls   `json:"controls"`
	Source    ClipCandidatePreviewSourceInfo `json:"source"`
	Summary   ClipCandidateTuningSummary     `json:"summary"`
	Persisted bool                           `json:"persisted"`
}

type ClipCandidatePreviewControls struct {
	MaxCandidates              int     `json:"maxCandidates"`
	MinScore                   int     `json:"minScore"`
	MinConfidence              float64 `json:"minConfidence"`
	MinChatCount               int     `json:"minChatCount"`
	MaxChatCount               int     `json:"maxChatCount"`
	MinEmoteCount              int     `json:"minEmoteCount"`
	MinProviderEmoteCount      int     `json:"minProviderEmoteCount"`
	ProviderEmoteProvider      string  `json:"providerEmoteProvider,omitempty"`
	MinNonMissingRollupMinutes int     `json:"minNonMissingRollupMinutes"`
	DuplicateRadiusSeconds     int     `json:"duplicateRadiusSeconds"`
	MaxCandidatesPerHour       int     `json:"maxCandidatesPerHour"`
	RequireSourceAvailable     bool    `json:"requireSourceAvailable"`
}

type ClipCandidatePreviewSourceInfo struct {
	StreamID                string `json:"streamId"`
	Login                   string `json:"login"`
	VodID                   string `json:"vodId,omitempty"`
	DurationSeconds         int    `json:"durationSeconds"`
	NonMissingRollupMinutes int    `json:"nonMissingRollupMinutes"`
	MissingWindowCount      int    `json:"missingWindowCount"`
	TopMomentCount          int    `json:"topMomentCount"`
	RecapCandidateCount     int    `json:"recapCandidateCount"`
}

type ClipCandidateTuningSummary struct {
	CandidatePoolCount int `json:"candidatePoolCount"`
	SelectedCount      int `json:"selectedCount"`
	MinChatObserved    int `json:"minChatObserved"`
	MaxChatObserved    int `json:"maxChatObserved"`
	BelowMinChatCount  int `json:"belowMinChatCount"`
	AboveMaxChatCount  int `json:"aboveMaxChatCount"`
	InChatRangeCount   int `json:"inChatRangeCount"`
}

func clipCandidatePreviewControlsFromOptions(opts ClipCandidateBuildOptions) ClipCandidatePreviewControls {
	return ClipCandidatePreviewControls{
		MaxCandidates:              effectiveClipCandidateMax(opts.MaxCandidates),
		MinScore:                   clipMaxInt(0, opts.MinScore),
		MinConfidence:              clipClampFloat(opts.MinConfidence, 0, 1),
		MinChatCount:               clipMaxInt(0, opts.MinChatCount),
		MaxChatCount:               clipMaxInt(0, opts.MaxChatCount),
		MinEmoteCount:              clipMaxInt(0, opts.MinEmoteCount),
		MinProviderEmoteCount:      clipMaxInt(0, opts.MinProviderEmoteCount),
		ProviderEmoteProvider:      strings.TrimSpace(strings.ToLower(opts.ProviderEmoteProvider)),
		MinNonMissingRollupMinutes: clipMaxInt(0, opts.MinNonMissingRollupMinutes),
		DuplicateRadiusSeconds:     clipMaxInt(0, opts.DuplicateRadiusSeconds),
		MaxCandidatesPerHour:       clipMaxInt(0, opts.MaxCandidatesPerHour),
		RequireSourceAvailable:     opts.RequireSourceAvailable,
	}
}

func clipCandidatePreviewSourceFromRecap(stream *StreamRecord, rec recap.StreamRecap) ClipCandidatePreviewSourceInfo {
	source := ClipCandidatePreviewSourceInfo{
		StreamID:                firstNonEmptyClipString(rec.StreamID, streamIDFromRecord(stream)),
		Login:                   normalizeLogin(firstNonEmptyClipString(rec.Login, loginFromRecord(stream))),
		DurationSeconds:         clipMaxInt(0, rec.DurationSeconds),
		NonMissingRollupMinutes: clipMaxInt(0, rec.NonMissingRollupMinutes),
		MissingWindowCount:      len(rec.MissingWindows),
		TopMomentCount:          len(rec.TopMoments),
		RecapCandidateCount:     len(rec.ClipCandidates),
	}
	if vodID := candidateVodID(stream, rec); vodID != nil {
		source.VodID = *vodID
	}
	return source
}

func clipCandidateTuningSummaryFromRecap(rec recap.StreamRecap, selected []ClipCandidate, opts ClipCandidateBuildOptions) ClipCandidateTuningSummary {
	moments := rec.ClipCandidates
	if len(moments) == 0 {
		moments = rec.TopMoments
	}
	summary := ClipCandidateTuningSummary{
		CandidatePoolCount: len(moments),
		SelectedCount:      len(selected),
	}
	for i, moment := range moments {
		chatCount := clipMaxInt(0, moment.ChatCount)
		if i == 0 || chatCount < summary.MinChatObserved {
			summary.MinChatObserved = chatCount
		}
		if i == 0 || chatCount > summary.MaxChatObserved {
			summary.MaxChatObserved = chatCount
		}
		if opts.MinChatCount > 0 && chatCount < opts.MinChatCount {
			summary.BelowMinChatCount++
			continue
		}
		if opts.MaxChatCount > 0 && chatCount > opts.MaxChatCount {
			summary.AboveMaxChatCount++
			continue
		}
		summary.InChatRangeCount++
	}
	return summary
}

func streamIDFromRecord(stream *StreamRecord) string {
	if stream == nil {
		return ""
	}
	return stream.StreamID
}

func loginFromRecord(stream *StreamRecord) string {
	if stream == nil {
		return ""
	}
	return stream.Login
}

type UpdateClipCandidateStateRequest struct {
	Status               *string `json:"status,omitempty"`
	TitleOverride        *string `json:"titleOverride,omitempty"`
	StartSecondsOverride *int    `json:"startSecondsOverride,omitempty"`
	EndSecondsOverride   *int    `json:"endSecondsOverride,omitempty"`
	Notes                *string `json:"notes,omitempty"`
}

type clipCandidateStatePatch struct {
	Status               *string
	TitleOverride        *string
	StartSecondsOverride *int
	EndSecondsOverride   *int
	Notes                *string
}

type ListClipCandidatesFilter struct {
	Login        string
	StreamID     string
	Status       string
	PrincipalID  string
	Limit        int
	MinChatCount int
	MaxChatCount int
	Cursor       *ClipCandidateCursor
	Before       time.Time
}

type ClipCandidateCursor struct {
	Score     int
	CreatedAt time.Time
	ID        string
}

func BuildClipCandidatesFromRecap(stream *StreamRecord, rec recap.StreamRecap, opts ClipCandidateBuildOptions) []ClipCandidate {
	if stream == nil {
		return nil
	}
	if opts.MinNonMissingRollupMinutes > 0 && rec.NonMissingRollupMinutes < opts.MinNonMissingRollupMinutes {
		return nil
	}
	limit := opts.MaxCandidates
	limit = effectiveClipCandidateMax(limit)
	moments := rec.ClipCandidates
	if len(moments) == 0 {
		moments = rec.TopMoments
	}
	out := make([]ClipCandidate, 0, clipMinInt(limit, len(moments)))
	perHour := map[int]int{}
	missingWindows := opts.MissingWindows
	if len(missingWindows) == 0 {
		missingWindows = clipCandidateWindowsFromRecap(rec.MissingWindows)
	}
	for _, moment := range moments {
		if len(out) == limit {
			break
		}
		if moment.OffsetSeconds < 0 {
			continue
		}
		if !clipMomentPassesPolicy(moment, opts) {
			continue
		}
		if clipMomentInMissingWindow(moment.OffsetSeconds, missingWindows) {
			continue
		}
		if clipMomentWithinDuplicateRadius(moment.OffsetSeconds, out, opts.DuplicateRadiusSeconds) {
			continue
		}
		if opts.MaxCandidatesPerHour > 0 {
			hour := moment.OffsetSeconds / 3600
			if perHour[hour] >= opts.MaxCandidatesPerHour {
				continue
			}
		}
		reason := firstClipReason(moment.Reasons)
		pickReason := clipCandidatePickReasonFromMoment(moment)
		vodID := candidateVodID(stream, rec)
		sourceStatus := ClipCandidateSourceMissing
		if vodID != nil && strings.TrimSpace(*vodID) != "" {
			sourceStatus = ClipCandidateSourceAvailable
		}
		if opts.RequireSourceAvailable && sourceStatus != ClipCandidateSourceAvailable {
			continue
		}
		startSeconds, endSeconds := suggestedClipRange(moment.OffsetSeconds, rec.DurationSeconds)
		var startedAtPtr, minuteTS *time.Time
		if !stream.StartedAt.IsZero() {
			startedAt := stream.StartedAt.UTC()
			startedAtPtr = &startedAt
			ts := stream.StartedAt.Add(time.Duration(moment.OffsetSeconds) * time.Second).UTC()
			minuteTS = &ts
		}
		candidate := ClipCandidate{
			ID:             newClipCandidateID(stream.StreamID, moment.OffsetSeconds, reason),
			Login:          normalizeLogin(firstNonEmptyClipString(stream.Login, rec.Login)),
			StreamID:       stream.StreamID,
			VodID:          vodID,
			StreamTitle:    stream.Title,
			StreamCategory: stream.Category,
			StartedAt:      startedAtPtr,
			MinuteTS:       minuteTS,
			OffsetSeconds:  moment.OffsetSeconds,
			StartSeconds:   startSeconds,
			EndSeconds:     endSeconds,
			Score:          clipClampInt(moment.Score, 0, 100),
			Confidence:     clipClampFloat(moment.Confidence, 0, 1),
			Reason:         reason,
			PickReason:     pickReason,
			ChatCount:      clipMaxInt(0, moment.ChatCount),
			EmoteCount:     clipMaxInt(0, moment.EmoteCount),
			ViewerCount:    clipMaxInt(0, moment.ViewerCount),
			TopEmotes:      clipCandidateEmotesFromRecap(moment.TopEmotes),
			SourceKind:     ClipCandidateSourceRecap,
			CoverageState:  clipCandidateCoverageState(sourceStatus),
			SourceStatus:   sourceStatus,
			Signals: map[string]interface{}{
				"chatCount":   clipMaxInt(0, moment.ChatCount),
				"emoteCount":  clipMaxInt(0, moment.EmoteCount),
				"viewerCount": clipMaxInt(0, moment.ViewerCount),
				"confidence":  clipClampFloat(moment.Confidence, 0, 1),
				"pickReason":  pickReason,
			},
		}
		enrichClipCandidateInbox(&candidate)
		out = append(out, candidate)
		if opts.MaxCandidatesPerHour > 0 {
			perHour[moment.OffsetSeconds/3600]++
		}
	}
	return out
}

func effectiveClipCandidateMax(value int) int {
	if value <= 0 {
		return defaultClipCandidateLimit
	}
	if value > maxClipCandidateLimit {
		return maxClipCandidateLimit
	}
	return value
}

func clipMomentPassesPolicy(moment recap.Moment, opts ClipCandidateBuildOptions) bool {
	if opts.MinScore > 0 && moment.Score < opts.MinScore {
		return false
	}
	if opts.MinConfidence > 0 && moment.Confidence < opts.MinConfidence {
		return false
	}
	if opts.MinChatCount > 0 && moment.ChatCount < opts.MinChatCount {
		return false
	}
	if opts.MaxChatCount > 0 && moment.ChatCount > opts.MaxChatCount {
		return false
	}
	if opts.MinEmoteCount > 0 && moment.EmoteCount < opts.MinEmoteCount {
		return false
	}
	if opts.MinProviderEmoteCount > 0 && clipProviderEmoteCount(moment.TopEmotes, opts.ProviderEmoteProvider) < opts.MinProviderEmoteCount {
		return false
	}
	if opts.MinViewerCount > 0 && moment.ViewerCount < opts.MinViewerCount {
		return false
	}
	if opts.RequireSignalForUnknown && moment.ChatCount <= 0 && moment.EmoteCount <= 0 && (!opts.ViewerOnlyAllowed || moment.ViewerCount <= 0) {
		return false
	}
	return true
}

func clipCandidateWindowsFromRecap(windows []recap.MissingWindow) []ClipCandidateWindow {
	if len(windows) == 0 {
		return nil
	}
	out := make([]ClipCandidateWindow, 0, len(windows))
	for _, window := range windows {
		out = append(out, ClipCandidateWindow{
			StartSeconds: window.StartSeconds,
			EndSeconds:   window.EndSeconds,
		})
	}
	return out
}

func clipProviderEmoteCount(emotes []recap.Emote, provider string) int {
	provider = strings.TrimSpace(strings.ToLower(provider))
	total := 0
	for _, emote := range emotes {
		count := clipMaxInt(0, emote.Count)
		if count == 0 {
			continue
		}
		emoteProvider := strings.TrimSpace(strings.ToLower(emote.Provider))
		if provider != "" {
			if emoteProvider == provider {
				total += count
			}
			continue
		}
		if emoteProvider != "" {
			total += count
		}
	}
	return total
}

func clipMomentInMissingWindow(offsetSeconds int, windows []ClipCandidateWindow) bool {
	for _, window := range windows {
		start := clipMaxInt(0, window.StartSeconds)
		end := clipMaxInt(start, window.EndSeconds)
		if offsetSeconds >= start && offsetSeconds <= end {
			return true
		}
	}
	return false
}

func clipMomentWithinDuplicateRadius(offsetSeconds int, selected []ClipCandidate, radiusSeconds int) bool {
	if radiusSeconds <= 0 {
		return false
	}
	for _, item := range selected {
		if clipAbsInt(offsetSeconds-item.OffsetSeconds) <= radiusSeconds {
			return true
		}
	}
	return false
}

func clipCandidateCoverageState(sourceStatus string) string {
	if sourceStatus == ClipCandidateSourceAvailable {
		return "ready"
	}
	return "source_missing"
}

func firstClipReason(reasons []string) string {
	for _, reason := range reasons {
		if v := strings.TrimSpace(strings.ToLower(reason)); v != "" {
			return v
		}
	}
	return "manual"
}

func candidateVodID(stream *StreamRecord, rec recap.StreamRecap) *string {
	if stream != nil && strings.TrimSpace(stream.VodID) != "" {
		v := strings.TrimSpace(stream.VodID)
		return &v
	}
	if rec.VodID != nil && strings.TrimSpace(*rec.VodID) != "" {
		v := strings.TrimSpace(*rec.VodID)
		return &v
	}
	return nil
}

func suggestedClipRange(offsetSeconds, durationSeconds int) (int, int) {
	start := clipMaxInt(0, offsetSeconds-20)
	end := offsetSeconds + 40
	if durationSeconds > 0 && end > durationSeconds {
		end = durationSeconds
	}
	if end <= start {
		end = start + 5
	}
	return start, end
}

func clipCandidateEmotesFromRecap(in []recap.Emote) []ClipCandidateEmote {
	if len(in) == 0 {
		return nil
	}
	limit := clipMinInt(5, len(in))
	out := make([]ClipCandidateEmote, 0, limit)
	for _, item := range in {
		if len(out) == limit {
			break
		}
		name := strings.TrimSpace(item.Code)
		if name == "" {
			continue
		}
		out = append(out, ClipCandidateEmote{
			Provider: strings.TrimSpace(strings.ToLower(item.Provider)),
			ID:       strings.TrimSpace(item.ID),
			Name:     name,
			Count:    clipMaxInt(0, item.Count),
			ImageURL: strings.TrimSpace(item.ImageURL),
		})
	}
	return out
}

func newClipCandidateID(streamID string, offsetSeconds int, reason string) string {
	sum := sha1.Sum([]byte(strings.TrimSpace(streamID) + ":" + strconv.Itoa(offsetSeconds) + ":" + strings.TrimSpace(strings.ToLower(reason))))
	return "cc_" + hex.EncodeToString(sum[:8])
}

func encodeClipCandidateCursor(candidate ClipCandidate) string {
	payload := struct {
		Score     int    `json:"score"`
		CreatedAt string `json:"createdAt"`
		ID        string `json:"id"`
	}{
		Score:     candidate.Score,
		CreatedAt: candidate.CreatedAt.UTC().Format(time.RFC3339Nano),
		ID:        candidate.ID,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return candidate.CreatedAt.UTC().Format(time.RFC3339Nano)
	}
	return clipCandidateCursorPrefix + base64.RawURLEncoding.EncodeToString(body)
}

func parseClipCandidateCursor(value string) (ClipCandidateCursor, bool, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return ClipCandidateCursor{}, false, nil
	}
	if !strings.HasPrefix(value, clipCandidateCursorPrefix) {
		before, err := time.Parse(time.RFC3339Nano, value)
		if err != nil {
			return ClipCandidateCursor{}, false, err
		}
		return ClipCandidateCursor{CreatedAt: before.UTC()}, true, nil
	}
	body, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(value, clipCandidateCursorPrefix))
	if err != nil {
		return ClipCandidateCursor{}, false, err
	}
	var payload struct {
		Score     int    `json:"score"`
		CreatedAt string `json:"createdAt"`
		ID        string `json:"id"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return ClipCandidateCursor{}, false, err
	}
	createdAt, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(payload.CreatedAt))
	if err != nil {
		return ClipCandidateCursor{}, false, err
	}
	id := strings.TrimSpace(payload.ID)
	if id == "" {
		return ClipCandidateCursor{}, false, fmt.Errorf("cursor missing id")
	}
	return ClipCandidateCursor{Score: payload.Score, CreatedAt: createdAt.UTC(), ID: id}, false, nil
}

func normalizeClipCandidateStatePatch(req UpdateClipCandidateStateRequest) (clipCandidateStatePatch, error) {
	var patch clipCandidateStatePatch
	if req.Status != nil {
		status := strings.TrimSpace(strings.ToLower(*req.Status))
		switch status {
		case ClipCandidateStatusNew, ClipCandidateStatusSaved, ClipCandidateStatusDismissed:
			patch.Status = &status
		default:
			return clipCandidateStatePatch{}, errors.New("invalid_status")
		}
	}
	if req.TitleOverride != nil {
		title := clampText(strings.TrimSpace(*req.TitleOverride), 160)
		patch.TitleOverride = &title
	}
	if req.StartSecondsOverride != nil {
		if *req.StartSecondsOverride < 0 {
			return clipCandidateStatePatch{}, errors.New("invalid_start_seconds")
		}
		patch.StartSecondsOverride = req.StartSecondsOverride
	}
	if req.EndSecondsOverride != nil {
		if *req.EndSecondsOverride < 0 {
			return clipCandidateStatePatch{}, errors.New("invalid_end_seconds")
		}
		patch.EndSecondsOverride = req.EndSecondsOverride
	}
	if patch.StartSecondsOverride != nil && patch.EndSecondsOverride != nil && *patch.EndSecondsOverride <= *patch.StartSecondsOverride {
		return clipCandidateStatePatch{}, errors.New("invalid_range")
	}
	if req.Notes != nil {
		notes := clampText(strings.TrimSpace(*req.Notes), 1000)
		patch.Notes = &notes
	}
	if patch.Status == nil && patch.TitleOverride == nil && patch.StartSecondsOverride == nil && patch.EndSecondsOverride == nil && patch.Notes == nil {
		return clipCandidateStatePatch{}, errors.New("empty_patch")
	}
	return patch, nil
}

func newClipCandidateStateID(candidateID, principalID string) string {
	sum := sha1.Sum([]byte(candidateID + ":" + principalID))
	return "ccs_" + hex.EncodeToString(sum[:8])
}

func firstNonEmptyClipString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func clipClampInt(value, lo, hi int) int {
	if value < lo {
		return lo
	}
	if value > hi {
		return hi
	}
	return value
}

func clipClampFloat(value, lo, hi float64) float64 {
	if value < lo {
		return lo
	}
	if value > hi {
		return hi
	}
	return value
}

func clipMaxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func clipMinInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func clipAbsInt(value int) int {
	if value < 0 {
		return -value
	}
	return value
}
