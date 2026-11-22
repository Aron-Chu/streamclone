package seeder

import (
	"reflect"
	"testing"
)

func TestCanonicalProvidersAreAllFourInEnsureOrder(t *testing.T) {
	want := []Provider{ProviderSevenTV, ProviderTwitch, ProviderFFZ, ProviderBTTV}
	if got := CanonicalProviders(); !reflect.DeepEqual(got, want) {
		t.Fatalf("CanonicalProviders() = %v, want %v", got, want)
	}
	// Callers get their own copy.
	got := CanonicalProviders()
	got[0] = ProviderBTTV
	if CanonicalProviders()[0] != ProviderSevenTV {
		t.Fatal("CanonicalProviders shares its backing array with callers")
	}
}

// The canonical set keeps the name a four-provider ensure has always given a
// channel's set, so channels seeded before this change keep their set.
func TestCanonicalSetNameMatchesFourProviderEnsureName(t *testing.T) {
	if got, want := CanonicalSetName("xqc"), "xqc provider emotes (seventv+twitch+ffz+bttv)"; got != want {
		t.Fatalf("CanonicalSetName = %q, want %q", got, want)
	}
	if got, want := CanonicalSetName("  XQC "), "xqc provider emotes (seventv+twitch+ffz+bttv)"; got != want {
		t.Fatalf("CanonicalSetName normalises the login: got %q, want %q", got, want)
	}
}

func TestIsCanonicalSetName(t *testing.T) {
	tests := []struct {
		name    string
		setName string
		login   string
		want    bool
	}{
		{name: "canonical", setName: "xqc provider emotes (seventv+twitch+ffz+bttv)", login: "xqc", want: true},
		{name: "login case", setName: "xqc provider emotes (seventv+twitch+ffz+bttv)", login: "XQC", want: true},
		{name: "old preloader set", setName: "xqc provider emotes (seventv+ffz)", login: "xqc", want: false},
		{name: "watch ui subset set", setName: "xqc provider emotes (seventv+twitch)", login: "xqc", want: false},
		// A substring check took this set for one that has BTTV, because the
		// login contains "bttv".
		{name: "login contains a provider name", setName: "bttvfan provider emotes (seventv+ffz)", login: "bttvfan", want: false},
		{name: "another channel's set", setName: "shroud provider emotes (seventv+twitch+ffz+bttv)", login: "xqc", want: false},
		{name: "curated set", setName: "my curated set", login: "xqc", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsCanonicalSetName(tt.setName, tt.login); got != tt.want {
				t.Fatalf("IsCanonicalSetName(%q, %q) = %v, want %v", tt.setName, tt.login, got, tt.want)
			}
		})
	}
}
