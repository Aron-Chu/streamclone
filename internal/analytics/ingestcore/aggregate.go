package ingestcore

import (
	"hash/fnv"
	"strconv"
	"strings"
	"sync"
	"time"

	"streamclone/internal/metrics"
)

type queuedChat struct {
	msg       ParsedChat
	enqueued  time.Time
	emoteKeys []string
	sevenTV   int
}

type shardWorker struct {
	id       int
	inbox    chan queuedChat
	agg      *Aggregator
	cfg      Config
	dropTier func(IngestTier)
}

// Aggregator owns per-stream minute rings sharded across workers.
type Aggregator struct {
	cfg      Config
	shards   []shardWorker
	streams  sync.Map // streamID -> *MinuteRing
	loginMap sync.Map // login -> streamID
}

// NewAggregator builds fixed shard workers.
func NewAggregator(cfg Config) *Aggregator {
	n := cfg.ShardCount
	if n < 1 {
		n = 1
	}
	a := &Aggregator{cfg: cfg, shards: make([]shardWorker, n)}
	for i := range a.shards {
		a.shards[i] = shardWorker{
			id:    i,
			inbox: make(chan queuedChat, cfg.ShardQueueSize),
			agg:   a,
			cfg:   cfg,
			dropTier: func(t IngestTier) {
				metrics.IngestMessagesDroppedTotal.WithLabelValues(t.Label()).Inc()
			},
		}
	}
	return a
}

// Start launches shard worker goroutines.
func (a *Aggregator) Start() {
	for i := range a.shards {
		go a.shards[i].loop()
	}
}

func shardIndex(streamID string, n int) int {
	if streamID == "" || n <= 1 {
		return 0
	}
	h := fnv.New32a()
	_, _ = h.Write([]byte(streamID))
	return int(h.Sum32() % uint32(n))
}

// BindStream associates login with streamID for IRC routing.
func (a *Aggregator) BindStream(login, streamID string) {
	login = normalizeLogin(login)
	if login == "" || streamID == "" {
		return
	}
	a.loginMap.Store(login, streamID)
}

// StreamIDForLogin resolves stream id for a channel login.
func (a *Aggregator) StreamIDForLogin(login string) string {
	login = normalizeLogin(login)
	if v, ok := a.loginMap.Load(login); ok {
		return v.(string)
	}
	return ""
}

// Enqueue tries to place a parsed chat message on the correct shard inbox.
func (a *Aggregator) Enqueue(msg ParsedChat, globalDepth func(int), p0Used, p0Max int) bool {
	if msg.StreamID == "" {
		msg.StreamID = a.StreamIDForLogin(msg.Channel)
	}
	if msg.StreamID == "" {
		return false
	}
	keys, seven := EmoteKeysFromFragments(msg.Fragments)
	item := queuedChat{
		msg:       msg,
		enqueued:  time.Now().UTC(),
		emoteKeys: keys,
		sevenTV:   seven,
	}
	idx := shardIndex(msg.StreamID, len(a.shards))
	sh := &a.shards[idx]

	// P0 reserved capacity: allow if under reserve OR normal queue has space.
	if msg.Tier == TierP0Always {
		if p0Used < p0Max || len(sh.inbox) < cap(sh.inbox) {
			select {
			case sh.inbox <- item:
				metrics.IngestShardQueueDepth.WithLabelValues(labelShard(idx)).Set(float64(len(sh.inbox)))
				return true
			default:
				sh.dropTier(msg.Tier)
				return false
			}
		}
	}
	if len(sh.inbox) >= cap(sh.inbox) {
		if msg.Tier != TierP0Always {
			sh.dropTier(msg.Tier)
			return false
		}
	}
	select {
	case sh.inbox <- item:
		if globalDepth != nil {
			globalDepth(0) // caller updates global depth
		}
		metrics.IngestShardQueueDepth.WithLabelValues(labelShard(idx)).Set(float64(len(sh.inbox)))
		age := time.Since(item.enqueued).Seconds()
		metrics.IngestShardQueueAgeSeconds.WithLabelValues(labelShard(idx)).Observe(age)
		return true
	default:
		if msg.Tier != TierP0Always {
			sh.dropTier(msg.Tier)
		} else {
			sh.dropTier(msg.Tier)
		}
		return false
	}
}

func (sw *shardWorker) loop() {
	for item := range sw.inbox {
		age := time.Since(item.enqueued).Seconds()
		metrics.IngestShardQueueAgeSeconds.WithLabelValues(labelShard(sw.id)).Observe(age)
		sw.process(item)
		metrics.IngestShardQueueDepth.WithLabelValues(labelShard(sw.id)).Set(float64(len(sw.inbox)))
	}
}

func (sw *shardWorker) process(item queuedChat) {
	streamID := item.msg.StreamID
	minute := item.msg.Timestamp.UTC().Truncate(time.Minute)
	var ring *MinuteRing
	if v, ok := sw.agg.streams.Load(streamID); ok {
		ring = v.(*MinuteRing)
	} else {
		ring = newMinuteRing(streamID, minute, sw.cfg.TopEmotesPerMinute)
		sw.agg.streams.Store(streamID, ring)
	}
	for _, key := range item.emoteKeys {
		is7 := false
		for _, frag := range item.msg.Fragments {
			if emoteKeyFromParts(frag.Provider, frag.ID, frag.C) == key && isSevenTVProvider(frag.Provider) {
				is7 = true
				break
			}
		}
		ring.AddChat(item.msg.Timestamp, key, is7)
	}
	if len(item.emoteKeys) == 0 && strings.TrimSpace(item.msg.Text) != "" {
		ring.AddChat(item.msg.Timestamp, "", false)
	}
}

// CollectFlushCandidates gathers rollups ready to flush across all streams.
func (a *Aggregator) CollectFlushCandidates(forceOpen bool, openMinInterval time.Duration, lastOpenFlush map[string]time.Time, now time.Time) []RollupSnapshot {
	var out []RollupSnapshot
	a.streams.Range(func(key, value any) bool {
		ring := value.(*MinuteRing)
		out = append(out, ring.DrainClosed(a.cfg.TopEmotesPerMinute)...)
		if forceOpen {
			streamID := key.(string)
			if last, ok := lastOpenFlush[streamID]; ok && now.Sub(last) < openMinInterval {
				return true
			}
			snap := ring.SnapshotOpen(a.cfg.TopEmotesPerMinute)
			if snap.StreamID != "" {
				out = append(out, snap)
				lastOpenFlush[streamID] = now
			}
		}
		return true
	})
	return out
}

// DrainAll flushes all rings on shutdown.
func (a *Aggregator) DrainAll() []RollupSnapshot {
	var out []RollupSnapshot
	a.streams.Range(func(_, value any) bool {
		ring := value.(*MinuteRing)
		out = append(out, ring.DrainAll(a.cfg.TopEmotesPerMinute)...)
		return true
	})
	return out
}

func labelShard(id int) string {
	return strconv.Itoa(id)
}
