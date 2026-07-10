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

// TestClipperResponsibilityAllowListIsComplete asserts Streamclone owns no
// clipper product responsibilities after the boundary lock.
func TestClipperResponsibilityAllowListIsComplete(t *testing.T) {
	if len(ClipperResponsibilityAllowList) != 0 {
		t.Fatalf("ClipperResponsibilityAllowList must be empty after boundary lock; got %v",
			AllowedResponsibilityNames())
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

// TestNonRenderMetadataSurfacesDoNotTrip ensures ordinary metadata/API code
// without clip-render markers does not trip the allow-list guard.
func TestNonRenderMetadataSurfacesDoNotTrip(t *testing.T) {
	cases := []struct {
		name    string
		relPath string
		content string
	}{
		{
			name:    "metadata health handler",
			relPath: "internal/metadata/api/health.go",
			content: "package api\nfunc health(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) }\n",
		},
		{
			name:    "chat auth cookie helper",
			relPath: "internal/chat/auth/session.go",
			content: "package auth\nfunc ClearSession(w http.ResponseWriter) {}\n",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if isClipRenderTouching(tc.relPath, tc.content) {
				t.Fatalf("non-render surface %q must not trip the guard", tc.relPath)
			}
		})
	}
}

// --- Phase 5 integration surface (retired) --------------------------------
//
// Clip Studio / ReplayForge product surfaces were removed from Streamclone
// (no /studio redirect, no /v1/clipper proxy). These tests now assert absence
// of those surfaces and still detect render markers if someone reintroduces them.

var phase5RetiredIntegrationFiles = []string{
	"frontend/src/utils/studioLink.ts",
	"frontend/src/components/StudioRedirect.tsx",
}

var caddyProxyConfigFiles = []string{
	"deploy/Caddyfile",
	"deploy/Caddyfile.local-tunnel",
}

// integrationForbiddenMarkers are render/transcription/acquisition/artifact
// tokens that must never appear if clipper integration files are reintroduced.
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

// TestPhase5ClipperIntegrationSurfaceRemoved asserts Streamclone no longer ships
// ReplayForge deeplink files or a /v1/clipper Caddy proxy.
func TestPhase5ClipperIntegrationSurfaceRemoved(t *testing.T) {
	root := repoRoot(t)

	for _, rel := range phase5RetiredIntegrationFiles {
		path := filepath.Join(root, filepath.FromSlash(rel))
		if _, err := os.Stat(path); err == nil {
			t.Fatalf("retired ReplayForge integration file %q must not exist in Streamclone", rel)
		}
	}

	for _, rel := range caddyProxyConfigFiles {
		raw, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
		if err != nil {
			t.Fatalf("Caddy config %q must exist: %v", rel, err)
		}
		content := string(raw)
		if strings.Contains(content, "route @clipper") || strings.Contains(content, "path /v1/clipper") {
			t.Fatalf("%s: /v1/clipper proxy must be removed from Streamclone (ReplayForge is a sibling product)", rel)
		}
		if block := extractClipperProxyBlock(content); block != "" {
			t.Fatalf("%s: unexpected route @clipper block still present", rel)
		}
	}
}

// TestPhase5IntegrationGuardDetectsRenderMarkers is the negative control for the
// Phase 5 integration guard: it proves the scan fails when clip render /
// transcription / acquisition markers are added to a hypothetical integration
// surface, without dirtying the tree.
func TestPhase5IntegrationGuardDetectsRenderMarkers(t *testing.T) {
	cases := []struct {
		name    string
		relPath string
		content string
	}{
		{
			name:    "ffmpeg render in a studio link builder",
			relPath: "frontend/src/utils/studioLink.ts",
			content: "export function render(){ return spawn('ffmpeg', ['-i', src, '-c:v', 'libx264']) }\n",
		},
		{
			name:    "whisper transcription in a redirect component",
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
		"window.location.replace('http://example.invalid')\n",           // redirect
		"// mirror the last known Job_State and describe the failure\n", // mirror/describe are allowed
	}
	for _, c := range clean {
		if got := scanIntegrationContentForViolations("frontend/src/utils/studioLink.ts", c); len(got) != 0 {
			t.Fatalf("permitted content must not trip the guard, got: %v (content=%q)", got, c)
		}
	}
}
