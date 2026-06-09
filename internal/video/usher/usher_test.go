package usher

import (
	"errors"
	"testing"

	"streamclone/internal/upstream"
)

const masterFixture = `#EXTM3U
#EXT-X-TWITCH-INFO:NODE="video-edge-1.abc"
#EXT-X-MEDIA:TYPE=VIDEO,GROUP-ID="chunked",NAME="1080p60 (source)",AUTOSELECT=YES,DEFAULT=YES
#EXT-X-STREAM-INF:BANDWIDTH=6212976,RESOLUTION=1920x1080,CODECS="avc1.64002A,mp4a.40.2",VIDEO="chunked",FRAME-RATE=60.000
https://video-edge-1.abc/chunked/index.m3u8
#EXT-X-MEDIA:TYPE=VIDEO,GROUP-ID="720p60",NAME="720p60",AUTOSELECT=YES,DEFAULT=NO
#EXT-X-STREAM-INF:BANDWIDTH=3220000,RESOLUTION=1280x720,CODECS="avc1.4D401F,mp4a.40.2",VIDEO="720p60",FRAME-RATE=60.000
https://video-edge-1.abc/720p60/index.m3u8
#EXT-X-MEDIA:TYPE=VIDEO,GROUP-ID="audio_only",NAME="audio_only",AUTOSELECT=NO,DEFAULT=NO
#EXT-X-STREAM-INF:BANDWIDTH=160000,CODECS="mp4a.40.2",VIDEO="audio_only"
https://video-edge-1.abc/audio_only/index.m3u8`

func TestParseMaster(t *testing.T) {
	rs, err := ParseMaster(masterFixture)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rs) != 3 {
		t.Fatalf("expected 3 renditions, got %d", len(rs))
	}
	if rs[0].Name != "1080p60 (source)" || rs[0].Width != 1920 || rs[0].Height != 1080 || rs[0].FrameRate != 60 {
		t.Fatalf("source rendition mismatch: %+v", rs[0])
	}
	if rs[0].URL != "https://video-edge-1.abc/chunked/index.m3u8" {
		t.Fatalf("source url mismatch: %q", rs[0].URL)
	}
	if rs[1].Name != "720p60" || rs[1].Height != 720 {
		t.Fatalf("720 rendition mismatch: %+v", rs[1])
	}
	if rs[2].Name != "audio_only" || rs[2].Bandwidth != 160000 {
		t.Fatalf("audio rendition mismatch: %+v", rs[2])
	}
}

func TestParseMasterNotPlaylist(t *testing.T) {
	if _, err := ParseMaster(`[{"error":"not found"}]`); !errors.Is(err, upstream.ErrUpstreamSchema) {
		t.Fatalf("expected ErrUpstreamSchema, got %v", err)
	}
}
