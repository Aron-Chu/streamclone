package analytics

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"
)

type fakeProvider struct {
	streams map[string]LiveStream
}

func (f fakeProvider) StreamsByLogin(context.Context, []string) (map[string]LiveStream, error) {
	return f.streams, nil
}

func (f fakeProvider) UsersByLogin(context.Context, []string) (map[string]UserProfile, error) {
	return map[string]UserProfile{}, nil
}

func (f fakeProvider) VideoIDByStreamID(context.Context, string, string) (string, error) {
	return "", nil
}

type fakeJoiner struct {
	joined       []string
	parted       []string
	joinCanceled bool
}

func (f *fakeJoiner) Join(ctx context.Context, channel string) {
	f.joined = append(f.joined, channel)
	select {
	case <-ctx.Done():
		f.joinCanceled = true
	default:
	}
}

func (f *fakeJoiner) Part(_ context.Context, channel string) {
	f.parted = append(f.parted, channel)
}

type fakeStore struct {
	closed []string
	purged time.Time
}

func (f *fakeStore) UpsertLiveStream(context.Context, LiveStream, UserProfile, time.Time) error {
	return nil
}

func (f *fakeStore) CloseStream(_ context.Context, streamID string, _ time.Time) error {
	f.closed = append(f.closed, streamID)
	return nil
}

func (f *fakeStore) UpsertMinuteRollup(context.Context, string, MinuteRollup) error {
	return nil
}

func (f *fakeStore) PurgeOlderThan(_ context.Context, cutoff time.Time) error {
	f.purged = cutoff
	return nil
}

func (f *fakeStore) AddAlwaysTracked(context.Context, string) error {
	return nil
}

func (f *fakeStore) RemoveAlwaysTracked(context.Context, string) error {
	return nil
}

func (f *fakeStore) StreamByID(context.Context, string) (*StreamRecord, error) {
	return nil, nil
}

func (f *fakeStore) SetStreamVodID(context.Context, string, string, string) error {
	return nil
}

func TestWatchPoolCap(t *testing.T) {
	store := &fakeStore{}
	joiner := &fakeJoiner{}
	c := NewCollector(store, fakeProvider{}, joiner, nil, nilLogger(), 2, time.Hour, 30*24*time.Hour, 200)
	first := c.Watch(context.Background(), "one")
	second := c.Watch(context.Background(), "two")
	third := c.Watch(context.Background(), "three")
	if !first.Tracking || !second.Tracking {
		t.Fatalf("expected first two channels to track: %+v %+v", first, second)
	}
	if third.Tracking || third.Active != 2 || third.Max != 2 {
		t.Fatalf("expected third channel to be capped: %+v", third)
	}
}

func TestWatchUsesCollectorContextForIRCJoin(t *testing.T) {
	store := &fakeStore{}
	joiner := &fakeJoiner{}
	c := NewCollector(store, fakeProvider{}, joiner, nil, nilLogger(), 2, time.Hour, 30*24*time.Hour, 200)

	requestCtx, cancel := context.WithCancel(context.Background())
	cancel()
	resp := c.Watch(requestCtx, "one")
	if !resp.Tracking {
		t.Fatalf("expected channel to track: %+v", resp)
	}
	if joiner.joinCanceled {
		t.Fatalf("IRC join used the canceled request context")
	}
}

func TestTwoOfflinePollsCloseAndUntrack(t *testing.T) {
	store := &fakeStore{}
	joiner := &fakeJoiner{}
	c := NewCollector(store, fakeProvider{streams: map[string]LiveStream{}}, joiner, nil, nilLogger(), 50, time.Hour, 30*24*time.Hour, 200)
	c.tracked["chan"] = &trackedChannel{login: "chan", currentStreamID: "stream-1"}

	c.pollOnce(context.Background())
	if c.ActiveCount() != 1 || len(store.closed) != 0 {
		t.Fatalf("first offline poll should keep tracking, active=%d closed=%v", c.ActiveCount(), store.closed)
	}
	c.pollOnce(context.Background())
	if c.ActiveCount() != 0 {
		t.Fatalf("second offline poll should untrack channel, active=%d", c.ActiveCount())
	}
	if len(store.closed) != 1 || store.closed[0] != "stream-1" {
		t.Fatalf("expected stream close, got %v", store.closed)
	}
	if len(joiner.parted) != 1 || joiner.parted[0] != "chan" {
		t.Fatalf("expected IRC part, got %v", joiner.parted)
	}
}

func TestHelixBatchingLimit(t *testing.T) {
	items := make([]string, 205)
	for i := range items {
		items[i] = "chan"
	}
	chunks := chunkStrings(items, 100)
	if len(chunks) != 3 || len(chunks[0]) != 100 || len(chunks[1]) != 100 || len(chunks[2]) != 5 {
		t.Fatalf("unexpected chunks: %d/%d/%d/%d", len(chunks), len(chunks[0]), len(chunks[1]), len(chunks[2]))
	}
}

func TestMinuteRollupTruncatesTopEmotes(t *testing.T) {
	acc := &minuteAccumulator{streamID: "s", minute: time.Unix(0, 0), emotes: map[string]int{}}
	for i := 0; i < 250; i++ {
		acc.emotes[string(rune('a'+(i%26)))+time.Unix(int64(i), 0).Format("150405")] = i
	}
	rollup := acc.rollup(200)
	if len(rollup.Emotes) != 200 {
		t.Fatalf("expected top 200 emotes, got %d", len(rollup.Emotes))
	}
}

func TestCleanupUsesRetentionCutoff(t *testing.T) {
	store := &fakeStore{}
	c := NewCollector(store, fakeProvider{}, &fakeJoiner{}, nil, nilLogger(), 50, time.Hour, 48*time.Hour, 200)
	c.cleanup(context.Background())
	if store.purged.IsZero() {
		t.Fatalf("expected purge cutoff")
	}
	if age := time.Since(store.purged); age < 47*time.Hour || age > 49*time.Hour {
		t.Fatalf("expected roughly 48h cutoff, got %s", age)
	}
}

func nilLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}
