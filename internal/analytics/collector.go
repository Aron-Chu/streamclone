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
	MarkStreamVodUnlinked(ctx context.Context, streamID string) error
}

type channelJoiner interface {
	Join(ctx context.Context, channel string)
	Part(ctx context.Context, channel string)
}

type Collector struct {
	store                 RollupStore
	helix                 StreamProvider
	irc                   channelJoiner
	enricher              *enrich.Enricher
	log                   *slog.Logger
	maxTracked            int
	pollInterval          time.Duration
	retention             time.Duration
	topEmotes             int
	idleTTL               time.Duration
	pulseCacheInvalidator func(ctx context.Context, login, streamID string, includeHeatmap bool)
	// nowClock supports fake-clock VOD finalization tests; defaults to time.Now().UTC.
	nowClock            func() time.Time
	goLiveByStream      sync.Map // streamID -> pulseGoLiveObservation
	firstRollupRecorded sync.Map // streamID -> struct{}

	// vodResolveOffsets are the post-close offsets (relative to stream close)
	// at which the live collector attempts to resolve the VOD id via Helix.
	// The final offset bounds the 5-minute resolution window (Requirement 19.3).
	vodResolveOffsets []time.Duration

	mu            sync.Mutex
	tracked       map[string]*trackedChannel
	buckets       map[string]*minuteAccumulator
	runCtx        context.Context
	stop          chan struct{}
	startOnce     sync.Once
	stopOnce      sync.Once
	alwaysTracked map[string]bool
	liveEmote     *LiveEmoteEnsurer
}

type trackedChannel struct {
	login           string
	currentStreamID string
	offlinePolls    int
	addedAt         time.Time
	lastPollAt      time.Time
	lastViewedAt    time.Time
	refCounts       map[string]int
	poolAlwaysTrack bool
	watchPriority   int
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
		idleTTL:       15 * time.Minute,
		tracked:       map[string]*trackedChannel{},
		buckets:       map[string]*minuteAccumulator{},
		runCtx:        context.Background(),
		stop:          make(chan struct{}),
		alwaysTracked: map[string]bool{},
		// Resolve the VOD id at close, then at 30s / 2m / 5m / 15m / 60m after
		// close. The final offset is the auto-retry upper bound (Req 19.3/19.4).
		vodResolveOffsets: []time.Duration{0, 30 * time.Second, 2 * time.Minute, 5 * time.Minute, 15 * time.Minute, 60 * time.Minute},
	}
}

func (c *Collector) WithIdleTTL(d time.Duration) *Collector {
	if c != nil && d > 0 {
		c.idleTTL = d
	}
	return c
}

func (c *Collector) WithMaxTracked(max int) *Collector {
	if c != nil && max > 0 {
		c.maxTracked = max
	}
	return c
}

func (c *Collector) WithPulseCacheInvalidator(fn func(ctx context.Context, login, streamID string, includeHeatmap bool)) *Collector {
	if c != nil {
		c.pulseCacheInvalidator = fn
	}
	return c
}

func (c *Collector) WithNowClock(fn func() time.Time) *Collector {
	if c != nil && fn != nil {
		c.nowClock = fn
	}
	return c
}

func (c *Collector) nowUTC() time.Time {
	if c != nil && c.nowClock != nil {
		return c.nowClock()
	}
	return time.Now().UTC()
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
				c.tracked[normalized] = &trackedChannel{
					login:        normalized,
					addedAt:      time.Now().UTC(),
					lastViewedAt: time.Now().UTC(),
					refCounts:    map[string]int{},
				}
			}
		}
	}
	c.alwaysTracked = alwaysMap
	return c
}

func (c *Collector) WithLiveEmoteEnsurer(ensurer *LiveEmoteEnsurer) *Collector {
	if c != nil {
		c.liveEmote = ensurer
	}
	return c
}

func (c *Collector) EmoteSyncSnapshot(ctx context.Context, login string) EmoteSyncSnapshot {
	if c == nil || c.liveEmote == nil {
		return emoteSyncSnapshotForState(EmoteSyncUnavailable, false, "", nil)
	}
	return c.liveEmote.Snapshot(ctx, login, c.IsTracking(login))
}

func (c *Collector) kickoffLiveEmoteEnsure(login string) {
	if c == nil || c.liveEmote == nil {
		return
	}
	c.liveEmote.Kickoff(c.runCtx, login)
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
			c.kickoffLiveEmoteEnsure(login)
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
	return c.WatchForPrincipal(ctx, login, "")
}

func (c *Collector) WatchForPrincipal(ctx context.Context, login, principalID string) WatchResponse {
	incoming := TrackPriorityIdleNoRef
	if strings.TrimSpace(principalID) != "" {
		incoming = TrackPriorityManualWatch
	}
	return c.WatchWithPriority(ctx, login, principalID, incoming)
}

func (c *Collector) WatchWithPriority(ctx context.Context, login, principalID string, incomingPriority int) WatchResponse {
	login = normalizeLogin(login)
	now := time.Now().UTC()
	c.mu.Lock()
	if tc, ok := c.tracked[login]; ok {
		c.touchPrincipalLocked(tc, principalID, now)
		if incomingPriority > tc.watchPriority {
			tc.watchPriority = incomingPriority
		}
		if incomingPriority >= TrackPriorityPrincipalAlwaysTrack {
			tc.poolAlwaysTrack = true
		}
		active := len(c.tracked)
		c.mu.Unlock()
		c.kickoffLiveEmoteEnsure(login)
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
		evicted, evictedPriority, ok := c.evictOneForIncomingPriorityLocked(now, incomingPriority)
		if !ok {
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
		c.log.Info("collector preempted lower-priority channel",
			"evicted", evicted,
			"evicted_priority", evictedPriority,
			"incoming", login,
			"incoming_priority", incomingPriority,
		)
		c.irc.Part(c.runCtx, evicted)
	}
	tc := &trackedChannel{
		login:         login,
		addedAt:       now,
		lastViewedAt:  now,
		refCounts:     map[string]int{},
		watchPriority: incomingPriority,
	}
	if incomingPriority >= TrackPriorityPrincipalAlwaysTrack {
		tc.poolAlwaysTrack = true
	}
	c.touchPrincipalLocked(tc, principalID, now)
	c.tracked[login] = tc
	active := len(c.tracked)
	ircCtx := c.runCtx
	c.mu.Unlock()
	c.irc.Join(ircCtx, login)
	c.kickoffLiveEmoteEnsure(login)
	return WatchResponse{
		Channel:  login,
		Tracking: true,
		Active:   active,
		Max:      c.maxTracked,
		Message:  "tracking until stream ends",
		Sources:  []SourceStatus{{Source: "analytics_collector", State: "ready", Message: "tracking until offline"}},
	}
}

func (c *Collector) TouchForPrincipal(login, principalID string) {
	if principalID == "" {
		return
	}
	login = normalizeLogin(login)
	now := time.Now().UTC()
	c.mu.Lock()
	defer c.mu.Unlock()
	tc := c.tracked[login]
	if tc == nil {
		return
	}
	c.touchPrincipalLocked(tc, principalID, now)
}

func (c *Collector) ReleaseForPrincipal(login, principalID string) {
	if principalID == "" {
		return
	}
	login = normalizeLogin(login)
	c.mu.Lock()
	defer c.mu.Unlock()
	tc := c.tracked[login]
	if tc == nil || tc.refCounts == nil {
		return
	}
	if tc.refCounts[principalID] <= 1 {
		delete(tc.refCounts, principalID)
	} else {
		tc.refCounts[principalID]--
	}
}

func (c *Collector) SetPoolAlwaysTrack(login string, always bool) {
	login = normalizeLogin(login)
	c.mu.Lock()
	defer c.mu.Unlock()
	tc := c.tracked[login]
	if tc == nil {
		return
	}
	tc.poolAlwaysTrack = always
}

func (c *Collector) touchPrincipalLocked(tc *trackedChannel, principalID string, now time.Time) {
	if tc == nil {
		return
	}
	tc.lastViewedAt = now
	if principalID == "" {
		return
	}
	if tc.refCounts == nil {
		tc.refCounts = map[string]int{}
	}
	if tc.refCounts[principalID] == 0 {
		tc.refCounts[principalID] = 1
	}
}

func (c *Collector) channelRefCount(tc *trackedChannel) int {
	if tc == nil || len(tc.refCounts) == 0 {
		return 0
	}
	total := 0
	for _, n := range tc.refCounts {
		total += n
	}
	return total
}

func (c *Collector) shouldRetainTracked(login string, tc *trackedChannel) bool {
	if tc == nil {
		return false
	}
	if c.alwaysTracked[login] || tc.poolAlwaysTrack {
		return true
	}
	if len(tc.refCounts) == 0 {
		return false
	}
	return c.channelRefCount(tc) > 0
}

func (c *Collector) effectiveTrackingPriority(login string, tc *trackedChannel) int {
	if tc == nil {
		return TrackPriorityIdleNoRef
	}
	if c.alwaysTracked[login] {
		return TrackPriorityGlobalProtected
	}
	if tc.poolAlwaysTrack {
		return TrackPriorityPrincipalAlwaysTrack
	}
	if tc.watchPriority > 0 {
		return tc.watchPriority
	}
	if c.channelRefCount(tc) > 0 {
		return TrackPriorityManualWatch
	}
	return TrackPriorityIdleNoRef
}

func (c *Collector) evictOneForIncomingPriorityLocked(now time.Time, incomingPriority int) (string, int, bool) {
	type candidate struct {
		login    string
		tracked  *trackedChannel
		priority int
		idle     time.Duration
	}
	candidates := make([]candidate, 0, len(c.tracked))
	for login, tc := range c.tracked {
		victimPriority := c.effectiveTrackingPriority(login, tc)
		if !trackingPriorityCanPreempt(incomingPriority, victimPriority) {
			continue
		}
		idle := now.Sub(tc.lastViewedAt)
		candidates = append(candidates, candidate{login: login, tracked: tc, priority: victimPriority, idle: idle})
	}
	if len(candidates) == 0 {
		return "", 0, false
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].priority != candidates[j].priority {
			return candidates[i].priority < candidates[j].priority
		}
		if candidates[i].idle != candidates[j].idle {
			return candidates[i].idle > candidates[j].idle
		}
		if !candidates[i].tracked.addedAt.Equal(candidates[j].tracked.addedAt) {
			return candidates[i].tracked.addedAt.Before(candidates[j].tracked.addedAt)
		}
		return candidates[i].login < candidates[j].login
	})
	login := candidates[0].login
	priority := candidates[0].priority
	delete(c.tracked, login)
	return login, priority, true
}

func (c *Collector) TrackedStreamID(login string) string {
	login = normalizeLogin(login)
	c.mu.Lock()
	defer c.mu.Unlock()
	if tc := c.tracked[login]; tc != nil {
		return tc.currentStreamID
	}
	return ""
}

// ForceUntrack stops IRC collection for login immediately (lease release / collector handoff).
func (c *Collector) ForceUntrack(login string) {
	if c == nil {
		return
	}
	login = normalizeLogin(login)
	if login == "" {
		return
	}
	var part bool
	c.mu.Lock()
	if _, ok := c.tracked[login]; ok {
		delete(c.tracked, login)
		part = true
	}
	c.mu.Unlock()
	if part && c.irc != nil {
		c.irc.Part(context.Background(), login)
	}
}

func (c *Collector) evictIdleChannels(now time.Time) {
	var evict []string
	c.mu.Lock()
	for login, tc := range c.tracked {
		if c.shouldRetainTracked(login, tc) {
			continue
		}
		if tc.lastViewedAt.IsZero() || now.Sub(tc.lastViewedAt) < c.idleTTL {
			continue
		}
		evict = append(evict, login)
	}
	for _, login := range evict {
		delete(c.tracked, login)
	}
	c.mu.Unlock()
	for _, login := range evict {
		c.irc.Part(context.Background(), login)
	}
}

func (c *Collector) SetAlwaysTracked(ctx context.Context, login string, always bool, skipStoreWrite ...bool) WatchResponse {
	login = normalizeLogin(login)
	skipStore := len(skipStoreWrite) > 0 && skipStoreWrite[0]
	c.mu.Lock()
	if always {
		c.alwaysTracked[login] = true
		if _, ok := c.tracked[login]; !ok {
			c.tracked[login] = &trackedChannel{
				login:        login,
				addedAt:      time.Now().UTC(),
				lastViewedAt: time.Now().UTC(),
				refCounts:    map[string]int{},
			}
			c.irc.Join(c.runCtx, login)
		}
		if !skipStore {
			_ = c.store.AddAlwaysTracked(ctx, login)
		}
	} else {
		delete(c.alwaysTracked, login)
		if tc, ok := c.tracked[login]; ok && tc.currentStreamID == "" {
			c.irc.Part(c.runCtx, login)
			delete(c.tracked, login)
		}
		if !skipStore {
			_ = c.store.RemoveAlwaysTracked(ctx, login)
		}
	}
	active := len(c.tracked)
	c.mu.Unlock()
	if always {
		c.kickoffLiveEmoteEnsure(login)
	}

	return WatchResponse{
		Channel:  login,
		Tracking: true,
		Active:   active,
		Max:      c.maxTracked,
		Message:  fmt.Sprintf("always tracked set to %v", always),
		Sources:  []SourceStatus{{Source: "analytics_collector", State: "ready"}},
	}
}

func (c *Collector) IsTracking(login string) bool {
	if c == nil {
		return false
	}
	login = normalizeLogin(login)
	c.mu.Lock()
	defer c.mu.Unlock()
	_, ok := c.tracked[login]
	return ok
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

type TrackingSnapshot struct {
	TrackedChannels []string `json:"trackedChannels"`
	AlwaysTracked   []string `json:"alwaysTracked"`
	Active          int      `json:"active"`
	Max             int      `json:"max"`
}

func (c *Collector) TrackingSnapshot() TrackingSnapshot {
	c.mu.Lock()
	defer c.mu.Unlock()
	tracked := make([]string, 0, len(c.tracked))
	for login := range c.tracked {
		tracked = append(tracked, login)
	}
	sort.Strings(tracked)
	always := make([]string, 0, len(c.alwaysTracked))
	for login := range c.alwaysTracked {
		always = append(always, login)
	}
	sort.Strings(always)
	return TrackingSnapshot{
		TrackedChannels: tracked,
		AlwaysTracked:   always,
		Active:          len(c.tracked),
		Max:             c.maxTracked,
	}
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
	fragments := c.enricher.Tokenize(channel, msg.Text, msg.Emotes)
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
			helixStreamID := stream.ID
			c.mu.Unlock()
			if wasOffline {
				c.irc.Join(c.runCtx, login)
			}
			if err := c.store.UpsertLiveStream(ctx, stream, profiles[login], now); err != nil {
				c.log.Warn("analytics upsert stream failed", "stream_id", helixStreamID, "err", err)
				continue
			}
			c.mu.Lock()
			if tracked := c.tracked[login]; tracked != nil {
				tracked.currentStreamID = helixStreamID
			}
			c.mu.Unlock()
			c.addViewerSample(helixStreamID, now, stream.ViewerCount)
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
			if !c.shouldRetainTracked(login, tracked) {
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
	c.evictIdleChannels(now)
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
			continue
		}
		c.recordFirstRollupMetrics(acc.streamID, acc.minute)
	}
}

func (c *Collector) cleanup(ctx context.Context) {
	cutoff := time.Now().UTC().Add(-c.retention)
	if err := c.store.PurgeOlderThan(ctx, cutoff); err != nil {
		c.log.Warn("analytics retention cleanup failed", "err", err)
	}
}

func (c *Collector) scheduleVodIDResolve(streamID string) {
	go c.resolveVodIDWithRetry(streamID, time.Now().UTC())
}

// resolveVodIDWithRetry attempts to resolve the VOD id for a just-closed stream
// via Helix and stitch the live-collected heatmap points to the historical
// record. Live rollups are already stored under the stream id at minute-bucket
// offsets, so the stitch is the VOD association itself: once SetStreamVodID
// links the VOD, the historical stream record exposes the same minute-bucket
// scored points the live session produced (Requirement 19.3).
//
// Resolution is bounded to a 60-minute auto-retry window after close. If the VOD
// id does not resolve within that window, the rollups are retained under the
// live stream id and the record is marked soft "unlinked" so a later sync,
// manual retry, or validated extension hint can complete the association
// (Requirement 19.4).
func (c *Collector) resolveVodIDWithRetry(streamID string, closedAt time.Time) {
	offsets := c.vodResolveOffsets
	if len(offsets) == 0 {
		offsets = []time.Duration{0}
	}
	for attempt, offset := range offsets {
		if wait := closedAt.Add(offset).Sub(c.nowUTC()); wait > 0 {
			time.Sleep(wait)
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
			// Already linked (e.g. by a concurrent sync); nothing left to stitch.
			c.invalidatePulseBFFCache(ctx, rec.Login)
			return
		}
		broadcasterID := strings.TrimSpace(rec.BroadcasterID)
		if broadcasterID == "" {
			c.log.Debug("vod resolve skipped; broadcaster_id missing", "stream_id", streamID)
			c.markVodUnlinked(ctx, streamID, rec.Login)
			return
		}
		vodID, err := c.helix.VideoIDByStreamID(ctx, broadcasterID, streamID)
		if err != nil {
			c.log.Warn("helix vod resolve on stream close failed", "stream_id", streamID, "attempt", attempt+1, "err", err)
			continue
		}
		if vodID == "" {
			continue
		}
		// Stitch: link the live-collected rollups (same stream id + minute-bucket
		// offsets) to the historical VOD record.
		if err := c.store.SetStreamVodID(ctx, streamID, vodID, "helix_stream_match"); err != nil {
			c.log.Warn("failed to persist vod_id on stream close", "stream_id", streamID, "err", err)
			return
		}
		c.invalidatePulseBFFCache(ctx, rec.Login)
		c.log.Info("stitched live heatmap points to historical vod", "stream_id", streamID, "vod_id", vodID, "attempt", attempt+1)
		return
	}
	// VOD id did not resolve within the auto-retry window: retain live points
	// under the live stream id and mark the record soft-unlinked.
	ctx := context.Background()
	rec, _ := c.store.StreamByID(ctx, streamID)
	login := ""
	if rec != nil {
		login = rec.Login
	}
	c.markVodUnlinked(ctx, streamID, login)
}

func (c *Collector) markVodUnlinked(ctx context.Context, streamID, login string) {
	if err := c.store.MarkStreamVodUnlinked(ctx, streamID); err != nil {
		c.log.Warn("failed to mark stream vod unlinked", "stream_id", streamID, "err", err)
		return
	}
	c.invalidatePulseBFFCache(ctx, login)
	c.log.Info("vod unresolved within window; retained live heatmap points as unlinked", "stream_id", streamID)
}

func (c *Collector) invalidatePulseBFFCache(ctx context.Context, login string) {
	c.invalidatePulseCachesMode(ctx, login, "", false)
}

func (c *Collector) invalidatePulseCaches(ctx context.Context, login, streamID string) {
	c.invalidatePulseCachesMode(ctx, login, streamID, true)
}

func (c *Collector) invalidatePulseCachesMode(ctx context.Context, login, streamID string, includeHeatmap bool) {
	if c == nil || c.pulseCacheInvalidator == nil {
		return
	}
	c.pulseCacheInvalidator(ctx, login, streamID, includeHeatmap)
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
