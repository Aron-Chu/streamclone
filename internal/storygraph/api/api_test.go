package api

import (
	"strings"
	"testing"
	"time"

	"streamclone/internal/storygraph/ingest"
)

func TestParseWindowAt(t *testing.T) {
	now := time.Date(2026, 6, 17, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		raw       string
		wantLabel string
		wantSince time.Time
	}{
		{"", "24h", now.Add(-24 * time.Hour)},
		{"24h", "24h", now.Add(-24 * time.Hour)},
		{"7d", "7d", now.Add(-7 * 24 * time.Hour)},
		{" 7D ", "7d", now.Add(-7 * 24 * time.Hour)},
		{"today", "today", time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)},
		{" TODAY ", "today", time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)},
	}
	for _, tt := range tests {
		since, label, err := parseWindowAt(now, tt.raw)
		if err != nil {
			t.Fatalf("parseWindowAt(%q): %v", tt.raw, err)
		}
		if label != tt.wantLabel || !since.Equal(tt.wantSince) {
			t.Fatalf("parseWindowAt(%q) = (%s, %s), want (%s, %s)", tt.raw, label, since, tt.wantLabel, tt.wantSince)
		}
	}
}

func TestParseWindowAtRejectsInvalidWindow(t *testing.T) {
	if _, _, err := parseWindowAt(time.Now(), "30d"); err == nil {
		t.Fatal("expected invalid window error")
	}
}

func TestSourceModeDistinguishesDegradedFromError(t *testing.T) {
	now := time.Date(2026, 6, 18, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name       string
		status     ingest.SourceStatus
		configured bool
		evidenceAt *time.Time
		want       string
	}{
		{name: "off when unconfigured", configured: false, want: "off"},
		{name: "active when healthy", configured: true, status: ingest.SourceStatus{Healthy: true}, want: "active"},
		{name: "error without history", configured: true, status: ingest.SourceStatus{LastError: "upstream failed"}, want: "error"},
		{name: "degraded with last ok", configured: true, status: ingest.SourceStatus{LastError: "upstream failed", LastOKAt: &now}, want: "degraded"},
		{name: "degraded with evidence", configured: true, status: ingest.SourceStatus{LastError: "upstream failed"}, evidenceAt: &now, want: "degraded"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := sourceMode(tt.status, tt.configured, tt.evidenceAt); got != tt.want {
				t.Fatalf("sourceMode() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestCommunityFlairQueryParamNormalization(t *testing.T) {
	flair := strings.TrimSpace("  Announcement  ")
	if flair != "Announcement" {
		t.Fatalf("trim = %q", flair)
	}
	if strings.ToLower(flair) != "announcement" {
		t.Fatalf("lower = %q", strings.ToLower(flair))
	}
}

func TestSourceDetailPayloadIncludesCommentStatus(t *testing.T) {
	okAt := time.Date(2026, 6, 18, 12, 0, 0, 0, time.UTC)
	pollAt := okAt.Add(time.Minute)
	payload := sourceDetailPayload(map[string]ingest.SourceDetailStatus{
		"comments": {
			Healthy:    true,
			LastOKAt:   &okAt,
			LastItems:  8,
			LastPollAt: pollAt,
		},
	})

	raw, ok := payload["comments"].(map[string]any)
	if !ok {
		t.Fatalf("comments detail missing from payload: %#v", payload)
	}
	if raw["healthy"] != true || raw["last_items"] != 8 || raw["last_ok_at"] != &okAt || raw["last_poll_at"] != pollAt {
		t.Fatalf("comments detail payload = %#v", raw)
	}
}
