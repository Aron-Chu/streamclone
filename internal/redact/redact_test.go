package redact

import (
	"math/rand"
	"strings"
	"testing"
)

// TestRedactStripsEachTokenShape covers every token shape mirrored from the
// ReplayForge Python chokepoint: bearer, access, refresh, auth, and clip
// (oauth: + standalone opaque) tokens.
//
// Validates: Requirements 1.7
func TestRedactStripsEachTokenShape(t *testing.T) {
	cases := []struct {
		name   string
		in     string
		secret string
	}{
		{
			name:   "bearer header",
			in:     "Authorization: Bearer abc123DEF456ghi.jkl-mno",
			secret: "abc123DEF456ghi.jkl-mno",
		},
		{
			name:   "access_token kv",
			in:     "access_token=aabbccddeeff00112233",
			secret: "aabbccddeeff00112233",
		},
		{
			name:   "access-token colon",
			in:     "access-token: sometokenvalue12345",
			secret: "sometokenvalue12345",
		},
		{
			name:   "refresh_token kv",
			in:     "refresh_token=rt_9f3c8a7b6d5e4f3a2b1c",
			secret: "rt_9f3c8a7b6d5e4f3a2b1c",
		},
		{
			name:   "auth_token quoted",
			in:     `auth_token="at_secretvalue998877"`,
			secret: "at_secretvalue998877",
		},
		{
			name:   "oauth clip token",
			in:     "connecting with oauth:abcd1234efgh5678 now",
			secret: "oauth:abcd1234efgh5678",
		},
		{
			name:   "standalone opaque token",
			in:     "job failed for pmnbvcxzlkjhgfdsaqwertyuiop0918 today",
			secret: "pmnbvcxzlkjhgfdsaqwertyuiop0918",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Redact(tc.in)
			if strings.Contains(got, tc.secret) {
				t.Fatalf("Redact(%q) = %q; secret %q survived", tc.in, got, tc.secret)
			}
			if !strings.Contains(got, Placeholder) {
				t.Fatalf("Redact(%q) = %q; expected placeholder %q", tc.in, got, Placeholder)
			}
		})
	}
}

// TestRedactEmptyString confirms empty input stays empty.
func TestRedactEmptyString(t *testing.T) {
	if got := Redact(""); got != "" {
		t.Fatalf("Redact(\"\") = %q, want empty", got)
	}
}

// TestRedactIdempotent confirms re-redacting a redacted string is stable, so a
// value that flows through the chokepoint twice is unchanged.
func TestRedactIdempotent(t *testing.T) {
	in := "Bearer abcDEF123456ghijkl7890 access_token=zzzzyyyyxxxxwwwwvvvv"
	once := Redact(in)
	twice := Redact(once)
	if once != twice {
		t.Fatalf("Redact not idempotent: once=%q twice=%q", once, twice)
	}
}

// TestRedactPreservesNonSecretText confirms ordinary text and short lowercase
// runs (< 30 chars) are left alone; the chokepoint must not scrub benign
// display strings.
func TestRedactPreservesNonSecretText(t *testing.T) {
	cases := []string{
		"render complete for channel example",
		"state=rendering seq=7",
		"shorttoken abc123",
		"vod_unavailable: source removed at origin",
	}
	for _, in := range cases {
		if got := Redact(in); got != in {
			t.Fatalf("Redact(%q) = %q; expected unchanged benign text", in, got)
		}
	}
}

// TestRedactLeavesMixedCaseIdentifierIntact confirms a >=30 lowercase run that
// is part of a longer mixed-case identifier is not treated as a bare token,
// mirroring the Python lookbehind/lookahead boundary.
func TestRedactLeavesMixedCaseIdentifierIntact(t *testing.T) {
	// 30 lowercase chars immediately followed by an uppercase letter.
	id := "abcdefghijklmnopqrstuvwxyz0123X"
	if got := Redact(id); got != id {
		t.Fatalf("Redact(%q) = %q; mixed-case identifier should be preserved", id, got)
	}
}

const tokenAlphabet = "abcdefghijklmnopqrstuvwxyz0123456789"

func randomToken(r *rand.Rand, minLen, maxLen int) string {
	n := minLen + r.Intn(maxLen-minLen+1)
	var b strings.Builder
	for i := 0; i < n; i++ {
		b.WriteByte(tokenAlphabet[r.Intn(len(tokenAlphabet))])
	}
	return b.String()
}

// TestRedactNeverEmitsRawSecret is a randomized/table-driven check: across many
// generated secrets embedded in surrounding text, the raw secret value must
// never survive Redact. This exercises the universal redaction property on the
// mirror/log/display path.
//
// Validates: Requirements 1.7
func TestRedactNeverEmitsRawSecret(t *testing.T) {
	r := rand.New(rand.NewSource(1))
	shapes := []func(secret string) string{
		func(s string) string { return "Bearer " + s },
		func(s string) string { return "access_token=" + s },
		func(s string) string { return "refresh_token: " + s },
		func(s string) string { return `auth_token="` + s + `"` },
		func(s string) string { return "oauth:" + s },
		// Standalone opaque token bordered by non-alphanumeric separators so it
		// is recognized as a bare token (>= 30 chars).
		func(s string) string { return "(" + s + ")" },
	}

	for i := 0; i < 500; i++ {
		// Standalone shape needs a >=30 char token; the labelled shapes accept
		// any non-empty value. Use 30..48 so every shape's secret is redacted.
		secret := randomToken(r, 30, 48)
		shape := shapes[r.Intn(len(shapes))]
		prefix := randomFiller(r)
		suffix := randomFiller(r)
		line := prefix + " " + shape(secret) + " " + suffix

		got := Redact(line)
		if strings.Contains(got, secret) {
			t.Fatalf("iteration %d: raw secret %q survived redaction of %q -> %q", i, secret, line, got)
		}
		if !strings.Contains(got, Placeholder) {
			t.Fatalf("iteration %d: no placeholder in redaction of %q -> %q", i, line, got)
		}
	}
}

func randomFiller(r *rand.Rand) string {
	words := []string{"job", "state", "render", "channel", "clip", "vod", "queued", "seq"}
	n := 1 + r.Intn(3)
	parts := make([]string, 0, n)
	for i := 0; i < n; i++ {
		parts = append(parts, words[r.Intn(len(words))])
	}
	return strings.Join(parts, " ")
}

// TestLogRedactsFields confirms the Log helper redacts assembled key=value
// fields.
func TestLogRedactsFields(t *testing.T) {
	got := Log("callback failed", "job_id", "job_01H", "auth_token", "supersecrettokenvalue12345")
	if strings.Contains(got, "supersecrettokenvalue12345") {
		t.Fatalf("Log leaked token: %q", got)
	}
	if !strings.Contains(got, "job_id=job_01H") {
		t.Fatalf("Log dropped benign field: %q", got)
	}
}

// TestDisplayRedacts confirms the Display helper routes through Redact.
func TestDisplayRedacts(t *testing.T) {
	got := Display("show oauth:abcdef1234567890 to nobody")
	if strings.Contains(got, "oauth:abcdef1234567890") {
		t.Fatalf("Display leaked oauth token: %q", got)
	}
}
