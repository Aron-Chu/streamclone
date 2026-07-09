package analytics

import (
	"context"
	"strings"
)

type helixVodReader interface {
	Enabled() bool
	ResolveBroadcasterID(ctx context.Context, login, storedID string) string
	VideoIDByStreamID(ctx context.Context, broadcasterID, streamID string) (string, error)
}

// resolveStreamVodIDForRead returns a Twitch VOD id for portal/extension read paths.
// When the stream row has no vodId yet, it may resolve via Helix and persist the match.
func resolveStreamVodIDForRead(
	ctx context.Context,
	stream StreamRecord,
	helixVodEnabled bool,
	helix helixVodReader,
	persist func(ctx context.Context, streamID, vodID, source string) error,
) string {
	vodID := strings.TrimSpace(stream.VodID)
	if vodID != "" {
		return vodID
	}
	if !helixVodEnabled || helix == nil || !helix.Enabled() {
		return ""
	}
	broadcasterID := NormalizeBroadcasterID(stream.BroadcasterID)
	if broadcasterID == "" && stream.Login != "" {
		broadcasterID = helix.ResolveBroadcasterID(ctx, stream.Login, "")
	}
	if broadcasterID == "" {
		return ""
	}
	resolved, err := helix.VideoIDByStreamID(ctx, broadcasterID, stream.StreamID)
	if err != nil || resolved == "" {
		return ""
	}
	validated, err := validatePulseVODCandidate(stream, resolved)
	if err != nil {
		return ""
	}
	if persist != nil {
		_ = persist(ctx, stream.StreamID, validated, "helix_stream_match")
	}
	return validated
}

func (h *Handler) resolveStreamVodIDForRead(ctx context.Context, stream StreamRecord) string {
	if h == nil {
		return strings.TrimSpace(stream.VodID)
	}
	var persist func(ctx context.Context, streamID, vodID, source string) error
	if h.store != nil {
		persist = h.store.SetStreamVodID
	}
	return resolveStreamVodIDForRead(ctx, stream, h.pulseRuntimeConfig().HelixVodEnabled, h.helix, persist)
}
