package analytics

import (
	"os"
	"testing"
	"time"
)

func TestSelectIVRPeakRollups(t *testing.T) {
	rollups := []MinuteRollup{
		{ChatCount: 100},
		{ChatCount: 50},
		{ChatCount: 5},
		{ChatCount: 80},
		{ChatCount: 120},
		{ChatCount: 90},
	}
	out := selectIVRPeakRollups(rollups, 3, 10)
	if len(out) != 3 {
		t.Fatalf("expected 3 peaks, got %d", len(out))
	}
	if out[0].ChatCount != 120 || out[1].ChatCount != 100 || out[2].ChatCount != 90 {
		t.Fatalf("unexpected ranking: %+v", out)
	}
	for _, r := range out {
		if len(r.Emotes) != 0 || r.TotalEmoteCount != 0 || r.SevenTVEmoteCount != 0 {
			t.Fatal("peaks-only must strip emotes")
		}
		if r.ChatSource != RollupChatSourceIVR || r.SourceConfidence != SourceConfidenceProvisional {
			t.Fatalf("source metadata: src=%q conf=%q", r.ChatSource, r.SourceConfidence)
		}
		if r.ChatSourceDetail != RollupDetailIVRPeaksOnly {
			t.Fatalf("expected peaks-only detail, got %q", r.ChatSourceDetail)
		}
	}
}

func TestGoldIVRPeaksOnlyAllowed(t *testing.T) {
	g := NewGoldIVRService(GoldIVRConfig{
		Enabled:          true,
		PeaksOnlyEnabled: true,
		Allowlist:        ParseGoldIVRAllowlist("ludwig"),
	}, nil, nil, nil)
	ok, _ := g.allowed("ludwig", "")
	if !ok {
		t.Fatal("peaks-only should allow ludwig")
	}
	g2 := NewGoldIVRService(GoldIVRConfig{
		Enabled:          true,
		LiteEnabled:      true,
		PeaksOnlyEnabled: true,
		Allowlist:        ParseGoldIVRAllowlist("ludwig"),
	}, nil, nil, nil)
	ok, reason := g2.allowed("ludwig", "")
	if ok || reason != "ivr_lite_and_peaks_only_mutually_exclusive" {
		t.Fatalf("mutual exclusion: ok=%v reason=%q", ok, reason)
	}
}

func TestPruneGoldIVRShadowArtifacts(t *testing.T) {
	dir := t.TempDir()
	oldPath := dir + string(os.PathSeparator) + "old.json"
	newPath := dir + string(os.PathSeparator) + "new.json"
	if err := os.WriteFile(oldPath, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(newPath, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	oldTime := time.Now().Add(-10 * 24 * time.Hour)
	if err := os.Chtimes(oldPath, oldTime, oldTime); err != nil {
		t.Fatal(err)
	}
	if err := pruneGoldIVRShadowArtifacts(dir, 7, 1000); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
		t.Fatal("expected old artifact pruned")
	}
	if _, err := os.Stat(newPath); err != nil {
		t.Fatal("expected new artifact kept")
	}
}
