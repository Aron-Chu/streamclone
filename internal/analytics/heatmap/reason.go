package heatmap

const (
	ReasonChatSpike        = "chat_spike"
	ReasonSevenTVSpike     = "seventv_spike"
	ReasonTwitchEmoteSpike = "twitch_emote_spike"
	ReasonFFZSpike         = "ffz_spike"
	ReasonViewerSpike      = "viewer_spike"
	ReasonGameChange       = "game_change"
	ReasonManual           = "manual"
)

var validReasons = map[string]struct{}{
	ReasonChatSpike:        {},
	ReasonSevenTVSpike:     {},
	ReasonTwitchEmoteSpike: {},
	ReasonFFZSpike:         {},
	ReasonViewerSpike:      {},
	ReasonGameChange:       {},
	ReasonManual:           {},
}

func IsValidReason(label string) bool {
	_, ok := validReasons[label]
	return ok
}

// reasonZThreshold is the minimum individual z-score a signal must exceed for
// its label to win selection. When no signal clears this bar the window is
// labeled with the chat_spike fallback (Requirement 10.2).
const reasonZThreshold = 1.0

// selectReason chooses exactly one Reason_Label for a scored window from the
// per-window normalized signal map (Requirement 10.1, 10.2).
//
// Selection rule (Requirement 10.2): the signal component with the highest
// individual z-score wins. If that maximum z-score does not exceed
// reasonZThreshold (1.0), the window is labeled chat_spike as the default
// fallback. Components are evaluated in a fixed order so ties resolve
// deterministically (Requirement 9.6): chatRate, viewerMomentum, providerSpike,
// emoteRate, topEmoteDominance, novelty.
//
// Signal-to-label mapping:
//   - chatRate          -> chat_spike
//   - viewerMomentum    -> viewer_spike
//   - providerSpike     -> the dominant provider's spike label
//   - emoteRate         -> the dominant provider's spike label (emote-family
//     signal; the valid set has no generic emote label, only provider-specific
//     seventv_spike / twitch_emote_spike / ffz_spike)
//   - topEmoteDominance -> dominant provider, or chat_spike when no provider
//     emote data exists in the window
//   - novelty           -> dominant provider, or chat_spike when no provider
//     emote data exists in the window
//
// The provider-specific labels are resolved from the window's raw provider rates
// (raw.sevenTVRate / twitchRate / ffzRate) rather than the collapsed
// providerSpike z-score, so a provider-family winner always maps to a concrete
// provider label. dominantProviderReason returns chat_spike when the window has
// no provider emote activity (e.g. a high-novelty window that contains no
// emotes), guaranteeing the result is always a single label from the valid set.
func selectReason(signals map[string]float64, raw rawWindowSignals) string {
	type sig struct {
		key string
		z   float64
	}
	order := []sig{
		{sigChatRate, signals[sigChatRate]},
		{sigViewerMomentum, signals[sigViewerMomentum]},
		{sigProviderSpike, signals[sigProviderSpike]},
		{sigEmoteRate, signals[sigEmoteRate]},
		{sigTopEmoteDominance, signals[sigTopEmoteDominance]},
		{sigNovelty, signals[sigNovelty]},
	}

	best := order[0]
	for _, s := range order[1:] {
		if s.z > best.z {
			best = s
		}
	}

	if best.z <= reasonZThreshold {
		return ReasonChatSpike
	}

	switch best.key {
	case sigChatRate:
		return ReasonChatSpike
	case sigViewerMomentum:
		return ReasonViewerSpike
	case sigProviderSpike, sigEmoteRate, sigTopEmoteDominance, sigNovelty:
		return dominantProviderReason(raw)
	default:
		return ReasonChatSpike
	}
}

// dominantProviderReason maps the window's raw provider rates to the matching
// provider spike label. Providers are compared with strict greater-than in a
// fixed order (7TV, Twitch, FFZ) so ties resolve deterministically to the
// earlier provider (Requirement 9.6). When the window carries no provider emote
// activity it returns chat_spike, keeping the output within the valid label set.
func dominantProviderReason(raw rawWindowSignals) string {
	if raw.sevenTVRate <= 0 && raw.twitchRate <= 0 && raw.ffzRate <= 0 {
		return ReasonChatSpike
	}

	best := raw.sevenTVRate
	label := ReasonSevenTVSpike
	if raw.twitchRate > best {
		best = raw.twitchRate
		label = ReasonTwitchEmoteSpike
	}
	if raw.ffzRate > best {
		label = ReasonFFZSpike
	}
	return label
}
