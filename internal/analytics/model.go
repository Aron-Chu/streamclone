package analytics

import "time"

type SourceStatus struct {
	Source  string `json:"source"`
	State   string `json:"state"`
	Message string `json:"message,omitempty"`
}

type WatchResponse struct {
	Channel  string         `json:"channel"`
	Tracking bool           `json:"tracking"`
	Active   int            `json:"active"`
	Max      int            `json:"max"`
	Message  string         `json:"message,omitempty"`
	Sources  []SourceStatus `json:"sources"`
}

type StreamRecord struct {
	StreamID          string     `json:"streamId"`
	BroadcasterID     string     `json:"broadcasterId"`
	Login             string     `json:"login"`
	DisplayName       string     `json:"displayName,omitempty"`
	ProfileImageURL   string     `json:"profileImageUrl,omitempty"`
	Description       string     `json:"description,omitempty"`
	Title             string     `json:"title,omitempty"`
	Category          string     `json:"category,omitempty"`
	Tags              []string   `json:"tags"`
	Language          string     `json:"language,omitempty"`
	ThumbnailURL      string     `json:"thumbnailUrl,omitempty"`
	StartedAt         time.Time  `json:"startedAt"`
	EndedAt           *time.Time `json:"endedAt,omitempty"`
	LastSeenAt        time.Time  `json:"lastSeenAt"`
	CurrentViewers    int        `json:"currentViewers"`
	AvgViewers        int        `json:"avgViewers"`
	PeakViewers       int        `json:"peakViewers"`
	ViewerSamples     int        `json:"viewerSamples"`
	ChatMessages      int64      `json:"chatMessages"`
	TotalEmoteUses    int64      `json:"totalEmoteUses"`
	SevenTVEmoteUses  int64      `json:"seventvEmoteUses"`
	VodID             string     `json:"vodId,omitempty"`
	VodSource         string     `json:"vodSource,omitempty"`
	CanonicalStreamID string     `json:"canonicalStreamId,omitempty"`
	ViewerSource      string     `json:"viewerSource,omitempty"`
}

type MinuteRollup struct {
	MinuteTS          time.Time      `json:"minuteTs"`
	ViewerAvg         int            `json:"viewerAvg"`
	ViewerMax         int            `json:"viewerMax"`
	ViewerLatest      int            `json:"viewerLatest"`
	ViewerSamples     int            `json:"viewerSamples"`
	ChatCount         int            `json:"chatCount"`
	TotalEmoteCount   int            `json:"totalEmoteCount"`
	SevenTVEmoteCount int            `json:"seventvEmoteCount"`
	Emotes            map[string]int `json:"emotes,omitempty"`
	Missing           bool           `json:"missing,omitempty"`
	ChatSource        string         `json:"chatSource,omitempty"`
	SourceConfidence  string         `json:"sourceConfidence,omitempty"`
	ChatSourceDetail  string         `json:"chatSourceDetail,omitempty"`
}

type TopEmote struct {
	Key       string `json:"key"`
	Name      string `json:"name"`
	ID        string `json:"id,omitempty"`
	Provider  string `json:"provider,omitempty"`
	ImageURL  string `json:"imageUrl,omitempty"`
	Count     int    `json:"count"`
	ZeroWidth bool   `json:"zeroWidth,omitempty"`
	Animated  bool   `json:"animated,omitempty"`
}

type StreamDetailResponse struct {
	Channel         string                    `json:"channel"`
	State           string                    `json:"state"`
	Stream          *StreamRecord             `json:"stream,omitempty"`
	Rollups         []MinuteRollup            `json:"rollups"`
	TopEmotes       []TopEmote                `json:"topEmotes"`
	Sources         []SourceStatus            `json:"sources"`
	UpdatedAt       int64                     `json:"updatedAt"`
	VodID           string                    `json:"vodId,omitempty"`
	VodSource       string                    `json:"vodSource,omitempty"`
	SyncPhase       string                    `json:"syncPhase,omitempty"`
	ChatCoveragePct float64                   `json:"chatCoveragePct,omitempty"`
	VodDurationSec  int                       `json:"vodDurationSec,omitempty"`
	ChatCoverage    *ChatCoverageSummary      `json:"chatCoverage,omitempty"`
	ChatSourceMeta  *StreamChatSourceMetadata `json:"chatSource,omitempty"`
	ViewerSource    string                    `json:"viewerSource,omitempty"`
	StoredArtifacts *StoredArtifactsSummary   `json:"storedArtifacts,omitempty"`
}

type StreamsResponse struct {
	Channel   string         `json:"channel"`
	Items     []StreamRecord `json:"items"`
	Sources   []SourceStatus `json:"sources"`
	UpdatedAt int64          `json:"updatedAt"`
}

type StreamSummaryMetrics struct {
	ChatPerMin        float64 `json:"chat_per_min"`
	EmotesPerMin      float64 `json:"emotes_per_min"`
	SevenTVPerMin     float64 `json:"seventv_per_min"`
	ProviderSharePct  float64 `json:"provider_share_pct"`
	ReactionScore     float64 `json:"reaction_score_0_100"`
	ViewerMomentum5M  float64 `json:"viewer_momentum_5m"`
	DataCoveragePct   float64 `json:"data_coverage_pct"`
	SyncHealthState   string  `json:"sync_health_state"`
	MinutesWithData   int     `json:"minutesWithData"`
	ViewerSampleCount int     `json:"viewerSampleCount"`
}

type StreamSummaryResponse struct {
	Channel         string                  `json:"channel"`
	Stream          *StreamRecord           `json:"stream,omitempty"`
	Metrics         StreamSummaryMetrics    `json:"metrics"`
	TopEmotes       []TopEmote              `json:"topEmotes"`
	Sources         []SourceStatus          `json:"sources"`
	UpdatedAt       int64                   `json:"updatedAt"`
	StoredArtifacts *StoredArtifactsSummary `json:"storedArtifacts,omitempty"`
}

type RankedStreamsResponse struct {
	Channel   string         `json:"channel"`
	Sort      string         `json:"sort"`
	Period    string         `json:"period"`
	Items     []StreamRecord `json:"items"`
	Sources   []SourceStatus `json:"sources"`
	UpdatedAt int64          `json:"updatedAt"`
}

type LiveStream struct {
	ID            string
	BroadcasterID string
	Login         string
	DisplayName   string
	GameName      string
	Title         string
	Tags          []string
	ViewerCount   int
	StartedAt     time.Time
	Language      string
	ThumbnailURL  string
}

type UserProfile struct {
	ID              string
	Login           string
	DisplayName     string
	ProfileImageURL string
	Description     string
}

type GameSegment struct {
	ID              int       `json:"id"`
	StreamID        string    `json:"streamId"`
	GameName        string    `json:"gameName"`
	BoxArtURL       string    `json:"boxArtUrl"`
	OffsetSeconds   int       `json:"offsetSeconds"`
	DurationSeconds int       `json:"durationSeconds"`
	CreatedAt       time.Time `json:"createdAt"`
}

type SyncResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}
