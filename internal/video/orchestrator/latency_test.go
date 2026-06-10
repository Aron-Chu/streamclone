package orchestrator

import "testing"

func TestParseLatencyMode(t *testing.T) {
	tests := map[string]struct {
		mode string
		edge int
	}{
		"instant": {"instant", 1},
		"fast":    {"fast", 2},
		"stable":  {"stable", 3},
		"empty":   {"stable", 3},
	}
	for name, want := range tests {
		mode, edge := parseLatencyMode(name)
		if mode != want.mode || edge != want.edge {
			t.Fatalf("parseLatencyMode(%q) = (%q, %d), want (%q, %d)", name, mode, edge, want.mode, want.edge)
		}
	}
}

func TestAllowedProxyURL(t *testing.T) {
	cases := []struct {
		raw   string
		allow bool
	}{
		{"https://usher.ttvnw.net/api/channel/hls/ninja.m3u8", true},
		{"https://video-weaver.sfo01.hls.ttvnw.net/v1/playlist/foo.m3u8", true},
		{"file:///etc/passwd", false},
		{"http://127.0.0.1/secret", false},
		{"https://evil.example.com/playlist.m3u8", false},
	}
	for _, tc := range cases {
		_, err := allowedProxyURL(tc.raw)
		if tc.allow && err != nil {
			t.Fatalf("expected allow %q, got %v", tc.raw, err)
		}
		if !tc.allow && err == nil {
			t.Fatalf("expected reject %q", tc.raw)
		}
	}
}
