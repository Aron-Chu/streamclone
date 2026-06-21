package analytics

import (
	"encoding/json"
	"testing"
	"time"
)

func TestFilterVODsSince(t *testing.T) {
	now := time.Date(2026, 6, 21, 12, 0, 0, 0, time.UTC)
	since := now.Add(-45 * 24 * time.Hour)
	vods := []ArchivedVOD{
		{StreamID: "1", StartedAt: now.Add(-10 * 24 * time.Hour)},
		{StreamID: "2", StartedAt: now.Add(-60 * 24 * time.Hour)},
		{StreamID: "3", StartedAt: time.Time{}},
	}
	got := filterVODsSince(vods, since)
	if len(got) != 1 || got[0].StreamID != "1" {
		t.Fatalf("filterVODsSince = %+v, want stream 1 only", got)
	}
}

func TestParseVODIndexJSONL(t *testing.T) {
	line, err := json.Marshal(ArchivedVOD{
		StreamID:  "123",
		VideoID:   "v1",
		Title:     "Test",
		StartedAt: time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	got := parseVODIndexJSONL(append(line, '\n'))
	if len(got) != 1 || got[0].StreamID != "123" {
		t.Fatalf("parseVODIndexJSONL = %+v", got)
	}
}
