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
	if len(emotes) == 0 {
		return emotes
	}
	base := h.hostedEmoteCDNBase()
	metadata := h.lookupEmoteMetadataForTopEmotes(ctx, emotes)
	var lookup map[string]string
	if base != "" {
		lookup = h.lookupProviderIDsForTopEmotes(ctx, emotes)
	}
	if base == "" && len(metadata) == 0 {
		return emotes
	}
	out := make([]TopEmote, len(emotes))
	copy(out, emotes)
	for i := range out {
		id := strings.TrimSpace(out[i].ID)
		if meta, ok := metadata[id]; ok {
			out[i].ZeroWidth = meta.ZeroWidth
			out[i].Animated = meta.Animated
		}
		if base != "" {
			providerID := lookup[id]
			out[i].ImageURL = emoteimage.HostedBrowserURL(base, out[i].Provider, out[i].ID, providerID)
		}
	}
	return out
}

func (h *Handler) lookupEmoteMetadataForTopEmotes(ctx context.Context, emotes []TopEmote) map[string]EmoteMetadata {
	if h == nil || h.store == nil {
		return map[string]EmoteMetadata{}
	}
	ids := make([]string, 0, len(emotes))
	for _, e := range emotes {
		if id := strings.TrimSpace(e.ID); id != "" {
			ids = append(ids, id)
		}
	}
	lookup, err := h.store.LookupEmoteMetadata(ctx, ids)
	if err != nil {
		return map[string]EmoteMetadata{}
	}
	return lookup
}

func (h *Handler) lookupProviderIDsForTopEmotes(ctx context.Context, emotes []TopEmote) map[string]string {
	if h == nil || h.store == nil {
		return map[string]string{}
	}
	ids := make([]string, 0, len(emotes))
	for _, e := range emotes {
		if id := strings.TrimSpace(e.ID); id != "" {
			ids = append(ids, id)
		}
	}
	lookup, err := h.store.LookupProviderEmoteIDs(ctx, ids)
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

func decorateExtensionEmotes(emotes []ExtensionEmote, lookup map[string]string, metadata map[string]EmoteMetadata, cdnBase string) []ExtensionEmote {
	if len(emotes) == 0 {
		return emotes
	}
	out := rewriteHostedExtensionEmoteURLs(emotes, lookup, cdnBase)
	if len(metadata) == 0 {
		return out
	}
	for i := range out {
		if meta, ok := metadata[strings.TrimSpace(out[i].ID)]; ok {
			out[i].ZeroWidth = meta.ZeroWidth
			out[i].Animated = meta.Animated
		}
	}
	return out
}
