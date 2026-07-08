package ingestcore

import (
	"strings"
	"time"

	"streamclone/internal/chat/batch"
	"streamclone/internal/chat/enrich"
	"streamclone/internal/chat/parse"
)

// ParsedChat is the tokenization boundary output for one IRC PRIVMSG.
type ParsedChat struct {
	Channel   string
	StreamID  string
	Text      string
	Timestamp time.Time
	Tier      IngestTier
	Fragments []batch.Fragment
}

// Parser wraps IRC line parse + enricher tokenization.
type Parser struct {
	enricher *enrich.Enricher
}

// NewParser builds a parse boundary.
func NewParser(enricher *enrich.Enricher) *Parser {
	return &Parser{enricher: enricher}
}

// ChannelFromLine returns the Twitch channel login from a raw IRC line, if parseable.
func (p *Parser) ChannelFromLine(line string) (string, bool) {
	msg, ok := parse.ParseLine(line)
	if !ok {
		return "", false
	}
	channel := normalizeLogin(msg.Channel)
	if channel == "" {
		return "", false
	}
	return channel, true
}

// ParseIRCLine parses a raw IRC line into ParsedChat. Returns ok=false for non-chat lines.
func (p *Parser) ParseIRCLine(line string, streamID string, tier IngestTier) (ParsedChat, bool) {
	msg, ok := parse.ParseLine(line)
	if !ok {
		return ParsedChat{}, false
	}
	channel := normalizeLogin(msg.Channel)
	out := ParsedChat{
		Channel:   channel,
		StreamID:  streamID,
		Text:      msg.Text,
		Timestamp: time.UnixMilli(msg.TS).UTC(),
		Tier:      tier,
	}
	if p != nil && p.enricher != nil {
		out.Fragments = p.enricher.Tokenize(channel, msg.Text, msg.Emotes)
	}
	return out, true
}

// EmoteKeysFromFragments returns normalized emote keys and sevenTV count delta.
func EmoteKeysFromFragments(fragments []batch.Fragment) (keys []string, sevenTV int) {
	for _, frag := range fragments {
		if strings.TrimSpace(frag.C) == "" {
			continue
		}
		keys = append(keys, emoteKeyFromParts(frag.Provider, frag.ID, frag.C))
		if isSevenTVProvider(frag.Provider) {
			sevenTV++
		}
	}
	return keys, sevenTV
}
