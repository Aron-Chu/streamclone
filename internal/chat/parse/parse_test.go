package parse

import (
	"testing"
)

var privmsgLine = `@badge-info=;badges=moderator/1;color=#1E90FF;display-name=Viewer;emotes=;id=abc-123;login=viewer;tmi-sent-ts=1730000000000 :viewer!viewer@viewer.tmi.twitch.tv PRIVMSG #channel :Hello world`

var pingLine = `PING :tmi.twitch.tv`

var usernoticeLine = `@badge-info=;badges=moderator/1;color=#FF0000;display-name=User;id=xyz-456;msg-id=sub;tmi-sent-ts=1730000000001 :tmi.twitch.tv USERNOTICE #channel :Subbed!`

var roomstateLine = `@emote-only=0;followers-only=-1 :tmi.twitch.tv ROOMSTATE #channel`

var noTagPrivmsg = `:viewer2!viewer2@viewer2.tmi.twitch.tv PRIVMSG #channel :simple message`

func TestPrivmsg(t *testing.T) {
	msg, ok := ParseLine(privmsgLine)
	if !ok {
		t.Fatal("expected ok")
	}
	if msg.User != "Viewer" {
		t.Errorf("user: got %q want %q", msg.User, "Viewer")
	}
	if msg.Login != "viewer" {
		t.Errorf("login: got %q want %q", msg.Login, "viewer")
	}
	if msg.Color != "#1E90FF" {
		t.Errorf("color: got %q want %q", msg.Color, "#1E90FF")
	}
	if msg.ID != "abc-123" {
		t.Errorf("id: got %q want %q", msg.ID, "abc-123")
	}
	if msg.TS != 1730000000000 {
		t.Errorf("ts: got %d want %d", msg.TS, 1730000000000)
	}
	if msg.Text != "Hello world" {
		t.Errorf("text: got %q want %q", msg.Text, "Hello world")
	}
	if msg.Channel != "channel" {
		t.Errorf("channel: got %q want %q", msg.Channel, "channel")
	}
	if len(msg.Badges) != 1 || msg.Badges[0] != "moderator/1" {
		t.Errorf("badges: got %v", msg.Badges)
	}
}

func TestPing(t *testing.T) {
	if !IsPing(pingLine) {
		t.Fatal("expected IsPing true")
	}
	_, ok := ParseLine(pingLine)
	if ok {
		t.Fatal("ParseLine should return false for PING")
	}
	pong := PongFor(pingLine)
	if pong != "PONG :tmi.twitch.tv" {
		t.Errorf("pong: got %q", pong)
	}
}

func TestUsernotice(t *testing.T) {
	_, ok := ParseLine(usernoticeLine)
	if ok {
		t.Fatal("USERNOTICE should not produce a message")
	}
}

func TestRoomstate(t *testing.T) {
	_, ok := ParseLine(roomstateLine)
	if ok {
		t.Fatal("ROOMSTATE should not produce a message")
	}
}

func TestNoTagPrivmsg(t *testing.T) {
	msg, ok := ParseLine(noTagPrivmsg)
	if !ok {
		t.Fatal("expected ok")
	}
	if msg.Text != "simple message" {
		t.Errorf("text: got %q", msg.Text)
	}
}
