package dict

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"

	"streamclone/internal/emoteimage"
	"streamclone/internal/metrics"
)

type Entry struct {
	URL       string `json:"u"`
	ZeroWidth bool   `json:"zw"`
	ID        string `json:"id,omitempty"`
	Provider  string `json:"provider,omitempty"`
}

type Dict struct {
	rdb     *redis.Client
	cdnBase string
	ttl     time.Duration
}

const defaultDictionaryTTL = 24 * time.Hour

// ErrDictMissing is returned by AddEmote and RemoveEmote when the channel has
// no dictionary in Redis (expired or never built). Patching it would create a
// one-entry dictionary that readers take for the channel's whole set, so the
// caller rebuilds the full dictionary from PostgreSQL instead.
var ErrDictMissing = errors.New("emote dictionary not in redis")

// rebuildSuffix names the staging key a rebuild fills before it RENAMEs it
// over the live key. The staging key only exists inside one MULTI/EXEC.
const rebuildSuffix = ":rebuild"

const (
	defaultLegacyBackfillBatchSize  = 100
	defaultLegacyBackfillBatchPause = 50 * time.Millisecond
)

// LegacyBackfillOptions controls the one-time conversion of historical
// dictionaries from no-expiry keys into bounded cache entries.
type LegacyBackfillOptions struct {
	ScanCount  int64
	BatchSize  int
	BatchPause time.Duration
	TTLJitter  time.Duration
}

func New(rdb *redis.Client, cdnBase string) *Dict {
	return NewWithTTL(rdb, cdnBase, defaultDictionaryTTL)
}

func NewWithTTL(rdb *redis.Client, cdnBase string, ttl time.Duration) *Dict {
	return &Dict{rdb: rdb, cdnBase: cdnBase, ttl: ttl}
}

func (d *Dict) Ping(ctx context.Context) error {
	return d.rdb.Ping(ctx).Err()
}

func channelKey(login string) string {
	return fmt.Sprintf("channel:emotes:%s", login)
}

func deltaChannel(channel string) string {
	return fmt.Sprintf("emotes:delta:%s", channel)
}

func (d *Dict) EmoteURL(emoteID, scale string) string {
	return fmt.Sprintf("%s/%s/%s.webp", d.cdnBase, emoteID, scale)
}

func (d *Dict) BrowserURL(emoteID, provider, providerEmoteID, scale string) string {
	if providerEmoteID != "" {
		return emoteimage.ExtensionBrowserURL(provider, emoteID, providerEmoteID)
	}
	return d.EmoteURL(emoteID, scale)
}

// Rebuild replaces the channel's dictionary atomically.
//
// One MULTI/EXEC block fills a staging key, gives it the dictionary TTL and
// RENAMEs it over the live key (RENAME carries the expiry across). Readers see
// the old dictionary or the new one, never an empty or partial one, and with
// a TTL configured the live key never exists without an expiry. If Redis
// rejects a queued command (for example OOM under maxmemory-policy
// noeviction) the whole block is discarded and the old dictionary stays as it
// was; it still gets the TTL via EXPIRE NX, which Redis allows when out of
// memory. An empty emote list removes the dictionary.
func (d *Dict) Rebuild(ctx context.Context, login string, emotes []EmoteEntry) (err error) {
	started := time.Now()
	result := "error"
	defer func() {
		if err == nil {
			result = "success"
		}
		metrics.EmoteDictionaryRebuilds.WithLabelValues(result).Inc()
		metrics.EmoteDictionaryRebuildDuration.WithLabelValues(result).Observe(time.Since(started).Seconds())
	}()
	key := channelKey(login)
	staging := key + rebuildSuffix
	fields := make([]any, 0, 2*len(emotes))
	for _, e := range emotes {
		val, err := marshalEntry(d.BrowserURL(e.EmoteID, e.Provider, e.ProviderEmoteID, "1x"), e.ZeroWidth, e.EmoteID, e.Provider)
		if err != nil {
			return err
		}
		fields = append(fields, e.Name, val)
	}
	pipe := d.rdb.TxPipeline()
	pipe.Del(ctx, staging)
	if len(fields) == 0 {
		pipe.Unlink(ctx, key)
	} else {
		pipe.HSet(ctx, staging, fields...)
		if d.ttl > 0 {
			pipe.PExpire(ctx, staging, d.ttl)
		}
		pipe.Rename(ctx, staging, key)
	}
	if _, err = pipe.Exec(ctx); err != nil {
		if d.ttl > 0 {
			_ = d.rdb.ExpireNX(ctx, key, d.ttl).Err()
		}
		return err
	}
	ev, err := marshalDelta("reload", "", "", false)
	if err != nil {
		return err
	}
	err = d.rdb.Publish(ctx, deltaChannel(login), ev).Err()
	return err
}

// patchAddScript sets one entry only when the dictionary exists and renews
// its TTL (ARGV[1] milliseconds; 0 = no expiry configured) in the same atomic
// step. Returns 1 when patched, 0 when the dictionary is missing.
const patchAddScript = `if redis.call('EXISTS', KEYS[1]) == 0 then return 0 end
redis.call('HSET', KEYS[1], ARGV[2], ARGV[3])
if tonumber(ARGV[1]) > 0 then redis.call('PEXPIRE', KEYS[1], ARGV[1]) end
return 1`

// patchRemoveScript deletes one entry only when the dictionary exists and
// renews its TTL (the hash disappears with its last entry).
const patchRemoveScript = `if redis.call('EXISTS', KEYS[1]) == 0 then return 0 end
redis.call('HDEL', KEYS[1], ARGV[2])
if tonumber(ARGV[1]) > 0 and redis.call('EXISTS', KEYS[1]) == 1 then redis.call('PEXPIRE', KEYS[1], ARGV[1]) end
return 1`

var (
	patchAdd    = redis.NewScript(patchAddScript)
	patchRemove = redis.NewScript(patchRemoveScript)
)

// AddEmote adds one entry to an existing dictionary and renews its TTL. It
// returns ErrDictMissing (and publishes nothing) when the dictionary is not
// in Redis; the caller then rebuilds it in full.
func (d *Dict) AddEmote(ctx context.Context, login, name, emoteID string, zeroWidth bool) error {
	val, err := marshalEntry(d.EmoteURL(emoteID, "1x"), zeroWidth, emoteID, "custom")
	if err != nil {
		return err
	}
	if err := d.patch(ctx, patchAdd, login, name, val); err != nil {
		return err
	}
	ev, err := marshalDelta("add", name, d.EmoteURL(emoteID, "1x"), zeroWidth)
	if err != nil {
		return err
	}
	return d.rdb.Publish(ctx, deltaChannel(login), ev).Err()
}

// RemoveEmote removes one entry from an existing dictionary and renews its
// TTL. It returns ErrDictMissing when the dictionary is not in Redis.
func (d *Dict) RemoveEmote(ctx context.Context, login, name string) error {
	if err := d.patch(ctx, patchRemove, login, name, ""); err != nil {
		return err
	}
	ev, err := marshalDelta("remove", name, "", false)
	if err != nil {
		return err
	}
	return d.rdb.Publish(ctx, deltaChannel(login), ev).Err()
}

func (d *Dict) patch(ctx context.Context, script *redis.Script, login, name, val string) error {
	ttlMS := int64(0)
	if d.ttl > 0 {
		ttlMS = d.ttl.Milliseconds()
	}
	n, err := script.Run(ctx, d.rdb, []string{channelKey(login)}, strconv.FormatInt(ttlMS, 10), name, val).Int64()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrDictMissing
	}
	return nil
}

// BackfillLegacyTTLs attaches a bounded, deterministically jittered expiry to
// pre-existing dictionaries that have no expiry.
func (d *Dict) BackfillLegacyTTLs(ctx context.Context, scanCount int64) (int, error) {
	return d.BackfillLegacyTTLsWithOptions(ctx, LegacyBackfillOptions{
		ScanCount:  scanCount,
		BatchSize:  defaultLegacyBackfillBatchSize,
		BatchPause: defaultLegacyBackfillBatchPause,
		TTLJitter:  d.ttl,
	})
}

// BackfillLegacyTTLsWithOptions is bounded by Redis SCAN and paced pipeline
// batches. EXPIRE NX never shortens an existing expiry. Deterministic jitter
// spreads the first legacy expiry wave across a stable window without storing
// per-key migration state.
func (d *Dict) BackfillLegacyTTLsWithOptions(ctx context.Context, opts LegacyBackfillOptions) (int, error) {
	result := "success"
	scanned := 0
	updated := 0
	defer func() {
		metrics.EmoteDictionaryLegacyBackfillRuns.WithLabelValues(result).Inc()
		metrics.EmoteDictionaryLegacyLastScanKeys.Set(float64(scanned))
		metrics.EmoteDictionaryLegacyLastTTLsAttached.Set(float64(updated))
		if result == "success" {
			metrics.EmoteDictionaryLegacyNonExpiringRemaining.Set(0)
		} else if result == "error" {
			metrics.EmoteDictionaryLegacyNonExpiringRemaining.Set(-1)
		}
	}()
	if d == nil || d.rdb == nil || d.ttl <= 0 {
		result = "disabled"
		return 0, nil
	}
	opts = normalizeLegacyBackfillOptions(opts, d.ttl)
	var cursor uint64
	for {
		keys, next, err := d.rdb.Scan(ctx, cursor, "channel:emotes:*", opts.ScanCount).Result()
		if err != nil {
			result = "error"
			return updated, err
		}
		scanned += len(keys)
		metrics.EmoteDictionaryLegacyKeysScanned.Add(float64(len(keys)))
		for start := 0; start < len(keys); start += opts.BatchSize {
			end := start + opts.BatchSize
			if end > len(keys) {
				end = len(keys)
			}
			pipe := d.rdb.Pipeline()
			commands := make([]*redis.BoolCmd, 0, end-start)
			for _, key := range keys[start:end] {
				ttl := d.ttl + deterministicTTLJitter(key, opts.TTLJitter)
				commands = append(commands, pipe.ExpireNX(ctx, key, ttl))
			}
			if _, err := pipe.Exec(ctx); err != nil && err != redis.Nil {
				result = "error"
				return updated, err
			}
			attached := 0
			for _, command := range commands {
				ok, err := command.Result()
				if err == nil && ok {
					updated++
					attached++
				}
			}
			metrics.EmoteDictionaryLegacyTTLsAttached.Add(float64(attached))
			if end < len(keys) || next != 0 {
				if err := waitForBackfillPace(ctx, opts.BatchPause); err != nil {
					result = "error"
					return updated, err
				}
			}
		}
		cursor = next
		if cursor == 0 {
			return updated, nil
		}
	}
}

func normalizeLegacyBackfillOptions(opts LegacyBackfillOptions, ttl time.Duration) LegacyBackfillOptions {
	if opts.ScanCount <= 0 {
		opts.ScanCount = 500
	}
	if opts.BatchSize <= 0 {
		opts.BatchSize = defaultLegacyBackfillBatchSize
	}
	if opts.BatchPause < 0 {
		opts.BatchPause = 0
	}
	if opts.TTLJitter < 0 {
		opts.TTLJitter = 0
	}
	if opts.TTLJitter == 0 {
		opts.TTLJitter = ttl
	}
	return opts
}

func deterministicTTLJitter(key string, max time.Duration) time.Duration {
	if max <= 0 {
		return 0
	}
	h := fnv.New64a()
	_, _ = h.Write([]byte(key))
	return time.Duration(h.Sum64() % (uint64(max) + 1))
}

func waitForBackfillPace(ctx context.Context, pause time.Duration) error {
	if pause <= 0 {
		return nil
	}
	timer := time.NewTimer(pause)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

type EmoteEntry struct {
	Name            string
	EmoteID         string
	ProviderEmoteID string
	ZeroWidth       bool
	Provider        string
}

func marshalEntry(url string, zw bool, id string, provider string) (string, error) {
	b, err := json.Marshal(Entry{URL: url, ZeroWidth: zw, ID: id, Provider: provider})
	return string(b), err
}

type deltaEvent struct {
	Action string `json:"action"`
	Emote  struct {
		Name      string `json:"name"`
		URL       string `json:"u,omitempty"`
		ZeroWidth bool   `json:"zw,omitempty"`
	} `json:"emote"`
}

func marshalDelta(action, name, url string, zw bool) (string, error) {
	ev := deltaEvent{Action: action}
	ev.Emote.Name = name
	ev.Emote.URL = url
	ev.Emote.ZeroWidth = zw
	b, err := json.Marshal(ev)
	return string(b), err
}

func MarshalEntry(url string, zw bool) (string, error) {
	return marshalEntry(url, zw, "", "")
}

func MarshalEntryWithMetadata(url string, zw bool, id string, provider string) (string, error) {
	return marshalEntry(url, zw, id, provider)
}

func MarshalDelta(action, name, url string, zw bool) (string, error) {
	return marshalDelta(action, name, url, zw)
}

func IdempotencyKey(emoteID, sourceHash string) string {
	return emoteID + ":" + sourceHash
}
