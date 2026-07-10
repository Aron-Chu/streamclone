// Package boundaryguard holds the Streamclone ownership-boundary guard tests.
// It contains no runtime code.
//
// ReplayForge (sibling repo) owns FFmpeg clip rendering, Whisper transcription,
// and the clip editor/export pipeline. StreamPulse analytics lives in private
// streampulse-backend / streamclone-pulse. Streamclone Go services (cmd/*,
// internal/*) must not reintroduce that code.
//
// Playback relay (internal/video/*) legitimately shells out to FFmpeg and
// Streamlink to remux live/VOD HLS with "-c copy"; that is Streamclone's owned
// playback responsibility and is explicitly allow-listed by the guard.
//
// ClipperResponsibilityAllowList is empty after the product-boundary lock —
// Streamclone ships no Clip Studio / /studio / /v1/clipper surfaces. Phase 5
// tests assert those files and Caddy proxy blocks remain absent.
package boundaryguard
