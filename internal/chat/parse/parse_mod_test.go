package parse

import "testing"

func TestParseClearChatTimeout(t *testing.T) {
	line := `@ban-duration=600;login=mod;display-name=Mod;tmi-sent-ts=1730000000000 :tmi.twitch.tv CLEARCHAT #channel :baduser`
	ev, ok := ParseEvent(line)
	if !ok {
		t.Fatal("expected ok")
	}
	if ev.Kind != "timeout" {
		t.Fatalf("kind=%q", ev.Kind)
	}
	if ev.TargetLogin != "baduser" {
		t.Fatalf("target=%q", ev.TargetLogin)
	}
	if ev.DurationSec != 600 {
		t.Fatalf("duration=%d", ev.DurationSec)
	}
}

func TestParseClearChatClear(t *testing.T) {
	line := `@tmi-sent-ts=1730000000000 :tmi.twitch.tv CLEARCHAT #channel`
	ev, ok := ParseEvent(line)
	if !ok || ev.Kind != "clear_chat" {
		t.Fatalf("ev=%+v ok=%v", ev, ok)
	}
}

func TestParseClearMsg(t *testing.T) {
	line := `@login=viewer;target-msg-id=abc123;tmi-sent-ts=1730000000000 :tmi.twitch.tv CLEARMSG #channel :hello there`
	ev, ok := ParseEvent(line)
	if !ok || ev.Kind != "delete_message" {
		t.Fatalf("ev=%+v ok=%v", ev, ok)
	}
	if ev.MessageID != "abc123" {
		t.Fatalf("msg id=%q", ev.MessageID)
	}
}

func TestParseUsernoticeSub(t *testing.T) {
	line := `@msg-id=sub;login=viewer;tmi-sent-ts=1730000000000 :tmi.twitch.tv USERNOTICE #channel :Subscribed!`
	ev, ok := ParseEvent(line)
	if !ok || ev.Kind != "notice" {
		t.Fatalf("ev=%+v ok=%v", ev, ok)
	}
}
