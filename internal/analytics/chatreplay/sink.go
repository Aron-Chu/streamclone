package chatreplay

import (
	"context"
	"sync"
)

// Sink receives sanitized VOD chat messages during a GQL sync and persists them
// in batches. It is written by both the serial and parallel fetch paths
// alongside the rollup aggregation (see design: VOD Chat Replay Persistence).
//
// A nil sink (feature disabled) is a no-op: the *StoreSink methods are
// nil-receiver safe, and NopSink provides an explicit no-op implementation, so
// callers may invoke Add/FlushSegment/Flush without nil checks.
type Sink interface {
	// Add buffers one sanitized message. Safe for concurrent use by parallel
	// workers.
	Add(msg VODChatMessage)
	// FlushSegment persists buffered messages whose aligned minute falls within
	// [startMinute, endMinute] inclusive. Used by the parallel path's
	// per-segment checkpoint hook.
	FlushSegment(ctx context.Context, startMinute, endMinute int) error
	// Flush persists all remaining buffered messages. Called once at the end of
	// a fetch (and at serial checkpoint boundaries).
	Flush(ctx context.Context) error
}

// StoreSink is a Store-backed, segment-aware buffer implementing Sink. Messages
// are buffered in memory and written to Postgres in batches to avoid
// per-message round-trips during high-rate VODs.
//
// All methods are safe to call on a nil *StoreSink, in which case they are
// no-ops. This lets the analytics sync hold a nil *StoreSink when the chat
// replay feature is disabled.
type StoreSink struct {
	store *Store

	mu  sync.Mutex
	buf []VODChatMessage
}

// NewStoreSink constructs a StoreSink backed by the given Store. A nil store
// yields a sink whose operations are no-ops.
func NewStoreSink(store *Store) *StoreSink {
	return &StoreSink{store: store}
}

// Add buffers one message. Safe for concurrent use and on a nil receiver.
func (s *StoreSink) Add(msg VODChatMessage) {
	if s == nil || s.store == nil {
		return
	}
	if msg.StreamID == "" || msg.MessageID == "" {
		return
	}
	s.mu.Lock()
	s.buf = append(s.buf, msg)
	s.mu.Unlock()
}

// FlushSegment persists buffered messages whose offset minute falls within
// [startMinute, endMinute] inclusive, retaining the rest. Safe on a nil
// receiver.
func (s *StoreSink) FlushSegment(ctx context.Context, startMinute, endMinute int) error {
	if s == nil || s.store == nil {
		return nil
	}
	s.mu.Lock()
	if len(s.buf) == 0 {
		s.mu.Unlock()
		return nil
	}
	due := make([]VODChatMessage, 0, len(s.buf))
	keep := s.buf[:0:0]
	for _, msg := range s.buf {
		minute := msg.OffsetSeconds / 60
		if minute >= startMinute && minute <= endMinute {
			due = append(due, msg)
		} else {
			keep = append(keep, msg)
		}
	}
	s.buf = keep
	s.mu.Unlock()

	if len(due) == 0 {
		return nil
	}
	return s.store.BulkInsert(ctx, due)
}

// Flush persists all remaining buffered messages. Safe on a nil receiver.
func (s *StoreSink) Flush(ctx context.Context) error {
	if s == nil || s.store == nil {
		return nil
	}
	s.mu.Lock()
	if len(s.buf) == 0 {
		s.mu.Unlock()
		return nil
	}
	due := s.buf
	s.buf = nil
	s.mu.Unlock()

	return s.store.BulkInsert(ctx, due)
}

// NopSink is an explicit no-op Sink for when chat replay persistence is
// disabled but a non-nil Sink value is required.
type NopSink struct{}

func (NopSink) Add(VODChatMessage)                          {}
func (NopSink) FlushSegment(context.Context, int, int) error { return nil }
func (NopSink) Flush(context.Context) error                  { return nil }

// Compile-time checks that both implementations satisfy Sink.
var (
	_ Sink = (*StoreSink)(nil)
	_ Sink = NopSink{}
)
