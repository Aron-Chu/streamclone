package ingestcore

import (
	"sort"
	"strings"
	"time"
)

// MinuteCounters holds compact minute-level rollup counters (no raw messages).
type MinuteCounters struct {
	StreamID          string
	Minute            time.Time
	ViewerSum         int64
	ViewerSamples     int
	ViewerMax         int
	ViewerLatest      int
	ChatCount         int
	TotalEmoteCount   int
	SevenTVEmoteCount int
	Emotes            map[string]int
	Closed            bool
}

// MinuteRing keeps at most two minute slots per stream (closed + open).
type MinuteRing struct {
	topEmotes int
	slots     [2]*MinuteCounters
}

func newMinuteRing(streamID string, minute time.Time, topEmotes int) *MinuteRing {
	r := &MinuteRing{topEmotes: topEmotes}
	r.slots[1] = &MinuteCounters{
		StreamID: streamID,
		Minute:   minute.UTC().Truncate(time.Minute),
		Emotes:   map[string]int{},
	}
	return r
}

func (r *MinuteRing) current() *MinuteCounters {
	if r == nil {
		return nil
	}
	if r.slots[1] != nil {
		return r.slots[1]
	}
	return r.slots[0]
}

func (r *MinuteRing) ensureMinute(now time.Time) *MinuteCounters {
	if r == nil {
		return nil
	}
	minute := now.UTC().Truncate(time.Minute)
	cur := r.current()
	if cur == nil || cur.Minute != minute {
		r.rotate(minute)
		cur = r.current()
	}
	return cur
}

// AddChatMessage increments ChatCount once per accepted PRIVMSG.
func (r *MinuteRing) AddChatMessage(now time.Time) {
	cur := r.ensureMinute(now)
	if cur == nil {
		return
	}
	cur.ChatCount++
}

// AddEmote increments emote counters for one emote occurrence (repeats preserved).
func (r *MinuteRing) AddEmote(now time.Time, emoteKey string, isSevenTV bool) {
	if emoteKey == "" {
		return
	}
	cur := r.ensureMinute(now)
	if cur == nil {
		return
	}
	cur.TotalEmoteCount++
	if isSevenTV {
		cur.SevenTVEmoteCount++
	}
	if cur.Emotes == nil {
		cur.Emotes = map[string]int{}
	}
	cur.Emotes[emoteKey]++
}

// AddViewerSample records a Helix viewer sample on the open minute.
func (r *MinuteRing) AddViewerSample(count int) {
	cur := r.current()
	if cur == nil {
		return
	}
	cur.ViewerSum += int64(count)
	cur.ViewerSamples++
	if count > cur.ViewerMax {
		cur.ViewerMax = count
	}
	cur.ViewerLatest = count
}

func (r *MinuteRing) rotate(minute time.Time) {
	if r.slots[1] != nil {
		r.slots[1].Closed = true
		r.slots[0] = r.slots[1]
	}
	r.slots[1] = &MinuteCounters{
		StreamID: r.streamID(),
		Minute:   minute,
		Emotes:   map[string]int{},
	}
}

func (r *MinuteRing) streamID() string {
	if r.slots[1] != nil {
		return r.slots[1].StreamID
	}
	if r.slots[0] != nil {
		return r.slots[0].StreamID
	}
	return ""
}

// SnapshotRollup copies counters into a RollupSnapshot for flush/compare.
type RollupSnapshot struct {
	StreamID          string
	Minute            time.Time
	ViewerAvg         int
	ViewerMax         int
	ViewerLatest      int
	ViewerSamples     int
	ChatCount         int
	TotalEmoteCount   int
	SevenTVEmoteCount int
	Emotes            map[string]int
	Closed            bool
}

func (r *MinuteRing) SnapshotOpen(topN int) RollupSnapshot {
	cur := r.current()
	if cur == nil {
		return RollupSnapshot{}
	}
	return snapshotFromCounters(cur, topN, false)
}

// DrainClosed returns closed minute snapshots and clears closed slot.
func (r *MinuteRing) DrainClosed(topN int) []RollupSnapshot {
	if r == nil || r.slots[0] == nil || !r.slots[0].Closed {
		return nil
	}
	out := []RollupSnapshot{snapshotFromCounters(r.slots[0], topN, true)}
	r.slots[0] = nil
	return out
}

// DrainAll returns all slots for shutdown flush.
func (r *MinuteRing) DrainAll(topN int) []RollupSnapshot {
	var out []RollupSnapshot
	if r.slots[0] != nil {
		out = append(out, snapshotFromCounters(r.slots[0], topN, true))
		r.slots[0] = nil
	}
	if r.slots[1] != nil {
		out = append(out, snapshotFromCounters(r.slots[1], topN, false))
		r.slots[1] = nil
	}
	return out
}

func snapshotFromCounters(c *MinuteCounters, topN int, closed bool) RollupSnapshot {
	viewerAvg := 0
	if c.ViewerSamples > 0 {
		viewerAvg = int((c.ViewerSum + int64(c.ViewerSamples/2)) / int64(c.ViewerSamples))
	}
	emotes := topNEmotes(c.Emotes, topN)
	return RollupSnapshot{
		StreamID:          c.StreamID,
		Minute:            c.Minute,
		ViewerAvg:         viewerAvg,
		ViewerMax:         c.ViewerMax,
		ViewerLatest:      c.ViewerLatest,
		ViewerSamples:     c.ViewerSamples,
		ChatCount:         c.ChatCount,
		TotalEmoteCount:   c.TotalEmoteCount,
		SevenTVEmoteCount: c.SevenTVEmoteCount,
		Emotes:            emotes,
		Closed:            closed || c.Closed,
	}
}

func topNEmotes(emotes map[string]int, n int) map[string]int {
	if len(emotes) == 0 {
		return map[string]int{}
	}
	if n <= 0 || len(emotes) <= n {
		out := make(map[string]int, len(emotes))
		for k, v := range emotes {
			out[k] = v
		}
		return out
	}
	type kv struct {
		k string
		v int
	}
	items := make([]kv, 0, len(emotes))
	for k, v := range emotes {
		items = append(items, kv{k, v})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].v != items[j].v {
			return items[i].v > items[j].v
		}
		return items[i].k < items[j].k
	})
	out := make(map[string]int, n)
	for i := 0; i < n && i < len(items); i++ {
		out[items[i].k] = items[i].v
	}
	return out
}

func emoteKeyFromParts(provider, id, name string) string {
	provider = strings.ToLower(strings.TrimSpace(provider))
	id = strings.TrimSpace(id)
	name = strings.TrimSpace(name)
	if provider != "" && id != "" {
		return provider + ":" + id + ":" + name
	}
	if name != "" {
		return name
	}
	return "unknown"
}

func isSevenTVProvider(provider string) bool {
	p := strings.ToLower(strings.TrimSpace(provider))
	return p == "7tv" || p == "seventv"
}
