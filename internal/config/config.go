package config

import (
	"strings"
	"time"

	"github.com/caarlos0/env/v11"

	"streamclone/internal/upstream"
)

const defaultScraperAPIURL = "http://scraper:8000/v2/scrape"

type Config struct {
	HTTPAddr    string `env:"HTTP_ADDR" envDefault:":8080"`
	LogLevel    string `env:"LOG_LEVEL" envDefault:"info"`
	RedisURL    string `env:"REDIS_URL" envDefault:"redis://redis:6379/0"`
	DatabaseURL string `env:"DATABASE_URL"`

	MetaCacheTTL        time.Duration `env:"META_CACHE_TTL" envDefault:"30s"`
	StaleTTL            time.Duration `env:"STALE_TTL" envDefault:"24h"`
	TwitchTrackerAPIURL string        `env:"TWITCHTRACKER_API_URL" envDefault:"https://twitchtracker.com/api"`
	RedditAPIURL        string        `env:"REDDIT_API_URL" envDefault:"https://www.reddit.com"`
	RedditProvider      string        `env:"REDDIT_PROVIDER" envDefault:"auto"`
	RedditClientID      string        `env:"REDDIT_CLIENT_ID"`
	RedditClientSecret  string        `env:"REDDIT_CLIENT_SECRET"`
	RedditAccessToken   string        `env:"REDDIT_ACCESS_TOKEN"`
	RedditTokenURL      string        `env:"REDDIT_TOKEN_URL" envDefault:"https://www.reddit.com/api/v1/access_token"`
	RedditOAuthAPIURL   string        `env:"REDDIT_OAUTH_API_URL" envDefault:"https://oauth.reddit.com"`
	RedditHTMLFallback  bool          `env:"REDDIT_HTML_FALLBACK" envDefault:"false"`
	RedditThirdPartyURL string        `env:"REDDIT_THIRD_PARTY_URL"`
	RedditThirdPartyKey string        `env:"REDDIT_THIRD_PARTY_KEY"`
	ScraperAPIURL  string `env:"SCRAPER_API_URL"`
	ScraperAPIKey  string `env:"SCRAPER_API_KEY"`
	FirecrawlAPIURL string `env:"FIRECRAWL_API_URL"` // deprecated alias for SCRAPER_API_URL
	FirecrawlAPIKey string `env:"FIRECRAWL_API_KEY"` // deprecated alias for SCRAPER_API_KEY
	YouTubeAPIKey       string        `env:"YOUTUBE_API_KEY"`
	YouTubeProvider     string        `env:"YOUTUBE_PROVIDER" envDefault:"auto"`
	YouTubeAPIBaseURL   string        `env:"YOUTUBE_API_BASE_URL" envDefault:"https://www.googleapis.com/youtube/v3"`
	TwitchGQLURL        string        `env:"TWITCH_GQL_URL" envDefault:"https://gql.twitch.tv/gql"`
	TwitchClientID      string        `env:"TWITCH_CLIENT_ID" envDefault:"kimne78kx3ncx6brgo4mv6wki5h1ko"`
	EmoteServiceURL     string        `env:"EMOTE_SERVICE_URL"`

	StreamIdleTimeout    time.Duration `env:"STREAM_IDLE_TIMEOUT" envDefault:"60s"`
	MaxConcurrentStreams int           `env:"MAX_CONCURRENT_STREAMS" envDefault:"20"`
	StreamWorkerBackends string        `env:"STREAM_WORKER_BACKENDS" envDefault:"direct_hls,streamlink"`
	DefaultStreamQuality string        `env:"DEFAULT_STREAM_QUALITY" envDefault:"best"`
	MediaMTXRTMP         string        `env:"MEDIAMTX_RTMP" envDefault:"mediamtx:1935"`
	HLSInternalBase      string        `env:"HLS_INTERNAL_BASE" envDefault:"http://mediamtx:8888"`
	HLSPublicBase        string        `env:"HLS_PUBLIC_BASE" envDefault:"http://localhost:8888"`
	BackendVersion       string        `env:"BACKEND_VERSION" envDefault:"dev"`

	BatchWindowMS        int `env:"BATCH_WINDOW_MS" envDefault:"20"`
	ClientSendQueue      int `env:"CLIENT_SEND_QUEUE" envDefault:"256"`
	MaxChannelsPerSocket int `env:"MAX_CHANNELS_PER_SOCKET" envDefault:"30"`
	DeltaDebounceMS      int `env:"DELTA_DEBOUNCE_MS" envDefault:"300"`

	MaxConcurrentTrackedChannels int           `env:"MAX_CONCURRENT_TRACKED_CHANNELS" envDefault:"50"`
	AnalyticsPollInterval        time.Duration `env:"ANALYTICS_POLL_INTERVAL" envDefault:"15s"`
	AnalyticsRetentionDays       int           `env:"ANALYTICS_RETENTION_DAYS" envDefault:"30"`
	AnalyticsTopEmotesPerMinute  int           `env:"ANALYTICS_TOP_EMOTES_PER_MINUTE" envDefault:"200"`
	AnalyticsVODGQLPageDelayMS        int `env:"ANALYTICS_VOD_GQL_PAGE_DELAY_MS" envDefault:"0"`
	AnalyticsVODGQLConcurrency        int `env:"ANALYTICS_VOD_GQL_CONCURRENCY" envDefault:"3"`
	AnalyticsVODGQLConcurrencyMin     int `env:"ANALYTICS_VOD_GQL_CONCURRENCY_MIN" envDefault:"0"`
	AnalyticsVODGQLConcurrencyMax     int `env:"ANALYTICS_VOD_GQL_CONCURRENCY_MAX" envDefault:"0"`
	AnalyticsVODGQLSegmentSeconds          int  `env:"ANALYTICS_VOD_GQL_SEGMENT_SECONDS" envDefault:"600"`
	AnalyticsVODGQLDenseSegmentSeconds     int  `env:"ANALYTICS_VOD_GQL_DENSE_SEGMENT_SECONDS" envDefault:"120"`
	AnalyticsVODGQLHotSegmentPageThreshold int  `env:"ANALYTICS_VOD_GQL_HOT_SEGMENT_PAGE_THRESHOLD" envDefault:"50"`
	AnalyticsVODGQLIncrementalDB             bool `env:"ANALYTICS_VOD_GQL_INCREMENTAL_DB" envDefault:"true"`
	AnalyticsTrackerScrapeMS        int  `env:"ANALYTICS_TRACKER_SCRAPE_TIMEOUT_MS" envDefault:"120000"`
	AnalyticsPassTTMaxAge           bool `env:"ANALYTICS_PASS_TT_MAXAGE" envDefault:"true"`
	AnalyticsTTMaxAgeMS             int  `env:"ANALYTICS_TT_MAX_AGE_MS" envDefault:"0"`
	AnalyticsTTDirectHTTPEnabled    bool `env:"ANALYTICS_TT_DIRECT_HTTP_ENABLED" envDefault:"true"`
	AnalyticsTTDirectHTTPTimeoutMS  int  `env:"ANALYTICS_TT_DIRECT_HTTP_TIMEOUT_MS" envDefault:"1200"`
	AlwaysTrackedChannels        []string      `env:"ALWAYS_TRACKED_CHANNELS" envSeparator:","`

	TwitchOAuthClientID     string `env:"TWITCH_OAUTH_CLIENT_ID"`
	TwitchOAuthClientSecret string `env:"TWITCH_OAUTH_CLIENT_SECRET"`
	TwitchOAuthRedirectURL  string `env:"TWITCH_OAUTH_REDIRECT_URL" envDefault:"http://localhost:8083/v1/auth/twitch/callback"`
	TwitchAuthScopes        string `env:"TWITCH_AUTH_SCOPES" envDefault:"chat:read chat:edit user:read:follows clips:edit"`
	TwitchOAuthURL          string `env:"TWITCH_OAUTH_URL" envDefault:"https://id.twitch.tv/oauth2/authorize"`
	TwitchTokenURL          string `env:"TWITCH_TOKEN_URL" envDefault:"https://id.twitch.tv/oauth2/token"`
	TwitchValidateURL       string `env:"TWITCH_VALIDATE_URL" envDefault:"https://id.twitch.tv/oauth2/validate"`
	TwitchAPIURL            string `env:"TWITCH_API_URL" envDefault:"https://api.twitch.tv/helix"`
	TwitchDevTokenImport    bool   `env:"TWITCH_DEV_TOKEN_IMPORT_ENABLED" envDefault:"false"`
	ClipperAuthSyncPath     string `env:"CLIPPER_AUTH_SYNC_PATH"`
	StreamcloneProfile      string `env:"STREAMCLONE_PROFILE" envDefault:"core"`
	ClipperServiceURL       string `env:"CLIPPER_SERVICE_URL" envDefault:"http://clipper:8095"`
	FrontendOrigin          string `env:"FRONTEND_ORIGIN" envDefault:"http://localhost:5174"`
	AuthCookieSecret        string `env:"AUTH_COOKIE_SECRET" envDefault:"dev-insecure-cookie-secret"`
	AuthCookieSameSite      string `env:"AUTH_COOKIE_SAMESITE" envDefault:"lax"`

	S3Endpoint    string `env:"S3_ENDPOINT"`
	S3Bucket      string `env:"S3_BUCKET" envDefault:"emotes"`
	S3AccessKey   string `env:"S3_ACCESS_KEY"`
	S3SecretKey   string `env:"S3_SECRET_KEY"`
	CDNPublicBase string `env:"CDN_PUBLIC_BASE"`

	CuratorAPIToken           string `env:"CURATOR_API_TOKEN"`
	EmoteImportConcurrency    int    `env:"EMOTE_IMPORT_CONCURRENCY" envDefault:"8"`
	EmoteWorkerConcurrency    int    `env:"EMOTE_WORKER_CONCURRENCY" envDefault:"8"`
	EmoteDictionaryDebounceMS int    `env:"EMOTE_DICTIONARY_DEBOUNCE_MS" envDefault:"3000"`

	Upstream upstream.Endpoints
}

func Load() (Config, error) {
	var c Config
	if err := env.Parse(&c); err != nil {
		return Config{}, err
	}
	if strings.TrimSpace(c.ScraperAPIURL) == "" {
		if strings.TrimSpace(c.FirecrawlAPIURL) != "" {
			c.ScraperAPIURL = strings.TrimSpace(c.FirecrawlAPIURL)
		} else {
			c.ScraperAPIURL = defaultScraperAPIURL
		}
	}
	if strings.TrimSpace(c.ScraperAPIKey) == "" && strings.TrimSpace(c.FirecrawlAPIKey) != "" {
		c.ScraperAPIKey = strings.TrimSpace(c.FirecrawlAPIKey)
	}
	return c, nil
}
