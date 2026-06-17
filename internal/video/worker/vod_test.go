package worker

import "testing"

func TestValidVodID(t *testing.T) {
	cases := map[string]bool{
		"1234567890":  true,
		"98765432101": true,
		"1234":        false,
		"abc":         false,
		"":            false,
	}
	for in, want := range cases {
		if got := ValidVodID(in); got != want {
			t.Errorf("ValidVodID(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestVodSeekSeconds(t *testing.T) {
	if got := VodSeekSeconds(0); got != 0 {
		t.Fatalf("expected 0, got %d", got)
	}
	if got := VodSeekSeconds(100); got != 70 {
		t.Fatalf("expected 70, got %d", got)
	}
	if got := VodSeekSeconds(10); got != 0 {
		t.Fatalf("expected 0 for short offset, got %d", got)
	}
}

func TestVodRegistryAndMediaKeys(t *testing.T) {
	if got := VodRegistryKey("123"); got != "vod:123" {
		t.Fatalf("unexpected registry key: %q", got)
	}
	if got := VodMediaKey("123"); got != "vod_123" {
		t.Fatalf("unexpected media key: %q", got)
	}
}

func TestFormatVodTimeOffset(t *testing.T) {
	if got := FormatVodTimeOffset(0); got != "0s" {
		t.Fatalf("expected 0s, got %q", got)
	}
	if got := FormatVodTimeOffset(45); got != "45s" {
		t.Fatalf("expected 45s, got %q", got)
	}
	if got := FormatVodTimeOffset(4541); got != "1h15m41s" {
		t.Fatalf("expected 1h15m41s, got %q", got)
	}
}

func TestVodPageURLWithOffset(t *testing.T) {
	if got := VodPageURLWithOffset("2798379989", 0); got != "https://www.twitch.tv/videos/2798379989" {
		t.Fatalf("unexpected base url: %q", got)
	}
	want := "https://www.twitch.tv/videos/2798379989?t=1h15m11s"
	if got := VodPageURLWithOffset("2798379989", 4541); got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}
