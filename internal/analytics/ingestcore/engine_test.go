package ingestcore

import (
	"testing"

	"streamclone/internal/chat/enrich"
)

const testPrivmsgLine = `:viewer!viewer@viewer.tmi.twitch.tv PRIVMSG #xqc :hello chat`

func TestHandleIRCLineAllowlistUsesParsedChannel(t *testing.T) {
	cfg := Config{
		DualReadMode: true,
		ShadowMode:   true,
		ShadowAllowlist: map[string]struct{}{
			"xqc": {},
		},
		IRCQueueSize: 8,
	}
	e := NewEngine(EngineDeps{
		Config:   cfg,
		Enricher: enrich.New(nil, 0, nil),
	})
	e.HandleIRCLine(testPrivmsgLine, "", TierP1Hot)
	select {
	case item := <-e.ircQueue:
		if item.login != "xqc" {
			t.Fatalf("login = %q, want xqc", item.login)
		}
	default:
		t.Fatal("expected allowlisted line enqueued")
	}
}

func TestHandleIRCLineAllowlistRejectsNonListed(t *testing.T) {
	cfg := Config{
		DualReadMode: true,
		ShadowMode:   true,
		ShadowAllowlist: map[string]struct{}{
			"ludwig": {},
		},
		IRCQueueSize: 8,
	}
	e := NewEngine(EngineDeps{
		Config:   cfg,
		Enricher: enrich.New(nil, 0, nil),
	})
	e.HandleIRCLine(testPrivmsgLine, "", TierP1Hot)
	select {
	case <-e.ircQueue:
		t.Fatal("non-allowlisted channel should not enqueue")
	default:
	}
}

func TestChannelFromLine(t *testing.T) {
	p := NewParser(nil)
	channel, ok := p.ChannelFromLine(testPrivmsgLine)
	if !ok || channel != "xqc" {
		t.Fatalf("channel = %q ok=%v, want xqc true", channel, ok)
	}
}
