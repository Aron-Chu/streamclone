package dict

import (
	"encoding/json"
	"testing"
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
