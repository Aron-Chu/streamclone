package analytics

import (
	"context"
	"testing"
)

type stubHelixVodReader struct {
	enabled     bool
	broadcaster string
	vodID       string
	vodErr      error
}

func (s stubHelixVodReader) Enabled() bool { return s.enabled }

func (s stubHelixVodReader) ResolveBroadcasterID(_ context.Context, _, _ string) string {
	return s.broadcaster
}

func (s stubHelixVodReader) VideoIDByStreamID(context.Context, string, string) (string, error) {
	return s.vodID, s.vodErr
}

func TestResolveStreamVodIDForReadReturnsPersisted(t *testing.T) {
	got := resolveStreamVodIDForRead(context.Background(), StreamRecord{
		StreamID: "123456789",
		VodID:    "2809816759",
	}, true, stubHelixVodReader{enabled: true, vodID: "999"}, nil)
	if got != "2809816759" {
		t.Fatalf("got %q, want persisted vod", got)
	}
}

func TestResolveStreamVodIDForReadHelixMatch(t *testing.T) {
	var persisted vodSet
	stream := StreamRecord{
		StreamID:      "123456789",
		Login:         "caedrel",
		BroadcasterID: "999",
	}
	got := resolveStreamVodIDForRead(
		context.Background(),
		stream,
		true,
		stubHelixVodReader{enabled: true, broadcaster: "999", vodID: "2814106332"},
		func(_ context.Context, streamID, vodID, source string) error {
			persisted = vodSet{streamID: streamID, vodID: vodID, source: source}
			return nil
		},
	)
	if got != "2814106332" {
		t.Fatalf("got %q, want helix vod", got)
	}
	if persisted.streamID != "123456789" || persisted.vodID != "2814106332" || persisted.source != "helix_stream_match" {
		t.Fatalf("unexpected persist: %+v", persisted)
	}
}

func TestResolveStreamVodIDForReadSkipsWhenHelixDisabled(t *testing.T) {
	got := resolveStreamVodIDForRead(
		context.Background(),
		StreamRecord{StreamID: "123456789", Login: "caedrel"},
		false,
		stubHelixVodReader{enabled: true, vodID: "2814106332"},
		nil,
	)
	if got != "" {
		t.Fatalf("got %q, want empty when helix vod disabled", got)
	}
}
