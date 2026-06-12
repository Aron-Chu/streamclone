package analytics

import (
	"fmt"
	"time"

	"streamclone/internal/chat/batch"
	"streamclone/internal/chat/parse"
)

type CommentTokenizer interface {
	Tokenize(channel, text string, native []parse.EmoteRange) []batch.Fragment
}

type RollupViewerPoint struct {
	OffsetSeconds int
	Viewers       int
}

func InterpolateViewerCount(minute int, points []RollupViewerPoint) int {
	if len(points) == 0 {
		return 0
	}
	targetSec := minute * 60
	if targetSec <= points[0].OffsetSeconds {
		return points[0].Viewers
	}
	if targetSec >= points[len(points)-1].OffsetSeconds {
		return points[len(points)-1].Viewers
	}
	for i := 0; i < len(points)-1; i++ {
		p1 := points[i]
		p2 := points[i+1]
		if targetSec >= p1.OffsetSeconds && targetSec <= p2.OffsetSeconds {
			span := p2.OffsetSeconds - p1.OffsetSeconds
			if span == 0 {
				return p1.Viewers
			}
			pct := float64(targetSec-p1.OffsetSeconds) / float64(span)
			return p1.Viewers + int(pct*float64(p2.Viewers-p1.Viewers))
		}
	}
	return 0
}

type CachedMinuteRollup struct {
	ChatCount         int
	TotalEmoteCount   int
	SevenTVEmoteCount int
	Emotes            map[string]int
}

func BuildMinuteRollupsFromComments(
	login string,
	tokenizer CommentTokenizer,
	commentsMap map[int][]string,
	viewerPoints []RollupViewerPoint,
	rollupStart time.Time,
	durationSeconds int,
) []MinuteRollup {
	return BuildMinuteRollupsFromCommentsCached(login, tokenizer, commentsMap, viewerPoints, rollupStart, durationSeconds, nil)
}

func BuildMinuteRollupsFromCommentsCached(
	login string,
	tokenizer CommentTokenizer,
	commentsMap map[int][]string,
	viewerPoints []RollupViewerPoint,
	rollupStart time.Time,
	durationSeconds int,
	cached func(minute int) (CachedMinuteRollup, bool),
) []MinuteRollup {
	totalMinutes := durationSeconds / 60
	if totalMinutes <= 0 {
		totalMinutes = 1
	}
	viewerLookup := make(map[int]int)
	for _, pt := range viewerPoints {
		viewerLookup[pt.OffsetSeconds/60] = pt.Viewers
	}
	rollups := make([]MinuteRollup, 0, totalMinutes+1)
	for m := 0; m <= totalMinutes; m++ {
		minuteTS := rollupStart.Add(time.Duration(m) * time.Minute)
		viewerVal := 0
		if val, ok := viewerLookup[m]; ok {
			viewerVal = val
		} else {
			viewerVal = InterpolateViewerCount(m, viewerPoints)
		}
		chatCount := 0
		totalEmoteCount := 0
		seventvEmoteCount := 0
		emotesMap := make(map[string]int)
		if cached != nil {
			if snap, ok := cached(m); ok {
				chatCount = snap.ChatCount
				totalEmoteCount = snap.TotalEmoteCount
				seventvEmoteCount = snap.SevenTVEmoteCount
				emotesMap = snap.Emotes
				rollups = append(rollups, MinuteRollup{
					MinuteTS:          minuteTS,
					ViewerAvg:         viewerVal,
					ViewerMax:         viewerVal,
					ViewerLatest:      viewerVal,
					ViewerSamples:     1,
					ChatCount:         chatCount,
					TotalEmoteCount:   totalEmoteCount,
					SevenTVEmoteCount: seventvEmoteCount,
					Emotes:            emotesMap,
				})
				continue
			}
		}
		if comments, ok := commentsMap[m]; ok {
			chatCount = len(comments)
			for _, comment := range comments {
				// VOD GQL comments carry no IRC emote ranges; dictionary tokenization only.
				for _, frag := range tokenizer.Tokenize(login, comment, nil) {
					if frag.T == "emote" {
						totalEmoteCount++
						if frag.Provider == "seventv" {
							seventvEmoteCount++
						}
						key := fmt.Sprintf("%s:%s:%s", frag.Provider, frag.ID, frag.C)
						emotesMap[key]++
					}
				}
			}
		}
		rollups = append(rollups, MinuteRollup{
			MinuteTS:          minuteTS,
			ViewerAvg:         viewerVal,
			ViewerMax:         viewerVal,
			ViewerLatest:      viewerVal,
			ViewerSamples:     1,
			ChatCount:         chatCount,
			TotalEmoteCount:   totalEmoteCount,
			SevenTVEmoteCount: seventvEmoteCount,
			Emotes:            emotesMap,
		})
	}
	return rollups
}

func toRollupViewerPoints(points []parsedViewerPoint) []RollupViewerPoint {
	out := make([]RollupViewerPoint, len(points))
	for i, p := range points {
		out[i] = RollupViewerPoint{OffsetSeconds: p.OffsetSeconds, Viewers: p.Viewers}
	}
	return out
}
