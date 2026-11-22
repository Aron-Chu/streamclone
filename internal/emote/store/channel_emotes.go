package store

// channelEmoteProviderOrder ranks providers for same-name emotes within one
// scope (channel or global): custom uploads first, then 7TV, BTTV, FFZ and
// Twitch, the order 7TV and Chatterino users expect. Unknown providers come
// last.
var channelEmoteProviderOrder = map[string]int{
	"custom":  0,
	"seventv": 1,
	"bttv":    2,
	"ffz":     3,
	"twitch":  4,
}

func channelEmoteProviderRank(provider string) int {
	if rank, ok := channelEmoteProviderOrder[provider]; ok {
		return rank
	}
	return len(channelEmoteProviderOrder)
}

// channelEmoteBeats reports whether a wins over b when both have the same
// name: a channel's own emote beats a global one, then the provider order
// above decides, then the lower emote id, so every rebuild picks the same
// emote.
//
// The scope is the emote row's is_global flag. Emote rows are shared between
// channels, so a channel emote that is also some provider's global counts as
// a global here.
func channelEmoteBeats(a, b ChannelEmote) bool {
	if a.IsGlobal != b.IsGlobal {
		return !a.IsGlobal
	}
	if ra, rb := channelEmoteProviderRank(a.Provider), channelEmoteProviderRank(b.Provider); ra != rb {
		return ra < rb
	}
	return a.EmoteID < b.EmoteID
}

// resolveChannelEmotes keeps one emote per name, the winner under
// channelEmoteBeats. The winners keep the order of rows.
func resolveChannelEmotes(rows []ChannelEmote) []ChannelEmote {
	winner := make(map[string]int, len(rows))
	for i, row := range rows {
		if j, ok := winner[row.Name]; !ok || channelEmoteBeats(row, rows[j]) {
			winner[row.Name] = i
		}
	}
	out := make([]ChannelEmote, 0, len(winner))
	for i, row := range rows {
		if winner[row.Name] == i {
			out = append(out, row)
		}
	}
	return out
}
