package seeder

import (
	"fmt"
	"strings"
)

// canonicalProviders is every provider a channel's emote set holds, in the
// order a four-provider ensure has always named them.
var canonicalProviders = []Provider{ProviderSevenTV, ProviderTwitch, ProviderFFZ, ProviderBTTV}

// CanonicalProviders returns every provider a channel's set holds. Each call
// returns a new slice.
func CanonicalProviders() []Provider {
	return append([]Provider(nil), canonicalProviders...)
}

// CanonicalSetName is the name of a channel's one provider emote set. Every
// seed (ensure, preload, manual) writes into this set and makes it the
// channel's active set, so seeding some providers never drops the others from
// the channel's dictionary.
//
// It is the name a four-provider ensure has always used, so channels seeded
// that way keep their set.
func CanonicalSetName(login string) string {
	login = strings.ToLower(strings.TrimSpace(login))
	parts := make([]string, 0, len(canonicalProviders))
	for _, provider := range canonicalProviders {
		parts = append(parts, string(provider))
	}
	return fmt.Sprintf("%s provider emotes (%s)", login, strings.Join(parts, "+"))
}

// IsCanonicalSetName reports whether setName is login's canonical set. It
// compares the whole name, so a login that contains a provider name cannot
// pass for a set that holds that provider.
func IsCanonicalSetName(setName, login string) bool {
	return strings.TrimSpace(setName) == CanonicalSetName(login)
}
