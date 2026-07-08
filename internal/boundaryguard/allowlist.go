package boundaryguard

// This file is the machine-readable source of truth for the Streamclone
// clipper responsibility allow-list (Requirement 1.6). The boundary guard test
// consults it so that any clip render/transcription/editor/export code — which
// is NOT one of the allow-listed responsibilities — trips review.
//
// Human-readable companion: docs/clipper-responsibility-allowlist.md (mirrors
// this list; this Go file is authoritative).
//
// Requirement 1.6: Streamclone SHALL retain ONLY Analytics moments, moment
// context, the Export Moment trigger, the /studio redirect, mirrored Job_State
// (Job_Mirror), and callback authentication as clipper-related
// responsibilities. Everything else in the clip pipeline — FFmpeg render,
// Whisper transcription, the clip editor, export, durable artifacts, and signed
// playback — belongs to ReplayForge (sibling repo, host :8095).

// ClipperResponsibility describes one clipper-related responsibility that
// Streamclone Go is permitted to own. It is an integration/orchestration
// surface: it may build context, trigger, route, redirect, mirror state, or
// authenticate — but it must never render, transcode, transcribe, edit, or
// export a clip.
type ClipperResponsibility struct {
	// Name is the stable identifier used in the spec and docs.
	Name string
	// Requirement is the acceptance-criteria id this responsibility satisfies.
	Requirement string
	// Description explains the permitted, non-render surface.
	Description string
	// RoutePatterns are HTTP path substrings this responsibility legitimately
	// serves or redirects (never a render route).
	RoutePatterns []string
	// PackageHints are informational, repo-relative package areas where this
	// responsibility lives today. They are NOT used to rescue render-touching
	// code from the guard — render is never allowed regardless of package.
	PackageHints []string
}

// ClipperResponsibilityAllowList enumerates the ONLY clipper-related
// responsibilities Streamclone Go may own (Requirement 1.6). It is intentionally
// closed: adding clip render/transcription/editor/export here is not permitted —
// that code moves to ReplayForge.
var ClipperResponsibilityAllowList = []ClipperResponsibility{
	{
		Name:        "moment_context",
		Requirement: "1.6",
		Description: "Build the moment_context payload (channel, vod_id, start/end, reason) from Streamclone Analytics moments; no source download or render.",
		RoutePatterns: []string{
			"/v1/triggers/manual",
		},
		PackageHints: []string{
			"internal/analytics/",
			"internal/storygraph/clip/",
		},
	},
	{
		Name:        "export_moment_trigger",
		Requirement: "1.6",
		Description: "Send an authenticated Clip_Job creation request (moment_context + idempotency_key) to ReplayForge and store the returned job id; trigger only, never render.",
		RoutePatterns: []string{
			"/v1/jobs",
		},
		PackageHints: []string{
			"internal/analytics/",
		},
	},
	{
		Name:        "studio_redirect",
		Requirement: "1.6",
		Description: "Resolve a job id and redirect /studio to the ReplayForge Clip Studio URL; a redirect surface, not an editor.",
		RoutePatterns: []string{
			"/studio",
		},
		PackageHints: []string{
			"internal/metadata/",
		},
	},
	{
		Name:        "job_mirror",
		Requirement: "1.6",
		Description: "Read model of mirrored Job_State from the Job_State_Set, updated only via the authed idempotent Status_Callback and reconciled to the ReplayForge Job_Store; state display only, no job execution.",
		RoutePatterns: []string{
			"/v1/clipper/callback",
		},
		PackageHints: []string{
			"internal/metadata/",
		},
	},
	{
		Name:        "callback_auth",
		Requirement: "1.6",
		Description: "Authenticate Status_Callback and Clip_Job mutation requests (reject missing/invalid Auth_Token with 401); auth boundary only.",
		RoutePatterns: []string{
			"/v1/clipper/callback",
		},
		PackageHints: []string{
			"internal/metadata/",
			"internal/config/",
		},
	},
}

// AllowedResponsibilityNames returns the names of the permitted clipper-related
// responsibilities, in allow-list order. The guard uses this to render honest
// failure output that names what Streamclone IS allowed to own.
func AllowedResponsibilityNames() []string {
	names := make([]string, 0, len(ClipperResponsibilityAllowList))
	for _, r := range ClipperResponsibilityAllowList {
		names = append(names, r.Name)
	}
	return names
}
