package analytics

import (
	"context"
	"strings"

	"streamclone/internal/emoteimage"
)

func (h *Handler) hostedEmoteCDNBase() string {
	if h == nil || !h.pulseHosted.Hosted {
		return ""
	}
	return strings.TrimRight(strings.TrimSpace(h.cdnPublicBase), "/")
}

func (h *Handler) rewriteHostedTopEmotes(ctx context.Context, emotes []TopEmote) []TopEmote {
	base := h.hostedEmoteCDNBase()
	if base == "" || len(emotes) == 0 {
		return emotes
	}
	lookup := h.lookupSevenTVForTopEmotes(ctx, emotes)
	out := make([]TopEmote, len(emotes))
	copy(out, emotes)
	for i := range out {
		providerID := lookup[strings.TrimSpace(out[i].ID)]
		out[i].ImageURL = emoteimage.HostedBrowserURL(base, out[i].Provider, out[i].ID, providerID)
	}
	return out
}

func (h *Handler) lookupSevenTVForTopEmotes(ctx context.Context, emotes []TopEmote) map[string]string {
	if h == nil || h.store == nil {
		return map[string]string{}
	}
	ids := make([]string, 0, len(emotes))
	for _, e := range emotes {
		if id := strings.TrimSpace(e.ID); id != "" {
			ids = append(ids, id)
		}
	}
	lookup, err := h.store.LookupSevenTVProviderEmoteIDs(ctx, ids)
	if err != nil {
		return map[string]string{}
	}
	return lookup
}

func rewriteHostedExtensionEmoteURLs(emotes []ExtensionEmote, lookup map[string]string, cdnBase string) []ExtensionEmote {
	if len(emotes) == 0 {
		return emotes
	}
	out := make([]ExtensionEmote, len(emotes))
	copy(out, emotes)
	for i := range out {
		providerID := lookup[strings.TrimSpace(out[i].ID)]
		out[i].ImageURL = emoteimage.HostedBrowserURL(cdnBase, out[i].Provider, out[i].ID, providerID)
	}
	return out
}
