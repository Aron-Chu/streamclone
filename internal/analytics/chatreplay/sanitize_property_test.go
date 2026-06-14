package chatreplay

import (
	"strings"
	"testing"

	"pgregory.net/rapid"
)

// hexChars is the exact alphabet produced by hex.EncodeToString. A user id that
// contains any character outside this set can never appear as a substring of a
// HashSender digest, which lets the "no raw id" assertions stay deterministic.
const hexChars = "0123456789abcdef"

func containsNonHex(s string) bool {
	for _, r := range s {
		if !strings.ContainsRune(hexChars, r) {
			return true
		}
	}
	return false
}

// rawChatText is a smart generator for VOD chat input. It interleaves arbitrary
// unicode runs with control characters and URL-like tokens so the sanitizer is
// stressed across every behavior Property 33 constrains (control stripping, URL
// neutralization, length truncation) rather than only benign text.
func rawChatText() *rapid.Generator[string] {
	token := rapid.OneOf(
		// Arbitrary unicode text (rapid.String already emits control chars too).
		rapid.String(),
		// Explicit control characters that must be stripped (newline excluded by design).
		rapid.SampledFrom([]string{"\x00", "\x01", "\x07", "\x1b", "\x7f", "\t"}),
		// URL / bare-domain tokens that must be neutralized.
		rapid.SampledFrom([]string{
			"https://evil.example.com/path",
			"http://a.b.co/x?q=1",
			"www.scam.tv/claim",
			"free-nitro.gg/login",
			"totally.legit.io/drop",
			"sub.domain.app/path/y",
		}),
		// Newlines, which the sanitizer must preserve.
		rapid.SampledFrom([]string{"\n"}),
	)
	return rapid.Custom(func(t *rapid.T) string {
		parts := rapid.SliceOfN(token, 0, 40).Draw(t, "parts")
		return strings.Join(parts, " ")
	})
}

// TestSanitizeTextProperty is the property-based test for Property 33. For any
// arbitrary raw chat string and any configured maximum length, SanitizeText
// must produce output that (a) contains no control characters other than
// newline, (b) is at most maxLen runes long, and (c) contains no residual URL
// or bare-domain pattern.
//
// rapid runs at least 100 iterations by default (rapid.checks defaults to 100).
//
// Feature: moment-timeline, Property 33: VOD Chat Message Sanitization
//
// **Validates: Requirements 27.2, 30.1**
func TestSanitizeTextProperty(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		raw := rawChatText().Draw(t, "raw")
		maxLen := rapid.IntRange(1, 2000).Draw(t, "maxLen")

		got := SanitizeText(raw, maxLen)

		// (a) No control characters survive, except newline.
		for _, r := range got {
			if r == '\n' {
				continue
			}
			if r < 0x20 || r == 0x7f {
				t.Fatalf("control char 0x%x survived sanitization: input=%q output=%q", r, raw, got)
			}
		}

		// (b) Truncated to at most maxLen runes.
		if n := len([]rune(got)); n > maxLen {
			t.Fatalf("output longer than maxLen=%d: got %d runes (input=%q output=%q)", maxLen, n, raw, got)
		}

		// (c) No residual URL / bare-domain pattern.
		if urlPattern.MatchString(got) {
			t.Fatalf("URL pattern survived sanitization: input=%q output=%q", raw, got)
		}
	})
}

// TestHashSenderPrivacyProperty is the property-based test for Property 34 at
// the HashSender level. For any arbitrary non-empty user id and arbitrary salt,
// the digest must (a) never equal the raw id, (b) never contain the raw id when
// the id has a character that cannot appear in a hex digest, and (c) be
// deterministic for a fixed (id, salt) pair.
//
// rapid runs at least 100 iterations by default (rapid.checks defaults to 100).
//
// Feature: moment-timeline, Property 34: Privacy — No Raw User IDs in Storage
//
// **Validates: Requirements 27.2, 30.1**
func TestHashSenderPrivacyProperty(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		userID := rapid.StringN(1, 64, -1).Draw(t, "userID")
		salt := rapid.SliceOfN(rapid.Byte(), 0, 64).Draw(t, "salt")

		h1 := HashSender(userID, salt)
		h2 := HashSender(userID, salt)

		// (a) Digest never equals the raw id.
		if h1 == userID {
			t.Fatalf("hash equals raw user id: id=%q hash=%q", userID, h1)
		}

		// (b) Digest never contains the raw id (only assertable when the id has a
		// non-hex character, since the digest alphabet is exactly [0-9a-f]).
		if containsNonHex(userID) && strings.Contains(h1, userID) {
			t.Fatalf("hash contains raw user id: id=%q hash=%q", userID, h1)
		}

		// (c) Deterministic for a fixed (id, salt).
		if h1 != h2 {
			t.Fatalf("hash not deterministic for id=%q: %q vs %q", userID, h1, h2)
		}
	})
}

// TestBuildMessagePrivacyProperty is the property-based test for Property 34 at
// the BuildMessage level: a persisted VODChatMessage must never carry the raw
// Twitch user id in its SenderHash field. For any arbitrary non-empty sender id
// and arbitrary text, the resulting SenderHash must differ from the raw id and
// must not contain it (when the id has a non-hex character).
//
// rapid runs at least 100 iterations by default (rapid.checks defaults to 100).
//
// Feature: moment-timeline, Property 34: Privacy — No Raw User IDs in Storage
//
// **Validates: Requirements 27.2, 30.1**
func TestBuildMessagePrivacyProperty(t *testing.T) {
	cfg := SanitizeConfig{
		MaxTextLen:   DefaultMaxTextLen,
		SenderSalt:   []byte("server-side-salt"),
		BotUsernames: map[string]struct{}{},
	}

	rapid.Check(t, func(t *rapid.T) {
		senderID := rapid.StringN(1, 64, -1).Draw(t, "senderID")
		text := rawChatText().Draw(t, "text")

		raw := RawComment{
			StreamID:     "s1",
			MessageID:    "m1",
			DisplayName:  "viewer",
			SenderUserID: senderID,
			Text:         text,
			// An emote fragment keeps otherwise-empty (fully sanitized) text from
			// being dropped, so the SenderHash invariant is exercised on the widest
			// possible set of retained messages.
			EmoteFrags: []EmoteFrag{{Name: "KEKW", ID: "abc", Provider: "7tv", ImageURL: "/emotes/abc/1x.webp"}},
		}

		msg, keep := BuildMessage(raw, cfg)
		if !keep {
			// Property 34 constrains *stored* messages only. A dropped message
			// (e.g. URL-only text rejected by ShouldKeep) is never persisted, so
			// there is no SenderHash invariant to check for it.
			return
		}

		if msg.SenderHash == "" {
			t.Fatalf("sender hash empty for non-empty id=%q", senderID)
		}
		if msg.SenderHash == raw.SenderUserID {
			t.Fatalf("persisted SenderHash equals raw user id: id=%q hash=%q", senderID, msg.SenderHash)
		}
		if containsNonHex(senderID) && strings.Contains(msg.SenderHash, senderID) {
			t.Fatalf("persisted SenderHash contains raw user id: id=%q hash=%q", senderID, msg.SenderHash)
		}
	})
}
