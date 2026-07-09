package boundaryguard

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// scanRoots are the Streamclone Go source trees the ownership boundary applies
// to. ReplayForge owns clip render/transcription/editor/export; these trees
// must not reintroduce that code.
var scanRoots = []string{"cmd", "internal"}

// playbackExecAllowPrefixes lists the packages permitted to exec FFmpeg /
// Streamlink. Playback relay remuxes live/VOD HLS with "-c copy" and is a
// Streamclone-owned responsibility (see .kiro/steering/playback.md), distinct
// from clip render/transcription/editor/export owned by ReplayForge.
var playbackExecAllowPrefixes = []string{
	"internal/video/",
}

// renderMarker is a substring that indicates clip render, transcription, or
// editor/export code. None of these have a legitimate use in Streamclone Go
// services, so they are forbidden anywhere under cmd/ and internal/.
type renderMarker struct {
	// token is matched case-insensitively against source content.
	token string
	// why explains the ownership boundary the token would violate.
	why string
}

// forbiddenRenderMarkers denotes ReplayForge-owned responsibilities that must
// never appear in Streamclone Go source (Requirements 1.1, 1.2, 1.3).
var forbiddenRenderMarkers = []renderMarker{
	// Requirement 1.2 — Whisper transcription belongs to ReplayForge.
	{token: "whisper", why: "Whisper transcription is a ReplayForge responsibility (Req 1.2)"},
	{token: "transcribeclip", why: "clip transcription is a ReplayForge responsibility (Req 1.2)"},
	// Requirement 1.1 — FFmpeg clip render/re-encode belongs to ReplayForge.
	// Playback relay uses "-c copy" (remux only); these encode/filter tokens
	// mark a real render, which must not live in Streamclone Go.
	{token: "libx264", why: "clip video re-encode/render is a ReplayForge responsibility (Req 1.1)"},
	{token: "libx265", why: "clip video re-encode/render is a ReplayForge responsibility (Req 1.1)"},
	{token: "-c:v ", why: "clip video re-encode/render is a ReplayForge responsibility (Req 1.1)"},
	{token: "-filter_complex", why: "clip video filter/render is a ReplayForge responsibility (Req 1.1)"},
	{token: "-vf ", why: "clip video filter/render is a ReplayForge responsibility (Req 1.1)"},
	{token: "drawtext", why: "caption burn-in/render is a ReplayForge responsibility (Req 1.3)"},
	{token: "subtitles=", why: "subtitle burn-in/render is a ReplayForge responsibility (Req 1.3)"},
	// Requirement 1.3 — clip editor/export symbols belong to ReplayForge.
	{token: "renderclip", why: "clip render is a ReplayForge responsibility (Req 1.1)"},
	{token: "cliprender", why: "clip render is a ReplayForge responsibility (Req 1.1)"},
	{token: "renderworker", why: "the render worker is a ReplayForge responsibility (Req 1.1)"},
	{token: "burncaption", why: "caption burn-in is a ReplayForge responsibility (Req 1.3)"},
	{token: "burnsubtitle", why: "subtitle burn-in is a ReplayForge responsibility (Req 1.3)"},
	{token: "exportclip", why: "clip export is a ReplayForge responsibility (Req 1.3)"},
	{token: "exportprofile", why: "export profiles are a ReplayForge responsibility (Req 1.3)"},
}

// forbiddenExecCommands are process executions that must only appear in
// allow-listed playback packages. FFmpeg/Streamlink outside playback signals a
// clip acquisition/render pipeline leaking into Streamclone Go.
var forbiddenExecCommands = []string{
	`exec.Command("ffmpeg"`,
	`exec.Command("ffprobe"`,
	`exec.Command("streamlink"`,
	`exec.Command("whisper"`,
}

// repoRoot resolves the repository root from this test file's location
// (<root>/internal/boundaryguard/boundary_guard_test.go).
func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}

// isPlaybackExecAllowed reports whether relPath (forward-slash, repo-relative)
// is a package permitted to exec FFmpeg/Streamlink for playback relay.
func isPlaybackExecAllowed(relPath string) bool {
	for _, prefix := range playbackExecAllowPrefixes {
		if strings.HasPrefix(relPath, prefix) {
			return true
		}
	}
	return false
}

// scanContentForViolations applies the boundary rules to a single file's
// content and returns human-readable violation messages. relPath is the
// forward-slash, repo-relative path used for allow-list matching and messages.
func scanContentForViolations(relPath, content string) []string {
	var violations []string
	lower := strings.ToLower(content)

	for _, m := range forbiddenRenderMarkers {
		if strings.Contains(lower, strings.ToLower(m.token)) {
			violations = append(violations,
				relPath+": forbidden render/transcription/editor marker "+quote(m.token)+" — "+m.why)
		}
	}

	if !isPlaybackExecAllowed(relPath) {
		for _, cmd := range forbiddenExecCommands {
			if strings.Contains(content, cmd) {
				violations = append(violations,
					relPath+": forbidden FFmpeg/Streamlink/Whisper exec "+quote(cmd)+
						" — clip acquisition/render is a ReplayForge responsibility (Req 1.1/1.2)")
			}
		}
	}
	return violations
}

// quote wraps a token in double quotes for readable failure output without
// pulling in fmt.
func quote(s string) string { return "\"" + s + "\"" }

// clipMarkers are path/name fragments that identify clip-pipeline code.
var clipMarkers = []string{"clip"}

// renderVerbMarkers are path/name fragments that identify render / acquisition
// / editor / export work. Combined with a clip marker in the same package path
// they signal a clip render pipeline — a ReplayForge responsibility that is not
// on the Streamclone clipper allow-list (Requirement 1.6).
var renderVerbMarkers = []string{
	"render", "encode", "transcode", "transcrib", "editor",
	"export", "caption", "subtitle", "ffmpeg", "whisper", "burn",
}

// renderRouteLiterals are route-path substrings that would mean Streamclone Go
// is serving a clip render/acquisition route. Serving such a route is outside
// the allow-list (Requirement 1.6); the same-origin proxy path (/v1/clipper/*)
// is passthrough routing and deliberately excluded here.
var renderRouteLiterals = []string{
	"/v1/clipper/render",
	"/clipper/render",
	"/clip/render",
	"/clips/render",
	"/v1/clipper/transcode",
	"/v1/clipper/export",
}

// pathHasClipRenderSignal reports whether a repo-relative, forward-slash path
// looks like a clip render/editor/export package: it must contain a clip marker
// AND a render-verb marker (e.g. "internal/clipexport/", "internal/clipper/
// render.go"). This catches out-of-list clipper render packages even when they
// avoid the specific FFmpeg/Whisper tokens the render-marker blocklist knows.
func pathHasClipRenderSignal(relPath string) bool {
	lower := strings.ToLower(relPath)
	hasClip := false
	for _, m := range clipMarkers {
		if strings.Contains(lower, m) {
			hasClip = true
			break
		}
	}
	if !hasClip {
		return false
	}
	for _, v := range renderVerbMarkers {
		if strings.Contains(lower, v) {
			return true
		}
	}
	return false
}

// contentHasRenderRoute reports whether the source serves a clip render/
// acquisition route (as opposed to the allow-listed same-origin proxy).
func contentHasRenderRoute(content string) bool {
	lower := strings.ToLower(content)
	for _, lit := range renderRouteLiterals {
		if strings.Contains(lower, lit) {
			return true
		}
	}
	return false
}

// isClipRenderTouching reports whether a file performs (or routes to) clip
// render/transcription/editor/export/acquisition. It combines three signals:
//   - a forbidden render marker or FFmpeg/Whisper/Streamlink exec (token-level,
//     shared with the render-marker guard), OR
//   - a clip-render package path (name-level), OR
//   - a clip render route literal (route-level).
//
// Playback relay (internal/video/*) is excluded from the exec signal because it
// legitimately remuxes HLS with "-c copy"; it is not clip render.
func isClipRenderTouching(relPath, content string) bool {
	lower := strings.ToLower(content)
	for _, m := range forbiddenRenderMarkers {
		if strings.Contains(lower, strings.ToLower(m.token)) {
			return true
		}
	}
	if !isPlaybackExecAllowed(relPath) {
		for _, cmd := range forbiddenExecCommands {
			if strings.Contains(content, cmd) {
				return true
			}
		}
	}
	if pathHasClipRenderSignal(relPath) {
		return true
	}
	return contentHasRenderRoute(content)
}

// TestNoClipRenderOrTranscriptionInStreamcloneGo is the ownership-boundary
// guard. It statically scans cmd/* and internal/* and fails if clip render,
// Whisper transcription, or editor/export code is reintroduced, so CI blocks
// the boundary from eroding.
//
// Validates: Requirements 1.1, 1.2, 1.3
func TestNoClipRenderOrTranscriptionInStreamcloneGo(t *testing.T) {
	root := repoRoot(t)

	// Skip the guard's own package; it necessarily documents and lists the
	// forbidden tokens as string literals and prose.
	const selfPkgPrefix = "internal/boundaryguard/"

	var violations []string
	scanned := 0

	for _, sub := range scanRoots {
		start := filepath.Join(root, sub)
		if _, err := os.Stat(start); err != nil {
			continue
		}
		err := filepath.WalkDir(start, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				return nil
			}
			if !strings.HasSuffix(path, ".go") {
				return nil
			}
			rel, relErr := filepath.Rel(root, path)
			if relErr != nil {
				rel = path
			}
			rel = filepath.ToSlash(rel)
			if strings.HasPrefix(rel, selfPkgPrefix) {
				return nil
			}
			raw, readErr := os.ReadFile(path)
			if readErr != nil {
				return readErr
			}
			scanned++
			violations = append(violations, scanContentForViolations(rel, string(raw))...)
			return nil
		})
		if err != nil {
			t.Fatalf("walk %s: %v", start, err)
		}
	}

	if scanned == 0 {
		t.Fatalf("boundary guard scanned no Go files under %v; check repo root resolution", scanRoots)
	}
	if len(violations) > 0 {
		t.Fatalf("ownership boundary violated: ReplayForge owns FFmpeg/Whisper/editor/export, "+
			"Streamclone Go must not (Requirements 1.1-1.3). Move this code to ReplayForge "+
			"(sibling repo, host :8095):\n  - %s", strings.Join(violations, "\n  - "))
	}
}

// TestBoundaryGuardDetectsReintroducedSymbols is a negative control proving the
// guard actually fails when a forbidden symbol is added under cmd/ or
// internal/. It exercises the same scan logic against synthetic content so the
// "fails on reintroduction" acceptance is verified without dirtying the tree.
//
// Validates: Requirements 1.1, 1.2, 1.3
func TestBoundaryGuardDetectsReintroducedSymbols(t *testing.T) {
	cases := []struct {
		name    string
		relPath string
		content string
	}{
		{
			name:    "whisper transcription in analytics",
			relPath: "internal/analytics/transcribe.go",
			content: "package analytics\n// import whisper model for transcription\n",
		},
		{
			name:    "ffmpeg clip render in a new clipper package",
			relPath: "internal/clipper/render.go",
			content: "package clipper\nfunc render() { exec.Command(\"ffmpeg\", \"-i\", src, \"-c:v\", \"libx264\", out) }\n",
		},
		{
			name:    "caption burn-in editor code",
			relPath: "internal/clipeditor/captions.go",
			content: "package clipeditor\n// burnCaptions renders drawtext subtitles into the clip\n",
		},
		{
			name:    "streamlink acquisition outside playback",
			relPath: "cmd/clipper/main.go",
			content: "package main\nfunc main() { exec.Command(\"streamlink\", url, \"-o\", seg) }\n",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := scanContentForViolations(tc.relPath, tc.content); len(got) == 0 {
				t.Fatalf("expected boundary violation for %q, got none", tc.relPath)
			}
		})
	}
}

// TestBoundaryGuardAllowsPlaybackRemux ensures the guard does not false-positive
// on Streamclone's owned playback relay, which shells out to FFmpeg/Streamlink
// with "-c copy" to remux HLS. Playback stays in Streamclone Go.
func TestBoundaryGuardAllowsPlaybackRemux(t *testing.T) {
	playback := "package worker\n" +
		"func relay() { exec.Command(\"ffmpeg\", \"-i\", \"pipe:0\", \"-c\", \"copy\", \"-f\", \"flv\", rtmp) }\n" +
		"// sl := exec.Command(\"streamlink\", url, quality)\n"
	if got := scanContentForViolations("internal/video/worker/worker.go", playback); len(got) != 0 {
		t.Fatalf("playback remux must be allowed, got violations: %v", got)
	}
}

// TestClipperResponsibilityAllowListIsComplete asserts the documented allow-list
// (allowlist.go) enumerates exactly the five clipper-related responsibilities
// Streamclone is permitted to own (Requirement 1.6). If the inventory drifts,
// the guard's default-deny message would misstate what Streamclone may own, so
// the list is pinned here.
//
// Validates: Requirements 1.6
func TestClipperResponsibilityAllowListIsComplete(t *testing.T) {
	want := map[string]bool{
		"moment_context":        false,
		"export_moment_trigger": false,
		"studio_redirect":       false,
		"job_mirror":            false,
		"callback_auth":         false,
	}
	for _, r := range ClipperResponsibilityAllowList {
		if _, ok := want[r.Name]; !ok {
			t.Fatalf("allow-list contains unexpected responsibility %q; "+
				"Requirement 1.6 permits only moment_context, export_moment_trigger, "+
				"studio_redirect, job_mirror, callback_auth", r.Name)
		}
		if r.Requirement != "1.6" {
			t.Errorf("responsibility %q maps to requirement %q, want 1.6", r.Name, r.Requirement)
		}
		want[r.Name] = true
	}
	for name, seen := range want {
		if !seen {
			t.Errorf("allow-list is missing required clipper responsibility %q (Req 1.6)", name)
		}
	}
}

// TestClipperRenderStaysOutsideResponsibilityAllowList is the Requirement 1.6
// guard. Clip render/transcription/editor/export is NOT on the clipper
// responsibility allow-list (see ClipperResponsibilityAllowList), so any
// render-touching package or route under cmd/* or internal/* — detected by
// render markers, clip-render package paths, or clip render route literals —
// trips review. Playback relay (internal/video/*) is the one allow-listed
// FFmpeg/Streamlink user and is excluded.
//
// This complements the render-marker guard
// (TestNoClipRenderOrTranscriptionInStreamcloneGo, Req 1.1-1.3) by adding
// name-level and route-level signals and by framing the failure against the
// documented allow-list of what Streamclone IS allowed to own.
//
// Validates: Requirements 1.6
func TestClipperRenderStaysOutsideResponsibilityAllowList(t *testing.T) {
	root := repoRoot(t)
	const selfPkgPrefix = "internal/boundaryguard/"

	var violations []string
	scanned := 0

	for _, sub := range scanRoots {
		start := filepath.Join(root, sub)
		if _, err := os.Stat(start); err != nil {
			continue
		}
		err := filepath.WalkDir(start, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() || !strings.HasSuffix(path, ".go") {
				return nil
			}
			rel, relErr := filepath.Rel(root, path)
			if relErr != nil {
				rel = path
			}
			rel = filepath.ToSlash(rel)
			if strings.HasPrefix(rel, selfPkgPrefix) {
				return nil
			}
			raw, readErr := os.ReadFile(path)
			if readErr != nil {
				return readErr
			}
			scanned++
			if isClipRenderTouching(rel, string(raw)) {
				violations = append(violations, rel)
			}
			return nil
		})
		if err != nil {
			t.Fatalf("walk %s: %v", start, err)
		}
	}

	if scanned == 0 {
		t.Fatalf("allow-list guard scanned no Go files under %v; check repo root resolution", scanRoots)
	}
	if len(violations) > 0 {
		t.Fatalf("clipper responsibility allow-list violated (Requirement 1.6): the following "+
			"render-touching package(s)/route(s) are outside the allow-list. Streamclone Go may own "+
			"ONLY %v; clip render/transcription/editor/export belongs to ReplayForge (host :8095):\n  - %s",
			AllowedResponsibilityNames(), strings.Join(violations, "\n  - "))
	}
}

// TestClipperRenderOutsideAllowListTripsReview is the negative control for the
// allow-list guard: it proves render-touching packages/routes outside the
// allow-list are detected, including cases that carry NO FFmpeg/Whisper token
// (caught by clip-render package path) and clip render routes (caught by route
// literal). This verifies the "out-of-list render-touching route/package trips
// review" acceptance without dirtying the tree.
//
// Validates: Requirements 1.6
func TestClipperRenderOutsideAllowListTripsReview(t *testing.T) {
	cases := []struct {
		name    string
		relPath string
		content string
	}{
		{
			name:    "ffmpeg clip render package",
			relPath: "internal/clipper/render.go",
			content: "package clipper\nfunc render() { exec.Command(\"ffmpeg\", \"-i\", src, \"-c:v\", \"libx264\", out) }\n",
		},
		{
			name:    "clip export package by path, no known token",
			relPath: "internal/clipexport/profile.go",
			content: "package clipexport\n// applies the selected export profile params to the pipeline\n",
		},
		{
			name:    "clip editor package by path, no known token",
			relPath: "internal/clipeditor/timeline.go",
			content: "package clipeditor\n// trims the timeline before handing off\n",
		},
		{
			name:    "streamclone serving a clip render route",
			relPath: "internal/metadata/api/clip_routes.go",
			content: "package api\nfunc reg(r chi.Router) { r.Post(\"/v1/clipper/render\", h.render) }\n",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if !isClipRenderTouching(tc.relPath, tc.content) {
				t.Fatalf("expected %q to trip the allow-list guard, but it did not", tc.relPath)
			}
		})
	}
}

// TestAllowListedClipperResponsibilitiesDoNotTrip ensures the allow-list guard
// does not false-positive on the permitted clipper responsibilities. Building
// moment_context, triggering Export Moment, redirecting /studio, mirroring
// Job_State, and authenticating callbacks are integration surfaces with no
// render — they must pass.
//
// Validates: Requirements 1.6
func TestAllowListedClipperResponsibilitiesDoNotTrip(t *testing.T) {
	cases := []struct {
		name    string
		relPath string
		content string
	}{
		{
			name:    "moment_context builder",
			relPath: "internal/analytics/moment_context.go",
			content: "package analytics\n// build moment_context {channel, vod_id, start, end, reason}\n",
		},
		{
			name:    "export moment trigger client",
			relPath: "internal/analytics/export_moment.go",
			content: "package analytics\n// POST /v1/jobs with Auth_Token and idempotency_key, store returned job id\n",
		},
		{
			name:    "studio redirect handler",
			relPath: "internal/metadata/api/studio_redirect.go",
			content: "package api\n// redirect /studio?job={id} to the ReplayForge Clip Studio URL\n",
		},
		{
			name:    "job mirror callback auth handler",
			relPath: "internal/metadata/api/clipper_callback.go",
			content: "package api\n// authed idempotent Status_Callback updates the Job_Mirror; 401 on missing Auth_Token\n",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if isClipRenderTouching(tc.relPath, tc.content) {
				t.Fatalf("allow-listed responsibility %q must not trip the guard", tc.relPath)
			}
		})
	}
}

// --- Phase 5 integration surface (non-Go) --------------------------------
//
// The render-marker and allow-list guards above statically scan Streamclone Go
// (cmd/*, internal/*), which already covers the Phase 5 Go additions
// (internal/analytics/clip_replayforge*.go, job_mirror*.go). The Phase 5
// integration wiring also introduced non-Go surfaces the Go scan does not see:
// the frontend `/studio` redirect + Recent-Clips link builder and the Caddy
// same-origin `/v1/clipper/*` reverse proxy. Those are integration/orchestration
// surfaces (redirect, mirror display, passthrough proxy) and must stay within
// the allowed surface — they must never carry clip render / FFmpeg / Whisper /
// transcription / durable-artifact / caption-burn-in code, which belongs to
// ReplayForge (host :8095).

// phase5NonGoIntegrationFiles are the exact Phase 5 non-Go integration files the
// wiring added (RF-P5-005 / RF-P5-007 / RF-P5-008). They are enumerated
// explicitly (rather than scanning all of frontend/) so this guard stays scoped
// to the integration surface this spec owns and does not police unrelated,
// pre-existing frontend code.
var phase5NonGoIntegrationFiles = []string{
	"frontend/src/utils/studioLink.ts",
	"frontend/src/components/StudioRedirect.tsx",
}

// caddyProxyConfigFiles are the Caddy configs that define the same-origin
// `/v1/clipper/*` passthrough proxy to host ReplayForge :8095 (RF-P5-008).
var caddyProxyConfigFiles = []string{
	"deploy/Caddyfile",
	"deploy/Caddyfile.local-tunnel",
}

// integrationForbiddenMarkers are render/transcription/acquisition/artifact
// tokens that must never appear in the non-Go integration surface. Bare verbs
// that legitimately occur in redirect/proxy code (e.g. "encode" inside
// encodeURIComponent, or React "render") are deliberately excluded; only
// clip-render-specific tokens are matched so allowed integration terms
// (proxying, mirroring status, describing clipper failures) stay permitted.
var integrationForbiddenMarkers = []renderMarker{
	{token: "ffmpeg", why: "FFmpeg clip render/acquisition is a ReplayForge responsibility (Req 6.5)"},
	{token: "ffprobe", why: "FFmpeg/ffprobe media processing is a ReplayForge responsibility (Req 6.5)"},
	{token: "streamlink", why: "source acquisition is a ReplayForge responsibility (Req 6.5)"},
	{token: "whisper", why: "Whisper transcription is a ReplayForge responsibility (Req 1.2/6.5)"},
	{token: "transcrib", why: "transcription is a ReplayForge responsibility (Req 1.2/6.5)"},
	{token: "libx264", why: "clip video re-encode/render is a ReplayForge responsibility (Req 6.5)"},
	{token: "libx265", why: "clip video re-encode/render is a ReplayForge responsibility (Req 6.5)"},
	{token: "-filter_complex", why: "clip video filter/render is a ReplayForge responsibility (Req 6.5)"},
	{token: "-c:v ", why: "clip video re-encode/render is a ReplayForge responsibility (Req 6.5)"},
	{token: "-vf ", why: "clip video filter/render is a ReplayForge responsibility (Req 6.5)"},
	{token: "drawtext", why: "caption burn-in/render is a ReplayForge responsibility (Req 1.3/6.5)"},
	{token: "subtitles=", why: "subtitle burn-in/render is a ReplayForge responsibility (Req 1.3/6.5)"},
	{token: "burn-in", why: "caption/subtitle burn-in is a ReplayForge responsibility (Req 1.3/6.5)"},
	{token: "renderclip", why: "clip render is a ReplayForge responsibility (Req 6.5)"},
	{token: "cliprender", why: "clip render is a ReplayForge responsibility (Req 6.5)"},
	{token: "renderworker", why: "the render worker is a ReplayForge responsibility (Req 6.5)"},
	{token: "burncaption", why: "caption burn-in is a ReplayForge responsibility (Req 1.3/6.5)"},
	{token: "burnsubtitle", why: "subtitle burn-in is a ReplayForge responsibility (Req 1.3/6.5)"},
	{token: "exportclip", why: "clip export is a ReplayForge responsibility (Req 6.5)"},
}

// scanIntegrationContentForViolations applies integrationForbiddenMarkers to a
// single non-Go integration file's content, returning human-readable violations.
func scanIntegrationContentForViolations(relPath, content string) []string {
	var violations []string
	lower := strings.ToLower(content)
	for _, m := range integrationForbiddenMarkers {
		if strings.Contains(lower, strings.ToLower(m.token)) {
			violations = append(violations,
				relPath+": forbidden render/transcription/acquisition marker "+quote(m.token)+" — "+m.why)
		}
	}
	return violations
}

// extractClipperProxyBlock returns the `route @clipper { ... }` block from a
// Caddy config via brace matching, or "" if the block is absent. Scoping to the
// block keeps the guard from policing unrelated proxy config in the same file.
func extractClipperProxyBlock(content string) string {
	idx := strings.Index(content, "route @clipper")
	if idx < 0 {
		return ""
	}
	open := strings.Index(content[idx:], "{")
	if open < 0 {
		return ""
	}
	start := idx + open
	depth := 0
	for i := start; i < len(content); i++ {
		switch content[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return content[start : i+1]
			}
		}
	}
	return content[start:]
}

// TestPhase5ClipperIntegrationSurfaceStaysWithinAllowedSurface re-asserts the
// ownership boundary after the Phase 5 integration wiring (tasks RF-P5-001..008)
// for the non-Go surfaces the Go scan cannot see: the frontend `/studio`
// redirect + Recent-Clips link builder and the Caddy `/v1/clipper/*` proxy. It
// confirms the integration wiring did not smuggle clip render / transcription /
// acquisition / burn-in responsibilities into Streamclone, and that the clipper
// proxy is a passthrough to host ReplayForge :8095 (the allowed proxy surface).
//
// Validates: Requirements 6.5, 1.6
func TestPhase5ClipperIntegrationSurfaceStaysWithinAllowedSurface(t *testing.T) {
	root := repoRoot(t)

	var violations []string

	for _, rel := range phase5NonGoIntegrationFiles {
		raw, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
		if err != nil {
			t.Fatalf("Phase 5 integration file %q must exist for the guard to assert it: %v", rel, err)
		}
		violations = append(violations, scanIntegrationContentForViolations(rel, string(raw))...)
	}

	for _, rel := range caddyProxyConfigFiles {
		raw, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
		if err != nil {
			t.Fatalf("Caddy config %q must exist for the guard to assert the clipper proxy: %v", rel, err)
		}
		block := extractClipperProxyBlock(string(raw))
		if block == "" {
			t.Fatalf("%s: could not find the `route @clipper { ... }` proxy block", rel)
		}
		// The clipper proxy must be a passthrough to host ReplayForge :8095 —
		// an allowed routing surface, not a Streamclone-owned render route.
		if !strings.Contains(block, "reverse_proxy host.docker.internal:8095") {
			violations = append(violations,
				rel+": clipper proxy block must reverse_proxy to host ReplayForge (host.docker.internal:8095); "+
					"Streamclone must not terminate/render clipper requests locally (Req 6.6/6.5)")
		}
		violations = append(violations, scanIntegrationContentForViolations(rel+" (@clipper block)", block)...)
	}

	if len(violations) > 0 {
		t.Fatalf("Phase 5 clipper integration surface left the allowed surface (Requirements 6.5, 1.6): "+
			"the redirect/link builder and the /v1/clipper/* proxy may only redirect, mirror, and passthrough — "+
			"clip render/transcription/acquisition/burn-in belongs to ReplayForge (host :8095):\n  - %s",
			strings.Join(violations, "\n  - "))
	}
}

// TestPhase5IntegrationGuardDetectsRenderMarkers is the negative control for the
// Phase 5 integration guard: it proves the scan fails when clip render /
// transcription / acquisition markers are added to the non-Go integration
// surface, without dirtying the tree.
//
// Validates: Requirements 6.5, 1.6
func TestPhase5IntegrationGuardDetectsRenderMarkers(t *testing.T) {
	cases := []struct {
		name    string
		relPath string
		content string
	}{
		{
			name:    "ffmpeg render in the studio link builder",
			relPath: "frontend/src/utils/studioLink.ts",
			content: "export function render(){ return spawn('ffmpeg', ['-i', src, '-c:v', 'libx264']) }\n",
		},
		{
			name:    "whisper transcription in the redirect component",
			relPath: "frontend/src/components/StudioRedirect.tsx",
			content: "// call whisper to transcribe the clip before redirecting\n",
		},
		{
			name:    "caption burn-in via drawtext in a caddy block",
			relPath: "deploy/Caddyfile (@clipper block)",
			content: "route @clipper { exec ffmpeg -vf drawtext=subtitles=cap.srt }\n",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := scanIntegrationContentForViolations(tc.relPath, tc.content); len(got) == 0 {
				t.Fatalf("expected an integration boundary violation for %q, got none", tc.relPath)
			}
		})
	}
}

// TestPhase5IntegrationSurfaceCleanFilesDoNotTrip ensures the integration guard
// does not false-positive on the real, permitted integration content —
// including terms that merely resemble forbidden verbs (encodeURIComponent,
// React render, "describe failure", "mirror status").
//
// Validates: Requirements 6.5, 1.6
func TestPhase5IntegrationSurfaceCleanFilesDoNotTrip(t *testing.T) {
	clean := []string{
		"return `${base}${encodeURIComponent(jobId)}`\n",                // encode* is not a render token
		"window.location.replace(replayforgeStudioUrl(base, jobId))\n",  // redirect
		"// mirror the last known Job_State and describe the failure\n", // mirror/describe are allowed
		"route @clipper { reverse_proxy host.docker.internal:8095 }\n",  // passthrough proxy
	}
	for _, c := range clean {
		if got := scanIntegrationContentForViolations("frontend/src/utils/studioLink.ts", c); len(got) != 0 {
			t.Fatalf("permitted integration content must not trip the guard, got: %v (content=%q)", got, c)
		}
	}
}
