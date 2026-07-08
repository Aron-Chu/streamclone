// Package boundaryguard holds the Streamclone/ReplayForge ownership-boundary
// guard tests. It contains no runtime code.
//
// The boundary being enforced (see spec
// .kiro/specs/auto-clipper-replayforge-productization, Requirements 1.1-1.3):
// ReplayForge (the sibling repo, host :8095) owns FFmpeg clip rendering, Whisper
// transcription, and the clip editor/export pipeline. Streamclone Go services
// (cmd/*, internal/*) must not reintroduce that code. The guard test in this
// package is a static source scan that fails CI if a render/transcription/
// editor/export symbol is added under cmd/ or internal/.
//
// Playback relay (internal/video/*) legitimately shells out to FFmpeg and
// Streamlink to remux live/VOD HLS with "-c copy"; that is Streamclone's owned
// playback responsibility and is explicitly allow-listed by the guard. The
// guard targets clip render/transcription/editor/export, not playback remux.
//
// Responsibility allow-list (Requirement 1.6): allowlist.go holds the
// machine-readable ClipperResponsibilityAllowList — the only clipper-related
// responsibilities Streamclone Go may own (moment_context, export_moment_trigger,
// studio_redirect, job_mirror, callback_auth). Clip render is not on that list,
// so the allow-list guard test trips review when a render-touching package or
// route (detected by render markers, clip-render package paths, or clip render
// route literals) appears under cmd/ or internal/. The human-readable mirror is
// docs/clipper-responsibility-allowlist.md.
package boundaryguard
