package matcher_test

import (
	"testing"

	"streamclone/internal/storygraph/matcher"
)

func TestLexicalMatcherLinkThreshold(t *testing.T) {
	m := matcher.NewLexical(matcher.Config{LinkThreshold: 0.65, ReviewThreshold: 0.40})
	result := m.Score(matcher.Input{
		Quotes:      []string{"caseoh reacts to ai cover"},
		ItemText:    "caseoh reacts to ai cover on stream",
		EntityMatch: true,
		TimingScore: 0.8,
	})
	if result.Decision != "link" && result.Decision != "review" {
		t.Fatalf("expected link or review, got %s conf=%f", result.Decision, result.Confidence)
	}
}

func TestLexicalMatcherEmptyDiscards(t *testing.T) {
	m := matcher.NewLexical(matcher.Config{LinkThreshold: 0.65, ReviewThreshold: 0.40})
	result := m.Score(matcher.Input{ItemText: "unrelated headline"})
	if result.Decision != "discard" {
		t.Fatalf("expected discard, got %s", result.Decision)
	}
}

func TestTitleSimilarity(t *testing.T) {
	positive := matcher.TitleSimilarity(
		"xQc has been banned from Twitch after DMCA complaint",
		"xQc banned from Twitch after DMCA complaint",
	)
	if positive < 0.70 {
		t.Fatalf("similar titles scored too low: %f", positive)
	}

	negative := matcher.TitleSimilarity(
		"xQc wins a chess tournament",
		"xQc banned from Twitch after DMCA complaint",
	)
	if negative >= 0.45 {
		t.Fatalf("unrelated titles scored too high: %f", negative)
	}
}
