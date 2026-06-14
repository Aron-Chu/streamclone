package chatreplay

import (
	"strings"
	"testing"
)

func TestSanitizeTextStripsControlChars(t *testing.T) {
	raw := "hel\x00lo\x07 wor\x1bld\x7f"
	got := SanitizeText(raw, DefaultMaxTextLen)
	for _, r := range got {
		if r != '\n' && (r < 0x20 || r == 0x7f) {
			t.Fatalf("control char 0x%x survived sanitization: %q", r, got)
		}
	}
	if !strings.Contains(got, "hel") || !strings.Contains(got, "ld") {
		t.Fatalf("unexpected sanitized output: %q", got)
	}
}

func TestSanitizeTextPreservesNewline(t *testing.T) {
	got := SanitizeText("line1\nline2", DefaultMaxTextLen)
	if !strings.Contains(got, "\n") {
		t.Fatalf("newline should be preserved: %q", got)
	}
}

func TestSanitizeTextTruncates(t *testing.T) {
	raw := strings.Repeat("a", 1000)
	got := SanitizeText(raw, 500)
	if n := len([]rune(got)); n > 500 {
		t.Fatalf("expected <=500 runes, got %d", n)
	}
}

func TestSanitizeTextTruncatesByRunes(t *testing.T) {
	raw := strings.Repeat("é", 50) // multi-byte runes
	got := SanitizeText(raw, 10)
	if n := len([]rune(got)); n > 10 {
		t.Fatalf("expected <=10 runes, got %d", n)
	}
}

func TestSanitizeTextRemovesURLs(t *testing.T) {
	cases := []string{
		"check this https://evil.example.com/path out",
		"go to www.scam.tv now",
		"visit free-nitro.gg/claim please",
	}
	for _, raw := range cases {
		got := SanitizeText(raw, DefaultMaxTextLen)
		lower := strings.ToLower(got)
		if strings.Contains(lower, "http") || strings.Contains(lower, "www.") {
			t.Fatalf("URL artifact survived: input=%q output=%q", raw, got)
		}
		if urlPattern.MatchString(got) {
			t.Fatalf("bare URL pattern survived: input=%q output=%q", raw, got)
		}
	}
}

func TestSanitizeTextDefaultMaxWhenNonPositive(t *testing.T) {
	raw := strings.Repeat("x", 600)
	got := SanitizeText(raw, 0)
	if n := len([]rune(got)); n != DefaultMaxTextLen {
		t.Fatalf("expected default cap %d, got %d", DefaultMaxTextLen, n)
	}
}

func TestHashSenderNotRawAndDeterministic(t *testing.T) {
	salt := []byte("server-side-salt")
	const userID = "123456789"
	h1 := HashSender(userID, salt)
	h2 := HashSender(userID, salt)
	if h1 == "" {
		t.Fatal("hash should not be empty for non-empty user id")
	}
	if h1 != h2 {
		t.Fatalf("hash not deterministic: %q vs %q", h1, h2)
	}
	if h1 == userID {
		t.Fatal("hash must not equal the raw user id")
	}
	if strings.Contains(h1, userID) {
		t.Fatal("hash must not contain the raw user id")
	}
}

func TestHashSenderSaltMatters(t *testing.T) {
	a := HashSender("user", []byte("salt-a"))
	b := HashSender("user", []byte("salt-b"))
	if a == b {
		t.Fatal("different salts should produce different hashes")
	}
}

func TestHashSenderEmptyUser(t *testing.T) {
	if got := HashSender("", []byte("salt")); got != "" {
		t.Fatalf("expected empty hash for empty user id, got %q", got)
	}
}

func TestShouldKeepDropsBots(t *testing.T) {
	cfg := SanitizeConfig{BotUsernames: map[string]struct{}{"nightbot": {}}}
	if cfg.ShouldKeep("NightBot", "hello chat") {
		t.Fatal("expected bot message to be dropped")
	}
	if !cfg.ShouldKeep("realuser", "hello chat") {
		t.Fatal("expected normal user message to be kept")
	}
}

func TestShouldKeepDropsSpam(t *testing.T) {
	cfg := SanitizeConfig{SpamPatterns: []string{"free nitro"}}
	if cfg.ShouldKeep("user", "FREE NITRO giveaway") {
		t.Fatal("expected spam message to be dropped")
	}
}

func TestShouldKeepDropsURLOnly(t *testing.T) {
	cfg := SanitizeConfig{}
	if cfg.ShouldKeep("user", "https://example.com/x") {
		t.Fatal("expected URL-only message to be dropped")
	}
	if !cfg.ShouldKeep("user", "look https://example.com/x cool") {
		t.Fatal("expected message with text + URL to be kept")
	}
}

func TestBuildMessageHashesAndSanitizes(t *testing.T) {
	cfg := SanitizeConfig{
		MaxTextLen: 500,
		SenderSalt: []byte("salt"),
	}
	raw := RawComment{
		StreamID:      "s1",
		MessageID:     "m1",
		DisplayName:   "alice",
		SenderUserID:  "987654",
		Text:          "hi \x00 there https://x.com/y",
		OffsetSeconds: 42,
	}
	msg, keep := BuildMessage(raw, cfg)
	if !keep {
		t.Fatal("expected message to be kept")
	}
	if msg.SenderHash == "" || msg.SenderHash == raw.SenderUserID {
		t.Fatalf("sender hash invalid: %q", msg.SenderHash)
	}
	if strings.Contains(msg.Text, "\x00") || strings.Contains(strings.ToLower(msg.Text), "http") {
		t.Fatalf("text not sanitized: %q", msg.Text)
	}
	if msg.OffsetSeconds != 42 || msg.MessageID != "m1" || msg.StreamID != "s1" {
		t.Fatalf("unexpected message fields: %+v", msg)
	}
}

func TestBuildMessageDropsURLOnly(t *testing.T) {
	cfg := SanitizeConfig{MaxTextLen: 500, SenderSalt: []byte("salt")}
	raw := RawComment{StreamID: "s1", MessageID: "m1", Text: "www.spam.tv/x"}
	if _, keep := BuildMessage(raw, cfg); keep {
		t.Fatal("expected URL-only message to be dropped")
	}
}

func TestBuildMessageKeepsEmoteOnly(t *testing.T) {
	cfg := SanitizeConfig{MaxTextLen: 500, SenderSalt: []byte("salt")}
	raw := RawComment{
		StreamID:   "s1",
		MessageID:  "m1",
		Text:       "",
		EmoteFrags: []EmoteFrag{{Name: "KEKW", ID: "abc", Provider: "7tv", ImageURL: "/emotes/abc/1x.webp"}},
	}
	if _, keep := BuildMessage(raw, cfg); !keep {
		t.Fatal("expected emote-only message to be kept")
	}
}
