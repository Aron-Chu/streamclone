package analytics

import "testing"

func TestFilterCommentsMapByOffsetRange(t *testing.T) {
	m := map[int][]string{
		0:  {"a"},
		5:  {"b"},
		10: {"c"},
		20: {"d"},
	}
	filterCommentsMapByOffsetRange(m, 300, 900)
	if len(m) != 2 {
		t.Fatalf("expected 2 minutes, got %d", len(m))
	}
	if _, ok := m[5]; !ok {
		t.Fatal("expected minute 5 to remain")
	}
	if _, ok := m[10]; !ok {
		t.Fatal("expected minute 10 to remain")
	}
}

func TestEnrichCoverageChatSourcesManualImport(t *testing.T) {
	c := enrichCoverageChatSources(ExtensionCoverage{}, []MinuteRollup{
		{ChatCount: 10, ChatSource: RollupChatSourceLive, SourceConfidence: SourceConfidenceVerified},
		{ChatCount: 5, ChatSource: RollupChatSourceGQL, ChatSourceDetail: RollupDetailManualImport},
	})
	if c.ChatSource != ChatSourceGQL || c.ChatSourceDetail != RollupDetailManualImport {
		t.Fatalf("got source=%q detail=%q", c.ChatSource, c.ChatSourceDetail)
	}
}
