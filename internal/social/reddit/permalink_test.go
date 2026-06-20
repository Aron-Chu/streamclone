package reddit

import "testing"

func TestCanonicalPermalink(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{
			"https://old.reddit.com/r/LivestreamFail/comments/1uaby3n/destiny_reacts/",
			"https://www.reddit.com/r/LivestreamFail/comments/1uaby3n/destiny_reacts/",
		},
		{
			"/r/LivestreamFail/comments/1uaby3n/title/",
			"https://www.reddit.com/r/LivestreamFail/comments/1uaby3n/title/",
		},
		{
			"https://www.reddit.com/r/LivestreamFail/comments/abc/title/?utm_source=share",
			"https://www.reddit.com/r/LivestreamFail/comments/abc/title/",
		},
		{
			"https://redd.it/abc123",
			"https://www.reddit.com/comments/abc123/",
		},
	}
	for _, tc := range tests {
		if got := CanonicalPermalink(tc.in); got != tc.want {
			t.Fatalf("CanonicalPermalink(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
