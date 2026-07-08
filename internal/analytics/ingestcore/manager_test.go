package ingestcore

import (
	"context"
	"testing"

	"streamclone/internal/config"
)

type fakeIRC struct {
	joined []string
	parted []string
}

func (f *fakeIRC) Join(_ context.Context, channel string) { f.joined = append(f.joined, channel) }
func (f *fakeIRC) Part(_ context.Context, channel string) { f.parted = append(f.parted, channel) }

func TestCollectorManagerReconcileAdmitsUpToCap(t *testing.T) {
	cfg := Config{MaxActiveIRC: 2, TieringEnabled: false}
	irc := &fakeIRC{}
	m := NewCollectorManager(cfg, irc, nil)
	m.SetRunContext(context.Background())

	candidates := []DesiredChannel{
		{Login: "a", StreamID: "s1", Tier: TierP1Hot, TrackPriority: 10, HelixRank: 1},
		{Login: "b", StreamID: "s2", Tier: TierP1Hot, TrackPriority: 10, HelixRank: 2},
		{Login: "c", StreamID: "s3", Tier: TierP1Hot, TrackPriority: 10, HelixRank: 3},
	}
	res := m.Reconcile(candidates)
	if res.Active != 2 {
		t.Fatalf("active = %d, want 2", res.Active)
	}
	if len(irc.joined) != 2 {
		t.Fatalf("joined = %v, want 2", irc.joined)
	}
}

func TestAssignTierP0Protected(t *testing.T) {
	cfg := Config{P1HotLimit: 50, HubRosterLimit: 500}
	if got := AssignTier(cfg, 80, 5, true); got != TierP0Always {
		t.Fatalf("tier = %v, want P0", got)
	}
}

func TestEngineIsActiveLogin(t *testing.T) {
	cfg := Config{CoreEnabled: true, MaxActiveIRC: 10}
	m := NewCollectorManager(cfg, &fakeIRC{}, nil)
	m.SetRunContext(context.Background())
	m.Reconcile([]DesiredChannel{
		{Login: "xqc", StreamID: "s1", Tier: TierP1Hot, TrackPriority: 10, HelixRank: 1},
	})
	e := &Engine{cfg: cfg, manager: m}
	if !e.IsActiveLogin("xqc") {
		t.Fatal("expected xqc active")
	}
	if e.IsActiveLogin("missing") {
		t.Fatal("expected missing login inactive")
	}
}

func TestConfigFromAppDefaultsDisabled(t *testing.T) {
	cfg := ConfigFromApp(config.Config{})
	if cfg.CoreEnabled {
		t.Fatal("expected core disabled by default")
	}
	if cfg.ShardCount != defaultShardCount {
		t.Fatalf("shard count = %d", cfg.ShardCount)
	}
}

func TestCollectorManagerReconcileCorpusRosterPreemptsHelixFill(t *testing.T) {
	cfg := Config{MaxActiveIRC: 1, TieringEnabled: false}
	irc := &fakeIRC{}
	m := NewCollectorManager(cfg, irc, nil)
	m.SetRunContext(context.Background())

	m.Reconcile([]DesiredChannel{
		{Login: "helixfill", StreamID: "s1", Tier: TierP1Hot, TrackPriority: 9, HelixRank: 1},
	})
	if !m.IsActiveLogin("helixfill") {
		t.Fatal("expected helix fill admitted first")
	}

	res := m.Reconcile([]DesiredChannel{
		{Login: "helixfill", StreamID: "s1", Tier: TierP1Hot, TrackPriority: 9, HelixRank: 1},
		{Login: "corpus", StreamID: "s2", Tier: TierP1Hot, TrackPriority: 11, HelixRank: 2},
	})
	if res.Active != 1 {
		t.Fatalf("active = %d, want 1", res.Active)
	}
	if !m.IsActiveLogin("corpus") {
		t.Fatal("expected corpus roster to preempt helix fill")
	}
	if m.IsActiveLogin("helixfill") {
		t.Fatal("expected helix fill evicted")
	}
	if len(irc.parted) != 1 || irc.parted[0] != "helixfill" {
		t.Fatalf("parted = %v, want [helixfill]", irc.parted)
	}
}
