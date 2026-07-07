package render

import "testing"

func TestJobSourceKeyRoundTrip(t *testing.T) {
	key := JobSourceKey("abc123", []string{"1x", "2x"})
	hash, scales := ParseJobSourceKey(key)
	if hash != "abc123" {
		t.Fatalf("hash = %q", hash)
	}
	if len(scales) != 2 || scales[0] != "1x" || scales[1] != "2x" {
		t.Fatalf("scales = %#v", scales)
	}
}

func TestResolveScalesDefaultsToConfigured(t *testing.T) {
	got := ResolveScales(nil, []string{"1x"}, []string{"1x", "2x", "3x", "4x"})
	if len(got) != 1 || got[0] != "1x" {
		t.Fatalf("got %#v", got)
	}
}

func TestResolveScalesFiltersAllowed(t *testing.T) {
	got := ResolveScales([]string{"1x", "4x"}, []string{"1x"}, []string{"1x", "2x"})
	if len(got) != 1 || got[0] != "1x" {
		t.Fatalf("got %#v", got)
	}
}

func TestShouldEagerRenderDefaults(t *testing.T) {
	q := NewQueue(nil, nil, Config{}, nil)
	if q.ShouldEagerRender("twitch") {
		t.Fatal("twitch eager should default false")
	}
	if q.ShouldEagerRender("seventv") {
		t.Fatal("third-party eager should default false")
	}
	if !q.ShouldEagerRender("custom") {
		t.Fatal("custom uploads should eager render")
	}
}

func TestShouldEagerRenderLegacyFlags(t *testing.T) {
	q := NewQueue(nil, nil, Config{TwitchEager: true, ThirdpartyEager: true}, nil)
	if !q.ShouldEagerRender("twitch") || !q.ShouldEagerRender("ffz") {
		t.Fatal("expected eager when flags enabled")
	}
}
