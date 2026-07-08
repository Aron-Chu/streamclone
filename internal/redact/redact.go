// Package redact is the Streamclone Go token-redaction chokepoint on the
// mirror / log / display path (spec auto-clipper-replayforge-productization,
// RF-P0-004, Requirement 1.7).
//
// It mirrors the ReplayForge Python chokepoint
// (backend/liveclipper/redaction.py) shape-for-shape so both sides of the
// HTTP boundary strip the same secret shapes — bearer / access / refresh /
// auth / clip (oauth: and standalone opaque) tokens — before any string is
// emitted to a log line, URL, filename/object key, or display string.
//
// INVARIANT: every emitted string on the mirror/log/display path passes through
// Redact before it leaves the process, so a Clip_Job token, access token, or
// refresh token is never present in bundles, URLs, filenames, logs, or display
// strings (Requirement 1.7). Redact is idempotent: applying it to an
// already-redacted string leaves the placeholder untouched.
package redact

import (
	"regexp"
	"strings"
)

// Placeholder replaces every matched token shape. It matches the ReplayForge
// Python chokepoint placeholder so redacted output is identical across the
// boundary.
const Placeholder = "<redacted>"

const (
	// valueClass is the character class for an opaque token value. Kept broad
	// so a greedy match consumes the whole secret; the value ends at the first
	// character outside this class (e.g. '&', space, quote).
	valueClass = `[A-Za-z0-9._~+/=\-]+`
	// sepClass matches separators between a labelled key and its value
	// ('=', ':', quotes, whitespace).
	sepClass = `["'=:\s]+`
)

var (
	// bearerRe matches "Authorization: Bearer <token>".
	bearerRe = regexp.MustCompile(`(?i)\bbearer\s+` + valueClass)
	// refreshRe matches refresh_token / refresh-token / refreshtoken "<token>".
	refreshRe = regexp.MustCompile(`(?i)refresh[_-]?token` + sepClass + valueClass)
	// accessRe matches access_token and variants.
	accessRe = regexp.MustCompile(`(?i)access[_-]?token` + sepClass + valueClass)
	// authTokenRe matches auth_token and variants.
	authTokenRe = regexp.MustCompile(`(?i)auth[_-]?token` + sepClass + valueClass)
	// oauthRe matches the IRC clip/oauth token form "oauth:<token>".
	oauthRe = regexp.MustCompile(`(?i)oauth:[A-Za-z0-9]+`)
	// standaloneRe matches a bare Twitch-style opaque token: a run of >= 30
	// lowercase alphanumerics. Adjacency to another alphanumeric is checked
	// separately (RE2 has no lookaround) so a lowercase run embedded in a
	// mixed-case identifier is left alone, matching the Python chokepoint.
	standaloneRe = regexp.MustCompile(`[a-z0-9]{30,}`)
)

// labelledPatterns are the key/value and oauth: shapes applied in order. They
// mirror the Python SECRET_PATTERNS list (minus the standalone-token branch,
// which needs manual boundary handling below).
var labelledPatterns = []*regexp.Regexp{
	bearerRe,
	refreshRe,
	accessRe,
	authTokenRe,
	oauthRe,
}

// Redact replaces every known token shape in s with Placeholder. It is the
// single redaction chokepoint for Streamclone's mirror/log/display path and is
// idempotent.
func Redact(s string) string {
	if s == "" {
		return ""
	}
	out := s
	for _, re := range labelledPatterns {
		out = re.ReplaceAllString(out, Placeholder)
	}
	return redactStandaloneTokens(out)
}

// redactStandaloneTokens replaces bare opaque tokens (>= 30 lowercase
// alphanumerics) that are not part of a longer mixed-case identifier. Because
// the run is maximal, the character adjacent to a match can only be a
// non-lowercase-alphanumeric; the guard therefore excludes runs bordered by an
// uppercase letter (or any alphanumeric), matching the Python lookbehind /
// lookahead of (?<![A-Za-z0-9]) ... (?![A-Za-z0-9]).
func redactStandaloneTokens(s string) string {
	locs := standaloneRe.FindAllStringIndex(s, -1)
	if len(locs) == 0 {
		return s
	}
	var b strings.Builder
	last := 0
	for _, loc := range locs {
		start, end := loc[0], loc[1]
		if start > 0 && isTokenBoundaryAlnum(s[start-1]) {
			continue
		}
		if end < len(s) && isTokenBoundaryAlnum(s[end]) {
			continue
		}
		b.WriteString(s[last:start])
		b.WriteString(Placeholder)
		last = end
	}
	if last == 0 {
		return s
	}
	b.WriteString(s[last:])
	return b.String()
}

// isTokenBoundaryAlnum reports whether c is an ASCII alphanumeric, matching the
// [A-Za-z0-9] boundary class used by the Python chokepoint.
func isTokenBoundaryAlnum(c byte) bool {
	return (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9')
}

// Log formats message plus key=value fields and redacts the result. Use it
// instead of assembling log strings by hand so no log line can leak a token
// shape. It mirrors the Python redact_log helper.
func Log(message string, fields ...string) string {
	parts := make([]string, 0, 1+len(fields)/2)
	parts = append(parts, message)
	for i := 0; i+1 < len(fields); i += 2 {
		parts = append(parts, fields[i]+"="+fields[i+1])
	}
	// Tolerate an odd trailing field so callers never silently drop data.
	if len(fields)%2 == 1 {
		parts = append(parts, fields[len(fields)-1])
	}
	return Redact(strings.Join(parts, " "))
}

// Display redacts any user-facing string before it is shown. It mirrors the
// Python format_display helper.
func Display(s string) string {
	return Redact(s)
}
