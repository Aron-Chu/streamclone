package analytics

import (
	"hash/fnv"
	"sync"
)

const gqlDedupeShards = 32

type gqlCommentDeduper struct {
	shards [gqlDedupeShards]sync.Map
}

func (d *gqlCommentDeduper) markSeen(id string) (alreadySeen bool) {
	if id == "" {
		return false
	}
	h := fnv.New32a()
	_, _ = h.Write([]byte(id))
	shard := &d.shards[h.Sum32()%gqlDedupeShards]
	_, loaded := shard.LoadOrStore(id, struct{}{})
	return loaded
}

type gqlCommentsMap struct {
	shards [gqlDedupeShards]struct {
		mu   sync.Mutex
		data map[int][]string
	}
}

func (m *gqlCommentsMap) append(minute int, text string) {
	shard := &m.shards[uint32(minute)%gqlDedupeShards]
	shard.mu.Lock()
	defer shard.mu.Unlock()
	if shard.data == nil {
		shard.data = make(map[int][]string)
	}
	shard.data[minute] = append(shard.data[minute], text)
}

func (m *gqlCommentsMap) mergeInto(dst map[int][]string) {
	for i := range m.shards {
		shard := &m.shards[i]
		shard.mu.Lock()
		for minute, texts := range shard.data {
			dst[minute] = append(dst[minute], texts...)
		}
		shard.mu.Unlock()
	}
}

func (m *gqlCommentsMap) countMinuteRange(startMinute, endMinute int) int {
	if startMinute > endMinute {
		return 0
	}
	total := 0
	for minute := startMinute; minute <= endMinute; minute++ {
		shard := &m.shards[uint32(minute)%gqlDedupeShards]
		shard.mu.Lock()
		total += len(shard.data[minute])
		shard.mu.Unlock()
	}
	return total
}

// extractMinuteRangeInto moves minute buckets into dst so incremental rollup patches
// can read comments before the final parallel merge.
func (m *gqlCommentsMap) extractMinuteRangeInto(dst map[int][]string, startMinute, endMinute int) {
	if dst == nil || startMinute > endMinute {
		return
	}
	for minute := startMinute; minute <= endMinute; minute++ {
		shard := &m.shards[uint32(minute)%gqlDedupeShards]
		shard.mu.Lock()
		if texts, ok := shard.data[minute]; ok && len(texts) > 0 {
			dst[minute] = append(dst[minute], texts...)
			delete(shard.data, minute)
		}
		shard.mu.Unlock()
	}
}
