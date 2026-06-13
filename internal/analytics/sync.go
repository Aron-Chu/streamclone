package analytics

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math/rand"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/redis/go-redis/v9"

	"streamclone/internal/chat/enrich"
)

type SyncService struct {
	store          *Store
	enricher       *enrich.Enricher
	helix          *HelixClient
	emoteURL       string
	scraperURL   string
	scraperKey   string
	twitchGQLURL   string
	twitchClientID string
	userAgent           string
	client              *http.Client
	gqlClient           *http.Client
	vodGQLPageDelay        time.Duration
	vodGQLConcurrency      int
	vodGQLConcurrencyMin   int
	vodGQLConcurrencyMax   int
	vodGQLSegmentSeconds          int
	vodGQLDenseSegmentSeconds     int
	vodGQLHotSegmentPageThreshold int
	vodGQLHotSlowAdvanceSec       int
	vodGQLHotSlowAdvancePages     int
	vodGQLHotCommentsPerPage      int
	vodGQLPriorityEdgeSeconds     int
	vodGQLIncrementalDB           bool
	trackerScrapeTimeoutMS int
	passTTMaxAge              bool
	ttMaxAgeMSDefault         int
	ttStaleMaxAgeMS           int
	ttPrefetchEnabled         bool
	ttDirectHTTPEnabled       bool
	ttDirectHTTPStaleOnly     bool
	ttDirectHTTPTimeoutMS     int
	directHTTP                directHTTPTelemetry
	trackerPrefetch           trackerPrefetchState
	syncStatusCache        syncStatusCache
	log                 *slog.Logger
	rdb                 *redis.Client
}

var errTrackerAccessProtected = errors.New("tracker access protected")

func NewSyncService(
	store *Store,
	enricher *enrich.Enricher,
	helix *HelixClient,
	emoteURL string,
	scraperURL string,
	scraperKey string,
	twitchGQLURL string,
	twitchClientID string,
	userAgent string,
	logger *slog.Logger,
	rdb *redis.Client,
	vodGQLPageDelay time.Duration,
	vodGQLConcurrency int,
	vodGQLConcurrencyMin int,
	vodGQLConcurrencyMax int,
	vodGQLSegmentSeconds int,
	vodGQLDenseSegmentSeconds int,
	vodGQLHotSegmentPageThreshold int,
	vodGQLHotSlowAdvanceSec int,
	vodGQLHotSlowAdvancePages int,
	vodGQLHotCommentsPerPage int,
	vodGQLPriorityEdgeSeconds int,
	vodGQLIncrementalDB bool,
	trackerScrapeTimeoutMS int,
	passTTMaxAge bool,
	ttMaxAgeMSDefault int,
	ttStaleMaxAgeMS int,
	ttPrefetchEnabled bool,
	ttDirectHTTPEnabled bool,
	ttDirectHTTPStaleOnly bool,
	ttDirectHTTPTimeoutMS int,
) *SyncService {
	if trackerScrapeTimeoutMS <= 0 {
		trackerScrapeTimeoutMS = 120000
	}
	if vodGQLConcurrency <= 0 {
		vodGQLConcurrency = 1
	}
	if vodGQLConcurrencyMin <= 0 {
		vodGQLConcurrencyMin = 1
	}
	if vodGQLConcurrencyMax <= 0 {
		vodGQLConcurrencyMax = vodGQLConcurrency
	}
	if vodGQLConcurrencyMax > 8 {
		vodGQLConcurrencyMax = 8
	}
	if vodGQLConcurrencyMin > vodGQLConcurrencyMax {
		vodGQLConcurrencyMin = vodGQLConcurrencyMax
	}
	if vodGQLConcurrency > vodGQLConcurrencyMax {
		vodGQLConcurrency = vodGQLConcurrencyMax
	}
	if vodGQLConcurrency < vodGQLConcurrencyMin {
		vodGQLConcurrency = vodGQLConcurrencyMin
	}
	if vodGQLSegmentSeconds <= 0 {
		vodGQLSegmentSeconds = 600
	}
	if vodGQLDenseSegmentSeconds <= 0 {
		vodGQLDenseSegmentSeconds = vodGQLSegmentDenseVOD
	}
	if vodGQLHotSegmentPageThreshold <= 0 {
		vodGQLHotSegmentPageThreshold = 10
	}
	if vodGQLHotSlowAdvanceSec <= 0 {
		vodGQLHotSlowAdvanceSec = vodGQLHotSlowAdvanceSecDefault
	}
	if vodGQLHotSlowAdvancePages <= 0 {
		vodGQLHotSlowAdvancePages = vodGQLHotSlowAdvancePagesDefault
	}
	if vodGQLHotCommentsPerPage <= 0 {
		vodGQLHotCommentsPerPage = vodGQLHotCommentsPerPageDefault
	}
	if vodGQLPriorityEdgeSeconds <= 0 {
		vodGQLPriorityEdgeSeconds = gqlPriorityEdgeSecondsDefault
	}
	if ttDirectHTTPTimeoutMS <= 0 {
		ttDirectHTTPTimeoutMS = 1200
	}
	gqlTransport := &http.Transport{
		MaxIdleConns:        32,
		MaxIdleConnsPerHost: 16,
		IdleConnTimeout:     90 * time.Second,
	}
	return &SyncService{
		store:          store,
		enricher:       enricher,
		helix:          helix,
		emoteURL:       strings.TrimRight(emoteURL, "/"),
		scraperURL:   scraperURL,
		scraperKey:   scraperKey,
		twitchGQLURL:   twitchGQLURL,
		twitchClientID: twitchClientID,
		userAgent:      userAgent,
		client: &http.Client{
			Timeout: time.Duration(trackerScrapeTimeoutMS)*time.Millisecond + 45*time.Second,
		},
		gqlClient: &http.Client{
			Timeout:   30 * time.Second,
			Transport: gqlTransport,
		},
		vodGQLPageDelay:        vodGQLPageDelay,
		vodGQLConcurrency:      vodGQLConcurrency,
		vodGQLConcurrencyMin:   vodGQLConcurrencyMin,
		vodGQLConcurrencyMax:   vodGQLConcurrencyMax,
		vodGQLSegmentSeconds:          vodGQLSegmentSeconds,
		vodGQLDenseSegmentSeconds:     vodGQLDenseSegmentSeconds,
		vodGQLHotSegmentPageThreshold: vodGQLHotSegmentPageThreshold,
		vodGQLHotSlowAdvanceSec:       vodGQLHotSlowAdvanceSec,
		vodGQLHotSlowAdvancePages:     vodGQLHotSlowAdvancePages,
		vodGQLHotCommentsPerPage:      vodGQLHotCommentsPerPage,
		vodGQLPriorityEdgeSeconds:     vodGQLPriorityEdgeSeconds,
		vodGQLIncrementalDB:           vodGQLIncrementalDB,
		trackerScrapeTimeoutMS: trackerScrapeTimeoutMS,
		passTTMaxAge:              passTTMaxAge,
		ttMaxAgeMSDefault:         ttMaxAgeMSDefault,
		ttStaleMaxAgeMS:           ttStaleMaxAgeMS,
		ttPrefetchEnabled:         ttPrefetchEnabled,
		ttDirectHTTPEnabled:       ttDirectHTTPEnabled,
		ttDirectHTTPStaleOnly:     ttDirectHTTPStaleOnly,
		ttDirectHTTPTimeoutMS:     ttDirectHTTPTimeoutMS,
		trackerPrefetch:           *newTrackerPrefetchState(),
		log:                       logger.With("service", "sync"),
		rdb:                    rdb,
	}
}

type GQLRequest struct {
	OperationName string `json:"operationName"`
	Variables     struct {
		VideoID              string  `json:"videoID,omitempty"`
		ContentOffsetSeconds *int    `json:"contentOffsetSeconds,omitempty"`
		Cursor               *string `json:"cursor,omitempty"`
	} `json:"variables"`
	Extensions struct {
		PersistedQuery struct {
			Version    int    `json:"version"`
			SHA256Hash string `json:"sha256Hash"`
		} `json:"persistedQuery"`
	} `json:"extensions"`
}

type GQLCommentEdge struct {
	Cursor string `json:"cursor"`
	Node   struct {
		ID                   string `json:"id"`
		ContentOffsetSeconds int    `json:"contentOffsetSeconds"`
		Message              struct {
			Body      string `json:"body"`
			Fragments []struct {
				Text string `json:"text"`
			} `json:"fragments"`
		} `json:"message"`
	} `json:"node"`
}

type GQLResponse struct {
	Data struct {
		Video *struct {
			Comments *struct {
				Edges    []GQLCommentEdge `json:"edges"`
				PageInfo struct {
					HasNextPage bool `json:"hasNextPage"`
				} `json:"pageInfo"`
			} `json:"comments"`
		} `json:"video"`
	} `json:"data"`
	Errors []struct {
		Message string `json:"message"`
	} `json:"errors"`
}

func (s *SyncService) SyncHistoricalStream(ctx context.Context, streamID string, channelOpt string, viewersOnly bool, forceChat bool, hintVodID string) (string, error) {
	s.log.Info("starting historical stream sync", "stream_id", streamID, "channel_opt", channelOpt, "viewers_only", viewersOnly)
	s.setSyncPhase(ctx, streamID, SyncPhaseStarting, "Loading stream metadata", nil)

	var login string
	var startedAt time.Time
	var title string
	var category string
	var hasStreamRecord bool

	stream, err := s.store.StreamByID(ctx, streamID)
	if err == nil {
		login = stream.Login
		startedAt = stream.StartedAt
		title = stream.Title
		category = stream.Category
		hasStreamRecord = true
	} else {
		if !errors.Is(err, pgx.ErrNoRows) {
			return "", fmt.Errorf("failed to fetch stream details from DB: %w", err)
		}
		if channelOpt == "" {
			return "", fmt.Errorf("stream details not found in DB and no channel query parameter provided")
		}
		login = normalizeLogin(channelOpt)
	}

	broadcasterID := ""
	if hasStreamRecord && stream != nil {
		if s.helix != nil {
			broadcasterID = s.helix.ResolveBroadcasterID(ctx, login, stream.BroadcasterID)
		} else {
			broadcasterID = NormalizeBroadcasterID(stream.BroadcasterID)
		}
	}
	if broadcasterID == "" && login != "" && s.helix != nil {
		broadcasterID = s.helix.ResolveBroadcasterID(ctx, login, "")
		if broadcasterID == "" {
			s.log.Warn("helix user lookup failed or returned empty", "login", login, "stream_id", streamID)
		}
	}

	cachedVodID := strings.TrimSpace(hintVodID)
	if cachedVodID == "" && hasStreamRecord && stream != nil {
		cachedVodID = strings.TrimSpace(stream.VodID)
	}
	if strings.TrimSpace(hintVodID) != "" {
		if err := s.store.SetStreamVodID(ctx, streamID, cachedVodID, "client_hint"); err != nil {
			s.log.Warn("failed to persist hinted vod_id", "stream_id", streamID, "err", err)
		}
	}

	if broadcasterID != "" {
		_, dbErr := s.store.db.Exec(ctx, `
			UPDATE analytics_streams
			SET broadcaster_id = $2, updated_at = now()
			WHERE stream_id = $1 AND COALESCE(broadcaster_id, '') IN ('', 'pending')`,
			streamID, broadcasterID,
		)
		if dbErr != nil {
			s.log.Warn("failed to persist broadcaster_id before sync", "stream_id", streamID, "err", dbErr)
		}
	}

	if !hasStreamRecord && login != "" {
		if broadcasterID != "" && s.helix != nil && s.helix.Enabled() {
			if meta, metaErr := s.helix.VideoByStreamID(ctx, broadcasterID, streamID); metaErr != nil {
				s.log.Warn("helix video metadata lookup failed", "stream_id", streamID, "err", metaErr)
			} else {
				if !meta.CreatedAt.IsZero() {
					startedAt = meta.CreatedAt
				}
				if strings.TrimSpace(meta.Title) != "" {
					title = strings.TrimSpace(meta.Title)
				}
				if cachedVodID == "" && strings.TrimSpace(meta.VideoID) != "" {
					cachedVodID = strings.TrimSpace(meta.VideoID)
				}
			}
		}
		if err := s.store.UpsertStreamPlaceholder(ctx, streamID, broadcasterID, login, title, startedAt); err != nil {
			s.log.Warn("failed to upsert placeholder stream record", "stream_id", streamID, "err", err)
		} else {
			s.log.Info("upserted placeholder stream record for sync", "stream_id", streamID, "login", login)
		}
	}

	skipTracker := !viewersOnly && hasStreamRecord && stream != nil && s.shouldSkipTracker(ctx, stream)
	viewerStatus := "pending"
	if skipTracker {
		viewerStatus = "skipped"
	}

	var helixVodID string
	var helixResolveMS int64
	var helixWG sync.WaitGroup
	if cachedVodID == "" && broadcasterID != "" && s.helix != nil && s.helix.Enabled() {
		helixWG.Add(1)
		go func() {
			defer helixWG.Done()
			start := time.Now()
			resolved, helixErr := s.helix.VideoIDByStreamID(ctx, broadcasterID, streamID)
			helixResolveMS = time.Since(start).Milliseconds()
			if helixErr != nil {
				s.log.Warn("helix vod lookup failed (parallel)", "stream_id", streamID, "err", helixErr, "duration_ms", helixResolveMS)
			} else if resolved != "" {
				helixVodID = resolved
				s.log.Info("resolved VOD ID via Helix (parallel)", "vod_id", resolved, "duration_ms", helixResolveMS)
			}
		}()
	}

	commentsMap := make(map[int][]string) // offset minutes -> comments text
	chatCache := newChatRollupCache()
	var gameSegments []scrapedGame
	var commentsErr error
	var gqlFetchMS int64
	var rollupStartMu sync.RWMutex
	sharedRollupStart := time.Time{}
	if !startedAt.IsZero() {
		sharedRollupStart = startedAt.UTC().Truncate(time.Minute)
	}
	rollupStartFn := func() time.Time {
		rollupStartMu.RLock()
		defer rollupStartMu.RUnlock()
		return sharedRollupStart
	}
	setSharedRollupStart := func(ts time.Time) {
		if ts.IsZero() {
			return
		}
		rollupStartMu.Lock()
		sharedRollupStart = ts.UTC().Truncate(time.Minute)
		rollupStartMu.Unlock()
	}
	resolveChatAlignSec := func(vodID string) int {
		streamStart := sharedRollupStart
		if streamStart.IsZero() && !startedAt.IsZero() {
			streamStart = startedAt.UTC().Truncate(time.Minute)
		}
		if streamStart.IsZero() || vodID == "" || s.helix == nil || !s.helix.Enabled() {
			return 0
		}
		var vodCreated time.Time
		if broadcasterID != "" {
			if meta, err := s.helix.VideoByStreamID(ctx, broadcasterID, streamID); err == nil && !meta.CreatedAt.IsZero() {
				vodCreated = meta.CreatedAt
			}
		}
		if vodCreated.IsZero() {
			if createdAt, err := s.helix.VideoCreatedAt(ctx, vodID); err == nil {
				vodCreated = createdAt
			}
		}
		return vodChatAlignSeconds(streamStart, vodCreated)
	}

	var commentsWG sync.WaitGroup
	var commentsFetchStarted bool
	startVODCommentsFetch := func(vod string, viewerPts []parsedViewerPoint, scrapeGames []scrapedGame) {
		if viewersOnly || vod == "" || commentsFetchStarted {
			return
		}
		if !forceChat && hasStreamRecord && stream != nil && s.shouldSkipVODChat(ctx, stream, vod) {
			s.log.Info("skipping VOD chat GQL fetch; chat coverage already complete",
				"stream_id", streamID,
				"vod_id", vod,
				"chat_messages", stream.ChatMessages,
			)
			return
		}
		commentsFetchStarted = true
		commentsWG.Add(1)
		go func() {
			defer commentsWG.Done()
			s.setSyncPhase(ctx, streamID, SyncPhaseFetchingComments, "Fetching VOD chat via Twitch GQL", nil)
			start := time.Now()
			chatAlignSec := resolveChatAlignSec(vod)
			vodDur := s.vodDurationSeconds(ctx, vod)
			scheduleHints := s.gqlScheduleHintsForStream(ctx, streamID, vodDur, scrapeGames, viewerPts)
			commentsErr = s.fetchVODComments(ctx, streamID, login, vod, commentsMap, vodDur, chatAlignSec, rollupStartFn, chatCache, scheduleHints)
			gqlFetchMS = time.Since(start).Milliseconds()
			s.log.Info("sync phase complete", "stream_id", streamID, "phase", "gql_fetch", "duration_ms", gqlFetchMS, "chat_align_sec", chatAlignSec)
		}()
	}

	var html string
	var tracker trackerStreamData
	var trackerScrapeMS int64

	if viewersOnly || !skipTracker {
		// 2. Scrape TwitchTracker via local browser scraper
		trackerURL := fmt.Sprintf("https://twitchtracker.com/%s/streams/%s", login, streamID)
		s.log.Info("scraping TwitchTracker", "url", trackerURL)
		s.setSyncPhase(ctx, streamID, SyncPhaseScrapingTracker, "Scraping TwitchTracker page", func(st *SyncStatus) {
			st.Tracker = &SyncTrackerProgress{
				Active:  true,
				URL:     trackerURL,
				Message: "Browser scrape for viewer chart (meta#ecs)",
			}
		})

		trackerStart := time.Now()
		html, err = s.scrapeTwitchTracker(ctx, trackerURL, stream, viewersOnly)
		if err != nil && strings.Contains(strings.ToLower(err.Error()), "browser has been closed") {
			s.log.Warn("retrying TwitchTracker scrape after Camoufox browser crash", "stream_id", streamID)
			time.Sleep(2 * time.Second)
			html, err = s.scrapeTwitchTracker(ctx, trackerURL, stream, viewersOnly)
		}
		trackerScrapeMS = time.Since(trackerStart).Milliseconds()
		s.updateTrackerProgress(ctx, streamID, func(tp *SyncTrackerProgress) {
			tp.Active = false
			if err != nil {
				tp.Message = "TwitchTracker scrape failed"
			} else {
				tp.Message = "TwitchTracker page loaded"
			}
		})
		s.log.Info("sync phase complete", "stream_id", streamID, "phase", "tracker_scrape", "duration_ms", trackerScrapeMS)
		if err != nil {
			if viewersOnly {
				commentsWG.Wait()
				helixWG.Wait()
				if errors.Is(err, errTrackerAccessProtected) {
					return "", fmt.Errorf("tracker access protected (cloudflare challenge/block) — warm scraper profile and retry: %w", err)
				}
				return "", fmt.Errorf("failed to scrape TwitchTracker: %w", err)
			}
			viewerStatus = "failed"
			s.log.Warn("TwitchTracker scrape failed; continuing chat sync", "stream_id", streamID, "err", err)
			s.setSyncPhase(ctx, streamID, SyncPhaseScrapingTracker, "TwitchTracker failed — continuing chat sync", func(st *SyncStatus) {
				st.ViewerStatus = viewerStatus
				if st.Tracker == nil {
					st.Tracker = &SyncTrackerProgress{}
				}
				st.Tracker.Message = "Viewer chart unavailable — chat sync continues. Run scripts/scraper-preflight.ps1 or Re-sync viewers after scraper recovery."
			})
		} else {
			viewerStatus = "ok"
		}
	} else {
		viewerStatus = "skipped"
		s.log.Info("skipping TwitchTracker scrape; viewer timeline already synced", "stream_id", streamID)
		s.setSyncPhase(ctx, streamID, SyncPhaseScrapingTracker, "Skipping TwitchTracker (viewers already synced)", nil)
		s.setSyncPhase(ctx, streamID, SyncPhaseParsingTracker, "Loading viewer rollups from database", nil)
		tracker, err = s.trackerDataFromDB(ctx, stream)
		if err != nil {
			commentsWG.Wait()
			return "", err
		}
		if startedAt.IsZero() && hasStreamRecord && stream != nil {
			startedAt = stream.StartedAt
		}
		if !tracker.ChartStartedAt.IsZero() {
			setSharedRollupStart(tracker.ChartStartedAt)
		} else if !startedAt.IsZero() {
			setSharedRollupStart(startedAt)
		}
		startVODCommentsFetch(cachedVodID, tracker.ViewerPoints, tracker.Games)
	}

	// Parse stream metadata if this is a new stream record
	if startedAt.IsZero() && html != "" {
		reStart := regexp.MustCompile(`(?i)class="stream-timestamp-dt[^"]*">([^<]+)</div>`)
		startMatch := reStart.FindStringSubmatch(html)
		if len(startMatch) > 1 {
			parsedTime, parseErr := time.ParseInLocation("2006-01-02 15:04:05", strings.TrimSpace(startMatch[1]), time.UTC)
			if parseErr == nil {
				startedAt = parsedTime
			}
		}
		if startedAt.IsZero() {
			reDescStart := regexp.MustCompile(`(?i)stream on (\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2})`)
			descMatch := reDescStart.FindStringSubmatch(html)
			if len(descMatch) > 1 {
				parsedTime, parseErr := time.ParseInLocation("2006-01-02 15:04:05", strings.TrimSpace(descMatch[1]), time.UTC)
				if parseErr == nil {
					startedAt = parsedTime
				}
			}
		}
		if startedAt.IsZero() {
			startedAt = time.Now().UTC()
		}

		title = fmt.Sprintf("%s Stream on %s", login, startedAt.Format("2006-01-02"))
		reTitleTag := regexp.MustCompile(`(?i)<title>([^<]+)</title>`)
		if titleMatch := reTitleTag.FindStringSubmatch(html); len(titleMatch) > 1 {
			rawTitle := titleMatch[1]
			if idx := strings.Index(rawTitle, " - Stats"); idx != -1 {
				rawTitle = rawTitle[:idx]
			}
			title = strings.TrimSpace(rawTitle)
		}
	}

	// 3. Parse TwitchTracker HTML (when scraped)
	if html != "" {
		s.setSyncPhase(ctx, streamID, SyncPhaseParsingTracker, "Parsing viewer chart and stream stats", func(st *SyncStatus) {
			if st.Tracker == nil {
				st.Tracker = &SyncTrackerProgress{}
			}
			st.Tracker.Message = "Parsing meta#ecs viewer chart"
		})
		tracker, err = s.parseTwitchTrackerHTML(html, startedAt)
		if err != nil {
			s.log.Warn("twitchtracker parse partially failed or missing elements", "err", err)
		}
		if !tracker.ChartStartedAt.IsZero() {
			setSharedRollupStart(tracker.ChartStartedAt)
		} else if !startedAt.IsZero() {
			setSharedRollupStart(startedAt)
		}
		startVODCommentsFetch(cachedVodID, tracker.ViewerPoints, tracker.Games)
	}
	durationMinutes := tracker.DurationMinutes
	peakViewers := tracker.PeakViewers
	gameSegments = tracker.Games
	viewerPoints := tracker.ViewerPoints
	if html != "" {
		if fromHTML := parseTrackerDurationMinutesFromHTML(html); fromHTML > durationMinutes {
			durationMinutes = fromHTML
		}
	}

	if category == "" {
		if len(gameSegments) > 0 {
			category = gameSegments[0].Title
		} else {
			category = "Live"
		}
	}

	if !hasStreamRecord {
		// Insert placeholder stream record
		_, err = s.store.db.Exec(ctx, `
			INSERT INTO analytics_streams (
				stream_id, broadcaster_id, login, display_name, title, category, started_at, last_seen_at, tags, peak_viewers
			)
			VALUES ($1, $2, $3, $3, $4, $5, $6, $6, '[]'::jsonb, $7)
			ON CONFLICT (stream_id) DO NOTHING`,
			streamID, broadcasterID, login, title, category, startedAt, peakViewers,
		)
		if err != nil {
			s.log.Error("failed to insert placeholder stream record in DB", "err", err)
		}
	}

	// Fallback to DB duration if parsing failed
	durationSeconds := durationMinutes * 60
	if durationSeconds <= 0 && hasStreamRecord && stream != nil && stream.EndedAt != nil {
		if d := int(stream.EndedAt.Sub(stream.StartedAt).Seconds()); d > 0 {
			durationSeconds = d
		}
	}
	if durationSeconds <= 0 {
		durationSeconds = 60
	}
	if !hasCompleteViewerChart(viewerPoints, durationSeconds) {
		s.log.Warn("viewer chart incomplete",
			"stream_id", streamID,
			"last_offset_sec", lastViewerOffsetSeconds(viewerPoints),
			"duration_sec", durationSeconds,
			"point_count", len(viewerPoints),
			"has_ecs", strings.Contains(html, `id="ecs"`),
			"has_injected_chart", strings.Contains(html, `id="streamclone-viewer-chart"`),
		)
		viewerPoints = nil
	}

	rollupStartEarly := startedAt.UTC().Truncate(time.Minute)
	if !tracker.ChartStartedAt.IsZero() {
		rollupStartEarly = tracker.ChartStartedAt.UTC().Truncate(time.Minute)
	}
	if len(viewerPoints) > 0 && !viewersOnly && !skipTracker {
		s.persistEarlyViewerChart(ctx, streamID, rollupStartEarly, durationSeconds, peakViewers, tracker.AvgViewers, viewerPoints, gameSegments)
	}

	// 4. Resolve VOD id (DB cache → Helix → TwitchTracker HTML)
	s.setSyncPhase(ctx, streamID, SyncPhaseResolvingVOD, "Resolving Twitch VOD ID", func(st *SyncStatus) {
		if cachedVodID != "" {
			st.Message = fmt.Sprintf("VOD ID cached: %s", cachedVodID)
		} else {
			st.Message = "Looking up VOD via Helix / TwitchTracker"
		}
	})
	vodResolveStart := time.Now()
	vodID := cachedVodID
	vodSource := ""
	if vodID != "" {
		vodSource = "db_cache"
	} else {
		helixWG.Wait()
		if helixVodID != "" {
			vodID = helixVodID
			vodSource = "helix_stream_match"
		} else if html != "" {
			vodID = s.extractVodID(html)
			if vodID != "" {
				vodSource = "tracker_html"
				s.log.Info("extracted VOD ID from TwitchTracker HTML", "vod_id", vodID)
			}
		}
	}
	vodResolveMS := helixResolveMS
	if vodResolveMS == 0 {
		vodResolveMS = time.Since(vodResolveStart).Milliseconds()
	}
	s.log.Info("sync phase complete", "stream_id", streamID, "phase", "vod_resolve", "duration_ms", vodResolveMS, "vod_id", vodID, "vod_source", vodSource)
	if vodID != "" {
		if err := s.store.SetStreamVodID(ctx, streamID, vodID, vodSource); err != nil {
			s.log.Warn("failed to persist vod_id", "stream_id", streamID, "err", err)
		}
	}
	if !viewersOnly {
		s.preloadChannelEmotes(ctx, login, broadcasterID)
	}

	if broadcasterID != "" {
		_, dbErr := s.store.db.Exec(ctx, `
			UPDATE analytics_streams
			SET broadcaster_id = $2, updated_at = now()
			WHERE stream_id = $1 AND COALESCE(broadcaster_id, '') = ''`,
			streamID, broadcasterID,
		)
		if dbErr != nil {
			s.log.Warn("failed to update broadcaster_id on stream record", "stream_id", streamID, "err", dbErr)
		}
	}

	if viewersOnly {
		s.log.Info("viewers-only sync; skipping VOD comment fetch", "stream_id", streamID)
	} else if cachedVodID != "" {
		startVODCommentsFetch(cachedVodID, viewerPoints, gameSegments)
		s.setSyncPhase(ctx, streamID, SyncPhaseFetchingComments, "Waiting for VOD chat workers to finish", nil)
		commentsWG.Wait()
		if commentsErr != nil {
			s.log.Warn("failed to fetch VOD comments (parallel)", "err", commentsErr)
		}
	} else if vodID != "" {
		s.log.Info("fetching comments from Twitch GQL for VOD", "vod_id", vodID)
		s.setSyncPhase(ctx, streamID, SyncPhaseFetchingComments, "Fetching VOD chat comments", nil)
		gqlStart := time.Now()
		chatAlignSec := resolveChatAlignSec(vodID)
		vodDur := s.vodDurationSeconds(ctx, vodID)
		scheduleHints := s.gqlScheduleHintsForStream(ctx, streamID, vodDur, gameSegments, viewerPoints)
		if err := s.fetchVODComments(ctx, streamID, login, vodID, commentsMap, vodDur, chatAlignSec, rollupStartFn, chatCache, scheduleHints); err != nil {
			s.log.Warn("failed to fetch VOD comments (it may have been deleted)", "err", err)
		}
		gqlFetchMS = time.Since(gqlStart).Milliseconds()
		s.log.Info("sync phase complete", "stream_id", streamID, "phase", "gql_fetch", "duration_ms", gqlFetchMS)
	} else {
		commentsWG.Wait()
		reason := "Helix archive lookup returned no VOD for this stream ID"
		if NormalizeBroadcasterID(broadcasterID) == "" {
			reason = "broadcaster ID missing — check TWITCH_OAUTH_CLIENT_ID/SECRET on analytics service"
		} else if s.helix == nil || !s.helix.Enabled() {
			reason = "analytics Helix client not configured"
		}
		s.log.Warn("no VOD ID found; skipping chat comments sync",
			"stream_id", streamID,
			"broadcaster_id", broadcasterID,
			"reason", reason,
		)
	}

	helixVodDurationSec := 0
	if !viewersOnly && vodID != "" && s.helix != nil && s.helix.Enabled() {
		vodDuration, helixErr := s.helix.VideoDurationSeconds(ctx, vodID)
		if helixErr != nil {
			s.log.Warn("helix vod duration lookup failed", "vod_id", vodID, "err", helixErr)
		} else {
			helixVodDurationSec = vodDuration
			if vodDuration > 0 && durationSeconds > 0 && vodDuration < durationSeconds/2 {
				s.log.Warn("vod shorter than stream — chat coverage may be partial",
					"stream_id", streamID,
					"vod_id", vodID,
					"vod_duration_sec", vodDuration,
					"tracker_duration_sec", durationSeconds,
				)
			}
			if vodDuration > durationSeconds {
				s.log.Info("using helix vod duration for rollups",
					"vod_id", vodID,
					"tracker_seconds", durationSeconds,
					"vod_seconds", vodDuration,
				)
				durationSeconds = vodDuration
				if len(viewerPoints) > 0 && !hasCompleteViewerChart(viewerPoints, durationSeconds) {
					s.log.Warn("viewer chart shorter than helix vod duration; dropping sparse viewer timeline",
						"stream_id", streamID,
						"last_offset_sec", lastViewerOffsetSeconds(viewerPoints),
						"duration_sec", durationSeconds,
						"point_count", len(viewerPoints),
					)
					viewerPoints = nil
				}
			}
		}
	}

	if !viewersOnly && commentsFetchStarted {
		rollupsExpected := countChatMinutesInMap(commentsMap)
		s.updateChatProgress(ctx, streamID, func(cp *SyncChatProgress) {
			cp.Active = false
			cp.IndexPhase = "tokenizing"
			cp.StreamDurationSec = durationSeconds
			cp.RollupsExpected = rollupsExpected
			if helixVodDurationSec > 0 {
				cp.VodDurationSec = helixVodDurationSec
			}
		}, true)
	}
	// 5. Combine and build minute-by-minute rollups
	rollupStart := startedAt.UTC().Truncate(time.Minute)
	if !tracker.ChartStartedAt.IsZero() {
		rollupStart = tracker.ChartStartedAt.UTC().Truncate(time.Minute)
	}
	setSharedRollupStart(rollupStart)
	endTS := rollupStart.Add(time.Duration(durationSeconds) * time.Second)
	if !endTS.After(rollupStart) {
		s.log.Warn("sync clamped non-positive stream duration", "stream_id", streamID, "duration_seconds", durationSeconds)
		endTS = rollupStart.Add(time.Minute)
		durationSeconds = 60
	}
	totalMinutes := durationSeconds / 60
	if totalMinutes <= 0 {
		totalMinutes = 1
	}

	var rollups []MinuteRollup
	var tokenizeMS int64
	if skipTracker && !viewersOnly {
		rollups = nil
	} else {
	if !viewersOnly {
		s.setSyncPhase(ctx, streamID, SyncPhaseWritingRollups, "Tokenizing chat and emotes", func(st *SyncStatus) {
			if st.Chat != nil {
				st.Chat.IndexPhase = "tokenizing"
				st.Chat.StreamDurationSec = durationSeconds
			}
		})
	}
	tokenizeStart := time.Now()
	var cacheFn func(int) (CachedMinuteRollup, bool)
	if chatCache != nil {
		cacheFn = func(minute int) (CachedMinuteRollup, bool) {
			r, ok := chatCache.get(minute)
			if !ok {
				return CachedMinuteRollup{}, false
			}
			return CachedMinuteRollup{
				ChatCount:         r.ChatCount,
				TotalEmoteCount:   r.TotalEmoteCount,
				SevenTVEmoteCount: r.SevenTVEmoteCount,
				Emotes:            r.Emotes,
			}, true
		}
	}
	rollups = BuildMinuteRollupsFromCommentsCached(
		login,
		s.enricher,
		commentsMap,
		toRollupViewerPoints(viewerPoints),
		rollupStart,
		durationSeconds,
		cacheFn,
	)
	tokenizeMS = time.Since(tokenizeStart).Milliseconds()
	s.log.Info("sync phase complete", "stream_id", streamID, "phase", "tokenize", "duration_ms", tokenizeMS)
	}

	chatRollupsWritten := 0
	for _, rollup := range rollups {
		if rollup.ChatCount > 0 || rollup.SevenTVEmoteCount > 0 {
			chatRollupsWritten++
		}
	}

	// 6. Save data to database
	s.log.Info("saving historical rollups and game segments to database", "rollups_count", len(rollups), "segments_count", len(gameSegments))

	rollupWriteStart := time.Now()
	chatMinutesExpected := countChatMinutesInMap(commentsMap)
	if !viewersOnly {
		s.updateChatProgress(ctx, streamID, func(cp *SyncChatProgress) {
			cp.IndexPhase = "writing"
			cp.RollupsExpected = chatMinutesExpected
			cp.StreamDurationSec = durationSeconds
		}, true)
	}
	if skipTracker && !viewersOnly {
		s.setSyncPhase(ctx, streamID, SyncPhaseWritingRollups, fmt.Sprintf("Writing %d chat minutes", chatMinutesExpected), func(st *SyncStatus) {
			if st.Chat != nil {
				st.Chat.IndexPhase = "writing"
			}
		})
		err = s.writeChatRollupsOnly(ctx, streamID, login, rollupStart, commentsMap, chatCache)
		if err != nil {
			return "", fmt.Errorf("failed to save chat rollups to DB: %w", err)
		}
	} else {
		writeMsg := "Writing minute rollups to database"
		if !viewersOnly && chatMinutesExpected > 0 {
			writeMsg = fmt.Sprintf("Writing %d chat minutes", chatMinutesExpected)
		}
		s.setSyncPhase(ctx, streamID, SyncPhaseWritingRollups, writeMsg, func(st *SyncStatus) {
			if !viewersOnly {
				st.RollupsWritten = 0
				if st.Chat != nil {
					st.Chat.IndexPhase = "writing"
					st.Chat.RollupsExpected = chatMinutesExpected
				}
			} else {
				st.RollupsWritten = len(rollups)
			}
		})
		if viewersOnly {
			err = s.store.BulkPatchViewerRollups(ctx, streamID, rollups)
		} else {
			err = s.store.BulkUpsertMinuteRollups(ctx, streamID, rollups)
		}
		if err != nil {
			return "", fmt.Errorf("failed to save minute rollups to DB: %w", err)
		}

		segments := buildGameSegments(gameSegments, durationSeconds)
		for i := range segments {
			segments[i].StreamID = streamID
		}
		if err = s.store.SaveGameSegments(ctx, streamID, segments); err != nil {
			return "", fmt.Errorf("failed to save game segments to DB: %w", err)
		}
	}

	if !skipTracker || viewersOnly {
		// Update ended_at and TwitchTracker viewer summary on stream record
		_, err = s.store.db.Exec(ctx, `
			UPDATE analytics_streams
			SET ended_at = $2,
			    peak_viewers = GREATEST(peak_viewers, $3::int),
			    avg_viewers = CASE WHEN $4::int > 0 THEN GREATEST(avg_viewers, $4::int) ELSE avg_viewers END,
			    updated_at = now()
			WHERE stream_id = $1
		`, streamID, endTS, peakViewers, tracker.AvgViewers)
		if err != nil {
			s.log.Error("failed to update stream end metadata in DB", "err", err)
		}
	}

	rollupWriteMS := time.Since(rollupWriteStart).Milliseconds()
	s.log.Info("sync phase complete", "stream_id", streamID, "phase", "rollup_write", "duration_ms", rollupWriteMS)
	s.setSyncPhase(ctx, streamID, SyncPhaseWritingRollups, "Rollups saved", func(st *SyncStatus) {
		st.ViewerStatus = viewerStatus
		if !viewersOnly && chatRollupsWritten > 0 {
			st.RollupsWritten = chatRollupsWritten
		}
		if st.Chat != nil {
			st.Chat.IndexPhase = "done"
			st.Chat.Active = false
		}
		st.Timing = &SyncPhaseTiming{
			TrackerScrapeMS: trackerScrapeMS,
			VodResolveMS:    vodResolveMS,
			GQLFetchMS:      gqlFetchMS,
			TokenizeMS:      tokenizeMS,
			RollupWriteMS:   rollupWriteMS,
		}
	})

	chatComments := 0
	for _, comments := range commentsMap {
		chatComments += len(comments)
	}
	msg := "Stream synced successfully"
	if viewersOnly {
		if len(viewerPoints) == 0 {
			msg = "Chat/7TV unchanged — TwitchTracker viewer chart blocked (no minute-level viewers). Try again later or check streamclone-scraper logs."
		} else {
			msg = "Viewer timeline synced from TwitchTracker (chat/7TV unchanged)"
		}
	} else if len(viewerPoints) == 0 {
		if vodID == "" {
			msg = "Stream synced (chat skipped — VOD not found). Viewer chart blocked by TwitchTracker."
		} else if chatComments == 0 {
			msg = "Stream synced (chat unavailable). Viewer chart blocked by TwitchTracker."
		} else {
			msg = "Chat/emotes synced. Viewer chart incomplete — Re-sync viewers."
		}
	} else if vodID == "" {
		if NormalizeBroadcasterID(broadcasterID) == "" {
			msg = "Stream synced (viewers only — VOD chat skipped: broadcaster ID missing; re-sync after Helix credentials are set)"
		} else {
			msg = "Stream synced (viewers only — VOD not found in Helix/TwitchTracker; chat/7TV skipped)"
		}
	} else if chatComments == 0 {
		msg = "Stream synced (viewers only — VOD comments unavailable)"
	}
	if skipTracker && !viewersOnly && chatComments > 0 {
		msg = fmt.Sprintf("Chat/emotes synced (%s comments); viewer timeline unchanged", strconv.Itoa(chatComments))
	}
	coverageRollups := rollups
	if len(coverageRollups) == 0 && !viewersOnly && chatComments > 0 {
		if persisted, loadErr := s.store.RollupsByStream(ctx, streamID); loadErr == nil {
			coverageRollups = persisted
		}
	}
	if !viewersOnly && chatComments > 0 && len(coverageRollups) > 0 {
		streamForCoverage := stream
		if updated, loadErr := s.store.StreamByID(ctx, streamID); loadErr == nil {
			streamForCoverage = updated
		} else if streamForCoverage != nil {
			copy := *streamForCoverage
			streamForCoverage = &copy
			streamForCoverage.EndedAt = &endTS
		}
		summary := chatCoverageSummary(coverageRollups, streamForCoverage, helixVodDurationSec)
		s.log.Info("chat coverage after sync",
			"stream_id", streamID,
			"vod_id", vodID,
			"vod_duration_sec", helixVodDurationSec,
			"tracker_duration_sec", durationSeconds,
			"chat_span_minutes", summary.ChatSpanMinutes,
			"coverage_pct", summary.CoveragePct,
			"partial", summary.Partial,
		)
		if summary.Partial {
			msg = formatPartialChatCoverageMessage(vodID, summary)
		}
	}
	s.updateChatProgress(ctx, streamID, func(cp *SyncChatProgress) {
		cp.Active = false
		cp.IndexPhase = "done"
	}, true)
	s.log.Info("historical stream sync completed successfully", "stream_id", streamID, "chat_comments", chatComments, "vod_id", vodID)
	return msg, nil
}

type scrapedGame struct {
	Title           string
	BoxArt          string
	DurationMinutes int
}

type parsedViewerPoint struct {
	OffsetSeconds int
	Viewers       int
}

type trackerStreamData struct {
	DurationMinutes int
	PeakViewers     int
	AvgViewers      int
	ChartStartedAt  time.Time
	Games           []scrapedGame
	ViewerPoints    []parsedViewerPoint
}

func coerceChartInt(v any) int {
	switch t := v.(type) {
	case float64:
		return int(t)
	case int:
		return t
	case int64:
		return int(t)
	case string:
		n, _ := strconv.Atoi(strings.TrimSpace(t))
		return n
	default:
		return 0
	}
}

func chartMaxViewers(points []parsedViewerPoint) int {
	max := 0
	for _, pt := range points {
		if pt.Viewers > max {
			max = pt.Viewers
		}
	}
	return max
}

// hasCompleteViewerChart rejects peak-only/flat charts and partial tails.
func hasCompleteViewerChart(points []parsedViewerPoint, durationSeconds int) bool {
	if len(points) < 3 {
		return false
	}
	minV, maxV := points[0].Viewers, points[0].Viewers
	for _, pt := range points[1:] {
		if pt.Viewers < minV {
			minV = pt.Viewers
		}
		if pt.Viewers > maxV {
			maxV = pt.Viewers
		}
	}
	if maxV <= minV {
		return false
	}
	if durationSeconds <= 0 {
		return true
	}
	lastOffset := lastViewerOffsetSeconds(points)
	if lastOffset < (durationSeconds*85)/100 {
		return false
	}
	if durationSeconds > 30*60 {
		durationMinutes := (durationSeconds + 59) / 60
		minPoints := durationMinutes / 5
		if minPoints < 10 {
			minPoints = 10
		}
		if len(points) < minPoints {
			// SVG-injected charts sample ~40 points across the full timeline.
			if lastOffset < (durationSeconds*85)/100 {
				return false
			}
		}
	}
	return true
}

func lastViewerOffsetSeconds(points []parsedViewerPoint) int {
	lastOffset := 0
	for _, pt := range points {
		if pt.OffsetSeconds > lastOffset {
			lastOffset = pt.OffsetSeconds
		}
	}
	return lastOffset
}

func buildGameSegments(games []scrapedGame, totalDurationSeconds int) []GameSegment {
	if len(games) == 0 {
		return nil
	}
	if totalDurationSeconds <= 0 {
		totalDurationSeconds = len(games) * 3600
	}

	knownDuration := 0
	unknown := 0
	for _, g := range games {
		if g.DurationMinutes > 0 {
			knownDuration += g.DurationMinutes * 60
		} else {
			unknown++
		}
	}
	remaining := totalDurationSeconds - knownDuration
	if remaining < 0 {
		remaining = 0
	}
	fallbackEach := 0
	if unknown > 0 {
		fallbackEach = remaining / unknown
	}
	if fallbackEach <= 0 {
		fallbackEach = totalDurationSeconds / len(games)
	}

	segments := make([]GameSegment, 0, len(games))
	offset := 0
	for _, g := range games {
		dur := g.DurationMinutes * 60
		if dur <= 0 {
			dur = fallbackEach
		}
		segments = append(segments, GameSegment{
			GameName:        g.Title,
			BoxArtURL:       g.BoxArt,
			OffsetSeconds:   offset,
			DurationSeconds: dur,
		})
		offset += dur
	}
	return segments
}

func looksLikeCloudflareChallenge(body string) bool {
	lower := strings.ToLower(body)
	return strings.Contains(lower, "just a moment") ||
		strings.Contains(lower, "performing security verification") ||
		strings.Contains(lower, "cf_chl_opt")
}

func (s *SyncService) scrapeTwitchTrackerDirect(ctx context.Context, pageURL string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, pageURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "text/html,application/xhtml+xml")
	if s.userAgent != "" {
		req.Header.Set("User-Agent", s.userAgent)
	}
	client := s.client
	if s.ttDirectHTTPTimeoutMS > 0 {
		client = &http.Client{Timeout: time.Duration(s.ttDirectHTTPTimeoutMS) * time.Millisecond}
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 2*1024*1024))
	if err != nil {
		return "", err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("status %d", resp.StatusCode)
	}
	htmlBody := string(body)
	if looksLikeCloudflareChallenge(htmlBody) {
		return "", fmt.Errorf("cloudflare challenge page")
	}
	if !strings.Contains(htmlBody, `id="ecs"`) && !strings.Contains(strings.ToLower(htmlBody), "stream-timestamp-dt") {
		return "", fmt.Errorf("page did not contain TwitchTracker stream data")
	}
	return htmlBody, nil
}

func formatScraperConnectError(err error, scraperURL string) string {
	msg := err.Error()
	lower := strings.ToLower(msg)
	if strings.Contains(lower, "connection refused") || strings.Contains(lower, "no such host") || strings.Contains(lower, "actively refused") {
		return fmt.Sprintf(
			"%s — scraper service unreachable at %s. Ensure streamclone-scraper is running (docker compose ps scraper), on the same compose network, and SCRAPER_API_URL=http://scraper:8000/v2/scrape inside containers",
			msg,
			scraperURL,
		)
	}
	return msg
}

func (s *SyncService) scrapeTwitchTracker(ctx context.Context, url string, stream *StreamRecord, viewersOnly bool) (string, error) {
	tryDirect := s.shouldTryDirectHTTP(stream) && s.directHTTP.allowed()
	if tryDirect {
		htmlBody, directErr := s.scrapeTwitchTrackerDirect(ctx, url)
		if directErr == nil {
			s.directHTTP.record(true)
			s.log.Info("fetched TwitchTracker page via direct HTTP", "url", url)
			return htmlBody, nil
		}
		s.directHTTP.record(false)
		s.log.Info("direct TwitchTracker fetch unavailable, trying browser scraper", "url", url, "err", directErr)
	} else if s.ttDirectHTTPEnabled && s.ttDirectHTTPStaleOnly && !s.shouldTryDirectHTTP(stream) {
		s.log.Debug("direct TwitchTracker fetch skipped for recent stream", "url", url)
	} else if s.ttDirectHTTPEnabled && !s.directHTTP.allowed() {
		s.log.Debug("direct TwitchTracker fetch temporarily disabled due to low success rate", "url", url)
	}

	if s.scraperKey == "" {
		return "", fmt.Errorf("missing SCRAPER_API_KEY — TwitchTracker blocks direct scraping; set SCRAPER_API_KEY=local-dev-key in .env for the local scraper")
	}

	maxAge := s.trackerScrapeMaxAgeMS(stream, viewersOnly)
	reqBody, err := json.Marshal(map[string]any{
		"url":             url,
		"formats":         []string{"rawHtml"},
		"onlyMainContent": false,
		"useProxy":        false, // datacenter proxies are Cloudflare-blocked on TwitchTracker
		"timeout":         s.trackerScrapeTimeoutMS,
		"maxAge":          maxAge,
	})
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.scraperURL, bytes.NewReader(reqBody))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+s.scraperKey)

	resp, err := s.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to scrape TwitchTracker: %s", formatScraperConnectError(err, s.scraperURL))
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("scraper API returned status %d: %s", resp.StatusCode, string(body))
	}

	var fcResp struct {
		Success bool `json:"success"`
		Data    struct {
			HTML    string `json:"html"`
			RawHTML string `json:"rawHtml"`
			Validation struct {
				CloudflareState string `json:"cloudflareState"`
			} `json:"validation"`
			Protection struct {
				State string `json:"state"`
			} `json:"protection"`
		} `json:"data"`
		Error string `json:"error"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&fcResp); err != nil {
		return "", err
	}

	if !fcResp.Success {
		if strings.EqualFold(fcResp.Error, "access_protected") {
			state := fcResp.Data.Protection.State
			if state == "" {
				state = fcResp.Data.Validation.CloudflareState
			}
			if state == "" {
				state = "unknown_protection"
			}
			return "", fmt.Errorf("%w: %s", errTrackerAccessProtected, state)
		}
		return "", fmt.Errorf("scraper scrape failed: %s", fcResp.Error)
	}

	htmlBody := fcResp.Data.RawHTML
	if htmlBody == "" {
		htmlBody = fcResp.Data.HTML
	}
	if htmlBody == "" {
		return "", fmt.Errorf("scraper scrape returned empty html")
	}
	return htmlBody, nil
}

type ttTitleRaw struct {
	CreatedAt  string `json:"created_at"`
	Title      string `json:"title"`
	RelativeAt int    `json:"relative_at"`
}

type ttGameRaw struct {
	Date string `json:"date"`
	ID   any    `json:"id"`
	Name string `json:"name"`
}

func decodeBase64(s string) ([]byte, error) {
	s = strings.ReplaceAll(s, "#", "W")
	s = strings.TrimSpace(s)
	if len(s)%4 != 0 {
		s += strings.Repeat("=", 4-(len(s)%4))
	}
	return base64.StdEncoding.DecodeString(s)
}

func parseTime(tStr string) (time.Time, error) {
	tStr = strings.TrimSpace(tStr)
	if tStr == "" {
		return time.Time{}, errors.New("empty time string")
	}
	if t, err := time.Parse(time.RFC3339, tStr); err == nil {
		return t, nil
	}
	formats := []string{
		"2006-01-02 15:04:05",
		"2006-01-02T15:04:05",
		"2006-01-02T15:04:05.000000Z",
		"2006-01-02T15:04:05Z",
	}
	for _, f := range formats {
		if t, err := time.Parse(f, tStr); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("could not parse time: %s", tStr)
}

func (s *SyncService) parseTwitchTrackerMetadata(html string, startedAt time.Time) (trackerStreamData, error) {
	var out trackerStreamData
	var content string
	if match := regexp.MustCompile(`(?i)<meta\s+id="ecs"\s+content="([^"]+)"`).FindStringSubmatch(html); len(match) > 1 {
		content = match[1]
	} else if match := regexp.MustCompile(`(?i)<meta\s+content="([^"]+)"\s+id="ecs"`).FindStringSubmatch(html); len(match) > 1 {
		content = match[1]
	}

	if content == "" {
		return out, errors.New("meta#ecs content not found in HTML")
	}

	parts := strings.Split(content, "!")
	if len(parts) < 2 {
		return out, errors.New("invalid meta#ecs content format")
	}

	keysB64 := parts[len(parts)-1]
	keysBytes, err := decodeBase64(keysB64)
	if err != nil {
		return out, fmt.Errorf("failed to decode keys: %w", err)
	}

	var keys []string
	if err := json.Unmarshal(keysBytes, &keys); err != nil {
		return out, fmt.Errorf("failed to unmarshal keys: %w", err)
	}

	keyToIndex := make(map[string]int)
	for idx, key := range keys {
		keyToIndex[key] = idx
	}

	getPartBytes := func(key string) ([]byte, bool) {
		idx, ok := keyToIndex[key]
		if !ok || idx >= len(parts)-1 {
			return nil, false
		}
		decoded, err := decodeBase64(parts[idx])
		if err != nil {
			return nil, false
		}
		return decoded, true
	}

	var maxViewers int
	if b, ok := getPartBytes("max_viewers"); ok {
		_ = json.Unmarshal(b, &maxViewers)
	}

	var avgViewers int
	if b, ok := getPartBytes("avg_viewers"); ok {
		_ = json.Unmarshal(b, &avgViewers)
	}

	var createdAtStr string
	if b, ok := getPartBytes("created_at"); ok {
		_ = json.Unmarshal(b, &createdAtStr)
	}
	var finishedAtStr string
	if b, ok := getPartBytes("finished_at"); ok {
		_ = json.Unmarshal(b, &finishedAtStr)
	}

	createdAt, err := parseTime(createdAtStr)
	if err != nil {
		createdAt = startedAt
	}
	finishedAt, _ := parseTime(finishedAtStr)

	var durationMinutes int
	if !finishedAt.IsZero() && !createdAt.IsZero() {
		durationMinutes = int(finishedAt.Sub(createdAt).Minutes())
	}

	var chartPoints [][]any
	if b, ok := getPartBytes("chart"); ok {
		_ = json.Unmarshal(b, &chartPoints)
	}

	var viewerPoints []parsedViewerPoint
	for _, pt := range chartPoints {
		if len(pt) < 2 {
			continue
		}
		ptTimeStr, ok0 := pt[0].(string)
		if !ok0 {
			continue
		}
		ptTime, err := parseTime(ptTimeStr)
		if err != nil {
			continue
		}

		viewersVal := coerceChartInt(pt[1])
		if viewersVal == 0 && len(pt) > 2 {
			viewersVal = coerceChartInt(pt[2])
		}

		offsetSeconds := int(ptTime.Sub(createdAt).Seconds())
		viewerPoints = append(viewerPoints, parsedViewerPoint{
			OffsetSeconds: offsetSeconds,
			Viewers:       viewersVal,
		})
	}

	if durationMinutes <= 0 && len(viewerPoints) > 0 {
		durationMinutes = viewerPoints[len(viewerPoints)-1].OffsetSeconds / 60
	}

	var rawGames []ttGameRaw
	if b, ok := getPartBytes("games"); ok {
		_ = json.Unmarshal(b, &rawGames)
	}

	type gameWithTime struct {
		game      ttGameRaw
		startTime time.Time
	}

	var gamesWithTimes []gameWithTime
	for _, rg := range rawGames {
		gTime, err := parseTime(rg.Date)
		if err != nil {
			continue
		}
		gamesWithTimes = append(gamesWithTimes, gameWithTime{
			game:      rg,
			startTime: gTime,
		})
	}

	sort.Slice(gamesWithTimes, func(i, j int) bool {
		return gamesWithTimes[i].startTime.Before(gamesWithTimes[j].startTime)
	})

	var games []scrapedGame
	for i, gwt := range gamesWithTimes {
		var gameDurationMinutes int
		if i < len(gamesWithTimes)-1 {
			gameDurationMinutes = int(gamesWithTimes[i+1].startTime.Sub(gwt.startTime).Minutes())
		} else {
			var streamEndTime time.Time
			if !finishedAt.IsZero() {
				streamEndTime = finishedAt
			} else if len(viewerPoints) > 0 {
				streamEndTime = createdAt.Add(time.Duration(viewerPoints[len(viewerPoints)-1].OffsetSeconds) * time.Second)
			} else {
				streamEndTime = createdAt.Add(time.Duration(durationMinutes) * time.Minute)
			}
			gameDurationMinutes = int(streamEndTime.Sub(gwt.startTime).Minutes())
		}

		if gameDurationMinutes < 0 {
			gameDurationMinutes = 0
		}

		boxArt := ""
		if gwt.game.ID != nil {
			var gameID int64
			switch v := gwt.game.ID.(type) {
			case float64:
				gameID = int64(v)
			case string:
				gameID, _ = strconv.ParseInt(v, 10, 64)
			}
			if gameID > 0 {
				boxArt = fmt.Sprintf("https://static-cdn.jtvnw.net/ttv-boxart/%d-210x280.jpg", gameID)
			}
		}

		games = append(games, scrapedGame{
			Title:           gwt.game.Name,
			BoxArt:          boxArt,
			DurationMinutes: gameDurationMinutes,
		})
	}

	peakViewers := maxViewers
	if peakViewers <= 0 {
		peakViewers = avgViewers
	}
	for _, pt := range viewerPoints {
		if pt.Viewers > peakViewers {
			peakViewers = pt.Viewers
		}
	}

	if chartMaxViewers(viewerPoints) == 0 && (avgViewers > 0 || peakViewers > 0) {
		// Do not synthesize a flat peak-only line — that looks like a real chart but is misleading.
		s.log.Warn("meta#ecs parsed without chart points; viewer timeline unavailable",
			"peak", peakViewers,
			"avg", avgViewers,
			"duration_minutes", durationMinutes,
		)
	}

	out = trackerStreamData{
		DurationMinutes: durationMinutes,
		PeakViewers:     peakViewers,
		AvgViewers:      avgViewers,
		ChartStartedAt:  createdAt,
		Games:           games,
		ViewerPoints:    viewerPoints,
	}
	return out, nil
}

func synthesizeViewerPoints(durationMinutes, peakViewers, avgViewers int) []parsedViewerPoint {
	if durationMinutes <= 0 {
		durationMinutes = 60
	}
	baseline := avgViewers
	if baseline <= 0 {
		baseline = peakViewers
	}
	if baseline <= 0 {
		return nil
	}
	endSeconds := durationMinutes * 60
	if endSeconds <= 0 {
		endSeconds = 60
	}
	return []parsedViewerPoint{
		{OffsetSeconds: 0, Viewers: baseline},
		{OffsetSeconds: endSeconds, Viewers: baseline},
	}
}

func parseStreamcloneViewerChartJSON(html string, durationMinutes, peakViewers int) []parsedViewerPoint {
	re := regexp.MustCompile(`(?is)<script[^>]+id="streamclone-viewer-chart"[^>]*>(.*?)</script>`)
	match := re.FindStringSubmatch(html)
	if len(match) < 2 {
		return nil
	}
	var raw []struct {
		X float64 `json:"x"`
		Y float64 `json:"y"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(match[1])), &raw); err != nil || len(raw) < 3 {
		return nil
	}
	minX, maxX := raw[0].X, raw[0].X
	minY, maxY := raw[0].Y, raw[0].Y
	for _, p := range raw[1:] {
		if p.X < minX {
			minX = p.X
		}
		if p.X > maxX {
			maxX = p.X
		}
		if p.Y < minY {
			minY = p.Y
		}
		if p.Y > maxY {
			maxY = p.Y
		}
	}
	durationSeconds := durationMinutes * 60
	if durationSeconds <= 0 {
		durationSeconds = len(raw) * 60
	}
	svgPixelMode := maxX-minX > 0 && maxX < 5000 && maxY-minY > 1 && maxY < 5000
	points := make([]parsedViewerPoint, 0, len(raw))
	for i, p := range raw {
		var offsetSeconds int
		if p.X > 1_000_000_000_000 {
			offsetSeconds = int((p.X - minX) / 1000)
		} else if svgPixelMode && maxX > minX {
			offsetSeconds = int((p.X - minX) / (maxX - minX) * float64(durationSeconds))
		} else {
			offsetSeconds = int((float64(i) / float64(max(len(raw)-1, 1))) * float64(durationSeconds))
		}
		viewers := int(p.Y)
		if svgPixelMode && peakViewers > 0 && maxY > minY {
			pct := (maxY - p.Y) / (maxY - minY)
			viewers = int(pct * float64(peakViewers))
		}
		if viewers <= 0 {
			continue
		}
		points = append(points, parsedViewerPoint{
			OffsetSeconds: offsetSeconds,
			Viewers:       viewers,
		})
	}
	return points
}

func parseTrackerDurationMinutesFromHTML(html string) int {
	reHM := regexp.MustCompile(`(?is)g-x-s-value">\s*(\d+)\s*<small>h</small>\s*(\d+)\s*<small>m</small>\s*</div>\s*<div class="g-x-s-label[^"]*">Stream duration</div>`)
	if m := reHM.FindStringSubmatch(html); len(m) > 2 {
		hours, _ := strconv.Atoi(m[1])
		mins, _ := strconv.Atoi(m[2])
		if total := hours*60 + mins; total > 0 {
			return total
		}
	}
	reDuration := regexp.MustCompile(`(?i)to-time-lg">(\d+)</div>\s*<div class="g-x-s-label[^>]*>Stream duration</div>`)
	if durMatch := reDuration.FindStringSubmatch(html); len(durMatch) > 1 {
		if v, _ := strconv.Atoi(durMatch[1]); v > 0 {
			return v
		}
	}
	reDurationNew := regexp.MustCompile(`(?i)g-x-s-value">\s*([\d,]+)\s*</div>\s*<div class="g-x-s-label[^"]*">Stream duration</div>`)
	if durMatch := reDurationNew.FindStringSubmatch(html); len(durMatch) > 1 {
		if v, _ := strconv.Atoi(strings.ReplaceAll(durMatch[1], ",", "")); v > 0 {
			return v
		}
	}
	return 0
}

func peakViewersFromHTML(html string) int {
	rePeakNew := regexp.MustCompile(`(?i)g-x-s-value">\s*([\d,]+)\s*</div>\s*<div class="g-x-s-label[^"]*">Peak viewers</div>`)
	if peakMatch := rePeakNew.FindStringSubmatch(html); len(peakMatch) > 1 {
		v, _ := strconv.Atoi(strings.ReplaceAll(peakMatch[1], ",", ""))
		return v
	}
	rePeak := regexp.MustCompile(`(?i)to-number">(\d+)</div>\s*<div class="g-x-s-label[^>]*>Peak viewers</div>`)
	if peakMatch := rePeak.FindStringSubmatch(html); len(peakMatch) > 1 {
		v, _ := strconv.Atoi(peakMatch[1])
		return v
	}
	return 0
}

func (s *SyncService) parseTwitchTrackerHTML(html string, startedAt time.Time) (trackerStreamData, error) {
	var out trackerStreamData
	// 1. Try to parse meta#ecs metadata
	{
		if parsed, err := s.parseTwitchTrackerMetadata(html, startedAt); err == nil && (len(parsed.ViewerPoints) > 0 || parsed.DurationMinutes > 0 || parsed.PeakViewers > 0) {
			durSec := parsed.DurationMinutes * 60
			if durSec <= 0 && len(parsed.ViewerPoints) > 0 {
				durSec = lastViewerOffsetSeconds(parsed.ViewerPoints)
			}
			if len(parsed.ViewerPoints) >= 3 && hasCompleteViewerChart(parsed.ViewerPoints, durSec) {
				s.log.Info("successfully parsed stream data from meta#ecs block",
					"duration_minutes", parsed.DurationMinutes,
					"peak_viewers", parsed.PeakViewers,
					"avg_viewers", parsed.AvgViewers,
					"games_count", len(parsed.Games),
					"points_count", len(parsed.ViewerPoints),
				)
				return parsed, nil
			}
			s.log.Warn("meta#ecs present but viewer chart incomplete; falling back to HTML/injected chart",
				"duration_minutes", parsed.DurationMinutes,
				"point_count", len(parsed.ViewerPoints),
				"last_offset_sec", lastViewerOffsetSeconds(parsed.ViewerPoints),
			)
		} else if err != nil {
			s.log.Warn("failed to parse meta#ecs block, falling back to HTML scraping", "err", err)
		}
	}

	// 1. Parse Duration (h/m blocks, legacy to-time-lg, newer g-x-s-value)
	durationMinutes := parseTrackerDurationMinutesFromHTML(html)

	// 2. Parse Peak Viewers (legacy to-number and newer g-x-s-value blocks)
	peakViewers := 0
	rePeak := regexp.MustCompile(`(?i)to-number">(\d+)</div>\s*<div class="g-x-s-label[^>]*>Peak viewers</div>`)
	if peakMatch := rePeak.FindStringSubmatch(html); len(peakMatch) > 1 {
		peakViewers, _ = strconv.Atoi(peakMatch[1])
	}
	if peakViewers == 0 {
		rePeakNew := regexp.MustCompile(`(?i)g-x-s-value">\s*([\d,]+)\s*</div>\s*<div class="g-x-s-label[^"]*">Peak viewers</div>`)
		if peakMatch := rePeakNew.FindStringSubmatch(html); len(peakMatch) > 1 {
			peakViewers, _ = strconv.Atoi(strings.ReplaceAll(peakMatch[1], ",", ""))
		}
	}

	// 3. Parse Played Games
	var games []scrapedGame
	gamesSectionIdx := strings.Index(html, `id="stream-games"`)
	if gamesSectionIdx != -1 {
		sectionContent := html[gamesSectionIdx:]
		// Cut off after next section to avoid overflow
		if endIdx := strings.Index(sectionContent, `</section>`); endIdx != -1 {
			sectionContent = sectionContent[:endIdx]
		}

		gameBlocks := strings.Split(sectionContent, `class="g-x-wrapper"`)
		if len(gameBlocks) > 1 {
			for _, block := range gameBlocks[1:] {
				reTitle := regexp.MustCompile(`(?i)class="g-x-s-title"[^>]*>([^<]+)</a>`)
				titleMatch := reTitle.FindStringSubmatch(block)
				title := ""
				if len(titleMatch) > 1 {
					title = strings.TrimSpace(titleMatch[1])
				} else {
					reAlt := regexp.MustCompile(`(?i)alt="([^"]+)"`)
					altMatch := reAlt.FindStringSubmatch(block)
					if len(altMatch) > 1 {
						title = strings.TrimSpace(altMatch[1])
					}
				}

				reBox := regexp.MustCompile(`(?i)src="([^"]+)"`)
				boxMatch := reBox.FindStringSubmatch(block)
				boxArt := ""
				if len(boxMatch) > 1 {
					boxArt = boxMatch[1]
				}

				reGameDur := regexp.MustCompile(`(?i)to-time-lg">(\d+)</div>`)
				gameDurMatch := reGameDur.FindStringSubmatch(block)
				gameDur := 0
				if len(gameDurMatch) > 1 {
					gameDur, _ = strconv.Atoi(gameDurMatch[1])
				}

				if title != "" {
					games = append(games, scrapedGame{
						Title:           title,
						BoxArt:          boxArt,
						DurationMinutes: gameDur,
					})
				}
			}
		}
	}

	if injected := parseStreamcloneViewerChartJSON(html, durationMinutes, peakViewers); len(injected) >= 3 && hasCompleteViewerChart(injected, durationMinutes*60) {
		s.log.Info("parsed viewer chart from injected SVG sample JSON", "points", len(injected))
		out.ViewerPoints = injected
	}

	// 4. Parse SVG Path coordinates
	var viewerPoints []parsedViewerPoint
	svgStart := strings.Index(html, "<svg")
	svgEnd := strings.Index(html, "</svg>")
	if svgStart != -1 && svgEnd != -1 && svgEnd > svgStart {
		svg := html[svgStart : svgEnd+6]
		viewerPath := findViewerPath(svg)
		if viewerPath != "" {
			viewerPoints = parsePathPoints(viewerPath, durationMinutes*60, peakViewers)
		}
	}

	if chartMaxViewers(viewerPoints) == 0 && peakViewers > 0 && durationMinutes > 0 {
		s.log.Warn("html fallback found peak/duration but no chart path; skipping synthesized viewer line",
			"peak", peakViewers,
			"duration_minutes", durationMinutes,
		)
	}

	if len(out.ViewerPoints) == 0 {
		out.ViewerPoints = viewerPoints
	}
	out.DurationMinutes = durationMinutes
	out.PeakViewers = peakViewers
	out.ChartStartedAt = startedAt
	out.Games = games
	return out, nil
}

func findViewerPath(svg string) string {
	rePath := regexp.MustCompile(`(?i)<path\b([^>]*?)d="([^"]+)"([^>]*?)>`)
	matches := rePath.FindAllStringSubmatch(svg, -1)
	for _, m := range matches {
		attrs := m[1] + " " + m[3]
		if strings.Contains(attrs, `fill="none"`) &&
			!strings.Contains(attrs, `highcharts-grid-line`) &&
			!strings.Contains(attrs, `highcharts-plot-line`) &&
			!strings.Contains(attrs, `highcharts-axis-line`) {

			d := m[2]
			if len(strings.Split(d, " ")) > 20 {
				return d
			}
		}
	}
	return ""
}

func parsePathPoints(path string, durationSeconds int, peakViewers int) []parsedViewerPoint {
	tokens := strings.Fields(path)
	var rawPoints []struct{ x, y float64 }

	i := 0
	for i < len(tokens) {
		cmd := tokens[i]
		if cmd == "M" || cmd == "L" {
			if i+2 < len(tokens) {
				x, _ := strconv.ParseFloat(tokens[i+1], 64)
				y, _ := strconv.ParseFloat(tokens[i+2], 64)
				rawPoints = append(rawPoints, struct{ x, y float64 }{x, y})
			}
			i += 3
		} else if cmd == "C" {
			if i+6 < len(tokens) {
				x3, _ := strconv.ParseFloat(tokens[i+5], 64)
				y3, _ := strconv.ParseFloat(tokens[i+6], 64)
				rawPoints = append(rawPoints, struct{ x, y float64 }{x3, y3})
			}
			i += 7
		} else {
			i++
		}
	}

	if len(rawPoints) < 2 {
		return nil
	}

	// Find limits
	xMin := rawPoints[0].x
	xMax := rawPoints[0].x
	yMin := rawPoints[0].y
	yMax := rawPoints[0].y

	for _, p := range rawPoints {
		if p.x < xMin {
			xMin = p.x
		}
		if p.x > xMax {
			xMax = p.x
		}
		if p.y < yMin {
			yMin = p.y
		}
		if p.y > yMax {
			yMax = p.y
		}
	}

	// Reconstruct actual coordinates
	var points []parsedViewerPoint
	for _, p := range rawPoints {
		var offsetPct float64
		if xMax != xMin {
			offsetPct = (p.x - xMin) / (xMax - xMin)
		}
		offsetSeconds := int(offsetPct * float64(durationSeconds))

		var viewerPct float64
		if yMax != yMin {
			viewerPct = (yMax - p.y) / (yMax - yMin)
		}
		viewers := int(viewerPct * float64(peakViewers))

		points = append(points, parsedViewerPoint{
			OffsetSeconds: offsetSeconds,
			Viewers:       viewers,
		})
	}

	return points
}

func (s *SyncService) interpolateViewerCount(minute int, points []parsedViewerPoint) int {
	return InterpolateViewerCount(minute, toRollupViewerPoints(points))
}

func (s *SyncService) buildViewerMinuteRollups(rollupStart time.Time, durationSeconds int, viewerPoints []parsedViewerPoint) []MinuteRollup {
	totalMinutes := durationSeconds / 60
	if totalMinutes <= 0 {
		totalMinutes = 1
	}
	viewerLookup := make(map[int]int, len(viewerPoints))
	for _, pt := range viewerPoints {
		viewerLookup[pt.OffsetSeconds/60] = pt.Viewers
	}
	rollups := make([]MinuteRollup, 0, totalMinutes+1)
	for m := 0; m <= totalMinutes; m++ {
		viewerVal := 0
		if val, ok := viewerLookup[m]; ok {
			viewerVal = val
		} else {
			viewerVal = s.interpolateViewerCount(m, viewerPoints)
		}
		if viewerVal <= 0 {
			continue
		}
		rollups = append(rollups, MinuteRollup{
			MinuteTS:      rollupStart.Add(time.Duration(m) * time.Minute),
			ViewerAvg:     viewerVal,
			ViewerMax:     viewerVal,
			ViewerLatest:  viewerVal,
			ViewerSamples: 1,
		})
	}
	return rollups
}

func (s *SyncService) persistEarlyViewerChart(
	ctx context.Context,
	streamID string,
	rollupStart time.Time,
	durationSeconds int,
	peakViewers int,
	avgViewers int,
	viewerPoints []parsedViewerPoint,
	gameSegments []scrapedGame,
) {
	rollups := s.buildViewerMinuteRollups(rollupStart, durationSeconds, viewerPoints)
	if len(rollups) == 0 {
		return
	}
	writeStart := time.Now()
	if err := s.store.BulkPatchViewerRollups(ctx, streamID, rollups); err != nil {
		s.log.Warn("early viewer rollup write failed", "stream_id", streamID, "err", err)
		return
	}
	segments := buildGameSegments(gameSegments, durationSeconds)
	for i := range segments {
		segments[i].StreamID = streamID
	}
	if len(segments) > 0 {
		if err := s.store.SaveGameSegments(ctx, streamID, segments); err != nil {
			s.log.Warn("early game segment write failed", "stream_id", streamID, "err", err)
		}
	}
	endTS := rollupStart.Add(time.Duration(durationSeconds) * time.Second)
	if !endTS.After(rollupStart) {
		endTS = rollupStart.Add(time.Minute)
	}
	if _, err := s.store.db.Exec(ctx, `
		UPDATE analytics_streams
		SET ended_at = $2,
		    peak_viewers = GREATEST(peak_viewers, $3::int),
		    avg_viewers = CASE WHEN $4::int > 0 THEN GREATEST(avg_viewers, $4::int) ELSE avg_viewers END,
		    updated_at = now()
		WHERE stream_id = $1
	`, streamID, endTS, peakViewers, avgViewers); err != nil {
		s.log.Warn("early stream metadata update failed", "stream_id", streamID, "err", err)
	}
	s.log.Info("sync phase complete",
		"stream_id", streamID,
		"phase", "early_viewer_write",
		"duration_ms", time.Since(writeStart).Milliseconds(),
		"rollup_minutes", len(rollups),
	)
	s.setSyncPhase(ctx, streamID, SyncPhaseParsingTracker, "Viewer chart saved — chat indexing continues", func(st *SyncStatus) {
		st.ViewerStatus = "ok"
		st.RollupsWritten = len(rollups)
		if st.Tracker == nil {
			st.Tracker = &SyncTrackerProgress{}
		}
		st.Tracker.Active = false
		st.Tracker.Message = "Viewer chart available in UI"
	})
}

type emoteEnsureResponse struct {
	State     string `json:"state"`
	Count     int    `json:"count"`
	Pending   int    `json:"pending"`
	Providers []struct {
		Provider string `json:"provider"`
		State    string `json:"state"`
		Count    int    `json:"count"`
		Error    string `json:"error"`
	} `json:"providers"`
}

func (s *SyncService) preloadChannelEmotes(ctx context.Context, login, broadcasterID string) {
	if s.emoteURL == "" || login == "" || broadcasterID == "" {
		return
	}
	body, err := json.Marshal(map[string]any{
		"twitch_id": broadcasterID,
		"providers": []string{"seventv", "twitch", "ffz"},
	})
	if err != nil {
		return
	}
	url := fmt.Sprintf("%s/v1/channels/%s/emotes/ensure", s.emoteURL, login)
	deadline := time.Now().Add(2 * time.Minute)
	var last emoteEnsureResponse
	for attempt := 0; attempt < 60 && time.Now().Before(deadline); attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return
			case <-time.After(2 * time.Second):
			}
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
		if err != nil {
			return
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err := s.client.Do(req)
		if err != nil {
			s.log.Warn("emote ensure request failed", "login", login, "err", err)
			return
		}
		respBody, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		if readErr != nil {
			s.log.Warn("emote ensure read failed", "login", login, "err", readErr)
			return
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			s.log.Warn("emote ensure returned non-success status", "login", login, "status", resp.StatusCode)
			return
		}
		last = emoteEnsureResponse{}
		_ = json.Unmarshal(respBody, &last)
		if last.State == "ready" && last.Count > 0 {
			s.enricher.Invalidate(login)
			s.log.Info("preloaded channel emotes for historical sync", "login", login, "count", last.Count)
			return
		}
		if last.State == "failed" || (last.State == "ready" && last.Count == 0 && last.Pending == 0) {
			break
		}
	}
	var providerErrors []string
	for _, p := range last.Providers {
		if p.Error != "" {
			providerErrors = append(providerErrors, p.Provider+": "+p.Error)
		}
	}
	s.log.Warn("emote dictionary not ready before chat tokenize",
		"login", login,
		"state", last.State,
		"count", last.Count,
		"pending", last.Pending,
		"provider_errors", providerErrors,
	)
}

func (s *SyncService) extractVodID(html string) string {
	// Search in litebox player video parameters
	reVideo := regexp.MustCompile(`video=(\d+)`)
	videoMatch := reVideo.FindStringSubmatch(html)
	if len(videoMatch) > 1 {
		return videoMatch[1]
	}

	// Search in /videos/ID links
	reVodLink := regexp.MustCompile(`/videos/(\d+)`)
	vodMatch := reVodLink.FindStringSubmatch(html)
	if len(vodMatch) > 1 {
		return vodMatch[1]
	}

	return ""
}

func gqlCommentText(msg struct {
	Body      string `json:"body"`
	Fragments []struct {
		Text string `json:"text"`
	} `json:"fragments"`
}) string {
	if text := strings.TrimSpace(msg.Body); text != "" {
		return msg.Body
	}
	var b strings.Builder
	for _, frag := range msg.Fragments {
		b.WriteString(frag.Text)
	}
	return b.String()
}

func buildVideoCommentsGQLRequest(videoID, sha256Hash string, useCursor bool, offset int, cursor string) GQLRequest {
	var req GQLRequest
	req.OperationName = "VideoCommentsByOffsetOrCursor"
	req.Extensions.PersistedQuery.Version = 1
	req.Extensions.PersistedQuery.SHA256Hash = sha256Hash
	req.Variables.VideoID = videoID
	if useCursor && cursor != "" {
		c := cursor
		req.Variables.Cursor = &c
	} else {
		o := offset
		req.Variables.ContentOffsetSeconds = &o
	}
	return req
}

func isGQLIntegrityError(resp GQLResponse) bool {
	for _, e := range resp.Errors {
		msg := strings.ToLower(e.Message)
		if strings.Contains(msg, "integrity") {
			return true
		}
	}
	return false
}

func (s *SyncService) trackerDataFromDB(ctx context.Context, stream *StreamRecord) (trackerStreamData, error) {
	if stream == nil {
		return trackerStreamData{}, fmt.Errorf("stream record missing")
	}
	rollups, err := s.store.RollupsByStream(ctx, stream.StreamID)
	if err != nil {
		return trackerStreamData{}, fmt.Errorf("load viewer rollups from DB: %w", err)
	}

	var points []parsedViewerPoint
	var chartStart time.Time
	for _, rollup := range rollups {
		if rollup.ViewerSamples == 0 && rollup.ViewerAvg == 0 && rollup.ViewerMax == 0 && rollup.ViewerLatest == 0 {
			continue
		}
		if chartStart.IsZero() {
			chartStart = rollup.MinuteTS.UTC()
		}
		offsetSec := int(rollup.MinuteTS.UTC().Sub(chartStart).Seconds())
		val := rollup.ViewerAvg
		if val == 0 {
			val = rollup.ViewerLatest
		}
		if val == 0 {
			val = rollup.ViewerMax
		}
		points = append(points, parsedViewerPoint{OffsetSeconds: offsetSec, Viewers: val})
	}

	durationMinutes := 0
	if stream.EndedAt != nil && !stream.StartedAt.IsZero() {
		durationMinutes = int(stream.EndedAt.Sub(stream.StartedAt).Minutes())
	}
	if durationMinutes <= 0 && len(rollups) > 0 {
		first := rollups[0].MinuteTS
		last := rollups[len(rollups)-1].MinuteTS
		durationMinutes = int(last.Sub(first).Minutes()) + 1
	}
	if chartStart.IsZero() && !stream.StartedAt.IsZero() {
		chartStart = stream.StartedAt.UTC()
	}

	return trackerStreamData{
		DurationMinutes: durationMinutes,
		PeakViewers:     stream.PeakViewers,
		AvgViewers:      stream.AvgViewers,
		ChartStartedAt:  chartStart,
		ViewerPoints:    points,
	}, nil
}

const syncChatRollupTopEmotes = 50

func buildChatMinuteRollup(login string, enricher *enrich.Enricher, comments []string) (chatCount, totalEmoteCount, seventvEmoteCount int, emotes map[string]int) {
	emotes = make(map[string]int)
	chatCount = len(comments)
	for _, comment := range comments {
		fragments := enricher.Tokenize(login, comment, nil)
		for _, frag := range fragments {
			if frag.T != "emote" {
				continue
			}
			totalEmoteCount++
			if frag.Provider == "seventv" {
				seventvEmoteCount++
			}
			key := fmt.Sprintf("%s:%s:%s", frag.Provider, frag.ID, frag.C)
			emotes[key]++
		}
	}
	emotes = topNMap(emotes, syncChatRollupTopEmotes)
	return chatCount, totalEmoteCount, seventvEmoteCount, emotes
}

type chatRollupCache struct {
	mu       sync.RWMutex
	byMinute map[int]MinuteRollup
}

func newChatRollupCache() *chatRollupCache {
	return &chatRollupCache{byMinute: make(map[int]MinuteRollup)}
}

func (c *chatRollupCache) store(rollupStart time.Time, rollups []MinuteRollup) {
	if c == nil || rollupStart.IsZero() {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, rollup := range rollups {
		offset := int(rollup.MinuteTS.Sub(rollupStart).Minutes())
		c.byMinute[offset] = rollup
	}
}

func (c *chatRollupCache) get(minute int) (MinuteRollup, bool) {
	if c == nil {
		return MinuteRollup{}, false
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	rollup, ok := c.byMinute[minute]
	return rollup, ok
}

func (c *chatRollupCache) has(minute int) bool {
	if c == nil {
		return false
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	_, ok := c.byMinute[minute]
	return ok
}

func (s *SyncService) patchChatRollupsForSegment(
	ctx context.Context,
	streamID, login string,
	rollupStartFn func() time.Time,
	commentsMap map[int][]string,
	seg gqlSegmentProgress,
	chatAlignSec int,
	cache *chatRollupCache,
) error {
	if rollupStartFn == nil || login == "" {
		return nil
	}
	rollupStart := rollupStartFn()
	if rollupStart.IsZero() {
		return nil
	}
	startMinute, endMinute := segmentAlignedMinuteBounds(seg, chatAlignSec)
	rollups := make([]MinuteRollup, 0, endMinute-startMinute+1)
	for minute := startMinute; minute <= endMinute; minute++ {
		comments, ok := commentsMap[minute]
		if !ok || len(comments) == 0 {
			continue
		}
		chatCount, totalEmoteCount, seventvEmoteCount, emotes := buildChatMinuteRollup(login, s.enricher, comments)
		rollups = append(rollups, MinuteRollup{
			MinuteTS:          rollupStart.Add(time.Duration(minute) * time.Minute),
			ChatCount:         chatCount,
			TotalEmoteCount:   totalEmoteCount,
			SevenTVEmoteCount: seventvEmoteCount,
			Emotes:            emotes,
		})
	}
	if len(rollups) == 0 {
		return nil
	}
	sort.Slice(rollups, func(i, j int) bool {
		return rollups[i].MinuteTS.Before(rollups[j].MinuteTS)
	})
	if cache != nil {
		cache.store(rollupStart, rollups)
	}
	s.log.Info("incremental chat rollup patch",
		"stream_id", streamID,
		"segment_start_sec", seg.StartSec,
		"segment_end_sec", seg.EndSec,
		"chat_align_sec", chatAlignSec,
		"minute_start", startMinute,
		"minute_end", endMinute,
		"rollup_minutes", len(rollups),
	)
	status := s.loadOrInitSyncStatus(ctx, streamID)
	status.RollupsWritten += len(rollups)
	if status.Chat != nil {
		status.Chat.IndexPhase = "writing"
	}
	if err := s.saveSyncStatus(ctx, *status); err != nil {
		s.log.Warn("failed to persist incremental rollup progress", "stream_id", streamID, "err", err)
	}
	return s.store.BulkPatchChatRollups(ctx, streamID, rollups)
}

func (s *SyncService) writeChatRollupsOnly(ctx context.Context, streamID, login string, rollupStart time.Time, commentsMap map[int][]string, cache *chatRollupCache) error {
	rollups := make([]MinuteRollup, 0, len(commentsMap))
	for minuteOffset, comments := range commentsMap {
		if len(comments) == 0 || cache.has(minuteOffset) {
			continue
		}
		chatCount, totalEmoteCount, seventvEmoteCount, emotes := buildChatMinuteRollup(login, s.enricher, comments)
		rollups = append(rollups, MinuteRollup{
			MinuteTS:          rollupStart.Add(time.Duration(minuteOffset) * time.Minute),
			ChatCount:         chatCount,
			TotalEmoteCount:   totalEmoteCount,
			SevenTVEmoteCount: seventvEmoteCount,
			Emotes:            emotes,
		})
	}
	sort.Slice(rollups, func(i, j int) bool {
		return rollups[i].MinuteTS.Before(rollups[j].MinuteTS)
	})
	s.setSyncPhase(ctx, streamID, SyncPhaseWritingRollups, fmt.Sprintf("Writing %d chat minutes", len(rollups)), func(st *SyncStatus) {
		st.RollupsWritten = len(rollups)
	})
	return s.store.BulkPatchChatRollups(ctx, streamID, rollups)
}

func (s *SyncService) gqlHTTPClient() *http.Client {
	if s.gqlClient != nil {
		return s.gqlClient
	}
	return s.client
}

func parseRetryAfter(h http.Header) time.Duration {
	raw := strings.TrimSpace(h.Get("Retry-After"))
	if raw == "" {
		return 0
	}
	if secs, err := strconv.Atoi(raw); err == nil && secs > 0 {
		return time.Duration(secs) * time.Second
	}
	if t, err := http.ParseTime(raw); err == nil {
		if d := time.Until(t); d > 0 {
			return d
		}
	}
	return 0
}

func gqlBackoffDelay(attempt int, retryAfter time.Duration) time.Duration {
	if retryAfter > 0 {
		return retryAfter
	}
	base := 500 * time.Millisecond
	delay := base * time.Duration(1<<attempt)
	const maxDelay = 30 * time.Second
	if delay > maxDelay {
		delay = maxDelay
	}
	jitter := time.Duration(rand.Int63n(int64(delay / 4)))
	return delay + jitter
}

const gqlVideoCommentsMaxRetries = 5

func (s *SyncService) postGQLVideoComments(ctx context.Context, reqBody GQLRequest, coord *gqlRateCoordinator) (GQLResponse, error) {
	bodyBytes, err := json.Marshal([]GQLRequest{reqBody})
	if err != nil {
		return GQLResponse{}, err
	}

	var count429, count503 int
	for attempt := 0; attempt <= gqlVideoCommentsMaxRetries; attempt++ {
		if coord != nil {
			if waitErr := coord.Wait(ctx); waitErr != nil {
				return GQLResponse{}, waitErr
			}
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.twitchGQLURL, bytes.NewReader(bodyBytes))
		if err != nil {
			return GQLResponse{}, err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Client-Id", s.twitchClientID)

		resp, err := s.gqlHTTPClient().Do(req)
		if err != nil {
			return GQLResponse{}, err
		}

		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode == http.StatusServiceUnavailable {
			if resp.StatusCode == http.StatusTooManyRequests {
				count429++
			} else {
				count503++
			}
			retryAfter := parseRetryAfter(resp.Header)
			_, _ = io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
			if attempt >= gqlVideoCommentsMaxRetries {
				if count429 > 0 || count503 > 0 {
					s.log.Warn("gql video comments exhausted retries", "429_count", count429, "503_count", count503)
				}
				return GQLResponse{}, fmt.Errorf("gql video comments status %d after %d retries", resp.StatusCode, attempt)
			}
			delay := gqlBackoffDelay(attempt, retryAfter)
			if coord != nil {
				coord.Throttle(retryAfter, attempt)
				coord.RecordRateLimit()
			}
			s.log.Warn("gql video comments throttled; backing off",
				"status", resp.StatusCode,
				"attempt", attempt+1,
				"delay_ms", delay.Milliseconds(),
			)
			select {
			case <-ctx.Done():
				return GQLResponse{}, ctx.Err()
			case <-time.After(delay):
			}
			continue
		}

		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
			resp.Body.Close()
			return GQLResponse{}, fmt.Errorf("gql video comments status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
		}

		var respBody []GQLResponse
		if err := json.NewDecoder(resp.Body).Decode(&respBody); err != nil {
			resp.Body.Close()
			return GQLResponse{}, err
		}
		resp.Body.Close()
		if len(respBody) == 0 {
			return GQLResponse{}, fmt.Errorf("gql video comments response empty")
		}
		if count429 > 0 || count503 > 0 {
			s.log.Info("gql video comments recovered after throttle", "429_count", count429, "503_count", count503)
		}
		if coord != nil {
			coord.RecordSuccess()
		}
		return respBody[0], nil
	}
	return GQLResponse{}, fmt.Errorf("gql video comments retry loop exhausted")
}

func (s *SyncService) vodDurationSeconds(ctx context.Context, videoID string) int {
	if s.helix == nil || !s.helix.Enabled() || videoID == "" {
		return 0
	}
	d, err := s.helix.VideoDurationSeconds(ctx, videoID)
	if err != nil {
		s.log.Warn("helix vod duration lookup failed", "vod_id", videoID, "err", err)
		return 0
	}
	return d
}

func (s *SyncService) generateMockComments(ctx context.Context, login string, streamID string, durationSeconds int, commentsMap map[int][]string) {
	// 1. Get channel emotes from Redis hash
	redisKey := "channel:emotes:" + strings.ToLower(login)
	emoteNames, err := s.rdb.HKeys(ctx, redisKey).Result()
	if err != nil || len(emoteNames) == 0 {
		// Fallback to standard Twitch/generic emotes if Redis is empty
		emoteNames = []string{"LUL", "PogChamp", "Kappa", "SeemsGood", "o7"}
	}

	// Seed random based on streamID
	seed := int64(0)
	for _, c := range streamID {
		seed += int64(c)
	}
	r := rand.New(rand.NewSource(seed))

	totalMinutes := durationSeconds / 60
	if totalMinutes <= 0 {
		totalMinutes = 1
	}

	messages := []string{
		"lmao", "lol", "nice", "wtf", "gg", "wow", "oh my god", "EZ Clap", "o7",
		"so true", "huge", "what?", "hype", "no way", "monkaS", "pepega",
	}

	for m := 0; m <= totalMinutes; m++ {
		// Generate random number of comments for this minute (e.g. 5 to 50 comments)
		numComments := r.Intn(45) + 5
		if m%15 == 0 { // Spike every 15 mins
			numComments = r.Intn(150) + 100
		}

		comments := make([]string, 0, numComments)
		for i := 0; i < numComments; i++ {
			var text string
			if r.Float32() < 0.6 {
				// Message with emote
				emoteName := emoteNames[r.Intn(len(emoteNames))]
				if r.Float32() < 0.5 {
					text = fmt.Sprintf("%s %s", messages[r.Intn(len(messages))], emoteName)
				} else {
					text = emoteName
				}
			} else {
				text = messages[r.Intn(len(messages))]
			}
			comments = append(comments, text)
		}
		commentsMap[m] = comments
	}
}
