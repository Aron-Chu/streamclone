package analytics

import "testing"

func TestTopEmotesFromRollupsImageURLs(t *testing.T) {
	rollups := []MinuteRollup{{
		Emotes: map[string]int{
			"twitch:304894101:SomeEmote":                                      10,
			"twitch:emotesv2_34dda6b8341e46d0b2118a9cabbe6a2e:Other":          5,
			"seventv:75f49395-d5fc-41da-998c-880c6d8fddcb:KEKW":               3,
			"seventv:62a3bf572b964d6cc2766004:LegacySevenTV":                  2,
		},
	}}
	top := TopEmotesFromRollups(rollups, 10)
	byID := map[string]string{}
	for _, emote := range top {
		byID[emote.ID] = emote.ImageURL
	}
	if got := byID["304894101"]; got != "https://static-cdn.jtvnw.net/emoticons/v2/304894101/default/dark/2.0" {
		t.Fatalf("twitch numeric imageUrl = %q", got)
	}
	if got := byID["emotesv2_34dda6b8341e46d0b2118a9cabbe6a2e"]; got != "https://static-cdn.jtvnw.net/emoticons/v2/emotesv2_34dda6b8341e46d0b2118a9cabbe6a2e/default/dark/2.0" {
		t.Fatalf("twitch emotesv2 imageUrl = %q", got)
	}
	if got := byID["75f49395-d5fc-41da-998c-880c6d8fddcb"]; got != "/emotes/75f49395-d5fc-41da-998c-880c6d8fddcb/1x.webp" {
		t.Fatalf("synced seventv imageUrl = %q", got)
	}
	if got := byID["62a3bf572b964d6cc2766004"]; got != "https://cdn.7tv.app/emote/62a3bf572b964d6cc2766004/4x.webp" {
		t.Fatalf("legacy seventv provider id imageUrl = %q", got)
	}
}
