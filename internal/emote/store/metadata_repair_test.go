package store

import "testing"

func TestIsProviderMetadataReady(t *testing.T) {
	cases := []struct {
		name string
		e    Emote
		want bool
	}{
		{
			name: "provider emote with id",
			e:    Emote{Provider: "seventv", ProviderEmoteID: "abc"},
			want: true,
		},
		{
			name: "custom upload pending",
			e:    Emote{Provider: "custom", ProviderEmoteID: ""},
			want: false,
		},
		{
			name: "provider missing id",
			e:    Emote{Provider: "ffz", ProviderEmoteID: ""},
			want: false,
		},
		{
			name: "twitch native",
			e:    Emote{Provider: "twitch", ProviderEmoteID: "304894101"},
			want: true,
		},
	}
	for _, tc := range cases {
		if got := IsProviderMetadataReady(&tc.e); got != tc.want {
			t.Fatalf("%s: got %v want %v", tc.name, got, tc.want)
		}
	}
}
