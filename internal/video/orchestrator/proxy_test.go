package orchestrator

import (
	"strings"
	"testing"
)

func TestFilterTwitchAdSegments_NoAds(t *testing.T) {
	manifest := `#EXTM3U
#EXT-X-VERSION:3
#EXT-X-TARGETDURATION:2
#EXT-X-MEDIA-SEQUENCE:100
#EXT-X-PROGRAM-DATE-TIME:2026-06-07T12:00:00Z
#EXTINF:2.000,
index-0001.ts
#EXT-X-PROGRAM-DATE-TIME:2026-06-07T12:00:02Z
#EXTINF:2.000,
index-0002.ts`

	sourceURL := "https://video-weaver.sfo01.hls.ttvnw.net/v1/playlist/xxx.m3u8"
	output := filterTwitchAdSegments(manifest, sourceURL)

	if !strings.Contains(output, "https://video-weaver.sfo01.hls.ttvnw.net/v1/playlist/index-0001.ts") {
		t.Errorf("Expected index-0001.ts to be resolved to absolute URL, got:\n%s", output)
	}
	if !strings.Contains(output, "https://video-weaver.sfo01.hls.ttvnw.net/v1/playlist/index-0002.ts") {
		t.Errorf("Expected index-0002.ts to be resolved to absolute URL, got:\n%s", output)
	}
}

func TestFilterTwitchAdSegments_WithAds(t *testing.T) {
	manifest := `#EXTM3U
#EXT-X-VERSION:3
#EXT-X-TARGETDURATION:2
#EXT-X-MEDIA-SEQUENCE:100
#EXT-X-DATERANGE:ID="stitched-ad-123",CLASS="twitch-stitched-ad",START-DATE="2026-06-07T12:00:02.000Z",DURATION=4.000
#EXT-X-PROGRAM-DATE-TIME:2026-06-07T12:00:00.000Z
#EXTINF:2.000,
index-0001.ts
#EXT-X-PROGRAM-DATE-TIME:2026-06-07T12:00:02.000Z
#EXTINF:2.000,
ad-0001.ts
#EXT-X-PROGRAM-DATE-TIME:2026-06-07T12:00:04.000Z
#EXTINF:2.000,
ad-0002.ts
#EXT-X-PROGRAM-DATE-TIME:2026-06-07T12:00:06.000Z
#EXTINF:2.000,
index-0002.ts`

	sourceURL := "https://video-weaver.sfo01.hls.ttvnw.net/v1/playlist/xxx.m3u8"
	output := filterTwitchAdSegments(manifest, sourceURL)

	if !strings.Contains(output, "https://video-weaver.sfo01.hls.ttvnw.net/v1/playlist/index-0001.ts") {
		t.Errorf("Expected index-0001.ts to be preserved, got:\n%s", output)
	}
	if strings.Contains(output, "ad-0001.ts") || strings.Contains(output, "ad-0002.ts") {
		t.Errorf("Expected ad segments to be stripped, got:\n%s", output)
	}
	if !strings.Contains(output, "https://video-weaver.sfo01.hls.ttvnw.net/v1/playlist/index-0002.ts") {
		t.Errorf("Expected post-ad index-0002.ts to be preserved and resolved, got:\n%s", output)
	}
	if strings.Contains(output, "twitch-stitched-ad") {
		t.Errorf("Expected ad daterange tags to be stripped, got:\n%s", output)
	}

	// Verify that the program date-time is correctly outputted for the non-ad segment
	expectedTimeLine := "#EXT-X-PROGRAM-DATE-TIME:2026-06-07T12:00:06.000Z"
	if !strings.Contains(output, expectedTimeLine) {
		t.Errorf("Expected reconstructed program date-time %q in output, got:\n%s", expectedTimeLine, output)
	}
}
