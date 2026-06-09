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
	firecrawlURL   string
	firecrawlKey   string
	twitchGQLURL   string
	twitchClientID string
	userAgent           string
	client              *http.Client
	gqlClient           *http.Client
	vodGQLPageDelay     time.Duration
	trackerScrapeTimeoutMS int
	log                 *slog.Logger
	rdb                 *redis.Client
}

func NewSyncService(
	store *Store,
	enricher *enrich.Enricher,
	helix *HelixClient,
	emoteURL string,
	firecrawlURL string,
	firecrawlKey string,
	twitchGQLURL string,
	twitchClientID string,
	userAgent string,
	logger *slog.Logger,
	rdb *redis.Client,
	vodGQLPageDelay time.Duration,
	trackerScrapeTimeoutMS int,
) *SyncService {
	if trackerScrapeTimeoutMS <= 0 {
		trackerScrapeTimeoutMS = 60000
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
		firecrawlURL:   firecrawlURL,
		firecrawlKey:   firecrawlKey,
		twitchGQLURL:   twitchGQLURL,
		twitchClientID: twitchClientID,
		userAgent:      userAgent,
		client:         &http.Client{Timeout: 90 * time.Second},
		gqlClient: &http.Client{
			Timeout:   30 * time.Second,
			Transport: gqlTransport,
		},
		vodGQLPageDelay:        vodGQLPageDelay,
		trackerScrapeTimeoutMS: trackerScrapeTimeoutMS,
		log:                    logger.With("service", "sync"),
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
		ContentOffsetSeconds int `json:"contentOffsetSeconds"`
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

func (s *SyncService) SyncHistoricalStream(ctx context.Context, streamID string, channelOpt string, viewersOnly bool) (string, error) {
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
	if hasStreamRecord && stream != nil && stream.BroadcasterID != "" {
		broadcasterID = stream.BroadcasterID
	}
	if broadcasterID == "" && s.helix != nil && s.helix.Enabled() && login != "" {
		profiles, helixErr := s.helix.UsersByLogin(ctx, []string{login})
		if helixErr != nil {
			s.log.Warn("helix user lookup failed", "login", login, "err", helixErr)
		} else if profile, ok := profiles[login]; ok {
			broadcasterID = profile.ID
		}
	}

	skipTracker := !viewersOnly && hasStreamRecord && stream != nil && stream.ViewerSamples > 0
	cachedVodID := ""
	if hasStreamRecord && stream != nil {
		cachedVodID = strings.TrimSpace(stream.VodID)
	}

	commentsMap := make(map[int][]string) // offset minutes -> comments text
	var commentsErr error
	var commentsWG sync.WaitGroup
	if !viewersOnly && cachedVodID != "" {
		commentsWG.Add(1)
		go func() {
			defer commentsWG.Done()
			s.setSyncPhase(ctx, streamID, SyncPhaseFetchingComments, "Fetching VOD chat (parallel)", nil)
			commentsErr = s.fetchVODComments(ctx, streamID, cachedVodID, commentsMap)
		}()
	}

	var html string
	var tracker trackerStreamData

	if viewersOnly || !skipTracker {
		// 2. Scrape TwitchTracker via local scraper / Firecrawl
		trackerURL := fmt.Sprintf("https://twitchtracker.com/%s/streams/%s", login, streamID)
		s.log.Info("scraping TwitchTracker", "url", trackerURL)
		s.setSyncPhase(ctx, streamID, SyncPhaseScrapingTracker, "Scraping TwitchTracker page", nil)

		html, err = s.scrapeTwitchTracker(ctx, trackerURL)
		if err != nil {
			commentsWG.Wait()
			return "", fmt.Errorf("failed to scrape TwitchTracker: %w", err)
		}
	} else {
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
		s.setSyncPhase(ctx, streamID, SyncPhaseParsingTracker, "Parsing viewer chart and stream stats", nil)
		tracker, err = s.parseTwitchTrackerHTML(html, startedAt)
		if err != nil {
			s.log.Warn("twitchtracker parse partially failed or missing elements", "err", err)
		}
	}
	durationMinutes := tracker.DurationMinutes
	peakViewers := tracker.PeakViewers
	gameSegments := tracker.Games
	viewerPoints := tracker.ViewerPoints
	if !hasRealViewerChart(viewerPoints) {
		s.log.Warn("twitchtracker viewer chart missing or unusable; skipping viewer rollups",
			"stream_id", streamID,
			"points", len(viewerPoints),
			"peak", peakViewers,
		)
		viewerPoints = nil
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

	// 4. Resolve VOD id and fetch comments
	s.setSyncPhase(ctx, streamID, SyncPhaseResolvingVOD, "Resolving Twitch VOD ID", nil)
	vodID := cachedVodID
	if vodID == "" && html != "" {
		vodID = s.extractVodID(html)
		if vodID != "" {
			s.log.Info("extracted VOD ID from TwitchTracker HTML", "vod_id", vodID)
		}
	}
	if vodID == "" && s.helix != nil && s.helix.Enabled() && broadcasterID != "" {
		resolved, helixErr := s.helix.VideoIDByStreamID(ctx, broadcasterID, streamID)
		if helixErr != nil {
			s.log.Warn("helix vod lookup failed", "stream_id", streamID, "err", helixErr)
		} else if resolved != "" {
			vodID = resolved
			s.log.Info("resolved VOD ID via Helix", "vod_id", vodID)
		}
	}
	if vodID != "" {
		if err := s.store.SetStreamVodID(ctx, streamID, vodID); err != nil {
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
		commentsWG.Wait()
		if commentsErr != nil {
			s.log.Warn("failed to fetch VOD comments (parallel)", "err", commentsErr)
		}
	} else if vodID != "" {
		s.log.Info("fetching comments from Twitch GQL for VOD", "vod_id", vodID)
		s.setSyncPhase(ctx, streamID, SyncPhaseFetchingComments, "Fetching VOD chat comments", nil)
		if err := s.fetchVODComments(ctx, streamID, vodID, commentsMap); err != nil {
			s.log.Warn("failed to fetch VOD comments (it may have been deleted)", "err", err)
		}
	} else {
		commentsWG.Wait()
		s.log.Warn("no VOD ID found; skipping chat comments sync", "stream_id", streamID, "broadcaster_id", broadcasterID)
	}

	if !viewersOnly && vodID != "" && s.helix != nil && s.helix.Enabled() {
		vodDuration, helixErr := s.helix.VideoDurationSeconds(ctx, vodID)
		if helixErr != nil {
			s.log.Warn("helix vod duration lookup failed", "vod_id", vodID, "err", helixErr)
		} else if vodDuration > durationSeconds {
			s.log.Info("using helix vod duration for rollups",
				"vod_id", vodID,
				"tracker_seconds", durationSeconds,
				"vod_seconds", vodDuration,
			)
			durationSeconds = vodDuration
		}
	}
	// 5. Combine and build minute-by-minute rollups
	rollupStart := startedAt.UTC().Truncate(time.Minute)
	if !tracker.ChartStartedAt.IsZero() {
		rollupStart = tracker.ChartStartedAt.UTC().Truncate(time.Minute)
	}
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
	if skipTracker && !viewersOnly {
		rollups = nil
	} else {
	rollups = make([]MinuteRollup, 0, totalMinutes+1)

	// Create a fast lookup for viewer counts by minute offset
	viewerLookup := make(map[int]int)
	for _, pt := range viewerPoints {
		minOffset := pt.OffsetSeconds / 60
		viewerLookup[minOffset] = pt.Viewers
	}

	for m := 0; m <= totalMinutes; m++ {
		minuteTS := rollupStart.Add(time.Duration(m) * time.Minute)
		
		// Interpolate viewer count if not explicitly mapped
		viewerVal := 0
		if val, ok := viewerLookup[m]; ok {
			viewerVal = val
		} else {
			// Linear interpolation from nearest neighbors
			viewerVal = s.interpolateViewerCount(m, viewerPoints)
		}

		// Process comments in this minute bucket
		chatCount := 0
		totalEmoteCount := 0
		seventvEmoteCount := 0
		emotesMap := make(map[string]int)

		if comments, ok := commentsMap[m]; ok {
			chatCount = len(comments)
			for _, comment := range comments {
				fragments := s.enricher.Tokenize(login, comment)
				for _, frag := range fragments {
					if frag.T == "emote" {
						totalEmoteCount++
						if frag.Provider == "seventv" {
							seventvEmoteCount++
						}
						// Emote key is provider:id:name
						key := fmt.Sprintf("%s:%s:%s", frag.Provider, frag.ID, frag.C)
						emotesMap[key]++
					}
				}
			}
		}

		rollups = append(rollups, MinuteRollup{
			MinuteTS:          minuteTS,
			ViewerAvg:         viewerVal,
			ViewerMax:         viewerVal,
			ViewerLatest:      viewerVal,
			ViewerSamples:     1,
			ChatCount:         chatCount,
			TotalEmoteCount:   totalEmoteCount,
			SevenTVEmoteCount: seventvEmoteCount,
			Emotes:            emotesMap,
		})
	}
	}

	// 6. Save data to database
	s.log.Info("saving historical rollups and game segments to database", "rollups_count", len(rollups), "segments_count", len(gameSegments))

	if skipTracker && !viewersOnly {
		s.setSyncPhase(ctx, streamID, SyncPhaseWritingRollups, "Writing chat rollups to database", nil)
		err = s.writeChatRollupsOnly(ctx, streamID, login, rollupStart, commentsMap)
		if err != nil {
			return "", fmt.Errorf("failed to save chat rollups to DB: %w", err)
		}
	} else {
		s.setSyncPhase(ctx, streamID, SyncPhaseWritingRollups, "Writing minute rollups to database", func(st *SyncStatus) {
			st.RollupsWritten = len(rollups)
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

	chatComments := 0
	for _, comments := range commentsMap {
		chatComments += len(comments)
	}
	msg := "Stream synced successfully"
	if viewersOnly {
		if len(viewerPoints) == 0 {
			msg = "Chat/7TV unchanged — TwitchTracker viewer chart blocked (no minute-level viewers). Try again later or use cloud Firecrawl."
		} else {
			msg = "Viewer timeline synced from TwitchTracker (chat/7TV unchanged)"
		}
	} else if len(viewerPoints) == 0 {
		if vodID == "" {
			msg = "Stream synced (chat skipped — VOD not found). Viewer chart blocked by TwitchTracker."
		} else if chatComments == 0 {
			msg = "Stream synced (chat unavailable). Viewer chart blocked by TwitchTracker."
		} else {
			msg = "Chat/emotes synced. Viewer chart blocked by TwitchTracker — use Re-sync viewers when scrape works."
		}
	} else if vodID == "" {
		msg = "Stream synced (viewers only — VOD not found, chat/7TV skipped)"
	} else if chatComments == 0 {
		msg = "Stream synced (viewers only — VOD comments unavailable)"
	}
	if skipTracker && !viewersOnly && chatComments > 0 {
		msg = fmt.Sprintf("Chat/emotes synced (%s comments); viewer timeline unchanged", strconv.Itoa(chatComments))
	}
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

// hasRealViewerChart rejects peak-only / flat fallbacks (need varied minute-level samples).
func hasRealViewerChart(points []parsedViewerPoint) bool {
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
	return maxV > minV
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
	resp, err := s.client.Do(req)
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

func formatFirecrawlConnectError(err error, firecrawlURL string) string {
	msg := err.Error()
	lower := strings.ToLower(msg)
	if strings.Contains(lower, "connection refused") || strings.Contains(lower, "no such host") || strings.Contains(lower, "actively refused") {
		return fmt.Sprintf(
			"%s — scraper at %s is not reachable. Set FIRECRAWL_API_URL to https://api.firecrawl.dev/v2/scrape with a FIRECRAWL_API_KEY from https://firecrawl.dev, or run a self-hosted Firecrawl instance (default port 3002) and point FIRECRAWL_API_URL at it",
			msg,
			firecrawlURL,
		)
	}
	return msg
}

func (s *SyncService) scrapeTwitchTracker(ctx context.Context, url string) (string, error) {
	if htmlBody, err := s.scrapeTwitchTrackerDirect(ctx, url); err == nil {
		s.log.Info("fetched TwitchTracker page via direct HTTP", "url", url)
		return htmlBody, nil
	} else {
		s.log.Info("direct TwitchTracker fetch unavailable, trying Firecrawl", "url", url, "err", err)
	}

	if s.firecrawlKey == "" {
		return "", fmt.Errorf("missing FIRECRAWL_API_KEY — TwitchTracker blocks direct scraping; add a key from https://firecrawl.dev or run self-hosted Firecrawl")
	}

	reqBody, err := json.Marshal(map[string]any{
		"url":             url,
		"formats":         []string{"rawHtml"},
		"onlyMainContent": false,
		"useProxy":        false, // datacenter proxies are Cloudflare-blocked on TwitchTracker
		"timeout":         s.trackerScrapeTimeoutMS,
	})
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.firecrawlURL, bytes.NewReader(reqBody))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+s.firecrawlKey)

	resp, err := s.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to scrape TwitchTracker: %s", formatFirecrawlConnectError(err, s.firecrawlURL))
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("firecrawl API returned status %d: %s", resp.StatusCode, string(body))
	}

	var fcResp struct {
		Success bool `json:"success"`
		Data    struct {
			HTML    string `json:"html"`
			RawHTML string `json:"rawHtml"`
		} `json:"data"`
		Error string `json:"error"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&fcResp); err != nil {
		return "", err
	}

	if !fcResp.Success {
		return "", fmt.Errorf("firecrawl scrape failed: %s", fcResp.Error)
	}

	htmlBody := fcResp.Data.RawHTML
	if htmlBody == "" {
		htmlBody = fcResp.Data.HTML
	}
	if htmlBody == "" {
		return "", fmt.Errorf("firecrawl scrape returned empty html")
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
			s.log.Info("successfully parsed stream data from meta#ecs block",
				"duration_minutes", parsed.DurationMinutes,
				"peak_viewers", parsed.PeakViewers,
				"avg_viewers", parsed.AvgViewers,
				"games_count", len(parsed.Games),
				"points_count", len(parsed.ViewerPoints),
			)
			return parsed, nil
		} else if err != nil {
			s.log.Warn("failed to parse meta#ecs block, falling back to HTML scraping", "err", err)
		}
	}

	// 1. Parse Duration (legacy to-time-lg and newer g-x-s-value blocks)
	durationMinutes := 0
	reDuration := regexp.MustCompile(`(?i)to-time-lg">(\d+)</div>\s*<div class="g-x-s-label[^>]*>Stream duration</div>`)
	if durMatch := reDuration.FindStringSubmatch(html); len(durMatch) > 1 {
		durationMinutes, _ = strconv.Atoi(durMatch[1])
	}
	if durationMinutes == 0 {
		reDurationNew := regexp.MustCompile(`(?i)g-x-s-value">\s*([\d,]+)\s*</div>\s*<div class="g-x-s-label[^"]*">Stream duration</div>`)
		if durMatch := reDurationNew.FindStringSubmatch(html); len(durMatch) > 1 {
			durationMinutes, _ = strconv.Atoi(strings.ReplaceAll(durMatch[1], ",", ""))
		}
	}

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

	if injected := parseStreamcloneViewerChartJSON(html, durationMinutes, peakViewers); len(injected) >= 3 && hasRealViewerChart(injected) {
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
	if len(points) == 0 {
		return 0
	}
	targetSec := minute * 60

	// If offset is before the first point, return first point
	if targetSec <= points[0].OffsetSeconds {
		return points[0].Viewers
	}
	// If offset is after the last point, return last point
	if targetSec >= points[len(points)-1].OffsetSeconds {
		return points[len(points)-1].Viewers
	}

	// Find enclosing interval
	for i := 0; i < len(points)-1; i++ {
		p1 := points[i]
		p2 := points[i+1]
		if targetSec >= p1.OffsetSeconds && targetSec <= p2.OffsetSeconds {
			span := p2.OffsetSeconds - p1.OffsetSeconds
			if span == 0 {
				return p1.Viewers
			}
			pct := float64(targetSec-p1.OffsetSeconds) / float64(span)
			return p1.Viewers + int(pct*float64(p2.Viewers-p1.Viewers))
		}
	}

	return 0
}

func (s *SyncService) preloadChannelEmotes(ctx context.Context, login, broadcasterID string) {
	if s.emoteURL == "" || login == "" || broadcasterID == "" {
		return
	}
	body, err := json.Marshal(map[string]any{
		"twitch_id": broadcasterID,
		"providers": []string{"seventv", "twitch"},
	})
	if err != nil {
		return
	}
	url := fmt.Sprintf("%s/v1/channels/%s/emotes/ensure", s.emoteURL, login)
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
	resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		s.log.Warn("emote ensure returned non-success status", "login", login, "status", resp.StatusCode)
		return
	}
	s.enricher.Invalidate(login)
	s.log.Info("preloaded channel emotes for historical sync", "login", login)
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

func buildChatMinuteRollup(login string, enricher *enrich.Enricher, comments []string) (chatCount, totalEmoteCount, seventvEmoteCount int, emotes map[string]int) {
	emotes = make(map[string]int)
	chatCount = len(comments)
	for _, comment := range comments {
		fragments := enricher.Tokenize(login, comment)
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
	return chatCount, totalEmoteCount, seventvEmoteCount, emotes
}

func (s *SyncService) writeChatRollupsOnly(ctx context.Context, streamID, login string, rollupStart time.Time, commentsMap map[int][]string) error {
	rollups := make([]MinuteRollup, 0, len(commentsMap))
	for minuteOffset, comments := range commentsMap {
		if len(comments) == 0 {
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

func (s *SyncService) postGQLVideoComments(ctx context.Context, reqBody GQLRequest) (GQLResponse, error) {
	bodyBytes, err := json.Marshal([]GQLRequest{reqBody})
	if err != nil {
		return GQLResponse{}, err
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
	defer resp.Body.Close()

	var respBody []GQLResponse
	if err := json.NewDecoder(resp.Body).Decode(&respBody); err != nil {
		return GQLResponse{}, err
	}
	if len(respBody) == 0 {
		return GQLResponse{}, fmt.Errorf("gql video comments response empty")
	}
	return respBody[0], nil
}

func (s *SyncService) fetchVODComments(ctx context.Context, streamID, videoID string, commentsMap map[int][]string) error {
	const sha256Hash = "b70a3591ff0f4e0313d126c6a1502d79a1c02baebb288227c582044aa76adf6a"
	const maxComments = 200000
	const progressEvery = 200

	useCursor := false
	cursorFailed := false
	nextCursor := ""
	contentOffsetSeconds := 0
	commentsCount := 0
	lastOffset := 0

	reportProgress := func(force bool) {
		if !force && (commentsCount == 0 || commentsCount%progressEvery != 0) {
			return
		}
		count := commentsCount
		s.setSyncPhase(ctx, streamID, SyncPhaseFetchingComments, fmt.Sprintf("Fetched %d comments", count), func(st *SyncStatus) {
			st.CommentsFetched = count
		})
	}

	for {
		reqBody := buildVideoCommentsGQLRequest(videoID, sha256Hash, useCursor, contentOffsetSeconds, nextCursor)
		gqlResp, err := s.postGQLVideoComments(ctx, reqBody)
		if err != nil {
			return err
		}
		if isGQLIntegrityError(gqlResp) {
			if useCursor {
				s.log.Warn("GQL cursor pagination failed integrity check; falling back to offset",
					"stream_id", streamID,
					"offset", lastOffset+1,
				)
				useCursor = false
				cursorFailed = true
				nextCursor = ""
				contentOffsetSeconds = lastOffset + 1
				continue
			}
			return fmt.Errorf("gql video comments integrity error")
		}
		if len(gqlResp.Errors) > 0 {
			return fmt.Errorf("gql video comments error: %s", gqlResp.Errors[0].Message)
		}
		if gqlResp.Data.Video == nil || gqlResp.Data.Video.Comments == nil {
			break
		}

		commentsNode := gqlResp.Data.Video.Comments
		edges := commentsNode.Edges
		if len(edges) == 0 {
			break
		}

		for _, edge := range edges {
			minOffset := edge.Node.ContentOffsetSeconds / 60
			text := gqlCommentText(edge.Node.Message)
			if strings.TrimSpace(text) == "" {
				continue
			}
			commentsMap[minOffset] = append(commentsMap[minOffset], text)
			commentsCount++
			lastOffset = edge.Node.ContentOffsetSeconds
		}

		reportProgress(false)

		if commentsCount >= maxComments {
			s.log.Warn("safety threshold of 200,000 comments reached; truncating comments paging")
			break
		}
		if !commentsNode.PageInfo.HasNextPage {
			break
		}

		lastEdge := edges[len(edges)-1]
		if !cursorFailed && strings.TrimSpace(lastEdge.Cursor) != "" {
			useCursor = true
			nextCursor = lastEdge.Cursor
		} else {
			useCursor = false
			nextCursor = ""
			contentOffsetSeconds = lastOffset + 1
		}

		if s.vodGQLPageDelay > 0 {
			time.Sleep(s.vodGQLPageDelay)
		}
	}

	reportProgress(true)
	s.log.Info("finished VOD comments paging", "total_comments", commentsCount, "cursor_mode", !cursorFailed)
	return nil
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
