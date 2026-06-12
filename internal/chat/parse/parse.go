package parse

import (
	"strings"
	"time"
)

type Message struct {
	ID      string
	Channel string
	User    string
	Color   string
	Badges  []string
	TS      int64
	Text    string
	Emotes  []EmoteRange
}

func ParseLine(line string) (*Message, bool) {
	rest := line
	tags := map[string]string{}

	if strings.HasPrefix(rest, "@") {
		idx := strings.Index(rest, " ")
		if idx < 0 {
			return nil, false
		}
		rawTags := rest[1:idx]
		rest = rest[idx+1:]
		for _, part := range strings.Split(rawTags, ";") {
			if k, v, ok := strings.Cut(part, "="); ok {
				tags[k] = v
			}
		}
	}

	if strings.HasPrefix(rest, ":") {
		idx := strings.Index(rest, " ")
		if idx < 0 {
			return nil, false
		}
		rest = rest[idx+1:]
	}

	cmd, rest, _ := strings.Cut(rest, " ")
	if cmd == "PING" {
		return nil, false
	}
	if cmd != "PRIVMSG" {
		return nil, false
	}

	chanName, trailing, hasTrailing := strings.Cut(rest, " :")
	if !hasTrailing {
		return nil, false
	}
	channel := strings.TrimPrefix(strings.TrimSpace(chanName), "#")

	userRaw := tags["display-name"]
	if userRaw == "" {
		userRaw = tags["login"]
	}

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

	var badges []string
	if b := tags["badges"]; b != "" {
		for _, entry := range strings.Split(b, ",") {
			if entry != "" {
				badges = append(badges, entry)
			}
		}
	}

	return &Message{
		ID:      tags["id"],
		Channel: channel,
		User:    userRaw,
		Color:   tags["color"],
		Badges:  badges,
		TS:      ts,
		Text:    trailing,
		Emotes:  ParseEmotesTag(tags["emotes"]),
	}, true
}

func IsPing(line string) bool {
	return strings.HasPrefix(line, "PING")
}

func PongFor(line string) string {
	rest := strings.TrimPrefix(line, "PING")
	return "PONG" + rest
}
