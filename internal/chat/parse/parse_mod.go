package parse

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// ChatEvent is a non-PRIVMSG IRC line normalized for WebSocket relay and persistence.
type ChatEvent struct {
	Kind         string
	Channel      string
	TS           int64
	TargetLogin  string
	ActorLogin   string
	DurationSec  int
	Reason       string
	MessageID    string
	TextPreview  string
	NoticeMsgID  string
	DisplayText  string
}

func parseTags(line string) (map[string]string, string, bool) {
	rest := line
	tags := map[string]string{}
	if strings.HasPrefix(rest, "@") {
		idx := strings.Index(rest, " ")
		if idx < 0 {
			return nil, "", false
		}
		rawTags := rest[1:idx]
		rest = rest[idx+1:]
		for _, part := range strings.Split(rawTags, ";") {
			if k, v, ok := strings.Cut(part, "="); ok {
				tags[k] = v
			}
		}
	}
	return tags, rest, true
}

func parseCommand(rest string) (cmd, channel, trailing string, ok bool) {
	if strings.HasPrefix(rest, ":") {
		idx := strings.Index(rest, " ")
		if idx < 0 {
			return "", "", "", false
		}
		rest = rest[idx+1:]
	}
	cmd, rest, _ = strings.Cut(rest, " ")
	chanName, trailing, hasTrailing := strings.Cut(rest, " :")
	if !hasTrailing {
		trailing = ""
	}
	channel = strings.TrimPrefix(strings.TrimSpace(chanName), "#")
	return cmd, channel, trailing, channel != ""
}

func parseTS(tags map[string]string) int64 {
	ts := time.Now().UnixMilli()
	if raw := tags["tmi-sent-ts"]; raw != "" {
		var v int64
		for _, ch := range raw {
			if ch < '0' || ch > '9' {
				break
			}
			v = v*10 + int64(ch-'0')
		}
		if v > 0 {
			ts = v
		}
	}
	return ts
}

// ParseEvent parses CLEARCHAT, CLEARMSG, and USERNOTICE IRC lines.
func ParseEvent(line string) (*ChatEvent, bool) {
	tags, rest, ok := parseTags(line)
	if !ok {
		return nil, false
	}
	cmd, channel, trailing, ok := parseCommand(rest)
	if !ok {
		return nil, false
	}

	switch cmd {
	case "CLEARCHAT":
		ev := &ChatEvent{
			Kind:    "clear_chat",
			Channel: channel,
			TS:      parseTS(tags),
		}
		target := strings.TrimSpace(trailing)
		if target == "" {
			return ev, true
		}
		ev.TargetLogin = strings.ToLower(target)
		if dur := strings.TrimSpace(tags["ban-duration"]); dur != "" {
			if sec, err := strconv.Atoi(dur); err == nil && sec > 0 {
				ev.Kind = "timeout"
				ev.DurationSec = sec
			} else {
				ev.Kind = "ban"
			}
		} else {
			ev.Kind = "ban"
		}
		if reason := strings.TrimSpace(tags["ban-reason"]); reason != "" {
			ev.Reason = reason
		}
		return ev, true

	case "CLEARMSG":
		ev := &ChatEvent{
			Kind:        "delete_message",
			Channel:     channel,
			TS:          parseTS(tags),
			MessageID:   tags["target-msg-id"],
			TargetLogin: strings.ToLower(strings.TrimSpace(tags["login"])),
			TextPreview: trailing,
		}
		return ev, true

	case "USERNOTICE":
		msgID := tags["msg-id"]
		if msgID == "" {
			return nil, false
		}
		allowed := map[string]struct{}{
			"sub": {}, "resub": {}, "subgift": {}, "anonsubgift": {}, "submysterygift": {},
			"raid": {}, "ritual": {}, "viewermilestone": {},
		}
		if _, ok := allowed[msgID]; !ok {
			return nil, false
		}
		display := strings.TrimSpace(trailing)
		if display == "" {
			display = msgID
		}
		ev := &ChatEvent{
			Kind:        "notice",
			Channel:     channel,
			TS:          parseTS(tags),
			NoticeMsgID: msgID,
			DisplayText: display,
			ActorLogin:  strings.ToLower(strings.TrimSpace(tags["login"])),
		}
		if login := strings.TrimSpace(tags["msg-param-login"]); login != "" {
			ev.TargetLogin = strings.ToLower(login)
		}
		return ev, true
	}

	return nil, false
}

func (e ChatEvent) SummaryText() string {
	switch e.Kind {
	case "clear_chat":
		return "Chat was cleared by a moderator"
	case "timeout":
		if e.DurationSec > 0 {
			return fmt.Sprintf("%s was timed out for %ds", e.TargetLogin, e.DurationSec)
		}
		return fmt.Sprintf("%s was timed out", e.TargetLogin)
	case "ban":
		return fmt.Sprintf("%s was banned", e.TargetLogin)
	case "delete_message":
		if e.TargetLogin != "" {
			return fmt.Sprintf("A message from %s was deleted", e.TargetLogin)
		}
		return "A message was deleted"
	case "notice":
		return e.DisplayText
	default:
		return e.Kind
	}
}
