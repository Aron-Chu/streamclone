package analytics

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"time"

	"streamclone/internal/chat/batch"
	"streamclone/internal/chat/enrich"
	"streamclone/internal/chat/parse"
)

type StreamProvider interface {
	StreamsByLogin(ctx context.Context, logins []string) (map[string]LiveStream, error)
	UsersByLogin(ctx context.Context, logins []string) (map[string]UserProfile, error)
	VideoIDByStreamID(ctx context.Context, broadcasterID, streamID string) (string, error)
}

type RollupStore interface {
	UpsertLiveStream(ctx context.Context, stream LiveStream, profile UserProfile, seenAt time.Time) error
	CloseStream(ctx context.Context, streamID string, endedAt time.Time) error
	UpsertMinuteRollup(ctx context.Context, streamID string, rollup MinuteRollup) error
	PurgeOlderThan(ctx context.Context, cutoff time.Time) error
	AddAlwaysTracked(ctx context.Context, login string) error
	RemoveAlwaysTracked(ctx context.Context, login string) error
	StreamByID(ctx context.Context, streamID string) (*StreamRecord, error)
	SetStreamVodID(ctx context.Context, streamID, vodID, source string) error
}

type channelJoiner interface {
	Join(ctx context.Context, channel string)
	Part(ctx context.Context, channel string)
}

type Collector struct {
	store        RollupStore
	helix        StreamProvider
	irc          channelJoiner
	enricher     *enrich.Enricher
	log          *slog.Logger
	maxTracked   int
	pollInterval time.Duration
	retention    time.Duration
	topEmotes    int

	mu            sync.Mutex
	tracked       map[string]*trackedChannel
	buckets       map[string]*minuteAccumulator
	runCtx        context.Context
	stop          chan struct{}
	startOnce     sync.Once
	stopOnce      sync.Once
	alwaysTracked map[string]bool
}

type trackedChannel struct {
	login           string
	currentStreamID string
	offlinePolls    int
	addedAt         time.Time
	lastPollAt      time.Time
}

type minuteAccumulator struct {
	streamID          string
	minute            time.Time
	viewerSum         int
	viewerSamples     int
	viewerMax         int
	viewerLatest      int
	chatCount         int
	totalEmoteCount   int
	sevenTVEmoteCount int
	emotes            map[string]int
}

func NewCollector(
	store RollupStore,
	helix StreamProvider,
	irc channelJoiner,
	enricher *enrich.Enricher,
	logger *slog.Logger,
	maxTracked int,
	pollInterval time.Duration,
	retention time.Duration,
	topEmotes int,
) *Collector {
	if maxTracked <= 0 {
		maxTracked = 50
	}
	if pollInterval <= 0 {
		pollInterval = 15 * time.Second
	}
	if retention <= 0 {
		retention = 30 * 24 * time.Hour
	}
	if topEmotes <= 0 {
		topEmotes = 200
	}
	return &Collector{
		store:         store,
		helix:         helix,
		irc:           irc,
		enricher:      enricher,
		log:           logger,
		maxTracked:    maxTracked,
		pollInterval:  pollInterval,
		retention:     retention,
		topEmotes:     topEmotes,
		tracked:       map[string]*trackedChannel{},
		buckets:       map[string]*minuteAccumulator{},
		runCtx:        context.Background(),
		stop:          make(chan struct{}),
		alwaysTracked: map[string]bool{},
	}
}

func (c *Collector) WithAlwaysTracked(logins []string) *Collector {
	c.mu.Lock()
	defer c.mu.Unlock()
	alwaysMap := make(map[string]bool, len(logins))
	for _, login := range logins {
		normalized := normalizeLogin(login)
		if normalized != "" {
			alwaysMap[normalized] = true
			if _, ok := c.tracked[normalized]; !ok {
				c.tracked[normalized] = &trackedChannel{login: normalized, addedAt: time.Now().UTC()}
			}
		}
	}
	c.alwaysTracked = alwaysMap
	return c
}

func (c *Collector) Start(ctx context.Context) {
	c.startOnce.Do(func() {
		c.mu.Lock()
		c.runCtx = ctx
		
		// Collect always-tracked logins to join IRC on startup
		alwaysLogins := make([]string, 0, len(c.alwaysTracked))
		for login := range c.alwaysTracked {
			alwaysLogins = append(alwaysLogins, login)
		}
		c.mu.Unlock()

		for _, login := range alwaysLogins {
			c.irc.Join(ctx, login)
		}

		go c.loop(ctx)
	})
}

func (c *Collector) Stop() {
	c.stopOnce.Do(func() {
		close(c.stop)
	})
}

func (c *Collector) Watch(ctx context.Context, login string) WatchResponse {
	login = normalizeLogin(login)
	c.mu.Lock()
	if _, ok := c.tracked[login]; ok {
		active := len(c.tracked)
		c.mu.Unlock()
		return WatchResponse{
			Channel:  login,
			Tracking: true,
			Active:   active,
			Max:      c.maxTracked,
			Message:  "already tracking until stream ends",
			Sources:  []SourceStatus{{Source: "analytics_collector", State: "ready", Message: "tracking until offline"}},
		}
	}
	if len(c.tracked) >= c.maxTracked {
		active := len(c.tracked)
		c.mu.Unlock()
		return WatchResponse{
			Channel:  login,
			Tracking: false,
			Active:   active,
			Max:      c.maxTracked,
			Message:  "analytics tracking pool is full",
			Sources:  []SourceStatus{{Source: "analytics_collector", State: "limited", Message: "max tracked channels reached"}},
		}
	}
	c.tracked[login] = &trackedChannel{login: login, addedAt: time.Now().UTC()}
	active := len(c.tracked)
	ircCtx := c.runCtx
	c.mu.Unlock()
	c.irc.Join(ircCtx, login)
	return WatchResponse{
		Channel:  login,
		Tracking: true,
		Active:   active,
		Max:      c.maxTracked,
		Message:  "tracking until stream ends",
		Sources:  []SourceStatus{{Source: "analytics_collector", State: "ready", Message: "tracking until offline"}},
	}
}

func (c *Collector) SetAlwaysTracked(ctx context.Context, login string, always bool) WatchResponse {
	login = normalizeLogin(login)
	c.mu.Lock()
	if always {
		c.alwaysTracked[login] = true
		if _, ok := c.tracked[login]; !ok {
			c.tracked[login] = &trackedChannel{login: login, addedAt: time.Now().UTC()}
			c.irc.Join(c.runCtx, login)
		}
		_ = c.store.AddAlwaysTracked(ctx, login)
	} else {
		delete(c.alwaysTracked, login)
		if tc, ok := c.tracked[login]; ok && tc.currentStreamID == "" {
			c.irc.Part(c.runCtx, login)
			delete(c.tracked, login)
		}
		_ = c.store.RemoveAlwaysTracked(ctx, login)
	}
	active := len(c.tracked)
	c.mu.Unlock()

	return WatchResponse{
		Channel:  login,
		Tracking: true,
		Active:   active,
		Max:      c.maxTracked,
		Message:  fmt.Sprintf("always tracked set to %v", always),
		Sources:  []SourceStatus{{Source: "analytics_collector", State: "ready"}},
	}
}

func (c *Collector) GetAlwaysTracked() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	logins := make([]string, 0, len(c.alwaysTracked))
	for login := range c.alwaysTracked {
		logins = append(logins, login)
	}
	sort.Strings(logins)
	return logins
}

func (c *Collector) ActiveCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.tracked)
}

func (c *Collector) HandleIRCLine(line string) {
	msg, ok := parse.ParseLine(line)
	if !ok {
		return
	}
	channel := normalizeLogin(msg.Channel)
	c.mu.Lock()
	tracked := c.tracked[channel]
	streamID := ""
	if tracked != nil {
		streamID = tracked.currentStreamID
	}
	c.mu.Unlock()
	if streamID == "" {
		return
	}
	fragments := c.enricher.Tokenize(channel, msg.Text)
	c.addChat(streamID, msg.TS, fragments)
}

func (c *Collector) loop(ctx context.Context) {
	ticker := time.NewTicker(c.pollInterval)
	cleanupTicker := time.NewTicker(time.Hour)
	defer ticker.Stop()
	defer cleanupTicker.Stop()
	c.pollOnce(ctx)
	for {
		select {
		case <-ctx.Done():
			c.flushAll(context.Background())
			return
		case <-c.stop:
			c.flushAll(context.Background())
			return
		case <-ticker.C:
			c.pollOnce(ctx)
		case <-cleanupTicker.C:
			c.cleanup(ctx)
		}
	}
}

func (c *Collector) pollOnce(ctx context.Context) {
	logins := c.trackedLogins()
	if len(logins) == 0 {
		c.flushCompleted(ctx, time.Now().UTC())
		return
	}
	streams, err := c.helix.StreamsByLogin(ctx, logins)
	if err != nil {
		c.log.Warn("analytics helix streams failed", "err", err)
		return
	}
	liveLogins := make([]string, 0, len(streams))
	for login := range streams {
		liveLogins = append(liveLogins, login)
	}
	profiles, err := c.helix.UsersByLogin(ctx, liveLogins)
	if err != nil {
		c.log.Warn("analytics helix users failed", "err", err)
		profiles = map[string]UserProfile{}
	}

	now := time.Now().UTC()
	var remove []string
	var closeStreams []string
	for _, login := range logins {
		stream, live := streams[login]
		c.mu.Lock()
		tracked := c.tracked[login]
		if tracked == nil {
			c.mu.Unlock()
			continue
		}
		tracked.lastPollAt = now
		if live {
			wasOffline := tracked.currentStreamID == ""
			tracked.offlinePolls = 0
			tracked.currentStreamID = stream.ID
			c.mu.Unlock()
			if wasOffline {
				c.irc.Join(c.runCtx, login)
			}
			if err := c.store.UpsertLiveStream(ctx, stream, profiles[login], now); err != nil {
				c.log.Warn("analytics upsert stream failed", "stream_id", stream.ID, "err", err)
				continue
			}
			c.addViewerSample(stream.ID, now, stream.ViewerCount)
			continue
		}
		tracked.offlinePolls++
		isAlwaysTracked := c.alwaysTracked[login]
		if tracked.offlinePolls >= 2 {
			if tracked.currentStreamID != "" {
				closeStreams = append(closeStreams, tracked.currentStreamID)
				tracked.currentStreamID = ""
				if isAlwaysTracked {
					c.mu.Unlock()
					c.irc.Part(context.Background(), login)
					c.mu.Lock()
				}
			}
			if !isAlwaysTracked {
				remove = append(remove, login)
			}
		}
		c.mu.Unlock()
	}
	if len(remove) > 0 {
		c.mu.Lock()
		for _, login := range remove {
			delete(c.tracked, login)
		}
		c.mu.Unlock()
		for _, login := range remove {
			c.irc.Part(context.Background(), login)
		}
	}
	for _, streamID := range closeStreams {
		if err := c.store.CloseStream(ctx, streamID, now); err != nil {
			c.log.Warn("analytics close stream failed", "stream_id", streamID, "err", err)
			continue
		}
		c.scheduleVodIDResolve(streamID)
	}
	c.flushCompleted(ctx, now)
}

func (c *Collector) trackedLogins() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	logins := make([]string, 0, len(c.tracked))
	for login := range c.tracked {
		logins = append(logins, login)
	}
	sort.Strings(logins)
	return logins
}

func (c *Collector) addViewerSample(streamID string, at time.Time, viewers int) {
	if streamID == "" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	acc := c.bucketLocked(streamID, at)
	acc.viewerSum += viewers
	acc.viewerSamples++
	if viewers > acc.viewerMax {
		acc.viewerMax = viewers
	}
	acc.viewerLatest = viewers
}

func (c *Collector) addChat(streamID string, ts int64, fragments []batch.Fragment) {
	if streamID == "" {
		return
	}
	at := time.Now().UTC()
	if ts > 0 {
		at = time.UnixMilli(ts).UTC()
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	acc := c.bucketLocked(streamID, at)
	acc.chatCount++
	for _, fragment := range fragments {
		if fragment.T != "emote" {
			continue
		}
		key := emoteKey(fragment)
		acc.totalEmoteCount++
		if strings.EqualFold(fragment.Provider, "seventv") {
			acc.sevenTVEmoteCount++
		}
		acc.emotes[key]++
	}
}

func (c *Collector) bucketLocked(streamID string, at time.Time) *minuteAccumulator {
	minute := at.UTC().Truncate(time.Minute)
	key := streamID + "|" + minute.Format(time.RFC3339)
	acc := c.buckets[key]
	if acc == nil {
		acc = &minuteAccumulator{streamID: streamID, minute: minute, emotes: map[string]int{}}
		c.buckets[key] = acc
	}
	return acc
}

func (c *Collector) flushCompleted(ctx context.Context, now time.Time) {
	cutoff := now.UTC().Truncate(time.Minute)
	c.flushWhere(ctx, func(acc *minuteAccumulator) bool {
		return acc.minute.Before(cutoff)
	})
}

func (c *Collector) flushAll(ctx context.Context) {
	c.flushWhere(ctx, func(*minuteAccumulator) bool { return true })
}

func (c *Collector) flushWhere(ctx context.Context, shouldFlush func(*minuteAccumulator) bool) {
	var flush []*minuteAccumulator
	c.mu.Lock()
	for key, acc := range c.buckets {
		if shouldFlush(acc) {
			flush = append(flush, acc)
			delete(c.buckets, key)
		}
	}
	c.mu.Unlock()
	for _, acc := range flush {
		rollup := acc.rollup(c.topEmotes)
		if err := c.store.UpsertMinuteRollup(ctx, acc.streamID, rollup); err != nil {
			c.log.Warn("analytics flush rollup failed", "stream_id", acc.streamID, "minute", acc.minute, "err", err)
		}
	}
}

func (c *Collector) cleanup(ctx context.Context) {
	cutoff := time.Now().UTC().Add(-c.retention)
	if err := c.store.PurgeOlderThan(ctx, cutoff); err != nil {
		c.log.Warn("analytics retention cleanup failed", "err", err)
	}
}

func (c *Collector) scheduleVodIDResolve(streamID string) {
	go c.resolveVodIDWithRetry(streamID)
}

func (c *Collector) resolveVodIDWithRetry(streamID string) {
	delays := []time.Duration{0, 30 * time.Second, 2 * time.Minute, 5 * time.Minute}
	for attempt, delay := range delays {
		if delay > 0 {
			time.Sleep(delay)
		}
		ctx := context.Background()
		rec, err := c.store.StreamByID(ctx, streamID)
		if err != nil {
			c.log.Warn("vod resolve skipped; stream record missing", "stream_id", streamID, "err", err)
			return
		}
		if rec == nil {
			return
		}
		if strings.TrimSpace(rec.VodID) != "" {
			return
		}
		broadcasterID := strings.TrimSpace(rec.BroadcasterID)
		if broadcasterID == "" {
			c.log.Debug("vod resolve skipped; broadcaster_id missing", "stream_id", streamID)
			return
		}
		vodID, err := c.helix.VideoIDByStreamID(ctx, broadcasterID, streamID)
		if err != nil {
			c.log.Warn("helix vod resolve on stream close failed", "stream_id", streamID, "attempt", attempt+1, "err", err)
			continue
		}
		if vodID == "" {
			if attempt == len(delays)-1 {
				c.log.Info("vod not published yet after stream close", "stream_id", streamID)
			}
			continue
		}
		if err := c.store.SetStreamVodID(ctx, streamID, vodID, "helix_stream_match"); err != nil {
			c.log.Warn("failed to persist vod_id on stream close", "stream_id", streamID, "err", err)
			return
		}
		c.log.Info("persisted vod_id on stream close", "stream_id", streamID, "vod_id", vodID, "attempt", attempt+1)
		return
	}
}

func (a *minuteAccumulator) rollup(topN int) MinuteRollup {
	viewerAvg := 0
	if a.viewerSamples > 0 {
		viewerAvg = int((a.viewerSum + a.viewerSamples/2) / a.viewerSamples)
	}
	return MinuteRollup{
		MinuteTS:          a.minute,
		ViewerAvg:         viewerAvg,
		ViewerMax:         a.viewerMax,
		ViewerLatest:      a.viewerLatest,
		ViewerSamples:     a.viewerSamples,
		ChatCount:         a.chatCount,
		TotalEmoteCount:   a.totalEmoteCount,
		SevenTVEmoteCount: a.sevenTVEmoteCount,
		Emotes:            topNMap(a.emotes, topN),
	}
}

func emoteKey(fragment batch.Fragment) string {
	provider := strings.ToLower(strings.TrimSpace(fragment.Provider))
	id := strings.TrimSpace(fragment.ID)
	name := strings.TrimSpace(fragment.C)
	if provider != "" && id != "" {
		return provider + ":" + id + ":" + name
	}
	if name != "" {
		return name
	}
	return "unknown"
}
