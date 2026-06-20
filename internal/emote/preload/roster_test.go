package preload

import (
	"testing"
)

func TestMergeTargetsDedupesAlwaysAndTop(t *testing.T) {
	top := []streamItem{
		{ID: "1", Login: "xqc"},
		{ID: "2", Login: "shroud"},
	}
	always := []string{"sodapoppin", "xqc", "XQC"}

	targets := MergeTargets(top, always)
	if len(targets) != 3 {
		t.Fatalf("expected 3 targets, got %d", len(targets))
	}
	if targets[0].Login != "sodapoppin" {
		t.Fatalf("always-tracked first: got %q", targets[0].Login)
	}
	seen := map[string]bool{}
	for _, item := range targets {
		if seen[item.Login] {
			t.Fatalf("duplicate login %q", item.Login)
		}
		seen[item.Login] = true
	}
}

func TestNormalizeLogins(t *testing.T) {
	got := normalizeLogins([]string{" Foo ", "foo", "", "bar"})
	if len(got) != 2 || got[0] != "foo" || got[1] != "bar" {
		t.Fatalf("normalizeLogins: %#v", got)
	}
}
