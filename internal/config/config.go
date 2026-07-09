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

	MetaCacheTTL         time.Duration `env:"META_CACHE_TTL" envDefault:"30s"`
	StaleTTL             time.Duration `env:"STALE_TTL" envDefault:"24h"`
	TwitchTrackerAPIURL  string        `env:"TWITCHTRACKER_API_URL" envDefault:"https://twitchtracker.com/api"`
	RedditAPIURL         string        `env:"REDDIT_API_URL" envDefault:"https://www.reddit.com"`
	RedditProvider       string        `env:"REDDIT_PROVIDER" envDefault:"auto"`
	RedditClientID       string        `env:"REDDIT_CLIENT_ID"`
	RedditClientSecret   string        `env:"REDDIT_CLIENT_SECRET"`
	RedditAccessToken    string        `env:"REDDIT_ACCESS_TOKEN"`
	RedditTokenURL       string        `env:"REDDIT_TOKEN_URL" envDefault:"https://www.reddit.com/api/v1/access_token"`
	RedditOAuthAPIURL    string        `env:"REDDIT_OAUTH_API_URL" envDefault:"https://oauth.reddit.com"`
	RedditHTMLFallback   bool          `env:"REDDIT_HTML_FALLBACK" envDefault:"true"`
	RedditThirdPartyURL  string        `env:"REDDIT_THIRD_PARTY_URL"`
	RedditThirdPartyKey  string        `env:"REDDIT_THIRD_PARTY_KEY"`
	RedditLSFLowPriority bool          `env:"REDDIT_LSF_LOW_PRIORITY" envDefault:"true"`
	ScraperAPIURL        string        `env:"SCRAPER_API_URL"`
	ScraperAPIKey        string        `env:"SCRAPER_API_KEY"`
	FirecrawlAPIURL      string        `env:"FIRECRAWL_API_URL"` // deprecated alias for SCRAPER_API_URL
	FirecrawlAPIKey      string        `env:"FIRECRAWL_API_KEY"` // deprecated alias for SCRAPER_API_KEY
	YouTubeAPIKey        string        `env:"YOUTUBE_API_KEY"`
	YouTubeProvider      string        `env:"YOUTUBE_PROVIDER" envDefault:"auto"`
	YouTubeAPIBaseURL    string        `env:"YOUTUBE_API_BASE_URL" envDefault:"https://www.googleapis.com/youtube/v3"`
	TwitchGQLURL         string        `env:"TWITCH_GQL_URL" envDefault:"https://gql.twitch.tv/gql"`
	TwitchClientID       string        `env:"TWITCH_CLIENT_ID" envDefault:"kimne78kx3ncx6brgo4mv6wki5h1ko"`
	EmoteServiceURL      string        `env:"EMOTE_SERVICE_URL"`
	MetadataServiceURL   string        `env:"METADATA_SERVICE_URL" envDefault:"http://metadata:8080"`

	StreamIdleTimeout    time.Duration `env:"STREAM_IDLE_TIMEOUT" envDefault:"3m"`
	MaxConcurrentStreams int           `env:"MAX_CONCURRENT_STREAMS" envDefault:"20"`
	MaxConcurrentRelays  int           `env:"MAX_CONCURRENT_RELAYS" envDefault:"0"`
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

	TwitchOAuthClientID      string `env:"TWITCH_OAUTH_CLIENT_ID"`
	TwitchOAuthClientSecret  string `env:"TWITCH_OAUTH_CLIENT_SECRET"`
	TwitchOAuthRedirectURL   string `env:"TWITCH_OAUTH_REDIRECT_URL" envDefault:"http://localhost:8083/v1/auth/twitch/callback"`
	TwitchAuthScopes         string `env:"TWITCH_AUTH_SCOPES" envDefault:"chat:read chat:edit user:read:follows clips:edit"`
	TwitchOAuthURL           string `env:"TWITCH_OAUTH_URL" envDefault:"https://id.twitch.tv/oauth2/authorize"`
	TwitchTokenURL           string `env:"TWITCH_TOKEN_URL" envDefault:"https://id.twitch.tv/oauth2/token"`
	TwitchValidateURL        string `env:"TWITCH_VALIDATE_URL" envDefault:"https://id.twitch.tv/oauth2/validate"`
	TwitchAPIURL             string `env:"TWITCH_API_URL" envDefault:"https://api.twitch.tv/helix"`
	TwitchDevTokenImport     bool   `env:"TWITCH_DEV_TOKEN_IMPORT_ENABLED" envDefault:"false"`
	ClipperAuthSyncPath      string `env:"CLIPPER_AUTH_SYNC_PATH"`
	StreamcloneProfile       string `env:"STREAMCLONE_PROFILE" envDefault:"core"`
	ClipperServiceURL        string `env:"CLIPPER_SERVICE_URL" envDefault:"http://host.docker.internal:8095"`
	ClipperWebhookToken      string `env:"CLIPPER_WEBHOOK_TOKEN"`
	ReplayForgeCallbackToken string `env:"REPLAYFORGE_CALLBACK_TOKEN"`
	FrontendOrigin           string `env:"FRONTEND_ORIGIN" envDefault:"http://localhost:8090"`
	AuthCookieSecret         string `env:"AUTH_COOKIE_SECRET" envDefault:"dev-insecure-cookie-secret"`
	AuthCookieSameSite       string `env:"AUTH_COOKIE_SAMESITE" envDefault:"lax"`

	S3Endpoint    string `env:"S3_ENDPOINT"`
	S3Bucket      string `env:"S3_BUCKET" envDefault:"emotes"`
	S3Prefix      string `env:"S3_PREFIX"`
	S3AccessKey   string `env:"S3_ACCESS_KEY"`
	S3SecretKey   string `env:"S3_SECRET_KEY"`
	S3PublicRead  bool   `env:"S3_PUBLIC_READ" envDefault:"false"`
	CDNPublicBase string `env:"CDN_PUBLIC_BASE"`

	CuratorAPIToken   string `env:"CURATOR_API_TOKEN"`
	SetupControlToken string `env:"SETUP_CONTROL_TOKEN"`

	SocialRetentionDays       int    `env:"SOCIAL_RETENTION_DAYS" envDefault:"90"`
	RedditCommercialOK        bool   `env:"REDDIT_COMMERCIAL_OK" envDefault:"false"`
	SocialScrapeUseProxy      bool   `env:"SOCIAL_SCRAPE_USE_PROXY" envDefault:"false"`
	StreamerbansIngestEnabled bool   `env:"STREAMERBANS_INGEST_ENABLED" envDefault:"false"`
	StreamerbansHomeURL       string `env:"STREAMERBANS_HOME_URL" envDefault:"https://streamerbans.com/"`
	XUnofficialOK             bool   `env:"X_UNOFFICIAL_OK" envDefault:"false"`
	XAuthToken                string `env:"X_AUTH_TOKEN"`
	EmusksXAuthToken          string `env:"EMUSKS_X_AUTH_TOKEN"`
	XIngestURL                string `env:"X_INGEST_URL" envDefault:"http://x-ingest:8098"`

	AlwaysTrackedChannels []string `env:"ALWAYS_TRACKED_CHANNELS" envSeparator:","`

	EmoteImportConcurrency                 int           `env:"EMOTE_IMPORT_CONCURRENCY" envDefault:"8"`
	EmoteWorkerConcurrency                 int           `env:"EMOTE_WORKER_CONCURRENCY" envDefault:"8"`
	EmoteRenderTwitchEager                 bool          `env:"EMOTE_RENDER_TWITCH_EAGER" envDefault:"false"`
	EmoteRenderThirdpartyEager             bool          `env:"EMOTE_RENDER_THIRDPARTY_EAGER" envDefault:"false"`
	EmoteRenderOnChatObserved              bool          `env:"EMOTE_RENDER_ON_CHAT_OBSERVED" envDefault:"true"`
	EmoteRenderOnUIRequest                 bool          `env:"EMOTE_RENDER_ON_UI_REQUEST" envDefault:"true"`
	EmoteRenderDefaultScales               string        `env:"EMOTE_RENDER_DEFAULT_SCALES" envDefault:"1x"`
	EmoteRenderAllowedScales               string        `env:"EMOTE_RENDER_ALLOWED_SCALES" envDefault:"1x,2x,3x,4x"`
	EmoteRenderQueueMaxDepth               int           `env:"EMOTE_RENDER_QUEUE_MAX_DEPTH" envDefault:"5000"`
	EmoteRenderChatObservedRateLimitPerMin int           `env:"EMOTE_RENDER_CHAT_OBSERVED_RATE_LIMIT_PER_MIN" envDefault:"120"`
	EmoteRenderUIRequestRateLimitPerMin    int           `env:"EMOTE_RENDER_UI_REQUEST_RATE_LIMIT_PER_MIN" envDefault:"300"`
	EmoteRenderBackfillEnabled             bool          `env:"EMOTE_RENDER_BACKFILL_ENABLED" envDefault:"false"`
	EmoteDictionaryDebounceMS              int           `env:"EMOTE_DICTIONARY_DEBOUNCE_MS" envDefault:"3000"`
	EmoteRosterPreloadEnabled              bool          `env:"EMOTE_ROSTER_PRELOAD_ENABLED" envDefault:"false"`
	EmoteRosterPreloadInterval             time.Duration `env:"EMOTE_ROSTER_PRELOAD_INTERVAL" envDefault:"6h"`
	EmoteRosterPreloadTopN                 int           `env:"EMOTE_ROSTER_PRELOAD_TOP_N" envDefault:"200"`

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

// XContentToken returns the configured X auth token for unofficial ingest sidecars.
func (c Config) XContentToken() string {
	if t := strings.TrimSpace(c.XAuthToken); t != "" {
		return t
	}
	return strings.TrimSpace(c.EmusksXAuthToken)
}
