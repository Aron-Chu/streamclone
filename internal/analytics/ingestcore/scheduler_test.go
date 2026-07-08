package ingestcore

import "testing"

func TestMergeDesiredChannelsPrefersP0(t *testing.T) {
	base := []DesiredChannel{
		{Login: "xqc", Tier: TierP1Hot, TrackPriority: 10, HelixRank: 1},
	}
	extra := []DesiredChannel{
		{Login: "xqc", Tier: TierP0Always, TrackPriority: 70},
	}
	merged := mergeDesiredChannels(base, extra)
	if len(merged) != 1 {
		t.Fatalf("expected 1 channel, got %d", len(merged))
	}
	if merged[0].Tier != TierP0Always {
		t.Fatalf("expected P0 tier, got %v", merged[0].Tier)
	}
}

func TestEngineOwnsIRCAdmission(t *testing.T) {
	var nilEngine *Engine
	if nilEngine.OwnsIRCAdmission() {
		t.Fatal("nil engine should not own admission")
	}
	e := &Engine{cfg: Config{CoreEnabled: true}}
	if !e.OwnsIRCAdmission() {
		t.Fatal("core enabled engine should own admission")
	}
}
