package analytics

import (
	"context"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"
)

type fakeProvider struct {
	streams map[string]LiveStream
	vodID   string
	vodErr  error
}

func (f fakeProvider) StreamsByLogin(context.Context, []string) (map[string]LiveStream, error) {
	return f.streams, nil
}

func (f fakeProvider) UsersByLogin(context.Context, []string) (map[string]UserProfile, error) {
	return map[string]UserProfile{}, nil
}

func (f fakeProvider) VideoIDByStreamID(context.Context, string, string) (string, error) {
	return f.vodID, f.vodErr
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
	mu       sync.Mutex
	closed   []string
	purged   time.Time
	record   *StreamRecord
	vodSets  []vodSet
	unlinked []string
	// vodDone, when non-nil, receives a signal after the async VOD resolve path
	// records an outcome (SetStreamVodID or MarkStreamVodUnlinked). It lets
	// close-triggered integration tests wait deterministically for the goroutine
	// spawned by scheduleVodIDResolve without sleeping.
	vodDone chan struct{}
}

type vodSet struct {
	streamID string
	vodID    string
	source   string
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
	return f.record, nil
}

func (f *fakeStore) SetStreamVodID(_ context.Context, streamID, vodID, source string) error {
	f.mu.Lock()
	f.vodSets = append(f.vodSets, vodSet{streamID: streamID, vodID: vodID, source: source})
	f.mu.Unlock()
	f.signalVodDone()
	return nil
}

func (f *fakeStore) MarkStreamVodUnlinked(_ context.Context, streamID string) error {
	f.mu.Lock()
	f.unlinked = append(f.unlinked, streamID)
	f.mu.Unlock()
	f.signalVodDone()
	return nil
}

func (f *fakeStore) signalVodDone() {
	if f.vodDone == nil {
		return
	}
	select {
	case f.vodDone <- struct{}{}:
	default:
	}
}

// vodOutcome returns lock-protected snapshots of the VOD resolution outcome so
// tests can read results written by the async resolve goroutine safely.
func (f *fakeStore) vodOutcome() (sets []vodSet, unlinked []string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]vodSet(nil), f.vodSets...), append([]string(nil), f.unlinked...)
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

func TestResolveVodIDStitchesWhenHelixResolves(t *testing.T) {
	store := &fakeStore{record: &StreamRecord{StreamID: "stream-1", BroadcasterID: "bc-1"}}
	provider := fakeProvider{vodID: "vod-9"}
	c := NewCollector(store, provider, &fakeJoiner{}, nil, nilLogger(), 50, time.Hour, 30*24*time.Hour, 200)
	c.vodResolveOffsets = []time.Duration{0}

	c.resolveVodIDWithRetry("stream-1", time.Now().UTC())

	if len(store.vodSets) != 1 {
		t.Fatalf("expected one vod stitch, got %v", store.vodSets)
	}
	got := store.vodSets[0]
	if got.streamID != "stream-1" || got.vodID != "vod-9" || got.source != "helix_stream_match" {
		t.Fatalf("unexpected stitch: %+v", got)
	}
	if len(store.unlinked) != 0 {
		t.Fatalf("did not expect unlinked marking, got %v", store.unlinked)
	}
}

func TestResolveVodIDMarksUnlinkedWhenUnresolved(t *testing.T) {
	store := &fakeStore{record: &StreamRecord{StreamID: "stream-1", BroadcasterID: "bc-1"}}
	provider := fakeProvider{vodID: ""}
	c := NewCollector(store, provider, &fakeJoiner{}, nil, nilLogger(), 50, time.Hour, 30*24*time.Hour, 200)
	c.vodResolveOffsets = []time.Duration{0, 0, 0}

	c.resolveVodIDWithRetry("stream-1", time.Now().UTC())

	if len(store.vodSets) != 0 {
		t.Fatalf("did not expect a stitch, got %v", store.vodSets)
	}
	if len(store.unlinked) != 1 || store.unlinked[0] != "stream-1" {
		t.Fatalf("expected stream marked unlinked, got %v", store.unlinked)
	}
}

func TestResolveVodIDSkipsWhenAlreadyLinked(t *testing.T) {
	store := &fakeStore{record: &StreamRecord{StreamID: "stream-1", BroadcasterID: "bc-1", VodID: "vod-existing"}}
	c := NewCollector(store, fakeProvider{vodID: "vod-9"}, &fakeJoiner{}, nil, nilLogger(), 50, time.Hour, 30*24*time.Hour, 200)
	c.vodResolveOffsets = []time.Duration{0}

	c.resolveVodIDWithRetry("stream-1", time.Now().UTC())

	if len(store.vodSets) != 0 || len(store.unlinked) != 0 {
		t.Fatalf("expected no-op for already linked stream, sets=%v unlinked=%v", store.vodSets, store.unlinked)
	}
}

// waitVodDone waits for the async VOD resolve goroutine (spawned by
// scheduleVodIDResolve on stream close) to record an outcome, failing the test
// if it does not finish promptly. The resolve path uses vodResolveOffsets={0}
// (or {0,0,0}) in tests, so it never sleeps for real minutes.
func waitVodDone(t *testing.T, done <-chan struct{}) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for VOD resolve to complete after stream close")
	}
}

// TestStreamCloseSchedulesVodStitch drives the full close path: two offline
// polls close the stream, which triggers scheduleVodIDResolve. With Helix
// resolving the VOD inside the window, the live-collected rollups are stitched
// to the historical record via SetStreamVodID(helix_stream_match).
// Requirements: 19.3.
func TestStreamCloseSchedulesVodStitch(t *testing.T) {
	store := &fakeStore{
		record:  &StreamRecord{StreamID: "stream-1", BroadcasterID: "bc-1"},
		vodDone: make(chan struct{}, 1),
	}
	provider := fakeProvider{streams: map[string]LiveStream{}, vodID: "vod-9"}
	c := NewCollector(store, provider, &fakeJoiner{}, nil, nilLogger(), 50, time.Hour, 30*24*time.Hour, 200)
	c.vodResolveOffsets = []time.Duration{0}
	c.tracked["chan"] = &trackedChannel{login: "chan", currentStreamID: "stream-1"}

	c.pollOnce(context.Background())
	if len(store.closed) != 0 {
		t.Fatalf("first offline poll should not close stream, closed=%v", store.closed)
	}
	c.pollOnce(context.Background())
	if len(store.closed) != 1 || store.closed[0] != "stream-1" {
		t.Fatalf("second offline poll should close stream, closed=%v", store.closed)
	}

	waitVodDone(t, store.vodDone)
	sets, unlinked := store.vodOutcome()
	if len(sets) != 1 {
		t.Fatalf("expected one vod stitch from close path, got %v", sets)
	}
	if sets[0].streamID != "stream-1" || sets[0].vodID != "vod-9" || sets[0].source != "helix_stream_match" {
		t.Fatalf("unexpected stitch: %+v", sets[0])
	}
	if len(unlinked) != 0 {
		t.Fatalf("did not expect unlinked marking on successful stitch, got %v", unlinked)
	}
}

// TestStreamCloseMarksUnlinkedWhenVodUnresolved drives the close path when Helix
// returns no VOD across every attempt in the resolution window. The live points
// are retained under the live stream id and the record is marked unlinked.
// Requirements: 19.4.
func TestStreamCloseMarksUnlinkedWhenVodUnresolved(t *testing.T) {
	store := &fakeStore{
		record:  &StreamRecord{StreamID: "stream-1", BroadcasterID: "bc-1"},
		vodDone: make(chan struct{}, 1),
	}
	provider := fakeProvider{streams: map[string]LiveStream{}, vodID: ""}
	c := NewCollector(store, provider, &fakeJoiner{}, nil, nilLogger(), 50, time.Hour, 30*24*time.Hour, 200)
	c.vodResolveOffsets = []time.Duration{0, 0, 0}
	c.tracked["chan"] = &trackedChannel{login: "chan", currentStreamID: "stream-1"}

	c.pollOnce(context.Background())
	c.pollOnce(context.Background())
	if len(store.closed) != 1 {
		t.Fatalf("expected stream close after two offline polls, closed=%v", store.closed)
	}

	waitVodDone(t, store.vodDone)
	sets, unlinked := store.vodOutcome()
	if len(sets) != 0 {
		t.Fatalf("did not expect a stitch when VOD never resolves, got %v", sets)
	}
	if len(unlinked) != 1 || unlinked[0] != "stream-1" {
		t.Fatalf("expected stream retained as unlinked, got %v", unlinked)
	}
}

// TestVodResolveWindowBoundedToFiveMinutes asserts the default resolution window
// honors the 5-minute bound: retries happen at 0 / 30s / 2m / 5m after close,
// with the final offset capping the window (Requirement 19.3).
func TestVodResolveWindowBoundedToFiveMinutes(t *testing.T) {
	c := NewCollector(&fakeStore{}, fakeProvider{}, &fakeJoiner{}, nil, nilLogger(), 50, time.Hour, 30*24*time.Hour, 200)
	want := []time.Duration{0, 30 * time.Second, 2 * time.Minute, 5 * time.Minute}
	if len(c.vodResolveOffsets) != len(want) {
		t.Fatalf("expected %d resolve offsets, got %v", len(want), c.vodResolveOffsets)
	}
	for i, w := range want {
		if c.vodResolveOffsets[i] != w {
			t.Fatalf("offset %d: expected %s, got %s", i, w, c.vodResolveOffsets[i])
		}
	}
	last := c.vodResolveOffsets[len(c.vodResolveOffsets)-1]
	if last != 5*time.Minute {
		t.Fatalf("resolution window must be bounded to 5m, got final offset %s", last)
	}
}
