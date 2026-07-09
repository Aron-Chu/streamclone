package jobstate

import "testing"

func TestInSetAcceptsEveryCanonicalMember(t *testing.T) {
	for _, state := range All() {
		if !InSet(state) {
			t.Fatalf("expected canonical state %q to be in the Job_State_Set", state)
		}
	}
}

func TestInSetRejectsOutOfSetValues(t *testing.T) {
	rejected := []string{
		"",
		" queued",
		"queued ",
		"QUEUED",
		"Queued",
		"ready", // legacy candidate-job status, not a Job_State
		"unknown_state",
		"rendering\n",
		"complete;drop",
	}
	for _, state := range rejected {
		if InSet(state) {
			t.Fatalf("expected non-member %q to be rejected by InSet", state)
		}
	}
}

func TestAllReturnsIndependentCopy(t *testing.T) {
	first := All()
	if len(first) == 0 {
		t.Fatal("expected a non-empty Job_State_Set")
	}
	first[0] = "mutated"
	second := All()
	if second[0] == "mutated" {
		t.Fatal("All must return an independent copy; internal ordering was mutated")
	}
}

func TestAllHasNoDuplicates(t *testing.T) {
	seen := make(map[string]struct{})
	for _, state := range All() {
		if _, dup := seen[state]; dup {
			t.Fatalf("duplicate state %q in Job_State_Set", state)
		}
		seen[state] = struct{}{}
	}
	if len(seen) != len(membership) {
		t.Fatalf("All (%d) and membership set (%d) disagree in size", len(seen), len(membership))
	}
}
