package model

import "time"

type SourceStatus struct {
	Source       string `json:"source"`
	State        string `json:"state"`
	Message      string `json:"message,omitempty"`
	Provider     string `json:"provider,omitempty"`
	BackoffUntil int64  `json:"backoffUntil,omitempty"`
}

type ChatBadge struct {
	SetID       string `json:"setId"`
	VersionID   string `json:"versionId"`
	Title       string `json:"title,omitempty"`
	Description string `json:"description,omitempty"`
	ClickURL    string `json:"clickUrl,omitempty"`
	ImageURL1X  string `json:"imageUrl1x,omitempty"`
	ImageURL2X  string `json:"imageUrl2x,omitempty"`
	ImageURL4X  string `json:"imageUrl4x,omitempty"`
}

type ChatBadgeCatalog struct {
	Channel   string               `json:"channel"`
	Badges    map[string]ChatBadge `json:"badges"`
	Sources   []SourceStatus       `json:"sources"`
	UpdatedAt int64                `json:"updatedAt"`
}

type ChannelDetails struct {
	ID           string         `json:"id"`
	Login        string         `json:"login"`
	DisplayName  string         `json:"displayName"`
	ProfileImage string         `json:"profileImage,omitempty"`
	Description  string         `json:"description,omitempty"`
	CreatedAt    string         `json:"createdAt,omitempty"`
	AboutPanels  []AboutPanel   `json:"aboutPanels,omitempty"`
	SocialLinks  []SocialLink   `json:"socialLinks,omitempty"`
	IsLive       bool           `json:"isLive"`
	StreamID     string         `json:"streamId,omitempty"`
	StreamTitle  string         `json:"streamTitle,omitempty"`
	Category     string         `json:"category,omitempty"`
	Viewers      int            `json:"viewers,omitempty"`
	ThumbnailURL string         `json:"thumbnailUrl,omitempty"`
	StartedAt    string         `json:"startedAt,omitempty"`
	UpdatedAt    int64          `json:"updatedAt"`
	Sources      []SourceStatus `json:"sources,omitempty"`
}

type AboutPanel struct {
	ID          string `json:"id,omitempty"`
	Type        string `json:"type,omitempty"`
	Title       string `json:"title,omitempty"`
	Description string `json:"description,omitempty"`
	ImageURL    string `json:"imageUrl,omitempty"`
	LinkURL     string `json:"linkUrl,omitempty"`
}

type SocialLink struct {
	ID    string `json:"id,omitempty"`
	Title string `json:"title,omitempty"`
	URL   string `json:"url,omitempty"`
}

type ClipCard struct {
	ID              string  `json:"id"`
	Title           string  `json:"title"`
	URL             string  `json:"url"`
	EmbedURL        string  `json:"embedUrl,omitempty"`
	ThumbnailURL    string  `json:"thumbnailUrl,omitempty"`
	BroadcasterName string  `json:"broadcasterName,omitempty"`
	CreatorName     string  `json:"creatorName,omitempty"`
	ViewCount       int     `json:"viewCount,omitempty"`
	CreatedAt       string  `json:"createdAt,omitempty"`
	DurationSeconds float64 `json:"durationSeconds,omitempty"`
}

type ClipsResponse struct {
	Items     []ClipCard     `json:"items"`
	Sources   []SourceStatus `json:"sources"`
	Period    string         `json:"period"`
	Cursor    string         `json:"cursor,omitempty"`
	UpdatedAt int64          `json:"updatedAt"`
}

type ClipQuery struct {
	Limit     int
	Period    string
	Cursor    string
	StartedAt *time.Time
	EndedAt   *time.Time
}

type TwitchTrackerSummary struct {
	Rank            int `json:"rank"`
	MinutesStreamed int `json:"minutes_streamed"`
	AvgViewers      int `json:"avg_viewers"`
	MaxViewers      int `json:"max_viewers"`
	HoursWatched    int `json:"hours_watched"`
	Followers       int `json:"followers"`
	FollowersTotal  int `json:"followers_total"`
}

type StatsTimelinePoint struct {
	Label        string `json:"label"`
	AvgViewers   int    `json:"avgViewers"`
	PeakViewers  int    `json:"peakViewers"`
	HoursWatched int    `json:"hoursWatched,omitempty"`
}

type StreamStat struct {
	ID              string `json:"id"`
	VideoID         string `json:"videoId,omitempty"`
	Title           string `json:"title"`
	Category        string `json:"category,omitempty"`
	ThumbnailURL    string `json:"thumbnailUrl,omitempty"`
	StartedAt       string `json:"startedAt,omitempty"`
	EndedAt         string `json:"endedAt,omitempty"`
	DurationMinutes int    `json:"durationMinutes,omitempty"`
	AvgViewers      int    `json:"avgViewers"`
	PeakViewers     int    `json:"peakViewers"`
	HoursWatched    int    `json:"hoursWatched,omitempty"`
}

type StatsDerived struct {
	HoursStreamed            float64 `json:"hoursStreamed,omitempty"`
	ViewerHoursPerStreamHour float64 `json:"viewerHoursPerStreamHour,omitempty"`
	PeakToAverageRatio       float64 `json:"peakToAverageRatio,omitempty"`
	FollowersPerStreamHour   float64 `json:"followersPerStreamHour,omitempty"`
	ClipsLoaded              int     `json:"clipsLoaded"`
	LSFPostsLoaded           int     `json:"lsfPostsLoaded"`
	HasRealStreamHistory     bool    `json:"hasRealStreamHistory"`
}

type RedditPost struct {
	ID           string   `json:"id"`
	Title        string   `json:"title"`
	URL          string   `json:"url"`
	Permalink    string   `json:"permalink"`
	Thumbnail    string   `json:"thumbnail,omitempty"`
	Author       string   `json:"author,omitempty"`
	Score        int      `json:"score"`
	Comments     int      `json:"comments"`
	CreatedUTC   int64    `json:"createdUtc"`
	Subreddit    string   `json:"subreddit,omitempty"`
	FlairText    string   `json:"flairText,omitempty"`
	StreamerTags []string `json:"streamerTags"`
	Provider     string   `json:"provider,omitempty"`
}

type YouTubeVideo struct {
	ID           string `json:"id"`
	Title        string `json:"title"`
	URL          string `json:"url"`
	ThumbnailURL string `json:"thumbnailUrl,omitempty"`
	PublishedAt  string `json:"publishedAt,omitempty"`
	ViewCount    int64  `json:"viewCount,omitempty"`
}

type YouTubeChannelInfo struct {
	ChannelID        string         `json:"channelId,omitempty"`
	Title            string         `json:"title,omitempty"`
	Handle           string         `json:"handle,omitempty"`
	CustomURL        string         `json:"customUrl,omitempty"`
	SubscriberCount  *int64         `json:"subscriberCount,omitempty"`
	SubscriberHidden bool           `json:"subscriberCountHidden,omitempty"`
	VideoCount       *int64         `json:"videoCount,omitempty"`
	ProfileImageURL  string         `json:"profileImageUrl,omitempty"`
	LatestVideos     []YouTubeVideo `json:"latestVideos"`
}

type YouTubeResponse struct {
	Channel   string              `json:"channel"`
	YouTube   *YouTubeChannelInfo `json:"youtube,omitempty"`
	Sources   []SourceStatus      `json:"sources"`
	UpdatedAt int64               `json:"updatedAt"`
}

type InsightsResponse struct {
	Channel       string                `json:"channel"`
	Period        string                `json:"period"`
	ClipPeriod    string                `json:"clipPeriod"`
	LSFPeriod     string                `json:"lsfPeriod"`
	Stats         *TwitchTrackerSummary `json:"stats,omitempty"`
	StatsDerived  *StatsDerived         `json:"statsDerived,omitempty"`
	StatsTimeline []StatsTimelinePoint  `json:"statsTimeline"`
	StreamHistory []StreamStat          `json:"streamHistory"`
	Clips         []ClipCard            `json:"clips"`
	LSF           []RedditPost          `json:"lsf"`
	Sources       []SourceStatus        `json:"sources"`
	UpdatedAt     int64                 `json:"updatedAt"`
}
