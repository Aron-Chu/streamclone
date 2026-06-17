package emoteimage

import "testing"

func TestURLTwitchNativeIDsUseCDN(t *testing.T) {
	cases := []struct {
		id   string
		want string
	}{
		{"304894101", "https://static-cdn.jtvnw.net/emoticons/v2/304894101/default/dark/2.0"},
		{"22639", "https://static-cdn.jtvnw.net/emoticons/v2/22639/default/dark/2.0"},
		{"emotesv2_34dda6b8341e46d0b2118a9cabbe6a2e", "https://static-cdn.jtvnw.net/emoticons/v2/emotesv2_34dda6b8341e46d0b2118a9cabbe6a2e/default/dark/2.0"},
	}
	for _, tc := range cases {
		if got := URL("twitch", tc.id, "1x"); got != tc.want {
			t.Fatalf("URL(twitch, %q) = %q, want %q", tc.id, got, tc.want)
		}
	}
}

func TestURLSyncedEmoteUsesLocalPath(t *testing.T) {
	localID := "75f49395-d5fc-41da-998c-880c6d8fddcb"
	want := "/emotes/" + localID + "/1x.webp"
	for _, provider := range []string{"seventv", "ffz", "bttv", "custom"} {
		if got := URL(provider, localID, "1x"); got != want {
			t.Fatalf("URL(%q, local uuid) = %q, want %q", provider, got, want)
		}
	}
}

func TestURLSevenTVProviderIDUsesCDN(t *testing.T) {
	got := URL("seventv", "62a3bf572b964d6cc2766004", "1x")
	want := "https://cdn.7tv.app/emote/62a3bf572b964d6cc2766004/4x.webp"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestURLEmptyID(t *testing.T) {
	if got := URL("twitch", "", "1x"); got != "" {
		t.Fatalf("expected empty url, got %q", got)
	}
}
