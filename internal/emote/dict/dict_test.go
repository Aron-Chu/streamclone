package dict

import (
	"encoding/json"
	"testing"
	"time"
)

func TestMarshalEntry(t *testing.T) {
	val, err := MarshalEntry("https://cdn.example.com/emotes/abc/1x.webp", false)
	if err != nil {
		t.Fatal(err)
	}
	var e Entry
	if err := json.Unmarshal([]byte(val), &e); err != nil {
		t.Fatal(err)
	}
	if e.URL != "https://cdn.example.com/emotes/abc/1x.webp" {
		t.Fatalf("unexpected url: %q", e.URL)
	}
	if e.ZeroWidth {
		t.Fatal("expected zw=false")
	}
}

func TestMarshalEntryZeroWidth(t *testing.T) {
	val, err := MarshalEntry("https://cdn.example.com/emotes/xyz/1x.webp", true)
	if err != nil {
		t.Fatal(err)
	}
	var e Entry
	if err := json.Unmarshal([]byte(val), &e); err != nil {
		t.Fatal(err)
	}
	if !e.ZeroWidth {
		t.Fatal("expected zw=true")
	}
}

func TestMarshalEntryFields(t *testing.T) {
	val, err := MarshalEntry("u", true)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(val), &m); err != nil {
		t.Fatal(err)
	}
	if _, ok := m["u"]; !ok {
		t.Fatal("missing field u")
	}
	if _, ok := m["zw"]; !ok {
		t.Fatal("missing field zw")
	}
}

func TestIdempotencyKey(t *testing.T) {
	k := IdempotencyKey("emote-id-1", "hashval")
	expected := "emote-id-1:hashval"
	if k != expected {
		t.Fatalf("expected %q got %q", expected, k)
	}
}

func TestIdempotencyKeyUnique(t *testing.T) {
	k1 := IdempotencyKey("id1", "hash1")
	k2 := IdempotencyKey("id2", "hash1")
	k3 := IdempotencyKey("id1", "hash2")
	if k1 == k2 || k1 == k3 || k2 == k3 {
		t.Fatal("keys should be distinct")
	}
}

func TestMarshalReloadDelta(t *testing.T) {
	val, err := MarshalDelta("reload", "", "", false)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(val), &m); err != nil {
		t.Fatal(err)
	}
	if m["action"] != "reload" {
		t.Fatalf("unexpected action: %+v", m)
	}
}

func TestBrowserURLPrefersProviderCDN(t *testing.T) {
	d := New(nil, "http://localhost:8090/emotes")
	localID := "75f49395-d5fc-41da-998c-880c6d8fddcb"
	if got := d.BrowserURL(localID, "bttv", "abcdef", "1x"); got != "https://cdn.betterttv.net/emote/abcdef/3x" {
		t.Fatalf("provider cdn url = %q", got)
	}
	if got := d.BrowserURL(localID, "bttv", "", "1x"); got != "http://localhost:8090/emotes/"+localID+"/1x.webp" {
		t.Fatalf("fallback url = %q", got)
	}
}

func TestDictionaryTTLDefaultsAndOverride(t *testing.T) {
	if got := New(nil, "").ttl; got != 24*time.Hour {
		t.Fatalf("default ttl = %s, want 24h", got)
	}
	if got := NewWithTTL(nil, "", 6*time.Hour).ttl; got != 6*time.Hour {
		t.Fatalf("override ttl = %s, want 6h", got)
	}
	if got := NewWithTTL(nil, "", 0).ttl; got != 0 {
		t.Fatalf("disabled ttl = %s, want 0", got)
	}
}

func TestDeterministicTTLJitterIsStableAndBounded(t *testing.T) {
	const max = 24 * time.Hour
	first := deterministicTTLJitter("channel:emotes:example", max)
	second := deterministicTTLJitter("channel:emotes:example", max)
	if first != second {
		t.Fatalf("jitter changed for same key: %s != %s", first, second)
	}
	if first < 0 || first > max {
		t.Fatalf("jitter = %s, want within [0,%s]", first, max)
	}
	if other := deterministicTTLJitter("channel:emotes:other", max); other == first {
		t.Fatalf("expected distinct keys to spread expiry, both got %s", first)
	}
}

func TestNormalizeLegacyBackfillOptions(t *testing.T) {
	const ttl = 24 * time.Hour
	got := normalizeLegacyBackfillOptions(LegacyBackfillOptions{
		BatchPause: -time.Second,
		TTLJitter:  -time.Second,
	}, ttl)
	if got.ScanCount != 500 {
		t.Fatalf("scan count = %d, want 500", got.ScanCount)
	}
	if got.BatchSize != 100 {
		t.Fatalf("batch size = %d, want 100", got.BatchSize)
	}
	if got.BatchPause != 0 {
		t.Fatalf("batch pause = %s, want 0", got.BatchPause)
	}
	if got.TTLJitter != ttl {
		t.Fatalf("ttl jitter = %s, want %s", got.TTLJitter, ttl)
	}
}
