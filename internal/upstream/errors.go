package upstream

import "errors"

var (
	ErrUpstreamSchema = errors.New("upstream response schema mismatch")
	ErrPlaybackToken  = errors.New("playback token error")
)
