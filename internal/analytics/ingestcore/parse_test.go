package ingestcore

import (
	"testing"

	"streamclone/internal/chat/batch"
)

func TestEmoteKeysFromFragments_preservesOccurrences(t *testing.T) {
	frags := []batch.Fragment{
		{T: "emote", C: "KEKW", Provider: "7tv", ID: "e1"},
		{T: "emote", C: "KEKW", Provider: "7tv", ID: "e1"},
		{T: "emote", C: "KEKW", Provider: "7tv", ID: "e1"},
		{T: "emote", C: "KEKW", Provider: "7tv", ID: "e1"},
		{T: "emote", C: "KEKW", Provider: "7tv", ID: "e1"},
	}
	keys, seven := EmoteKeysFromFragments(frags)
	if len(keys) != 5 {
		t.Fatalf("keys len = %d, want 5 (no dedupe)", len(keys))
	}
	if seven != 5 {
		t.Fatalf("sevenTV = %d, want 5", seven)
	}
}

func TestEmoteKeysFromFragments_ignoresTextFragments(t *testing.T) {
	frags := []batch.Fragment{
		{T: "text", C: "hello"},
		{T: "emote", C: "LUL", Provider: "twitch", ID: "25"},
	}
	keys, seven := EmoteKeysFromFragments(frags)
	if len(keys) != 1 {
		t.Fatalf("keys len = %d, want 1", len(keys))
	}
	if seven != 0 {
		t.Fatalf("sevenTV = %d, want 0", seven)
	}
}
