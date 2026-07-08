package ingestcore

import (
	"testing"
	"time"
)

func TestRing_oneChatPerMessage_multiEmote(t *testing.T) {
	now := time.Date(2026, 7, 8, 12, 0, 30, 0, time.UTC)
	r := newMinuteRing("stream-1", now, 200)
	for i := 0; i < 8; i++ {
		r.AddEmote(now, "7tv:e1:KEKW", true)
	}
	r.AddChatMessage(now)
	snap := r.SnapshotOpen(200)
	if snap.ChatCount != 1 {
		t.Fatalf("ChatCount = %d, want 1", snap.ChatCount)
	}
	if snap.TotalEmoteCount != 8 {
		t.Fatalf("TotalEmoteCount = %d, want 8", snap.TotalEmoteCount)
	}
}

func TestRing_plainTextNoEmotes(t *testing.T) {
	now := time.Date(2026, 7, 8, 12, 0, 30, 0, time.UTC)
	r := newMinuteRing("stream-1", now, 200)
	r.AddChatMessage(now)
	snap := r.SnapshotOpen(200)
	if snap.ChatCount != 1 {
		t.Fatalf("ChatCount = %d, want 1", snap.ChatCount)
	}
	if snap.TotalEmoteCount != 0 {
		t.Fatalf("TotalEmoteCount = %d, want 0", snap.TotalEmoteCount)
	}
	if len(snap.Emotes) != 0 {
		t.Fatalf("Emotes = %v, want empty", snap.Emotes)
	}
}

func TestRing_repeatedSameEmote(t *testing.T) {
	now := time.Date(2026, 7, 8, 12, 0, 30, 0, time.UTC)
	r := newMinuteRing("stream-1", now, 200)
	key := "7tv:e1:KEKW"
	for i := 0; i < 5; i++ {
		r.AddEmote(now, key, true)
	}
	r.AddChatMessage(now)
	snap := r.SnapshotOpen(200)
	if snap.ChatCount != 1 {
		t.Fatalf("ChatCount = %d, want 1", snap.ChatCount)
	}
	if snap.TotalEmoteCount != 5 {
		t.Fatalf("TotalEmoteCount = %d, want 5", snap.TotalEmoteCount)
	}
	if snap.Emotes[key] != 5 {
		t.Fatalf("Emotes[%q] = %d, want 5", key, snap.Emotes[key])
	}
}

func TestMinuteBucket_messageTimestamp(t *testing.T) {
	r := newMinuteRing("stream-1", time.Date(2026, 7, 8, 12, 0, 0, 0, time.UTC), 200)
	t59 := time.Date(2026, 7, 8, 12, 0, 59, 0, time.UTC)
	t00 := time.Date(2026, 7, 8, 12, 1, 0, 0, time.UTC)
	r.AddChatMessage(t59)
	r.AddChatMessage(t00)
	snap := r.SnapshotOpen(200)
	if snap.Minute != t00.Truncate(time.Minute) {
		t.Fatalf("open minute = %v, want 12:01", snap.Minute)
	}
	if snap.ChatCount != 1 {
		t.Fatalf("open ChatCount = %d, want 1 (second minute only)", snap.ChatCount)
	}
	closed := r.DrainClosed(200)
	if len(closed) != 1 {
		t.Fatalf("closed snapshots = %d, want 1", len(closed))
	}
	if closed[0].ChatCount != 1 {
		t.Fatalf("closed ChatCount = %d, want 1", closed[0].ChatCount)
	}
	if !closed[0].Minute.Equal(t59.Truncate(time.Minute)) {
		t.Fatalf("closed minute = %v, want 12:00", closed[0].Minute)
	}
}

func TestMinuteRingAddChatMessageAndSnapshot(t *testing.T) {
	now := time.Date(2026, 7, 8, 12, 0, 30, 0, time.UTC)
	r := newMinuteRing("stream-1", now, 200)
	r.AddChatMessage(now)
	r.AddEmote(now, "7tv:e1:KEKW", true)
	snap := r.SnapshotOpen(200)
	if snap.ChatCount != 1 {
		t.Fatalf("chat = %d, want 1", snap.ChatCount)
	}
	if snap.TotalEmoteCount != 1 {
		t.Fatalf("emotes = %d, want 1", snap.TotalEmoteCount)
	}
}
