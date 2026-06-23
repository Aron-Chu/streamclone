package analytics

import (
	"testing"
	"time"
)

func TestTTScrapeBackoffTTL(t *testing.T) {
	tests := []struct {
		reason string
		want   time.Duration
	}{
		{TTScrapeReasonCloudflareChallenge, 30 * time.Minute},
		{TTScrapeReasonHTTP403, 30 * time.Minute},
		{TTScrapeReasonBrowserCrash, 5 * time.Minute},
		{TTScrapeReasonScraperUnreachable, 2 * time.Minute},
		{TTScrapeReasonTimeoutNavigation, 10 * time.Minute},
		{TTScrapeReasonMissingMetaECS, 5 * time.Minute},
	}
	for _, tc := range tests {
		if got := ttScrapeBackoffTTL(tc.reason); got != tc.want {
			t.Fatalf("ttScrapeBackoffTTL(%q) = %v, want %v", tc.reason, got, tc.want)
		}
	}
}

func TestIsTTScrapeGlobalBackoffReason(t *testing.T) {
	if !isTTScrapeGlobalBackoffReason(TTScrapeReasonCloudflareChallenge) {
		t.Fatal("expected cloudflare to be global backoff reason")
	}
	if isTTScrapeGlobalBackoffReason(TTScrapeReasonMissingMetaECS) {
		t.Fatal("expected missing_meta_ecs to be stream-scoped only")
	}
}
