package chatreplay

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"regexp"
	"strings"
	"time"
)

// DefaultMaxTextLen is the default maximum stored message length in runes
// (Requirement 27.2). Override via ANALYTICS_VOD_CHAT_MAX_LEN.
const DefaultMaxTextLen = 500

// defaultBotUsernames is the baseline bot blocklist used when
// ANALYTICS_VOD_CHAT_BOT_USERNAMES is unset (Requirement 30.5). Comparison is
// case-insensitive.
var defaultBotUsernames = []string{
	"nightbot",
	"streamelements",
	"streamlabs",
	"moobot",
	"fossabot",
	"wizebot",
}

// urlPattern matches scheme-qualified URLs, www-prefixed hosts, and bare domain
// references so embedded links can be stripped from stored text (Requirement
// 27.2) and used to detect URL-only messages.
var urlPattern = regexp.MustCompile(`(?i)\b(?:https?://|www\.)\S+|\b[a-z0-9](?:[a-z0-9-]*[a-z0-9])?(?:\.[a-z0-9-]+)+\.(?:com|net|org|gg|tv|io|co|me|xyz|live|stream|link|app|dev)(?:/\S*)?`)

// SanitizeConfig holds the privacy/sanitization settings for VOD chat
// persistence. Construct it with LoadConfig (env-driven) so the server-side
// salt and limits are never hardcoded. The pure helpers (SanitizeText,
// HashSender) take their inputs directly and do not read this struct, keeping
// them trivially testable.
type SanitizeConfig struct {
	// MaxTextLen is the maximum stored message length in runes.
	MaxTextLen int
	// SenderSalt is the server-side HMAC key used to anonymize sender ids. It is
	// sourced from ANALYTICS_VOD_CHAT_SENDER_SALT and never persisted.
	SenderSalt []byte
	// BotUsernames is the set of lower-cased bot display names whose messages are
	// dropped (Requirement 30.5).
	BotUsernames map[string]struct{}
	// SpamPatterns is a configurable lower-cased substring blocklist; any message
	// containing one of these is dropped (Requirement 30.5).
	SpamPatterns []string
	// PreserveURLs skips URL stripping when ANALYTICS_VOD_CHAT_PRESERVE_URLS=1.
	PreserveURLs bool
}

// LoadConfig builds a SanitizeConfig from the environment. It reads:
//   - ANALYTICS_VOD_CHAT_SENDER_SALT  (HMAC salt; required for meaningful anonymization)
//   - ANALYTICS_VOD_CHAT_MAX_LEN      (max stored length, default 500)
//   - ANALYTICS_VOD_CHAT_BOT_USERNAMES (comma-separated; replaces the default list when set)
//   - ANALYTICS_VOD_CHAT_SPAM_PATTERNS (comma-separated substrings, case-insensitive)
//
// The salt is intentionally not defaulted to a literal: an unset salt yields an
// empty key (still a one-way keyed hash, but operators should configure it).
func LoadConfig() SanitizeConfig {
	cfg := SanitizeConfig{
		MaxTextLen:   DefaultMaxTextLen,
		SenderSalt:   []byte(os.Getenv("ANALYTICS_VOD_CHAT_SENDER_SALT")),
		BotUsernames: map[string]struct{}{},
	}

	if v := strings.TrimSpace(os.Getenv("ANALYTICS_VOD_CHAT_MAX_LEN")); v != "" {
		if n := atoiSafe(v); n > 0 {
			cfg.MaxTextLen = n
		}
	}

	bots := defaultBotUsernames
	if v := strings.TrimSpace(os.Getenv("ANALYTICS_VOD_CHAT_BOT_USERNAMES")); v != "" {
		bots = splitCSV(v)
	}
	for _, b := range bots {
		cfg.BotUsernames[strings.ToLower(strings.TrimSpace(b))] = struct{}{}
	}

	if v := strings.TrimSpace(os.Getenv("ANALYTICS_VOD_CHAT_SPAM_PATTERNS")); v != "" {
		for _, p := range splitCSV(v) {
			p = strings.ToLower(strings.TrimSpace(p))
			if p != "" {
				cfg.SpamPatterns = append(cfg.SpamPatterns, p)
			}
		}
	}

	cfg.PreserveURLs = strings.TrimSpace(os.Getenv("ANALYTICS_VOD_CHAT_PRESERVE_URLS")) == "1"

	return cfg
}

// SanitizeText returns a display-safe version of raw suitable for persistence
// (Requirement 27.2, Property 33). It:
//   - strips control characters (0x00–0x1F and 0x7F) except newline,
//   - removes embedded URLs and bare domain references (unless preserveURLs),
//   - collapses runs of spaces introduced by removal and trims the result,
//   - truncates to maxLen runes (DefaultMaxTextLen when maxLen <= 0).
func SanitizeText(raw string, maxLen int) string {
	return SanitizeTextWithOptions(raw, maxLen, false)
}

func SanitizeTextWithOptions(raw string, maxLen int, preserveURLs bool) string {
	if maxLen <= 0 {
		maxLen = DefaultMaxTextLen
	}

	// 1. Strip control characters, preserving newlines.
	var b strings.Builder
	b.Grow(len(raw))
	for _, r := range raw {
		if r == '\n' {
			b.WriteRune(r)
			continue
		}
		if r < 0x20 || r == 0x7f {
			// Replace with a space so surrounding tokens do not fuse.
			b.WriteByte(' ')
			continue
		}
		b.WriteRune(r)
	}
	text := b.String()

	// 2. Remove URLs / bare domains unless preserving for logs fidelity.
	if !preserveURLs {
		text = urlPattern.ReplaceAllString(text, " ")
	}

	// 3. Collapse redundant spaces and trim.
	text = collapseSpaces(text)

	// 4. Truncate to maxLen runes.
	if r := []rune(text); len(r) > maxLen {
		text = strings.TrimRight(string(r[:maxLen]), " ")
	}

	return text
}

// HashSender returns a hex-encoded HMAC-SHA256 of the raw sender id keyed by the
// server-side salt (Requirement 30.1, Property 34). The raw id is never stored;
// only this one-way digest is persisted. An empty userID yields an empty string
// (nothing to anonymize).
func HashSender(userID string, salt []byte) string {
	if userID == "" {
		return ""
	}
	mac := hmac.New(sha256.New, salt)
	mac.Write([]byte(userID))
	return hex.EncodeToString(mac.Sum(nil))
}

// RawComment is the unsanitized projection of a Twitch GQL VOD comment edge,
// passed into BuildMessage. It carries the raw sender id (used only to derive
// the hash) which must not be persisted directly.
type RawComment struct {
	StreamID      string
	MessageID     string
	DisplayName   string
	CommenterLogin string
	SenderUserID  string
	Text          string
	EmoteFrags    []EmoteFrag
	OffsetSeconds int
	MinuteTS      time.Time
}

// ShouldKeep reports whether a message should be persisted, given its raw
// display name and raw text (Requirement 30.5). A message is dropped when it is
// URL-only (no content after URL removal and it actually contained a URL),
// authored by a configured bot username, or matches a configured spam pattern.
func (c SanitizeConfig) ShouldKeep(displayName, rawText string) bool {
	// Bot author check (case-insensitive).
	if _, isBot := c.BotUsernames[strings.ToLower(strings.TrimSpace(displayName))]; isBot {
		return false
	}

	lower := strings.ToLower(rawText)
	for _, p := range c.SpamPatterns {
		if p != "" && strings.Contains(lower, p) {
			return false
		}
	}

	// URL-only detection: if the message had a URL and stripping it leaves
	// nothing, drop it.
	if urlPattern.MatchString(rawText) {
		stripped := collapseSpaces(urlPattern.ReplaceAllString(rawText, " "))
		if stripped == "" {
			return false
		}
	}

	return true
}

// BuildMessage turns a raw GQL comment into a sanitized, privacy-preserving
// VODChatMessage ready for sink.Add. The second return value reports whether
// the message should be kept; when false the message must be dropped
// (Requirement 30.5). The function is pure: all configuration is supplied via
// cfg, and the raw sender id is converted to an HMAC digest, never stored.
func BuildMessage(raw RawComment, cfg SanitizeConfig) (VODChatMessage, bool) {
	if !cfg.ShouldKeep(raw.DisplayName, raw.Text) {
		return VODChatMessage{}, false
	}

	text := SanitizeTextWithOptions(raw.Text, cfg.MaxTextLen, cfg.PreserveURLs)
	// Drop messages that have no remaining text and no emotes to render.
	if text == "" && len(raw.EmoteFrags) == 0 {
		return VODChatMessage{}, false
	}

	msg := VODChatMessage{
		StreamID:       raw.StreamID,
		MinuteTS:       raw.MinuteTS,
		MessageID:      raw.MessageID,
		DisplayName:    raw.DisplayName,
		CommenterLogin: strings.ToLower(strings.TrimSpace(raw.CommenterLogin)),
		SenderHash:     HashSender(raw.SenderUserID, cfg.SenderSalt),
		Text:           text,
		EmoteFrags:     raw.EmoteFrags,
		OffsetSeconds:  raw.OffsetSeconds,
	}
	if msg.CommenterLogin == "" {
		msg.CommenterLogin = strings.ToLower(strings.TrimSpace(raw.DisplayName))
	}
	return msg, true
}

// collapseSpaces collapses runs of spaces and tabs into a single space and
// trims leading/trailing spaces, preserving newlines.
func collapseSpaces(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	prevSpace := false
	for _, r := range s {
		if r == ' ' {
			if !prevSpace {
				b.WriteRune(r)
			}
			prevSpace = true
			continue
		}
		prevSpace = false
		b.WriteRune(r)
	}
	return strings.TrimSpace(b.String())
}

func splitCSV(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}

func atoiSafe(s string) int {
	n := 0
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0
		}
		n = n*10 + int(r-'0')
	}
	return n
}
