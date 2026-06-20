package api

import (
	"testing"
	"time"
)

func TestParseWindow(t *testing.T) {
	now := time.Date(2026, 6, 17, 12, 0, 0, 0, time.UTC)
	t.Run("default24h", func(t *testing.T) {
		since, label, err := parseWindowAt(now, "")
		if err != nil || label != "24h" {
			t.Fatalf("parseWindowAt(\"\") = (%v, %q, %v)", since, label, err)
		}
		want := now.Add(-24 * time.Hour)
		if !since.Equal(want) {
			t.Fatalf("since = %v want %v", since, want)
		}
	})
	t.Run("7d", func(t *testing.T) {
		since, label, err := parseWindowAt(now, "7d")
		if err != nil || label != "7d" {
			t.Fatalf("parseWindowAt(7d) = label %q err %v", label, err)
		}
		want := now.Add(-7 * 24 * time.Hour)
		if !since.Equal(want) {
			t.Fatalf("since = %v want %v", since, want)
		}
	})
	t.Run("today", func(t *testing.T) {
		since, label, err := parseWindowAt(now, "today")
		if err != nil || label != "today" {
			t.Fatalf("parseWindowAt(today) = label %q err %v", label, err)
		}
		want := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
		if !since.Equal(want) {
			t.Fatalf("since = %v want %v", since, want)
		}
	})
	t.Run("invalid", func(t *testing.T) {
		if _, _, err := parseWindowAt(now, "30d"); err == nil {
			t.Fatal("expected error for invalid window")
		}
	})
}
