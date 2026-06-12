package parse

import (
	"sort"
	"strconv"
	"strings"
)

// EmoteRange is a native Twitch emote span in message text (rune indices, inclusive end).
type EmoteRange struct {
	ID    string
	Start int
	End   int
}

// ParseEmotesTag parses the Twitch IRC emotes tag (id:start-end/start-end:id format).
func ParseEmotesTag(raw string) []EmoteRange {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}

	var out []EmoteRange
	for _, group := range strings.Split(raw, "/") {
		id, ranges, ok := strings.Cut(group, ":")
		id = strings.TrimSpace(id)
		if !ok || id == "" {
			continue
		}
		for _, span := range strings.Split(ranges, ",") {
			span = strings.TrimSpace(span)
			if span == "" {
				continue
			}
			startStr, endStr, ok := strings.Cut(span, "-")
			if !ok {
				continue
			}
			start, err1 := strconv.Atoi(strings.TrimSpace(startStr))
			end, err2 := strconv.Atoi(strings.TrimSpace(endStr))
			if err1 != nil || err2 != nil || start < 0 || end < start {
				continue
			}
			out = append(out, EmoteRange{ID: id, Start: start, End: end})
		}
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].Start == out[j].Start {
			return out[i].End < out[j].End
		}
		return out[i].Start < out[j].Start
	})
	return out
}
