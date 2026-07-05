package config

import (
	"os"
	"strings"
	"time"

	"github.com/caarlos0/env/v11"

	"streamclone/internal/upstream"
)

const defaultScraperAPIURL = "http://scraper:8000/v2/scrape"
const maxCorpusTopN = 1000
const MaxLiveAdmissionTopN = 5000
const DefaultLiveAdmissionTopN = 100

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
	AnalyticsTTScrapeBackoffEnabled        bool          `env:"ANALYTICS_TT_SCRAPE_BACKOFF_ENABLED" envDefault:"true"`
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

	PulseClipMaxCandidates              int     `env:"PULSE_CLIP_MAX_CANDIDATES" envDefault:"0"`
	PulseClipMinScore                   int     `env:"PULSE_CLIP_MIN_SCORE" envDefault:"0"`
	PulseClipMinConfidence              float64 `env:"PULSE_CLIP_MIN_CONFIDENCE" envDefault:"0"`
	PulseClipMinChatCount               int     `env:"PULSE_CLIP_MIN_CHAT_COUNT" envDefault:"0"`
	PulseClipMaxChatCount               int     `env:"PULSE_CLIP_MAX_CHAT_COUNT" envDefault:"0"`
	PulseClipMinEmoteCount              int     `env:"PULSE_CLIP_MIN_EMOTE_COUNT" envDefault:"0"`
	PulseClipMinProviderEmoteCount      int     `env:"PULSE_CLIP_MIN_PROVIDER_EMOTE_COUNT" envDefault:"0"`
	PulseClipProviderEmoteProvider      string  `env:"PULSE_CLIP_PROVIDER_EMOTE_PROVIDER" envDefault:"seventv"`
	PulseClipMinNonMissingRollupMinutes int     `env:"PULSE_CLIP_MIN_NON_MISSING_ROLLUP_MINUTES" envDefault:"0"`
	PulseClipDuplicateRadiusSeconds     int     `env:"PULSE_CLIP_DUPLICATE_RADIUS_SECONDS" envDefault:"0"`
	PulseClipMaxCandidatesPerHour       int     `env:"PULSE_CLIP_MAX_CANDIDATES_PER_HOUR" envDefault:"0"`
	PulseClipRequireSourceAvailable     bool    `env:"PULSE_CLIP_REQUIRE_SOURCE_AVAILABLE" envDefault:"false"`

	S3Endpoint    string `env:"S3_ENDPOINT"`
	S3Bucket      string `env:"S3_BUCKET" envDefault:"emotes"`
	S3Prefix      string `env:"S3_PREFIX"`
	S3AccessKey   string `env:"S3_ACCESS_KEY"`
	S3SecretKey   string `env:"S3_SECRET_KEY"`
	S3PublicRead  bool   `env:"S3_PUBLIC_READ" envDefault:"false"`
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
	ArchivePrimaryProvider           string        `env:"ARCHIVE_PRIMARY_PROVIDER" envDefault:"azure"`
	ArchiveReadThrough               bool          `env:"ARCHIVE_READ_THROUGH" envDefault:"false"`
	ArchiveDualWrite                 bool          `env:"ARCHIVE_DUAL_WRITE" envDefault:"false"`
	ArchiveR2Bucket                  string        `env:"ARCHIVE_R2_BUCKET"`
	ArchiveR2AccountID               string        `env:"ARCHIVE_R2_ACCOUNT_ID"`
	ArchiveR2Prefix                  string        `env:"ARCHIVE_R2_PREFIX" envDefault:"archive"`
	ArchiveR2Endpoint                string        `env:"ARCHIVE_R2_ENDPOINT"`
	ArchiveR2AccessKeyIDFile         string        `env:"ARCHIVE_R2_ACCESS_KEY_ID_FILE"`
	ArchiveR2SecretAccessKeyFile     string        `env:"ARCHIVE_R2_SECRET_ACCESS_KEY_FILE"`
	ArchiveExportInterval            time.Duration `env:"ARCHIVE_EXPORT_INTERVAL" envDefault:"1h"`
	ArchivePGDumpNightly             bool          `env:"ARCHIVE_PG_DUMP_NIGHTLY" envDefault:"false"`
	ArchiveContentHashEnabled        bool          `env:"ARCHIVE_CONTENT_HASH_ENABLED" envDefault:"true"`
	ArchiveWriteSidecarManifest      bool          `env:"ARCHIVE_WRITE_SIDECAR_MANIFEST" envDefault:"false"`
	ArchiveParserVersion             string        `env:"ARCHIVE_PARSER_VERSION" envDefault:"v1"`
	CorpusWorkersEnabled             bool          `env:"CORPUS_WORKERS_ENABLED" envDefault:"false"`
	CorpusTargetTopN                 int           `env:"CORPUS_TARGET_TOP_N" envDefault:"0"`
	ArchiveJobProgressEnabled        bool          `env:"ARCHIVE_JOB_PROGRESS_ENABLED" envDefault:"true"`
	ArchiveJobHeartbeatInterval      time.Duration `env:"ARCHIVE_JOB_HEARTBEAT_INTERVAL" envDefault:"15s"`
	ArchiveJobStaleAfter             time.Duration `env:"ARCHIVE_JOB_STALE_AFTER" envDefault:"10m"`
	ArchiveJobEventLogEnabled        bool          `env:"ARCHIVE_JOB_EVENT_LOG_ENABLED" envDefault:"true"`
	AdminArchiveEnabled              bool          `env:"ADMIN_ARCHIVE_ENABLED" envDefault:"false"`
	AdminArchiveRequireToken         bool          `env:"ADMIN_ARCHIVE_REQUIRE_TOKEN" envDefault:"true"`
	AdminArchiveToken                string        `env:"ADMIN_ARCHIVE_TOKEN"`
	AdminArchiveTokenFile            string        `env:"ADMIN_ARCHIVE_TOKEN_FILE"`

	Tier0Enabled        bool          `env:"TIER0_ENABLED" envDefault:"false"`
	Tier0SampleInterval time.Duration `env:"TIER0_SAMPLE_INTERVAL" envDefault:"45s"`
	Tier0RosterInterval time.Duration `env:"TIER0_ROSTER_INTERVAL" envDefault:"5m"`
	Tier0RosterTopN     int           `env:"TIER0_ROSTER_TOP_N" envDefault:"200"`

	Top500MetadataEnabled                 bool          `env:"TOP500_METADATA_ENABLED" envDefault:"false"`
	Top500MetadataDryRun                  bool          `env:"TOP500_METADATA_DRY_RUN" envDefault:"true"`
	Top500MetadataTopN                    int           `env:"TOP500_METADATA_TOP_N" envDefault:"100"`
	Top500MetadataWriteEnabled            bool          `env:"TOP500_METADATA_WRITE_ENABLED" envDefault:"false"`
	Top500MetadataLiveInterval            time.Duration `env:"TOP500_METADATA_LIVE_INTERVAL" envDefault:"60s"`
	Top500MetadataOfflineInterval         time.Duration `env:"TOP500_METADATA_OFFLINE_INTERVAL" envDefault:"10m"`
	Top500MetadataBatchSize               int           `env:"TOP500_METADATA_BATCH_SIZE" envDefault:"100"`
	Top500MetadataFixtureProvider         bool          `env:"TOP500_METADATA_FIXTURE_PROVIDER" envDefault:"false"`
	Top500MetadataDBP95HoldMS             int           `env:"TOP500_METADATA_DB_P95_HOLD_MS" envDefault:"50"`
	Top500MetadataRollbackDBP95MS         int           `env:"TOP500_METADATA_ROLLBACK_DB_P95_MS" envDefault:"200"`
	Top500MetadataRollbackDiskFreePercent int           `env:"TOP500_METADATA_ROLLBACK_DISK_FREE_PERCENT" envDefault:"15"`

	Top500SilverGateEnabled           bool          `env:"TOP500_SILVER_GATE_ENABLED" envDefault:"false"`
	Top500SilverGateDryRun            bool          `env:"TOP500_SILVER_GATE_DRY_RUN" envDefault:"true"`
	Top500SilverGateWriteEnabled      bool          `env:"TOP500_SILVER_GATE_WRITE_ENABLED" envDefault:"false"`
	Top500SilverGateMaxCandidates     int           `env:"TOP500_SILVER_GATE_MAX_CANDIDATES" envDefault:"5"`
	Top500SilverGateMaxEnqueuePerRun  int           `env:"TOP500_SILVER_GATE_MAX_ENQUEUE_PER_RUN" envDefault:"1"`
	Top500SilverGateInterval          time.Duration `env:"TOP500_SILVER_GATE_INTERVAL" envDefault:"10m"`
	Top500SilverGateFixtureCandidates bool          `env:"TOP500_SILVER_GATE_FIXTURE_CANDIDATES" envDefault:"false"`

	Top500GoldVODInventoryEnabled       bool          `env:"TOP500_GOLD_VOD_INVENTORY_ENABLED" envDefault:"false"`
	Top500GoldVODInventoryDirectEnqueue bool          `env:"TOP500_GOLD_VOD_DIRECT_ENQUEUE" envDefault:"false"`
	Top500GoldVODInventorySinceDays     int           `env:"TOP500_GOLD_VOD_SINCE_DAYS" envDefault:"90"`
	Top500GoldVODInventoryTopN          int           `env:"TOP500_GOLD_VOD_TOP_N" envDefault:"500"`
	Top500GoldVODInventoryMaxPerRun     int           `env:"TOP500_GOLD_VOD_MAX_PER_RUN" envDefault:"25"`
	Top500GoldVODInventoryInterval      time.Duration `env:"TOP500_GOLD_VOD_INTERVAL" envDefault:"15m"`
	BackfillSilverWorkerCount           int           `env:"BACKFILL_SILVER_WORKER_COUNT" envDefault:"1"`
	BackfillGoldWorkerCount             int           `env:"BACKFILL_GOLD_WORKER_COUNT" envDefault:"1"`
	GoldMaxParallelVODs                 int           `env:"GOLD_MAX_PARALLEL_VODS" envDefault:"2"`
	GoldMaxSegmentsPerVOD               int           `env:"GOLD_MAX_SEGMENTS_PER_VOD" envDefault:"4"`
	GoldGlobalGQLRPM                    int           `env:"GOLD_GLOBAL_GQL_RPM" envDefault:"120"`
	GoldPerVODGQLRPM                    int           `env:"GOLD_PER_VOD_GQL_RPM" envDefault:"30"`
	GoldSegmentSizeSeconds              int           `env:"GOLD_SEGMENT_SIZE_SECONDS" envDefault:"600"`
	GoldRetryMax                        int           `env:"GOLD_RETRY_MAX" envDefault:"3"`
	GoldLeaseTTLSeconds                 int           `env:"GOLD_LEASE_TTL_SECONDS" envDefault:"120"`
	GoldVODSegmentsEnabled              bool          `env:"GOLD_VOD_SEGMENTS_ENABLED" envDefault:"false"`

	PulseTop500AdmissionEnabled  bool          `env:"PULSE_TOP500_ADMISSION_ENABLED" envDefault:"false"`
	PulseTop500AdmissionTopN     int           `env:"PULSE_TOP500_ADMISSION_TOP_N" envDefault:"100"`
	PulseTop500AdmissionInterval time.Duration `env:"PULSE_TOP500_ADMISSION_INTERVAL" envDefault:"60s"`
	PulseTop500AdmissionSource   string        `env:"PULSE_TOP500_ADMISSION_SOURCE" envDefault:"helix_top_live"`
	LiveAdmissionTopN            int           `env:"LIVE_ADMISSION_TOP_N" envDefault:"0"`
	MaxActiveIRCChannels         int           `env:"MAX_ACTIVE_IRC_CHANNELS" envDefault:"0"`

	PulseHostedMode              bool          `env:"PULSE_HOSTED_MODE" envDefault:"false"`
	PulseBetaKeys                string        `env:"PULSE_BETA_KEYS"`
	PulseMaxActiveChannels       int           `env:"PULSE_MAX_ACTIVE_CHANNELS" envDefault:"0"`
	PulseMaxBackfills            int           `env:"PULSE_MAX_BACKFILLS" envDefault:"0"`
	PulseMaxChannelsPerPrincipal int           `env:"PULSE_MAX_CHANNELS_PER_PRINCIPAL" envDefault:"0"`
	PulseWatchRatePerMin         int           `env:"PULSE_WATCH_RATE_PER_MIN" envDefault:"0"`
	PulseBackfillRatePerHour     int           `env:"PULSE_BACKFILL_RATE_PER_HOUR" envDefault:"0"`
	PulseCFAccessTeamDomain      string        `env:"PULSE_CF_ACCESS_TEAM_DOMAIN"`
	PulseCFAccessAud             string        `env:"PULSE_CF_ACCESS_AUD"`
	PulseAdminLocalBypass        bool          `env:"PULSE_ADMIN_LOCAL_BYPASS" envDefault:"false"`
	PulseAutoBackfillEnabled     bool          `env:"PULSE_AUTO_BACKFILL_ENABLED" envDefault:"false"`
	PulseAutoBackfillInterval    time.Duration `env:"PULSE_AUTO_BACKFILL_INTERVAL" envDefault:"15m"`
	PulseAutoBackfillCooldown    time.Duration `env:"PULSE_AUTO_BACKFILL_COOLDOWN" envDefault:"30m"`
	PulseAutoBackfillSince       time.Duration `env:"PULSE_AUTO_BACKFILL_SINCE" envDefault:"48h"`
	PulseAutoBackfillMaxPerRun   int           `env:"PULSE_AUTO_BACKFILL_MAX_PER_RUN" envDefault:"1"`
	PulseAutoBackfillScanLimit   int           `env:"PULSE_AUTO_BACKFILL_SCAN_LIMIT" envDefault:"20"`

	BackfillEnabled                  bool          `env:"BACKFILL_ENABLED" envDefault:"false"`
	BackfillWorkerInterval           time.Duration `env:"BACKFILL_WORKER_INTERVAL" envDefault:"30s"`
	BackfillStaleRunningAfter        time.Duration `env:"BACKFILL_STALE_RUNNING_AFTER" envDefault:"15m"`
	BackfillHeartbeatInterval        time.Duration `env:"BACKFILL_HEARTBEAT_INTERVAL" envDefault:"60s"`
	BackfillGoldWorkerEnabled        bool          `env:"BACKFILL_GOLD_WORKER_ENABLED"`
	BackfillQueueMaintenanceEnabled  bool          `env:"BACKFILL_QUEUE_MAINTENANCE_ENABLED" envDefault:"true"`
	BackfillQueueMaintenanceInterval time.Duration `env:"BACKFILL_QUEUE_MAINTENANCE_INTERVAL" envDefault:"30m"`
	BackfillRequeueFailedMaxPerRun   int           `env:"BACKFILL_REQUEUE_FAILED_MAX_PER_RUN" envDefault:"25"`
	BackfillRepairSessionsMaxPerRun  int           `env:"BACKFILL_REPAIR_SESSIONS_MAX_PER_RUN" envDefault:"50"`
	SilverAutoEnqueueEnabled         bool          `env:"SILVER_AUTO_ENQUEUE_ENABLED" envDefault:"false"`
	SilverEnqueueSinceDays           int           `env:"SILVER_ENQUEUE_SINCE_DAYS" envDefault:"60"`
	SilverEnqueueTopN                int           `env:"SILVER_ENQUEUE_TOP_N" envDefault:"200"`
	SilverEnqueueMaxPerRun           int           `env:"SILVER_ENQUEUE_MAX_PER_RUN" envDefault:"25"`
	SilverEnqueueInterval            time.Duration `env:"SILVER_ENQUEUE_INTERVAL" envDefault:"15m"`

	GoldBackfillEnabled    bool          `env:"GOLD_BACKFILL_ENABLED" envDefault:"false"`
	GoldAutoEnqueueEnabled bool          `env:"GOLD_AUTO_ENQUEUE_ENABLED" envDefault:"false"`
	GoldMinPeakViewers     int           `env:"GOLD_MIN_PEAK_VIEWERS" envDefault:"0"`
	GoldMinDurationMinutes int           `env:"GOLD_MIN_DURATION_MINUTES" envDefault:"0"`
	GoldEnqueuerInterval   time.Duration `env:"GOLD_ENQUEUER_INTERVAL" envDefault:"5m"`
	GoldSyncTimeoutMS      int           `env:"GOLD_SYNC_TIMEOUT_MS" envDefault:"600000"`
	PostEndWaitMin         time.Duration `env:"POST_END_WAIT_MIN" envDefault:"10m"`
	PostEndWaitMax         time.Duration `env:"POST_END_WAIT_MAX" envDefault:"30m"`
	PostEndCoveragePct     float64       `env:"POST_END_COVERAGE_PCT" envDefault:"70"`

	BronzeEnabled               bool          `env:"BRONZE_ENABLED" envDefault:"false"`
	BronzeTopN                  int           `env:"BRONZE_TOP_N" envDefault:"500"`
	BronzeWorkerInterval        time.Duration `env:"BRONZE_WORKER_INTERVAL" envDefault:"5m"`
	BronzeHelixConcurrency      int           `env:"BRONZE_HELIX_CONCURRENCY" envDefault:"2"`
	BronzeTTSummaryConcurrency  int           `env:"BRONZE_TT_SUMMARY_CONCURRENCY" envDefault:"4"`
	BronzeVODIndexSinceDays     int           `env:"BRONZE_VOD_INDEX_SINCE_DAYS" envDefault:"365"`
	BronzeVODIndexMaxPages      int           `env:"BRONZE_VOD_INDEX_MAX_PAGES" envDefault:"10"`
	BronzeIdentityEnabled       bool          `env:"BRONZE_IDENTITY_ENABLED" envDefault:"true"`
	BronzeCrosswalkEnabled      bool          `env:"BRONZE_CROSSWALK_ENABLED" envDefault:"true"`
	BronzeTombstoneEnabled      bool          `env:"BRONZE_TOMBSTONE_ENABLED" envDefault:"true"`
	BronzeCoverageExportEnabled bool          `env:"BRONZE_COVERAGE_EXPORT_ENABLED" envDefault:"true"`
	EmoteGlobal7TVEnabled       bool          `env:"EMOTE_GLOBAL_7TV_ENABLED" envDefault:"true"`
	EmoteChangelogDiffEnabled   bool          `env:"EMOTE_CHANGELOG_DIFF_ENABLED" envDefault:"true"`
	EmoteFFZSnapshotEnabled     bool          `env:"EMOTE_FFZ_SNAPSHOT_ENABLED" envDefault:"false"`
	EmoteBTTVSnapshotEnabled    bool          `env:"EMOTE_BTTV_SNAPSHOT_ENABLED" envDefault:"false"`
	GoldLiteEnabled             bool          `env:"GOLD_LITE_ENABLED" envDefault:"false"`
	GoldLiteRequireRollups      bool          `env:"GOLD_LITE_REQUIRE_ROLLUPS" envDefault:"true"`
	GoldIVREnabled              bool          `env:"GOLD_IVR_ENABLED" envDefault:"false"`
	GoldIVRLiteEnabled          bool          `env:"GOLD_IVR_LITE_ENABLED" envDefault:"false"`
	GoldIVRCanonicalReplace     bool          `env:"GOLD_IVR_CANONICAL_REPLACE" envDefault:"false"`
	GoldIVRBaseURL              string        `env:"GOLD_IVR_BASE_URL" envDefault:"https://logs.ivr.fi"`
	GoldIVRMaxBytesPerJob       int64         `env:"GOLD_IVR_MAX_BYTES_PER_JOB" envDefault:"67108864"`
	GoldIVRMaxMessagesPerJob    int           `env:"GOLD_IVR_MAX_MESSAGES_PER_JOB" envDefault:"500000"`
	GoldIVRMaxDurationMinutes   int           `env:"GOLD_IVR_MAX_DURATION_MINUTES" envDefault:"180"`
	GoldIVRHTTPTimeoutSeconds   int           `env:"GOLD_IVR_HTTP_TIMEOUT_SECONDS" envDefault:"30"`
	GoldIVRMaxRetries           int           `env:"GOLD_IVR_MAX_RETRIES" envDefault:"2"`
	// Comma-separated Twitch logins (lowercase) and/or numeric broadcaster IDs. Empty = deny all when IVR enabled.
	GoldIVREnabledChannelAllowlist     string `env:"GOLD_IVR_ENABLED_CHANNEL_ALLOWLIST" envDefault:""`
	GoldIVRShadowMode                  bool   `env:"GOLD_IVR_SHADOW_MODE" envDefault:"false"`
	GoldIVRShadowArtifactDir           string `env:"GOLD_IVR_SHADOW_ARTIFACT_DIR" envDefault:"runtime/ivr-shadow"`
	GoldIVRShadowArtifactRetentionDays int    `env:"GOLD_IVR_SHADOW_ARTIFACT_RETENTION_DAYS" envDefault:"7"`
	GoldIVRShadowArtifactMaxFiles      int    `env:"GOLD_IVR_SHADOW_ARTIFACT_MAX_FILES" envDefault:"1000"`
	GoldIVRPeaksOnlyEnabled            bool   `env:"GOLD_IVR_PEAKS_ONLY_ENABLED" envDefault:"false"`
	GoldIVRPeaksOnlyMaxMinutes         int    `env:"GOLD_IVR_PEAKS_ONLY_MAX_MINUTES" envDefault:"5"`
	GoldIVRPeaksOnlyMinChatCount       int    `env:"GOLD_IVR_PEAKS_ONLY_MIN_CHAT_COUNT" envDefault:"10"`
	// GoldArchiveRequired gates backfill gold run-once on Azure archive export (set false for local shadow/GQL proofs).
	GoldArchiveRequired           bool          `env:"GOLD_ARCHIVE_REQUIRED" envDefault:"true"`
	GoldFullEnabled               bool          `env:"GOLD_FULL_ENABLED" envDefault:"false"`
	GoldFullOperatorOnly          bool          `env:"GOLD_FULL_OPERATOR_ONLY" envDefault:"true"`
	GoldFullMinPeakViewers        int           `env:"GOLD_FULL_MIN_PEAK_VIEWERS" envDefault:"5000"`
	GoldFullMinDurationMinutes    int           `env:"GOLD_FULL_MIN_DURATION_MINUTES" envDefault:"60"`
	SilverRawTTChartJSON          bool          `env:"SILVER_RAW_TT_CHART_JSON" envDefault:"true"`
	SilverRawTTMaxBytes           int           `env:"SILVER_RAW_TT_MAX_BYTES" envDefault:"8388608"`
	SilverPartialMinCoverage      float64       `env:"SILVER_PARTIAL_MIN_COVERAGE" envDefault:"0.5"`
	AnalyticsTTUseProxy           bool          `env:"ANALYTICS_TT_USE_PROXY" envDefault:"false"`
	ViteAdminArchiveUIEnabled     bool          `env:"VITE_ADMIN_ARCHIVE_UI_ENABLED" envDefault:"false"`
	ArchiveMetricsRefreshInterval time.Duration `env:"ARCHIVE_METRICS_REFRESH_INTERVAL" envDefault:"30s"`
	PulsewireArchiveEnabled       bool          `env:"PULSEWIRE_ARCHIVE_ENABLED" envDefault:"false"`

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
	EmoteHistorySnapshotEnabled            bool          `env:"EMOTE_HISTORY_SNAPSHOT_ENABLED" envDefault:"false"`
	EmoteHistorySnapshotInterval           time.Duration `env:"EMOTE_HISTORY_SNAPSHOT_INTERVAL" envDefault:"6h"`
	EmoteHistorySnapshotBatchSize          int           `env:"EMOTE_HISTORY_SNAPSHOT_BATCH_SIZE" envDefault:"25"`
	EmoteHistoryNormalizeEnabled           bool          `env:"EMOTE_HISTORY_NORMALIZE_ENABLED" envDefault:"false"`
	EmoteHistoryNormalizeInterval          time.Duration `env:"EMOTE_HISTORY_NORMALIZE_INTERVAL" envDefault:"15m"`
	EmoteHistoryNormalizeSince             time.Duration `env:"EMOTE_HISTORY_NORMALIZE_SINCE" envDefault:"720h"`
	EmoteHistoryNormalizeBatchSize         int           `env:"EMOTE_HISTORY_NORMALIZE_BATCH_SIZE" envDefault:"25"`
	PublicEmoteProviderRefreshEnabled      bool          `env:"PUBLIC_EMOTE_PROVIDER_REFRESH_ENABLED" envDefault:"false"`
	PublicEmoteProviderRefreshInterval     time.Duration `env:"PUBLIC_EMOTE_PROVIDER_REFRESH_INTERVAL" envDefault:"15m"`

	Upstream upstream.Endpoints
}

func applyEnvAlias(canonical, legacy string) {
	if strings.TrimSpace(os.Getenv(canonical)) != "" {
		return
	}
	if v := strings.TrimSpace(os.Getenv(legacy)); v != "" {
		_ = os.Setenv(canonical, v)
	}
}

func Load() (Config, error) {
	// Operator docs still reference PULSE_TOP_ROSTER_*; Go reads PULSE_TOP500_*.
	applyEnvAlias("PULSE_TOP500_ADMISSION_ENABLED", "PULSE_TOP_ROSTER_ADMISSION_ENABLED")
	applyEnvAlias("PULSE_TOP500_ADMISSION_TOP_N", "PULSE_TOP_ROSTER_ADMISSION_TOP_N")
	applyEnvAlias("PULSE_TOP500_ADMISSION_INTERVAL", "PULSE_TOP_ROSTER_ADMISSION_INTERVAL")
	if strings.TrimSpace(os.Getenv("PULSE_TOP500_ADMISSION_ENABLED")) == "" {
		applyEnvAlias("PULSE_TOP500_ADMISSION_ENABLED", "PULSE_TOP_ROSTER_POLL_ENABLED")
	}

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
	if strings.TrimSpace(os.Getenv("BACKFILL_GOLD_WORKER_ENABLED")) == "" {
		c.BackfillGoldWorkerEnabled = c.GoldBackfillEnabled
	}
	if c.CorpusTargetTopN > 0 {
		c.CorpusTargetTopN = clampCorpusTopN(c.CorpusTargetTopN)
		if strings.TrimSpace(os.Getenv("TOP500_METADATA_TOP_N")) == "" {
			c.Top500MetadataTopN = c.CorpusTargetTopN
		}
		if strings.TrimSpace(os.Getenv("PULSE_TOP500_ADMISSION_TOP_N")) == "" && c.LiveAdmissionTopN <= 0 {
			c.PulseTop500AdmissionTopN = c.CorpusTargetTopN
		}
		if strings.TrimSpace(os.Getenv("TOP500_GOLD_VOD_TOP_N")) == "" {
			c.Top500GoldVODInventoryTopN = c.CorpusTargetTopN
		}
		if strings.TrimSpace(os.Getenv("SILVER_ENQUEUE_TOP_N")) == "" {
			c.SilverEnqueueTopN = c.CorpusTargetTopN
		}
	}
	if c.LiveAdmissionTopN > 0 && strings.TrimSpace(os.Getenv("PULSE_TOP500_ADMISSION_TOP_N")) == "" {
		c.PulseTop500AdmissionTopN = c.LiveAdmissionTopN
	}
	if c.MaxActiveIRCChannels > 0 && strings.TrimSpace(os.Getenv("PULSE_MAX_ACTIVE_CHANNELS")) == "" {
		c.PulseMaxActiveChannels = c.MaxActiveIRCChannels
	}
	if c.Top500MetadataTopN <= 0 || c.Top500MetadataTopN > maxCorpusTopN {
		c.Top500MetadataTopN = 100
	}
	if c.Top500MetadataBatchSize <= 0 || c.Top500MetadataBatchSize > 100 {
		c.Top500MetadataBatchSize = 100
	}
	maxIRC := 0
	if c.PulseMaxActiveChannels > 0 {
		maxIRC = c.PulseMaxActiveChannels
	}
	c.PulseTop500AdmissionTopN = ClampLiveAdmissionTopN(c.PulseTop500AdmissionTopN, maxIRC)
	if c.PulseTop500AdmissionInterval <= 0 {
		c.PulseTop500AdmissionInterval = 60 * time.Second
	}
	switch strings.ToLower(strings.TrimSpace(c.PulseTop500AdmissionSource)) {
	case "roster":
		c.PulseTop500AdmissionSource = "roster"
	default:
		c.PulseTop500AdmissionSource = "helix_top_live"
	}
	if c.Top500SilverGateMaxCandidates <= 0 || c.Top500SilverGateMaxCandidates > 100 {
		c.Top500SilverGateMaxCandidates = 5
	}
	if c.Top500SilverGateMaxEnqueuePerRun <= 0 || c.Top500SilverGateMaxEnqueuePerRun > 10 {
		c.Top500SilverGateMaxEnqueuePerRun = 1
	}
	if c.Top500SilverGateInterval <= 0 {
		c.Top500SilverGateInterval = 10 * time.Minute
	}
	if c.Top500GoldVODInventoryTopN <= 0 || c.Top500GoldVODInventoryTopN > maxCorpusTopN {
		c.Top500GoldVODInventoryTopN = 500
	}
	if c.Top500GoldVODInventorySinceDays <= 0 {
		c.Top500GoldVODInventorySinceDays = 90
	}
	if c.Top500GoldVODInventoryMaxPerRun <= 0 {
		c.Top500GoldVODInventoryMaxPerRun = 25
	}
	if c.Top500GoldVODInventoryInterval <= 0 {
		c.Top500GoldVODInventoryInterval = 15 * time.Minute
	}
	if c.BackfillSilverWorkerCount <= 0 {
		c.BackfillSilverWorkerCount = 1
	}
	if c.BackfillSilverWorkerCount > 4 {
		c.BackfillSilverWorkerCount = 4
	}
	if c.BackfillGoldWorkerCount <= 0 {
		c.BackfillGoldWorkerCount = 1
	}
	if c.BackfillGoldWorkerCount > 4 {
		c.BackfillGoldWorkerCount = 4
	}
	if c.GoldMaxParallelVODs <= 0 {
		c.GoldMaxParallelVODs = 2
	}
	if c.GoldMaxParallelVODs > 16 {
		c.GoldMaxParallelVODs = 16
	}
	if c.GoldMaxSegmentsPerVOD <= 0 {
		c.GoldMaxSegmentsPerVOD = 4
	}
	if c.GoldMaxSegmentsPerVOD > 64 {
		c.GoldMaxSegmentsPerVOD = 64
	}
	if c.GoldGlobalGQLRPM <= 0 {
		c.GoldGlobalGQLRPM = 120
	}
	if c.GoldPerVODGQLRPM <= 0 {
		c.GoldPerVODGQLRPM = 30
	}
	if c.GoldSegmentSizeSeconds <= 0 {
		c.GoldSegmentSizeSeconds = c.AnalyticsVODGQLSegmentSeconds
	}
	if c.GoldSegmentSizeSeconds <= 0 {
		c.GoldSegmentSizeSeconds = 600
	}
	if c.GoldSegmentSizeSeconds < 60 {
		c.GoldSegmentSizeSeconds = 60
	}
	if c.GoldSegmentSizeSeconds > 3600 {
		c.GoldSegmentSizeSeconds = 3600
	}
	if c.GoldRetryMax <= 0 {
		c.GoldRetryMax = 3
	}
	if c.GoldRetryMax > 10 {
		c.GoldRetryMax = 10
	}
	if c.GoldLeaseTTLSeconds <= 0 {
		c.GoldLeaseTTLSeconds = 120
	}
	if c.GoldLeaseTTLSeconds < 30 {
		c.GoldLeaseTTLSeconds = 30
	}
	if c.GoldLeaseTTLSeconds > 3600 {
		c.GoldLeaseTTLSeconds = 3600
	}
	if c.BackfillQueueMaintenanceInterval <= 0 {
		c.BackfillQueueMaintenanceInterval = 30 * time.Minute
	}
	if c.BackfillRequeueFailedMaxPerRun <= 0 {
		c.BackfillRequeueFailedMaxPerRun = 25
	}
	if c.BackfillRepairSessionsMaxPerRun <= 0 {
		c.BackfillRepairSessionsMaxPerRun = 50
	}
	if c.PulseAutoBackfillInterval <= 0 {
		c.PulseAutoBackfillInterval = 15 * time.Minute
	}
	if c.PulseAutoBackfillCooldown <= 0 {
		c.PulseAutoBackfillCooldown = 30 * time.Minute
	}
	if c.PulseAutoBackfillSince <= 0 {
		c.PulseAutoBackfillSince = 48 * time.Hour
	}
	if c.PulseAutoBackfillMaxPerRun <= 0 {
		c.PulseAutoBackfillMaxPerRun = 1
	}
	if c.PulseAutoBackfillMaxPerRun > 5 {
		c.PulseAutoBackfillMaxPerRun = 5
	}
	if c.PulseAutoBackfillScanLimit <= 0 {
		c.PulseAutoBackfillScanLimit = c.PulseAutoBackfillMaxPerRun * 20
	}
	if c.PulseAutoBackfillScanLimit < c.PulseAutoBackfillMaxPerRun {
		c.PulseAutoBackfillScanLimit = c.PulseAutoBackfillMaxPerRun
	}
	if c.PulseAutoBackfillScanLimit > 200 {
		c.PulseAutoBackfillScanLimit = 200
	}
	return c, nil
}

func clampCorpusTopN(n int) int {
	if n <= 0 {
		return 0
	}
	if n > maxCorpusTopN {
		return maxCorpusTopN
	}
	return n
}

// ClampLiveAdmissionTopN bounds IRC live-admission roster size separately from
// corpus metadata top-N. When maxIRC is set, admission cannot exceed the IRC slot ceiling.
func ClampLiveAdmissionTopN(n, maxIRC int) int {
	if n <= 0 {
		return DefaultLiveAdmissionTopN
	}
	ceiling := MaxLiveAdmissionTopN
	if maxIRC > 0 && maxIRC < ceiling {
		ceiling = maxIRC
	}
	if n > ceiling {
		return ceiling
	}
	return n
}

// XContentToken returns the configured X auth token for unofficial ingest sidecars.
func (c Config) XContentToken() string {
	if t := strings.TrimSpace(c.XAuthToken); t != "" {
		return t
	}
	return strings.TrimSpace(c.EmusksXAuthToken)
}
