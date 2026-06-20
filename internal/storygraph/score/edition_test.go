package score

import (
	"testing"

	"streamclone/internal/storygraph/store"
)

func TestFilterCorroboratedRequiresTwoSources(t *testing.T) {
	cards := []store.StoryCard{
		{WindowScores: &store.WindowScore{SourceCount: 1}},
		{WindowScores: &store.WindowScore{SourceCount: 2}},
		{WindowScores: &store.WindowScore{SourceCount: 3}},
	}
	got := FilterCorroborated(cards)
	if len(got) != 2 {
		t.Fatalf("FilterCorroborated len = %d, want 2", len(got))
	}
}

func TestBuildEditionSectionsToday(t *testing.T) {
	mover := store.RisingStreamer{Login: "xqc"}
	sections := BuildEditionSections(WireEdition{
		Window:       "today",
		Breaking:     []store.StoryCard{{Cluster: store.StoryCluster{ID: 1, Title: "breaking"}}},
		BiggestMover: &mover,
		NewEntrants:  []store.RisingStreamer{{Login: "new1"}},
	})
	if len(sections) != 3 {
		t.Fatalf("today sections = %d, want 3", len(sections))
	}
	if sections[0].ID != "breaking" || len(sections[0].Stories) != 1 {
		t.Fatalf("breaking section = %+v", sections[0])
	}
}
