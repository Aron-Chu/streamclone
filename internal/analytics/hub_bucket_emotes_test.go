package analytics

import "testing"

func TestHubBucketEmotesFromTopPreservesImageURL(t *testing.T) {
	got := hubBucketEmotesFromTop([]TopEmote{{
		Name:     "BigSad",
		Provider: "twitch",
		ImageURL: "https://static-cdn.jtvnw.net/emoticons/v2/emotesv2_test/default/dark/2.0",
		Count:    42,
	}})
	if len(got) != 1 {
		t.Fatalf("len = %d, want 1", len(got))
	}
	if got[0].ImageURL == "" {
		t.Fatal("expected imageUrl on bucket emote")
	}
	if got[0].Name != "BigSad" || got[0].Count != 42 {
		t.Fatalf("unexpected bucket emote: %+v", got[0])
	}
}
