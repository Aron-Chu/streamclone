package dict

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"streamclone/internal/redisutil/redistest"
)

const testKey = "channel:emotes:somechannel"

// newFakeDict returns a Dict with the default TTL served by the in-process
// fake.
func newFakeDict(t *testing.T) (*Dict, *redistest.Server) {
	t.Helper()
	return newFakeDictWithTTL(t, -1)
}

// newFakeDictWithTTL is newFakeDict with an explicit TTL (ttl < 0 means the
// New default; 0 means expiry disabled, as NewWithTTL allows). The Go
// stand-ins for the two patch scripts are keyed by the exact Lua source, so
// changing a script without changing its stand-in here fails the tests
// (NOSCRIPT) instead of testing stale behaviour.
func newFakeDictWithTTL(t *testing.T, ttl time.Duration) (*Dict, *redistest.Server) {
	t.Helper()
	srv := redistest.New()
	srv.HandleScript(patchAddScript, func(st *redistest.State, keys, args []string) (any, error) {
		if !st.Exists(keys[0]) {
			return int64(0), nil
		}
		st.HSet(keys[0], args[1], args[2])
		if ttl := scriptTTL(t, args[0]); ttl > 0 {
			st.PExpire(keys[0], ttl)
		}
		return int64(1), nil
	})
	srv.HandleScript(patchRemoveScript, func(st *redistest.State, keys, args []string) (any, error) {
		if !st.Exists(keys[0]) {
			return int64(0), nil
		}
		st.HDel(keys[0], args[1])
		if ttl := scriptTTL(t, args[0]); ttl > 0 && st.Exists(keys[0]) {
			st.PExpire(keys[0], ttl)
		}
		return int64(1), nil
	})
	rdb := srv.Client()
	t.Cleanup(func() { _ = rdb.Close() })
	if ttl < 0 {
		return New(rdb, "/emotes"), srv
	}
	return NewWithTTL(rdb, "/emotes", ttl), srv
}

func scriptTTL(t *testing.T, ms string) time.Duration {
	n, err := strconv.ParseInt(ms, 10, 64)
	if err != nil {
		t.Errorf("script ttl %q: %v", ms, err)
	}
	return time.Duration(n) * time.Millisecond
}

func entries(names ...string) []EmoteEntry {
	out := make([]EmoteEntry, 0, len(names))
	for i, n := range names {
		out = append(out, EmoteEntry{Name: n, EmoteID: fmt.Sprintf("id-%d", i), Provider: "custom"})
	}
	return out
}

func fieldNames(h map[string]string) []string {
	out := make([]string, 0, len(h))
	for k := range h {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func assertAllKeysExpire(t *testing.T, srv *redistest.Server) {
	t.Helper()
	if keys := srv.KeysWithoutTTL(); len(keys) != 0 {
		t.Fatalf("keys without an expiry: %v", keys)
	}
}

func assertTTL(t *testing.T, srv *redistest.Server, key string, want time.Duration) {
	t.Helper()
	ttl, ok := srv.TTL(key)
	if !ok {
		t.Fatalf("%s missing", key)
	}
	if ttl != want {
		t.Fatalf("TTL(%s) = %v, want %v", key, ttl, want)
	}
}

func deltaTypes(srv *redistest.Server) []string {
	var out []string
	for _, m := range srv.Published() {
		var ev deltaEvent
		_ = json.Unmarshal([]byte(m.Payload), &ev)
		out = append(out, m.Channel+":"+ev.Action)
	}
	return out
}

// EMOTE_DICTIONARY_TTL=0 keeps its meaning (no expiry): writes stay atomic
// and exists-only, they just do not set a TTL.
func TestDisabledTTLStaysAtomicWithoutExpiry(t *testing.T) {
	d, srv := newFakeDictWithTTL(t, 0)
	ctx := context.Background()
	if err := d.Rebuild(ctx, "somechannel", entries("KEKW", "Pog")); err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, c := range srv.Commands() {
		names = append(names, c[0])
	}
	want := []string{"multi", "del", "hset", "rename", "exec", "publish"}
	if !reflect.DeepEqual(names, want) {
		t.Fatalf("commands = %v, want %v", names, want)
	}
	if err := d.AddEmote(ctx, "somechannel", "Added", "id-x", false); err != nil {
		t.Fatal(err)
	}
	if err := d.RemoveEmote(ctx, "somechannel", "KEKW"); err != nil {
		t.Fatal(err)
	}
	if got := fieldNames(srv.Hash(testKey)); !reflect.DeepEqual(got, []string{"Added", "Pog"}) {
		t.Fatalf("fields = %v", got)
	}
	if ttl, _ := srv.TTL(testKey); ttl >= 0 {
		t.Fatalf("TTL = %v with expiry disabled", ttl)
	}
	if keys := srv.Keys(); !reflect.DeepEqual(keys, []string{testKey}) {
		t.Fatalf("keys = %v", keys)
	}
	if err := d.AddEmote(ctx, "nochannel", "y", "id-y", false); !errors.Is(err, ErrDictMissing) {
		t.Fatalf("AddEmote on missing dict = %v, want ErrDictMissing", err)
	}
}

func TestRebuildWritesDictWithTTLAndNoStagingKey(t *testing.T) {
	d, srv := newFakeDict(t)
	ctx := context.Background()
	if err := d.Rebuild(ctx, "somechannel", entries("KEKW", "OMEGALUL", "Pog")); err != nil {
		t.Fatal(err)
	}
	if got := fieldNames(srv.Hash(testKey)); !reflect.DeepEqual(got, []string{"KEKW", "OMEGALUL", "Pog"}) {
		t.Fatalf("fields = %v", got)
	}
	assertTTL(t, srv, testKey, defaultDictionaryTTL)
	if keys := srv.Keys(); !reflect.DeepEqual(keys, []string{testKey}) {
		t.Fatalf("keys = %v, want only the live dictionary", keys)
	}
	assertAllKeysExpire(t, srv)
	if got := deltaTypes(srv); !reflect.DeepEqual(got, []string{"emotes:delta:somechannel:reload"}) {
		t.Fatalf("deltas = %v", got)
	}
}

// Every command that writes the dictionary runs inside one MULTI/EXEC, and
// the live key is only ever written by RENAME of a staging key that already
// has its expiry.
func TestRebuildIsOneTransactionEndingInRename(t *testing.T) {
	d, srv := newFakeDict(t)
	if err := d.Rebuild(context.Background(), "somechannel", entries("a", "b")); err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, c := range srv.Commands() {
		names = append(names, c[0])
	}
	want := []string{"multi", "del", "hset", "pexpire", "rename", "exec", "publish"}
	if !reflect.DeepEqual(names, want) {
		t.Fatalf("commands = %v, want %v", names, want)
	}
	cmds := srv.Commands()
	staging := testKey + rebuildSuffix
	if cmds[2][1] != staging || cmds[3][1] != staging {
		t.Fatalf("hset/pexpire target %q/%q, want staging key %q", cmds[2][1], cmds[3][1], staging)
	}
	if cmds[4][1] != staging || cmds[4][2] != testKey {
		t.Fatalf("rename %v, want %s -> %s", cmds[4], staging, testKey)
	}
}

// A legacy dictionary written before dictionaries had an expiry is replaced
// whole, stale entries included, and gains an expiry.
func TestRebuildReplacesLegacyNoTTLDict(t *testing.T) {
	d, srv := newFakeDict(t)
	srv.SeedHash(testKey, map[string]string{"Stale": "{}", "KEKW": "{}"}, 0)
	if keys := srv.KeysWithoutTTL(); len(keys) != 1 {
		t.Fatalf("seed: keys without TTL = %v", keys)
	}
	if err := d.Rebuild(context.Background(), "somechannel", entries("KEKW", "Pog")); err != nil {
		t.Fatal(err)
	}
	if got := fieldNames(srv.Hash(testKey)); !reflect.DeepEqual(got, []string{"KEKW", "Pog"}) {
		t.Fatalf("fields = %v (stale entry kept?)", got)
	}
	assertTTL(t, srv, testKey, defaultDictionaryTTL)
	assertAllKeysExpire(t, srv)
}

func TestRebuildEmptyRemovesDict(t *testing.T) {
	d, srv := newFakeDict(t)
	srv.SeedHash(testKey, map[string]string{"KEKW": "{}"}, 0)
	if err := d.Rebuild(context.Background(), "somechannel", nil); err != nil {
		t.Fatal(err)
	}
	if keys := srv.Keys(); len(keys) != 0 {
		t.Fatalf("keys = %v, want none", keys)
	}
}

func TestRebuildExpiresAfterTTL(t *testing.T) {
	d, srv := newFakeDict(t)
	if err := d.Rebuild(context.Background(), "somechannel", entries("KEKW")); err != nil {
		t.Fatal(err)
	}
	srv.Advance(defaultDictionaryTTL - time.Second)
	if srv.Hash(testKey) == nil {
		t.Fatal("dictionary expired early")
	}
	srv.Advance(time.Second)
	if keys := srv.Keys(); len(keys) != 0 {
		t.Fatalf("keys after TTL = %v, want none", keys)
	}
}

// Redis rejects HSET when it is at maxmemory with noeviction. Inside MULTI
// that aborts the whole block (EXECABORT): the old dictionary must survive
// untouched, no staging key may be left behind, and the old dictionary must
// end up with an expiry (EXPIRE is allowed under OOM). The pre-fix code
// deleted the dictionary first and then lost the HSETs, blanking the channel.
func TestRebuildRejectedUnderOOMKeepsOldDictAndExpiresIt(t *testing.T) {
	for _, legacy := range []bool{true, false} {
		t.Run(fmt.Sprintf("legacyNoTTL=%v", legacy), func(t *testing.T) {
			d, srv := newFakeDict(t)
			old := map[string]string{"KEKW": `{"u":"old"}`, "Pog": `{"u":"old"}`}
			seedTTL := time.Duration(0)
			if !legacy {
				seedTTL = time.Hour
			}
			srv.SeedHash(testKey, old, seedTTL)
			oom := redistest.Error("OOM command not allowed when used memory > 'maxmemory'.")
			srv.FailWhen(func(cmd []string) error {
				if cmd[0] == "hset" {
					return oom
				}
				return nil
			})
			err := d.Rebuild(context.Background(), "somechannel", entries("New1", "New2"))
			if err == nil {
				t.Fatal("Rebuild succeeded under OOM")
			}
			if got := srv.Hash(testKey); !reflect.DeepEqual(got, old) {
				t.Fatalf("old dictionary changed: %v", got)
			}
			if keys := srv.Keys(); !reflect.DeepEqual(keys, []string{testKey}) {
				t.Fatalf("keys = %v (staging key left behind?)", keys)
			}
			assertAllKeysExpire(t, srv)
			want := defaultDictionaryTTL
			if !legacy {
				want = time.Hour // NX: an existing expiry is left alone
			}
			assertTTL(t, srv, testKey, want)
			if got := deltaTypes(srv); len(got) != 0 {
				t.Fatalf("published %v after a failed rebuild", got)
			}
		})
	}
}

// legacyRebuild is the pre-fix algorithm (plain pipeline: DEL, then one HSET
// per entry, no expiry). It is kept here only as the negative control for
// the partial-read test below.
func legacyRebuild(ctx context.Context, d *Dict, login string, emotes []EmoteEntry) error {
	pipe := d.rdb.Pipeline()
	pipe.Del(ctx, channelKey(login))
	for _, e := range emotes {
		pipe.HSet(ctx, channelKey(login), e.Name, "{}")
	}
	_, err := pipe.Exec(ctx)
	return err
}

// A reader running HGETALL between the commands of a refresh must see the
// whole old dictionary or the whole new one. The negative control shows the
// check catches the pre-fix DEL-then-HSET pipeline.
func TestRebuildReaderNeverSeesPartialDict(t *testing.T) {
	oldNames := []string{"a", "b", "c"}
	newNames := []string{"a", "b", "c", "d", "e", "f"}
	consistent := func(got []string) bool {
		return reflect.DeepEqual(got, oldNames) || reflect.DeepEqual(got, newNames)
	}

	t.Run("negative control: legacy pipeline is caught", func(t *testing.T) {
		d, srv := newFakeDict(t)
		srv.SeedHash(testKey, map[string]string{"a": "{}", "b": "{}", "c": "{}"}, 0)
		reader := srv.Client()
		defer reader.Close()
		var bad []string
		srv.BetweenPipelineCommands(func() {
			m, err := reader.HGetAll(context.Background(), testKey).Result()
			if err != nil {
				t.Errorf("reader: %v", err)
				return
			}
			if got := fieldNames(m); !consistent(got) {
				bad = append(bad, strings.Join(got, ","))
			}
		})
		if err := legacyRebuild(context.Background(), d, "somechannel", entries(newNames...)); err != nil {
			t.Fatal(err)
		}
		if len(bad) == 0 {
			t.Fatal("legacy DEL-then-HSET pipeline showed no partial dictionary; the check is blind")
		}
		if ttl, _ := srv.TTL(testKey); ttl >= 0 {
			t.Fatalf("legacy rebuild unexpectedly left a TTL (%v)", ttl)
		}
	})

	t.Run("deterministic: no reader slot inside the transaction", func(t *testing.T) {
		d, srv := newFakeDict(t)
		srv.SeedHash(testKey, map[string]string{"a": "{}", "b": "{}", "c": "{}"}, 0)
		reader := srv.Client()
		defer reader.Close()
		var bad []string
		srv.BetweenPipelineCommands(func() {
			m, _ := reader.HGetAll(context.Background(), testKey).Result()
			if got := fieldNames(m); !consistent(got) {
				bad = append(bad, strings.Join(got, ","))
			}
		})
		if err := d.Rebuild(context.Background(), "somechannel", entries(newNames...)); err != nil {
			t.Fatal(err)
		}
		if len(bad) != 0 {
			t.Fatalf("reader saw partial dictionaries: %v", bad)
		}
	})

	t.Run("concurrent readers during repeated rebuilds", func(t *testing.T) {
		d, srv := newFakeDict(t)
		if err := d.Rebuild(context.Background(), "somechannel", entries(oldNames...)); err != nil {
			t.Fatal(err)
		}
		var stop atomic.Bool
		var wg sync.WaitGroup
		var reads atomic.Int64
		errs := make(chan string, 16)
		for r := 0; r < 4; r++ {
			reader := srv.Client()
			defer reader.Close()
			wg.Add(1)
			go func() {
				defer wg.Done()
				for !stop.Load() {
					m, err := reader.HGetAll(context.Background(), testKey).Result()
					if err != nil {
						errs <- err.Error()
						return
					}
					reads.Add(1)
					if got := fieldNames(m); !consistent(got) {
						select {
						case errs <- strings.Join(got, ","):
						default:
						}
						return
					}
				}
			}()
		}
		for i := 0; i < 300; i++ {
			names := oldNames
			if i%2 == 0 {
				names = newNames
			}
			if err := d.Rebuild(context.Background(), "somechannel", entries(names...)); err != nil {
				t.Fatal(err)
			}
			assertAllKeysExpire(t, srv)
		}
		stop.Store(true)
		wg.Wait()
		close(errs)
		for e := range errs {
			t.Errorf("reader saw %q", e)
		}
		if reads.Load() == 0 {
			t.Fatal("readers never ran")
		}
	})
}

func TestAddAndRemoveRenewTTL(t *testing.T) {
	d, srv := newFakeDict(t)
	ctx := context.Background()
	if err := d.Rebuild(ctx, "somechannel", entries("KEKW", "Pog")); err != nil {
		t.Fatal(err)
	}
	srv.Advance(20 * time.Hour)
	if err := d.AddEmote(ctx, "somechannel", "Added", "id-x", true); err != nil {
		t.Fatal(err)
	}
	assertTTL(t, srv, testKey, defaultDictionaryTTL)
	if _, ok := srv.Hash(testKey)["Added"]; !ok {
		t.Fatal("AddEmote did not add the entry")
	}
	srv.Advance(20 * time.Hour)
	if err := d.RemoveEmote(ctx, "somechannel", "KEKW"); err != nil {
		t.Fatal(err)
	}
	assertTTL(t, srv, testKey, defaultDictionaryTTL)
	if got := fieldNames(srv.Hash(testKey)); !reflect.DeepEqual(got, []string{"Added", "Pog"}) {
		t.Fatalf("fields = %v", got)
	}
	assertAllKeysExpire(t, srv)
	want := []string{"emotes:delta:somechannel:reload", "emotes:delta:somechannel:add", "emotes:delta:somechannel:remove"}
	if got := deltaTypes(srv); !reflect.DeepEqual(got, want) {
		t.Fatalf("deltas = %v, want %v", got, want)
	}
}

// Patching a missing (expired) dictionary must not create a one-entry
// dictionary that readers would take for the channel's whole set.
func TestPatchOnMissingDictWritesNothing(t *testing.T) {
	d, srv := newFakeDict(t)
	ctx := context.Background()
	if err := d.AddEmote(ctx, "somechannel", "KEKW", "id-1", false); !errors.Is(err, ErrDictMissing) {
		t.Fatalf("AddEmote = %v, want ErrDictMissing", err)
	}
	if err := d.RemoveEmote(ctx, "somechannel", "KEKW"); !errors.Is(err, ErrDictMissing) {
		t.Fatalf("RemoveEmote = %v, want ErrDictMissing", err)
	}
	if keys := srv.Keys(); len(keys) != 0 {
		t.Fatalf("keys = %v, want none", keys)
	}
	if got := srv.Published(); len(got) != 0 {
		t.Fatalf("published %v for a missing dictionary", got)
	}
}

// A patch to a legacy dictionary without a TTL gives it one.
func TestPatchOnLegacyNoTTLDictAddsTTL(t *testing.T) {
	d, srv := newFakeDict(t)
	ctx := context.Background()
	srv.SeedHash(testKey, map[string]string{"KEKW": "{}", "Pog": "{}"}, 0)
	if err := d.AddEmote(ctx, "somechannel", "New", "id-1", false); err != nil {
		t.Fatal(err)
	}
	assertTTL(t, srv, testKey, defaultDictionaryTTL)

	srv.SeedHash(testKey, map[string]string{"KEKW": "{}", "Pog": "{}"}, 0)
	if err := d.RemoveEmote(ctx, "somechannel", "Pog"); err != nil {
		t.Fatal(err)
	}
	assertTTL(t, srv, testKey, defaultDictionaryTTL)
}

func TestRemoveLastEntryRemovesKey(t *testing.T) {
	d, srv := newFakeDict(t)
	srv.SeedHash(testKey, map[string]string{"KEKW": "{}"}, time.Hour)
	if err := d.RemoveEmote(context.Background(), "somechannel", "KEKW"); err != nil {
		t.Fatal(err)
	}
	if keys := srv.Keys(); len(keys) != 0 {
		t.Fatalf("keys = %v, want none", keys)
	}
}

// Every write path, in any order, leaves only keys that expire.
func TestEveryWritePathLeavesTTL(t *testing.T) {
	d, srv := newFakeDict(t)
	ctx := context.Background()
	steps := []struct {
		name string
		run  func() error
	}{
		{"rebuild", func() error { return d.Rebuild(ctx, "somechannel", entries("a", "b")) }},
		{"add", func() error { return d.AddEmote(ctx, "somechannel", "c", "id-c", false) }},
		{"remove", func() error { return d.RemoveEmote(ctx, "somechannel", "a") }},
		{"rebuild other", func() error { return d.Rebuild(ctx, "other", entries("x")) }},
		{"add missing", func() error {
			if err := d.AddEmote(ctx, "nochannel", "y", "id-y", false); !errors.Is(err, ErrDictMissing) {
				return fmt.Errorf("want ErrDictMissing, got %v", err)
			}
			return nil
		}},
		{"rebuild empty", func() error { return d.Rebuild(ctx, "other", nil) }},
		{"rebuild again", func() error { return d.Rebuild(ctx, "somechannel", entries("a", "b", "c", "d")) }},
		{"remove all but one", func() error {
			for _, n := range []string{"a", "b", "c"} {
				if err := d.RemoveEmote(ctx, "somechannel", n); err != nil {
					return err
				}
			}
			return nil
		}},
	}
	for _, s := range steps {
		if err := s.run(); err != nil {
			t.Fatalf("%s: %v", s.name, err)
		}
		if keys := srv.KeysWithoutTTL(); len(keys) != 0 {
			t.Fatalf("after %s: keys without an expiry: %v", s.name, keys)
		}
	}
	if keys := srv.Keys(); !reflect.DeepEqual(keys, []string{testKey}) {
		t.Fatalf("keys = %v", keys)
	}
}
