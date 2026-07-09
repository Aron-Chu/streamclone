package boundaryguard

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// docsScanRoots are the repo-relative directories (and root ".") to scan for
// unsupported cutover/production-ready claims. Spec docs (.kiro/specs/) are
// excluded since they define the rules rather than making claims.
// When scanning ".", only root-level .md files are checked (max depth 0);
// "docs" is walked recursively.
var docsScanRoots = []string{"docs"}

// docsScanExcludePrefixes are repo-relative forward-slash path prefixes that
// are excluded from the docs-guard scan. The spec directory defines the rules
// and legitimately references forbidden phrases as Non-Goals, Risk Register,
// or task descriptions — not as claims.
var docsScanExcludePrefixes = []string{
	".kiro/specs/",
	".kiro/steering/",
	".agents/",
	".claude/",
	".codex/",
	".cursor/",
	".codegraph/",
	".git/",
	"node_modules/",
	"frontend/",
}

// cutoverClaimPatterns are substrings that assert image cutover is complete.
// These MUST NOT appear in public docs without adjacent ops evidence or a
// qualifying negation/pending context. The guard below searches for these
// case-insensitively.
var cutoverClaimPatterns = []string{
	"cutover complete",
	"cutover is complete",
	"cutover has been completed",
	"cutover finished",
}

// productionReadyClaimPatterns are substrings that assert Auto Clipper or
// ReplayForge is production-ready. These MUST NOT appear without R2 + signed
// playback evidence context.
var productionReadyClaimPatterns = []string{
	"auto clipper production-ready",
	"auto clipper production ready",
	"auto-clipper production-ready",
	"auto-clipper production ready",
	"replayforge production-ready",
	"replayforge production ready",
	"clipper is production-ready",
	"clipper is production ready",
}

// cutoverQualifiers are phrases that, when found on the same line or within a
// narrow context window around a cutover/production-ready claim, indicate the
// claim is qualified (negated, conditional, or used as a rule/gate description)
// rather than an unsupported assertion. Case-insensitive matching.
var cutoverQualifiers = []string{
	"not complete",
	"not yet",
	"pending",
	"requires ops evidence",
	"without ops evidence",
	"do not claim",
	"must not claim",
	"shall not claim",
	"shall exclude",
	"no claim",
	"absent",
	"until",
	"before",
	"gate",
	"blocking claim",
	"evidence",
	"pre-cutover",
	"pre cutover",
	"without",
	"doc guard",
	"docs-guard",
	"docs guard",
}

// productionReadyQualifiers are phrases that qualify a production-ready claim
// as conditional, negated, or part of a gate definition.
var productionReadyQualifiers = []string{
	"not production",
	"not yet",
	"pending",
	"without r2",
	"absent",
	"no production-ready claim",
	"no production ready claim",
	"shall exclude",
	"gate",
	"requires",
	"evidence",
	"do not claim",
	"must not claim",
	"doc guard",
	"docs-guard",
	"docs guard",
	"until",
	"before",
	"blocking",
}

// isQualified reports whether the line containing the match also contains a
// qualifier that makes the claim conditional/negated rather than unsupported.
func isQualified(line string, qualifiers []string) bool {
	lower := strings.ToLower(line)
	for _, q := range qualifiers {
		if strings.Contains(lower, q) {
			return true
		}
	}
	return false
}

// scanDocFileForClaims scans a single markdown file's content for unsupported
// cutover-complete or production-ready claims. Returns human-readable violations.
func scanDocFileForClaims(relPath, content string) []string {
	var violations []string
	lines := strings.Split(content, "\n")

	for i, line := range lines {
		lower := strings.ToLower(line)

		// Check cutover claims.
		for _, pat := range cutoverClaimPatterns {
			if strings.Contains(lower, pat) && !isQualified(line, cutoverQualifiers) {
				violations = append(violations,
					relPath+":"+itoa(i+1)+": unsupported cutover claim "+
						quote(pat)+" without ops evidence qualifier (Requirement 7.7)")
			}
		}

		// Check production-ready claims.
		for _, pat := range productionReadyClaimPatterns {
			if strings.Contains(lower, pat) && !isQualified(line, productionReadyQualifiers) {
				violations = append(violations,
					relPath+":"+itoa(i+1)+": unsupported production-ready claim "+
						quote(pat)+" without R2 + signed-playback evidence qualifier (Requirement 8.7)")
			}
		}
	}
	return violations
}

// itoa is a minimal int-to-string without importing strconv.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	buf := [20]byte{}
	pos := len(buf)
	for n > 0 {
		pos--
		buf[pos] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[pos:])
}

// TestDocGuardBlocksUnsupportedCutoverClaims is the docs-guard test
// (RF-P6-012). It statically scans all .md files in docs/ and root-level .md
// files, then fails CI if any unsupported "image cutover complete" or "Auto
// Clipper production-ready" claim is found without qualifying evidence or
// negation context.
//
// Requirements 7.7: public repos SHALL exclude any claim that image cutover
// is complete without private-ops evidence (EOG-001/002/003).
// Requirement 8.7: public repos SHALL exclude any production-ready claim for
// Auto Clipper without R2 durable storage + signed-playback evidence.
//
// Validates: Requirements 7.7, 8.7
func TestDocGuardBlocksUnsupportedCutoverClaims(t *testing.T) {
	root := repoRoot(t)

	var violations []string
	scanned := 0

	// Scan docs/ recursively.
	for _, sub := range docsScanRoots {
		start := filepath.Join(root, sub)
		if _, err := os.Stat(start); err != nil {
			continue
		}
		err := filepath.WalkDir(start, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				rel, relErr := filepath.Rel(root, path)
				if relErr != nil {
					rel = path
				}
				rel = filepath.ToSlash(rel) + "/"
				for _, prefix := range docsScanExcludePrefixes {
					if strings.HasPrefix(rel, prefix) {
						return filepath.SkipDir
					}
				}
				return nil
			}
			if !strings.HasSuffix(strings.ToLower(path), ".md") {
				return nil
			}
			rel, relErr := filepath.Rel(root, path)
			if relErr != nil {
				rel = path
			}
			rel = filepath.ToSlash(rel)

			raw, readErr := os.ReadFile(path)
			if readErr != nil {
				return readErr
			}
			scanned++
			violations = append(violations, scanDocFileForClaims(rel, string(raw))...)
			return nil
		})
		if err != nil {
			t.Fatalf("walk %s: %v", start, err)
		}
	}

	// Scan root-level .md files (README.md, AGENTS.md, SECURITY.md, etc.).
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatalf("read root dir: %v", err)
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(strings.ToLower(e.Name()), ".md") {
			continue
		}
		path := filepath.Join(root, e.Name())
		raw, readErr := os.ReadFile(path)
		if readErr != nil {
			t.Fatalf("read root .md %s: %v", e.Name(), readErr)
		}
		scanned++
		violations = append(violations, scanDocFileForClaims(e.Name(), string(raw))...)
	}

	if scanned == 0 {
		t.Fatalf("docs guard scanned no .md files; check repo root resolution")
	}
	if len(violations) > 0 {
		t.Fatalf("docs guard: unsupported cutover/production-ready claims found "+
			"(Requirements 7.7, 8.7). These claims require private-ops evidence "+
			"(EOG-001/002/003) or R2 + signed-playback evidence before assertion. "+
			"Add a qualifier (e.g. 'pending', 'requires ops evidence', 'not yet', "+
			"'without', 'do not claim', 'gate') or remove the unsupported claim:\n  - %s",
			strings.Join(violations, "\n  - "))
	}
	t.Logf("docs guard: scanned %d .md files, no unsupported claims found", scanned)
}

// TestDocGuardDetectsUnsupportedClaims is the negative control proving the
// docs guard catches unsupported cutover and production-ready claims. Exercises
// the scan logic against synthetic content.
//
// Validates: Requirements 7.7, 8.7
func TestDocGuardDetectsUnsupportedClaims(t *testing.T) {
	cases := []struct {
		name    string
		content string
	}{
		{
			name:    "bare cutover complete claim",
			content: "## Status\n\nThe image cutover complete as of July 2026.\n",
		},
		{
			name:    "cutover is complete assertion",
			content: "Image cutover is complete and running on promoted images.\n",
		},
		{
			name:    "auto clipper production-ready claim",
			content: "Auto Clipper production-ready for all invite accounts.\n",
		},
		{
			name:    "replayforge production ready claim",
			content: "ReplayForge production ready and serving clips.\n",
		},
		{
			name:    "cutover has been completed",
			content: "The cutover has been completed successfully.\n",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := scanDocFileForClaims("docs/test.md", tc.content); len(got) == 0 {
				t.Fatalf("expected docs-guard violation for %q, got none", tc.name)
			}
		})
	}
}

// TestDocGuardAllowsQualifiedClaims ensures the docs guard does not
// false-positive on qualified, conditional, negated, or gate-definition
// mentions of cutover/production-ready. Docs that describe the gate, state
// "pending", or negate the claim should pass.
//
// Validates: Requirements 7.7, 8.7
func TestDocGuardAllowsQualifiedClaims(t *testing.T) {
	cases := []struct {
		name    string
		content string
	}{
		{
			name:    "negated cutover claim",
			content: "Do not claim cutover complete until private ops evidence shows promoted images.\n",
		},
		{
			name:    "pending cutover",
			content: "Image cutover complete is pending EOG-001 evidence.\n",
		},
		{
			name:    "gate description",
			content: "The docs-guard blocks 'cutover complete' without ops evidence gate.\n",
		},
		{
			name:    "without qualifier",
			content: "No cutover complete without ops evidence.\n",
		},
		{
			name:    "production-ready with evidence qualifier",
			content: "No Auto Clipper production-ready claim without R2 + signed-playback evidence.\n",
		},
		{
			name:    "production-ready gate description",
			content: "The production-ready gate blocks Auto Clipper production-ready until R2 evidence exists.\n",
		},
		{
			name:    "shall exclude auto clipper production-ready",
			content: "Public repos SHALL exclude any Auto Clipper production-ready claim.\n",
		},
		{
			name:    "pre-cutover context",
			content: "Pre-cutover: production may still use streamclone images. cutover complete requires ops.\n",
		},
		{
			name:    "absent qualifier for production-ready",
			content: "Absent R2 + signed-playback evidence, no auto clipper production-ready assertion.\n",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := scanDocFileForClaims("docs/test.md", tc.content); len(got) != 0 {
				t.Fatalf("qualified claim should not trigger docs-guard violation, got: %v", got)
			}
		})
	}
}
