package store

import (
	"encoding/json"
	"testing"
	"time"
)

func TestMentionsLogin(t *testing.T) {
	tests := []struct {
		text  string
		login string
		want  bool
	}{
		{"xqc reacts to drama", "xqc", true},
		{"Felix goes live", "xqc", false},
		{"xqcow", "xqc", true},
		{"", "xqc", false},
	}
	for _, tt := range tests {
		if got := mentionsLogin(tt.text, tt.login); got != tt.want {
			t.Fatalf("mentionsLogin(%q, %q) = %v, want %v", tt.text, tt.login, got, tt.want)
		}
	}
}

func TestSpreadRankScorePrefersSourceCountAndCredibility(t *testing.T) {
	now := time.Now()
	low := StoryCard{
		Cluster: StoryCluster{UpdatedAt: now},
		Scores:  Scores{Confidence: strPtr("developing")},
	}
	high := StoryCard{
		Cluster: StoryCluster{UpdatedAt: now.Add(-time.Hour)},
		WindowScores: &WindowScore{
			SourceCount:      3,
			CredibilityScore: 0.8,
			ComputedAt:       now,
		},
		Scores: Scores{Confidence: strPtr("corroborated")},
	}
	if spreadRankScore(low) >= spreadRankScore(high) {
		t.Fatalf("low=%d high=%d, expected high > low", spreadRankScore(low), spreadRankScore(high))
	}
}

func TestEntityDisplayAliasesMergesDisplayAndJSON(t *testing.T) {
	raw, _ := json.Marshal([]EntityAlias{{Platform: "reddit", Handle: "xQc"}})
	ent := &Entity{
		TwitchLogin: "xqc",
		DisplayName: "xQcOW",
		Aliases:     raw,
	}
	got := EntityDisplayAliases(ent)
	if len(got) < 2 {
		t.Fatalf("aliases = %#v, want display + stored alias", got)
	}
}

func strPtr(v string) *string { return &v }
