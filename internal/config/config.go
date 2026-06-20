package config

import (
	"os"
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
	StorygraphYTKeywords []string      `env:"STORYGRAPH_YT_KEYWORDS" envSeparator:","`
	TwitchGQLURL         string        `env:"TWITCH_GQL_URL" envDefault:"https://gql.twitch.tv/gql"`
	TwitchClientID       string        `env:"TWITCH_CLIENT_ID" envDefault:"kimne78kx3ncx6brgo4mv6wki5h1ko"`
	EmoteServiceURL      string        `env:"EMOTE_SERVICE_URL"`

	StreamIdleTimeout    time.Duration `env:"STREAM_IDLE_TIMEOUT" envDefault:"60s"`
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

	MaxConcurrentTrackedChannels           int           `env:"MAX_CONCURRENT_TRACKED_CHANNELS" envDefault:"50"`
	AnalyticsPollInterval                  time.Duration `env:"ANALYTICS_POLL_INTERVAL" envDefault:"15s"`
	AnalyticsRetentionDays                 int           `env:"ANALYTICS_RETENTION_DAYS" envDefault:"30"`
	AnalyticsVODChatRetentionDays          int           `env:"ANALYTICS_VOD_CHAT_RETENTION_DAYS" envDefault:"90"`
	ChatLogPersistEnabled                  bool          `env:"CHAT_LOG_PERSIST_ENABLED" envDefault:"false"`
	ChatLogRetentionDays                   int           `env:"CHAT_LOG_RETENTION_DAYS" envDefault:"14"`
	AnalyticsServiceURL                    string        `env:"ANALYTICS_SERVICE_URL" envDefault:"http://analytics:8080"`
	AnalyticsTopEmotesPerMinute            int           `env:"ANALYTICS_TOP_EMOTES_PER_MINUTE" envDefault:"200"`
	AnalyticsVODGQLPageDelayMS             int           `env:"ANALYTICS_VOD_GQL_PAGE_DELAY_MS" envDefault:"0"`
	AnalyticsVODGQLConcurrency             int           `env:"ANALYTICS_VOD_GQL_CONCURRENCY" envDefault:"3"`
	AnalyticsVODGQLConcurrencyMin          int           `env:"ANALYTICS_VOD_GQL_CONCURRENCY_MIN" envDefault:"0"`
	AnalyticsVODGQLConcurrencyMax          int           `env:"ANALYTICS_VOD_GQL_CONCURRENCY_MAX" envDefault:"0"`
	AnalyticsVODGQLSegmentSeconds          int           `env:"ANALYTICS_VOD_GQL_SEGMENT_SECONDS" envDefault:"600"`
	AnalyticsVODGQLDenseSegmentSeconds     int           `env:"ANALYTICS_VOD_GQL_DENSE_SEGMENT_SECONDS" envDefault:"120"`
	AnalyticsVODGQLHotSegmentPageThreshold int           `env:"ANALYTICS_VOD_GQL_HOT_SEGMENT_PAGE_THRESHOLD" envDefault:"10"`
	AnalyticsVODGQLHotSlowAdvanceSec       int           `env:"ANALYTICS_VOD_GQL_HOT_SLOW_ADVANCE_SEC" envDefault:"30"`
	AnalyticsVODGQLHotSlowAdvancePages     int           `env:"ANALYTICS_VOD_GQL_HOT_SLOW_ADVANCE_PAGES" envDefault:"5"`
	AnalyticsVODGQLHotCommentsPerPage      int           `env:"ANALYTICS_VOD_GQL_HOT_COMMENTS_PER_PAGE" envDefault:"80"`
	AnalyticsVODGQLPriorityEdgeSeconds     int           `env:"ANALYTICS_VOD_GQL_PRIORITY_EDGE_SECONDS" envDefault:"600"`
	AnalyticsVODGQLIncrementalDB           bool          `env:"ANALYTICS_VOD_GQL_INCREMENTAL_DB" envDefault:"true"`
	AnalyticsVODGQLDeferSummaryRefresh     bool          `env:"ANALYTICS_VOD_GQL_DEFER" envDefault:"true"`
	AnalyticsVODGQLRollupFlushSegments     int           `env:"ANALYTICS_VOD_GQL_ROLLUP_FLUSH_SEGMENTS" envDefault:"8"`
	AnalyticsVODGQLRollupFlushMS           int           `env:"ANALYTICS_VOD_GQL_ROLLUP_FLUSH_MS" envDefault:"2000"`
	AnalyticsTrackerScrapeMS               int           `env:"ANALYTICS_TRACKER_SCRAPE_TIMEOUT_MS" envDefault:"120000"`
	AnalyticsPassTTMaxAge                  bool          `env:"ANALYTICS_PASS_TT_MAXAGE" envDefault:"true"`
	AnalyticsTTMaxAgeMS                    int           `env:"ANALYTICS_TT_MAX_AGE_MS" envDefault:"0"`
	AnalyticsTTStaleMaxAgeMS               int           `env:"ANALYTICS_TT_STALE_MAX_AGE_MS" envDefault:"604800000"`
	AnalyticsTTPrefetchEnabled             bool          `env:"ANALYTICS_TT_PREFETCH_ENABLED" envDefault:"true"`
	AnalyticsTTDirectHTTPEnabled           bool          `env:"ANALYTICS_TT_DIRECT_HTTP_ENABLED" envDefault:"true"`
	AnalyticsTTDirectHTTPStaleOnly         bool          `env:"ANALYTICS_TT_DIRECT_HTTP_STALE_ONLY" envDefault:"false"`
	AnalyticsTTDirectHTTPTimeoutMS         int           `env:"ANALYTICS_TT_DIRECT_HTTP_TIMEOUT_MS" envDefault:"1200"`
	AnalyticsTTSyncTimeoutMS               int           `env:"ANALYTICS_TT_SYNC_TIMEOUT_MS" envDefault:"45000"`
	AnalyticsTTBackgroundRetryEnabled      bool          `env:"ANALYTICS_TT_BACKGROUND_RETRY_ENABLED" envDefault:"true"`
	AnalyticsTTViewerSmoothWindow          int           `env:"ANALYTICS_TT_VIEWER_SMOOTH_WINDOW" envDefault:"0"`
	AnalyticsVODGQLQuietSegmentSeconds     int           `env:"ANALYTICS_VOD_GQL_QUIET_SEGMENT_SECONDS" envDefault:"900"`
	AlwaysTrackedChannels                  []string      `env:"ALWAYS_TRACKED_CHANNELS" envSeparator:","`

	TimeseriesEnabled         bool   `env:"TIMESERIES_ENABLED" envDefault:"false"`
	TimeseriesBackend         string `env:"TIMESERIES_BACKEND" envDefault:"influxdb"`
	InfluxDBURL               string `env:"INFLUXDB_URL"`
	InfluxDBToken             string `env:"INFLUXDB_TOKEN"`
	InfluxDBOrg               string `env:"INFLUXDB_ORG"`
	InfluxDBBucket            string `env:"INFLUXDB_BUCKET" envDefault:"streamclone"`
	TimeseriesWriteTimeoutMS  int    `env:"TIMESERIES_WRITE_TIMEOUT_MS" envDefault:"1000"`
	TimeseriesQueueSize       int    `env:"TIMESERIES_QUEUE_SIZE" envDefault:"1024"`
	TimeseriesBackfillOnStart bool   `env:"TIMESERIES_BACKFILL_ON_START" envDefault:"false"`

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
	ClipperServiceURL       string `env:"CLIPPER_SERVICE_URL" envDefault:"http://host.docker.internal:8095"`
	FrontendOrigin          string `env:"FRONTEND_ORIGIN" envDefault:"http://localhost:8090"`
	AuthCookieSecret        string `env:"AUTH_COOKIE_SECRET" envDefault:"dev-insecure-cookie-secret"`
	AuthCookieSameSite      string `env:"AUTH_COOKIE_SAMESITE" envDefault:"lax"`

	S3Endpoint    string `env:"S3_ENDPOINT"`
	S3Bucket      string `env:"S3_BUCKET" envDefault:"emotes"`
	S3AccessKey   string `env:"S3_ACCESS_KEY"`
	S3SecretKey   string `env:"S3_SECRET_KEY"`
	CDNPublicBase string `env:"CDN_PUBLIC_BASE"`

	CuratorAPIToken   string `env:"CURATOR_API_TOKEN"`
	SetupControlToken string `env:"SETUP_CONTROL_TOKEN"`

	PulseWireEnabled             bool          `env:"PULSE_WIRE_ENABLED" envDefault:"false"`
	PulseWireSemantic            bool          `env:"PULSE_WIRE_SEMANTIC" envDefault:"false"`
	StorygraphServiceURL         string        `env:"STORYGRAPH_SERVICE_URL" envDefault:"http://storygraph:8080"`
	MediaMatcherURL              string        `env:"MEDIA_MATCHER_URL" envDefault:"http://media-matcher:8001"`
	SocialRetentionDays          int           `env:"SOCIAL_RETENTION_DAYS" envDefault:"90"`
	RedditCommercialOK           bool          `env:"REDDIT_COMMERCIAL_OK" envDefault:"false"`
	StreamerbansIngestEnabled    bool          `env:"STREAMERBANS_INGEST_ENABLED" envDefault:"false"`
	StreamerbansHomeURL          string        `env:"STREAMERBANS_HOME_URL" envDefault:"https://streamerbans.com/"`
	XUnofficialOK                bool          `env:"X_UNOFFICIAL_OK" envDefault:"false"`
	XAuthToken                   string        `env:"X_AUTH_TOKEN"`
	EmusksXAuthToken             string        `env:"EMUSKS_X_AUTH_TOKEN"`
	XIngestURL                   string        `env:"X_INGEST_URL" envDefault:"http://x-ingest:8098"`
	MetadataServiceURL           string        `env:"METADATA_SERVICE_URL" envDefault:"http://metadata:8080"`
	XMonthlyBudgetUSD            float64       `env:"X_MONTHLY_BUDGET_USD" envDefault:"0"`
	MatchLinkThreshold           float64       `env:"PULSE_WIRE_MATCH_LINK" envDefault:"0.65"`
	MatchReviewThreshold         float64       `env:"PULSE_WIRE_MATCH_REVIEW" envDefault:"0.40"`
	SocialBrowserFetchBudget     int           `env:"STORYGRAPH_SOCIAL_BROWSER_FETCH_BUDGET" envDefault:"0"`
	YouTubeBrowserFetchBudget    int           `env:"STORYGRAPH_YOUTUBE_BROWSER_FETCH_BUDGET" envDefault:"0"`
	SocialScrapeUseProxy         bool          `env:"STORYGRAPH_SOCIAL_SCRAPE_USE_PROXY" envDefault:"false"`
	IngestPollInterval           time.Duration `env:"STORYGRAPH_INGEST_INTERVAL" envDefault:"5m"`
	FingerprintPollInterval      time.Duration `env:"STORYGRAPH_FINGERPRINT_INTERVAL" envDefault:"2m"`
	PulseDirectorySampleInterval time.Duration `env:"PULSE_DIRECTORY_SAMPLE_INTERVAL" envDefault:"10m"`
	PulseDirectoryTopN           int           `env:"PULSE_DIRECTORY_TOP_N" envDefault:"200"`
	PulseDirectoryRetentionDays  int           `env:"PULSE_DIRECTORY_RETENTION_DAYS" envDefault:"30"`

	ArchiveProtectRetention          bool          `env:"ARCHIVE_PROTECT_RETENTION" envDefault:"false"`
	ArchiveExportOnSync              bool          `env:"ARCHIVE_EXPORT_ON_SYNC" envDefault:"false"`
	ArchiveEnabled                   bool          `env:"ARCHIVE_ENABLED" envDefault:"false"`
	ArchiveStorageProvider           string        `env:"ARCHIVE_STORAGE_PROVIDER" envDefault:"azure"`
	ArchiveAzureStorageAccount       string        `env:"ARCHIVE_AZURE_STORAGE_ACCOUNT"`
	ArchiveAzureContainer            string        `env:"ARCHIVE_AZURE_CONTAINER" envDefault:"streamclone-archive"`
	ArchiveAzurePrefix               string        `env:"ARCHIVE_AZURE_PREFIX" envDefault:"streamclone"`
	ArchiveAzureConnectionStringFile string        `env:"ARCHIVE_AZURE_CONNECTION_STRING_FILE"`
	ArchiveExportInterval            time.Duration `env:"ARCHIVE_EXPORT_INTERVAL" envDefault:"1h"`
	ArchivePGDumpNightly             bool          `env:"ARCHIVE_PG_DUMP_NIGHTLY" envDefault:"false"`

	Tier0Enabled        bool          `env:"TIER0_ENABLED" envDefault:"false"`
	Tier0SampleInterval time.Duration `env:"TIER0_SAMPLE_INTERVAL" envDefault:"45s"`
	Tier0RosterInterval time.Duration `env:"TIER0_ROSTER_INTERVAL" envDefault:"5m"`
	Tier0RosterTopN     int           `env:"TIER0_ROSTER_TOP_N" envDefault:"200"`

	BackfillEnabled        bool          `env:"BACKFILL_ENABLED" envDefault:"false"`
	BackfillWorkerInterval time.Duration `env:"BACKFILL_WORKER_INTERVAL" envDefault:"30s"`

	GoldBackfillEnabled    bool          `env:"GOLD_BACKFILL_ENABLED" envDefault:"false"`
	GoldMinPeakViewers     int           `env:"GOLD_MIN_PEAK_VIEWERS" envDefault:"0"`
	GoldMinDurationMinutes int           `env:"GOLD_MIN_DURATION_MINUTES" envDefault:"0"`
	GoldEnqueuerInterval   time.Duration `env:"GOLD_ENQUEUER_INTERVAL" envDefault:"5m"`
	GoldSyncTimeoutMS      int           `env:"GOLD_SYNC_TIMEOUT_MS" envDefault:"600000"`
	PostEndWaitMin         time.Duration `env:"POST_END_WAIT_MIN" envDefault:"10m"`
	PostEndWaitMax         time.Duration `env:"POST_END_WAIT_MAX" envDefault:"30m"`
	PostEndCoveragePct     float64       `env:"POST_END_COVERAGE_PCT" envDefault:"70"`

	BronzeEnabled              bool          `env:"BRONZE_ENABLED" envDefault:"false"`
	BronzeTopN                 int           `env:"BRONZE_TOP_N" envDefault:"500"`
	BronzeWorkerInterval       time.Duration `env:"BRONZE_WORKER_INTERVAL" envDefault:"5m"`
	BronzeHelixConcurrency     int           `env:"BRONZE_HELIX_CONCURRENCY" envDefault:"2"`
	BronzeTTSummaryConcurrency int           `env:"BRONZE_TT_SUMMARY_CONCURRENCY" envDefault:"4"`

	EmoteImportConcurrency     int           `env:"EMOTE_IMPORT_CONCURRENCY" envDefault:"8"`
	EmoteWorkerConcurrency     int           `env:"EMOTE_WORKER_CONCURRENCY" envDefault:"8"`
	EmoteDictionaryDebounceMS  int           `env:"EMOTE_DICTIONARY_DEBOUNCE_MS" envDefault:"3000"`
	EmoteRosterPreloadEnabled  bool          `env:"EMOTE_ROSTER_PRELOAD_ENABLED" envDefault:"false"`
	EmoteRosterPreloadInterval time.Duration `env:"EMOTE_ROSTER_PRELOAD_INTERVAL" envDefault:"6h"`
	EmoteRosterPreloadTopN     int           `env:"EMOTE_ROSTER_PRELOAD_TOP_N" envDefault:"200"`

	Upstream upstream.Endpoints
}

func Load() (Config, error) {
	var c Config
	if err := env.Parse(&c); err != nil {
		return Config{}, err
	}
	if strings.TrimSpace(os.Getenv("ANALYTICS_VOD_GQL_DEFER")) == "" {
		if legacy := strings.TrimSpace(os.Getenv("ANALYTICS_VOD_GQL_DEFER_SUMMARY_REFRESH")); legacy != "" {
			c.AnalyticsVODGQLDeferSummaryRefresh = legacy == "1" || strings.EqualFold(legacy, "true")
		}
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
	if len(c.StorygraphYTKeywords) == 0 && len(c.AlwaysTrackedChannels) > 0 {
		c.StorygraphYTKeywords = append([]string(nil), c.AlwaysTrackedChannels...)
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
