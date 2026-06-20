package reliability

import "testing"

func TestDefaultEntriesCoverCurrentWireSourceTypes(t *testing.T) {
	registry := NewRegistry(nil)
	registry.SeedDefaults()

	for sourceType, want := range map[string]float64{
		"pulse_origin":      1.00,
		"twitch_clip":       0.95,
		"news_article":      0.75,
		"reddit_thread":     0.70,
		"youtube_video":     0.70,
		"x_post":            0.60,
		"tiktok_video":      0.60,
		"instagram_post":    0.55,
		"kick_clip":         0.55,
		"streamerbans_post": 0.72,
		"manual_curation":   0.40,
	} {
		if got := registry.Weight(sourceType); got != want {
			t.Fatalf("Weight(%q) = %.2f, want %.2f", sourceType, got, want)
		}
	}
}
