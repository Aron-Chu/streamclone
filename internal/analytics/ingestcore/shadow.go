package ingestcore

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sync"
	"time"

	"streamclone/internal/metrics"
)

// CompareKey normalizes shadow comparison dimensions.
type CompareKey struct {
	StreamID string
	Channel  string
	Minute   time.Time
	Closed   bool
}

func (k CompareKey) String() string {
	status := "open"
	if k.Closed {
		status = "closed"
	}
	return fmt.Sprintf("%s|%s|%s|%s", k.StreamID, k.Channel, k.Minute.UTC().Format(time.RFC3339), status)
}

// ShadowRecord is one compared minute rollup.
type ShadowRecord struct {
	Key           CompareKey `json:"key"`
	LegacyChat    int        `json:"legacyChat"`
	ShadowChat    int        `json:"shadowChat"`
	LegacyEmotes  int        `json:"legacyEmotes"`
	ShadowEmotes  int        `json:"shadowEmotes"`
	LegacyViewers int        `json:"legacyViewers"`
	ShadowViewers int        `json:"shadowViewers"`
	Match         bool       `json:"match"`
	Reason        string     `json:"reason,omitempty"`
	RecordedAt    time.Time  `json:"recordedAt"`
}

// ShadowComparer compares legacy vs ingest-core rollups with normalization.
type ShadowComparer struct {
	cfg       Config
	mu        sync.Mutex
	legacy    map[string]RollupSnapshot
	shadow    map[string]RollupSnapshot
	artifact  *ShadowArtifactWriter
	tolerance float64
}

// NewShadowComparer builds a comparer.
func NewShadowComparer(cfg Config) *ShadowComparer {
	return &ShadowComparer{
		cfg:       cfg,
		legacy:    map[string]RollupSnapshot{},
		shadow:    map[string]RollupSnapshot{},
		artifact:  NewShadowArtifactWriter(cfg.ShadowArtifactDir),
		tolerance: cfg.ShadowTolerancePct,
	}
}

// RecordLegacy stores legacy path snapshot.
func (c *ShadowComparer) RecordLegacy(channel string, snap RollupSnapshot) {
	if c == nil {
		return
	}
	key := c.key(channel, snap)
	c.mu.Lock()
	c.legacy[key] = snap
	c.mu.Unlock()
	c.tryCompare(key, channel)
}

// RecordShadow stores ingest-core snapshot (no PG write in dual-read mode).
func (c *ShadowComparer) RecordShadow(channel string, snap RollupSnapshot) {
	if c == nil {
		return
	}
	if !c.allowChannel(channel) {
		return
	}
	key := c.key(channel, snap)
	c.mu.Lock()
	c.shadow[key] = snap
	c.mu.Unlock()
	c.tryCompare(key, channel)
}

func (c *ShadowComparer) allowChannel(channel string) bool {
	if len(c.cfg.ShadowAllowlist) == 0 {
		return true
	}
	_, ok := c.cfg.ShadowAllowlist[normalizeLogin(channel)]
	return ok
}

func (c *ShadowComparer) key(channel string, snap RollupSnapshot) string {
	return CompareKey{
		StreamID: snap.StreamID,
		Channel:  normalizeLogin(channel),
		Minute:   snap.Minute.UTC().Truncate(time.Minute),
		Closed:   snap.Closed,
	}.String()
}

func (c *ShadowComparer) tryCompare(key, channel string) {
	c.mu.Lock()
	leg, okL := c.legacy[key]
	sh, okS := c.shadow[key]
	c.mu.Unlock()
	if !okL || !okS {
		return
	}
	rec := ShadowRecord{
		Key: CompareKey{
			StreamID: sh.StreamID,
			Channel:  normalizeLogin(channel),
			Minute:   sh.Minute.UTC().Truncate(time.Minute),
			Closed:   sh.Closed,
		},
		LegacyChat:    leg.ChatCount,
		ShadowChat:    sh.ChatCount,
		LegacyEmotes:  leg.TotalEmoteCount,
		ShadowEmotes:  sh.TotalEmoteCount,
		LegacyViewers: leg.ViewerSamples,
		ShadowViewers: sh.ViewerSamples,
		RecordedAt:    time.Now().UTC(),
	}
	rec.Match, rec.Reason = withinTolerance(rec, c.tolerance)
	if !rec.Key.Closed && !rec.Match {
		if rec.Reason != "" {
			rec.Reason = "open_minute_excluded:" + rec.Reason
		} else {
			rec.Reason = "open_minute_excluded"
		}
	}
	if rec.Match {
		metrics.IngestShadowCompareMatchTotal.Inc()
	} else {
		metrics.IngestShadowCompareMismatchTotal.Inc()
	}
	if c.artifact != nil {
		_ = c.artifact.Append(rec)
	}
}

func withinTolerance(rec ShadowRecord, pct float64) (bool, string) {
	if pct <= 0 {
		pct = 2
	}
	if rec.LegacyChat == 0 && rec.ShadowChat == 0 {
		return true, ""
	}
	if rec.LegacyChat > 0 {
		diff := math.Abs(float64(rec.ShadowChat-rec.LegacyChat)) / float64(rec.LegacyChat) * 100
		if diff > pct {
			return false, fmt.Sprintf("chat_diff_pct=%.2f", diff)
		}
	} else if rec.ShadowChat > 0 {
		return false, "legacy_zero_shadow_nonzero"
	}
	if rec.LegacyViewers != rec.ShadowViewers && absInt(rec.LegacyViewers-rec.ShadowViewers) > 1 {
		return false, "viewer_sample_mismatch"
	}
	return true, ""
}

func absInt(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

const defaultShadowArtifactMaxBytes = 128 << 20 // 128 MiB before rotate

// ShadowArtifactWriter appends JSONL compare records.
type ShadowArtifactWriter struct {
	dir      string
	maxBytes int64
	mu       sync.Mutex
}

func NewShadowArtifactWriter(dir string) *ShadowArtifactWriter {
	if dir == "" {
		dir = "runtime/ingest-shadow"
	}
	return &ShadowArtifactWriter{dir: dir, maxBytes: defaultShadowArtifactMaxBytes}
}

func (w *ShadowArtifactWriter) rotateIfNeeded(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if info.Size() < w.maxBytes {
		return nil
	}
	rotated := filepath.Join(w.dir, "latest-"+time.Now().UTC().Format("20060102T150405Z")+".jsonl")
	return os.Rename(path, rotated)
}

func (w *ShadowArtifactWriter) Append(rec ShadowRecord) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if err := os.MkdirAll(w.dir, 0o755); err != nil {
		return err
	}
	path := filepath.Join(w.dir, "latest.jsonl")
	if err := w.rotateIfNeeded(path); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	b, err := json.Marshal(rec)
	if err != nil {
		return err
	}
	_, err = f.Write(append(b, '\n'))
	return err
}
