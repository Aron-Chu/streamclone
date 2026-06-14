package chatreplay

import (
	"context"
	"encoding/base64"
	"testing"
)

func TestCursorRoundTrip(t *testing.T) {
	cases := []struct {
		offset int
		id     int64
	}{
		{0, 0},
		{1, 1},
		{599, 1234567890},
		{86400, 9223372036854775807},
	}
	for _, c := range cases {
		token := encodeCursor(c.offset, c.id)
		gotOffset, gotID, err := decodeCursor(token)
		if err != nil {
			t.Fatalf("decodeCursor(%q) error: %v", token, err)
		}
		if gotOffset != c.offset || gotID != c.id {
			t.Fatalf("round-trip mismatch: got (%d,%d), want (%d,%d)", gotOffset, gotID, c.offset, c.id)
		}
	}
}

func TestDecodeCursorRejectsGarbage(t *testing.T) {
	if _, _, err := decodeCursor("!!!not-base64!!!"); err == nil {
		t.Fatal("expected error decoding non-base64 cursor")
	}
	if _, _, err := decodeCursor(base64.RawURLEncoding.EncodeToString([]byte("no-colon"))); err == nil {
		t.Fatal("expected error decoding malformed cursor")
	}
}

func TestMarshalFrags(t *testing.T) {
	got, err := marshalFrags(nil)
	if err != nil {
		t.Fatalf("marshalFrags(nil) error: %v", err)
	}
	if got != "[]" {
		t.Fatalf("marshalFrags(nil) = %q, want []", got)
	}

	got, err = marshalFrags([]EmoteFrag{{Name: "KEKW", ID: "abc", Provider: "7tv", ImageURL: "/emotes/abc/1x.webp"}})
	if err != nil {
		t.Fatalf("marshalFrags error: %v", err)
	}
	if got == "[]" || got == "" {
		t.Fatalf("marshalFrags returned empty for non-empty input: %q", got)
	}
}

func TestStoreSinkNilSafe(t *testing.T) {
	var s *StoreSink
	// Methods on a nil *StoreSink must be safe (feature disabled).
	s.Add(VODChatMessage{StreamID: "1", MessageID: "m1"})
	if err := s.FlushSegment(context.Background(), 0, 10); err != nil {
		t.Fatalf("nil FlushSegment error: %v", err)
	}
	if err := s.Flush(context.Background()); err != nil {
		t.Fatalf("nil Flush error: %v", err)
	}
}

func TestStoreSinkNilStoreNoop(t *testing.T) {
	s := NewStoreSink(nil)
	s.Add(VODChatMessage{StreamID: "1", MessageID: "m1", OffsetSeconds: 30})
	if err := s.FlushSegment(context.Background(), 0, 1); err != nil {
		t.Fatalf("FlushSegment error: %v", err)
	}
	if err := s.Flush(context.Background()); err != nil {
		t.Fatalf("Flush error: %v", err)
	}
}

func TestNopSink(t *testing.T) {
	var sink Sink = NopSink{}
	sink.Add(VODChatMessage{StreamID: "1", MessageID: "m1"})
	if err := sink.FlushSegment(context.Background(), 0, 5); err != nil {
		t.Fatalf("NopSink FlushSegment error: %v", err)
	}
	if err := sink.Flush(context.Background()); err != nil {
		t.Fatalf("NopSink Flush error: %v", err)
	}
}
