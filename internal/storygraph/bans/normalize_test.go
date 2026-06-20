package bans

import "testing"

func TestClassifyBanEventStreamerBans(t *testing.T) {
	eventType, confidence, ok := classifyBanEvent("streamerbans_post", "xQc has been banned on Twitch", "ban")
	if !ok || eventType != "banned" || confidence <= 0 {
		t.Fatalf("expected banned event, got type=%s conf=%v ok=%v", eventType, confidence, ok)
	}
	eventType, _, ok = classifyBanEvent("streamerbans_post", "Streamer has been unbanned on Twitch", "")
	if !ok || eventType != "unbanned" {
		t.Fatalf("expected unbanned, got %s ok=%v", eventType, ok)
	}
}

func TestClassifyBanEventRedditRequiresBanSignal(t *testing.T) {
	_, _, ok := classifyBanEvent("reddit", "funny clip moment", "clip")
	if ok {
		t.Fatal("non-ban reddit post should not become ban event")
	}
	eventType, confidence, ok := classifyBanEvent("reddit", "Streamer banned on Twitch after drama", "drama")
	if !ok || eventType != "banned" || confidence < 0.5 {
		t.Fatalf("expected reddit ban event, got type=%s conf=%v ok=%v", eventType, confidence, ok)
	}
}
