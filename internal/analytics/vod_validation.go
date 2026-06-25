package analytics

import (
	"context"
	"errors"
	"regexp"
	"strings"
)

var (
	ErrPulseInvalidVODID             = errors.New("invalid_vod_id")
	ErrPulseVODStreamMismatch        = errors.New("vod_stream_mismatch")
	ErrPulseVODValidationUnavailable = errors.New("vod_validation_unavailable")

	pulseVodIDPattern = regexp.MustCompile(`^\d{6,20}$`)
)

func normalizePulseVodID(raw string) (string, error) {
	vodID := strings.TrimSpace(raw)
	if !pulseVodIDPattern.MatchString(vodID) {
		return "", ErrPulseInvalidVODID
	}
	return vodID, nil
}

func validatePulseVODCandidate(stream StreamRecord, rawVodID string) (string, error) {
	vodID, err := normalizePulseVodID(rawVodID)
	if err != nil {
		return "", err
	}
	if vodID == strings.TrimSpace(stream.StreamID) || vodID == strings.TrimSpace(stream.CanonicalStreamID) {
		return "", ErrPulseInvalidVODID
	}
	return vodID, nil
}

func validatePulseVodViaHelix(
	ctx context.Context,
	helix *HelixClient,
	stream StreamRecord,
	login string,
	rawVodID string,
	helixVodEnabled bool,
) (string, error) {
	vodID, err := validatePulseVODCandidate(stream, rawVodID)
	if err != nil {
		return "", err
	}
	if !helixVodEnabled || helix == nil || !helix.Enabled() {
		return "", ErrPulseVODValidationUnavailable
	}
	login = normalizeLogin(login)
	if login == "" {
		login = normalizeLogin(stream.Login)
	}
	broadcasterID := helix.ResolveBroadcasterID(ctx, login, stream.BroadcasterID)
	if broadcasterID == "" {
		return "", ErrPulseVODValidationUnavailable
	}
	resolved, err := helix.VideoIDByStreamID(ctx, broadcasterID, strings.TrimSpace(stream.StreamID))
	if err != nil {
		return "", ErrPulseVODValidationUnavailable
	}
	if resolved == "" || resolved != vodID {
		return "", ErrPulseVODStreamMismatch
	}
	return vodID, nil
}

func pulseVodValidationHTTPError(err error) (int, string) {
	switch {
	case errors.Is(err, ErrPulseInvalidVODID):
		return 400, "invalid_vod_id"
	case errors.Is(err, ErrPulseVODStreamMismatch):
		return 409, "vod_stream_mismatch"
	case errors.Is(err, ErrPulseVODValidationUnavailable):
		return 503, "vod_validation_unavailable"
	default:
		return 500, err.Error()
	}
}
