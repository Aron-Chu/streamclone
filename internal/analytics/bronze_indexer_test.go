package analytics

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type mockBronzeWriter struct {
	top500    int
	vodIndex  map[string][]byte
	summaries map[string][]byte
}

func (m *mockBronzeWriter) ExportTop500(_ context.Context, _ []byte) error {
	m.top500++
	return nil
}

func (m *mockBronzeWriter) ExportVODIndex(_ context.Context, login string, lines []byte) error {
	if m.vodIndex == nil {
		m.vodIndex = map[string][]byte{}
	}
	m.vodIndex[login] = append([]byte(nil), lines...)
	return nil
}

func (m *mockBronzeWriter) ExportChannelSummary(_ context.Context, login string, payload []byte) error {
	if m.summaries == nil {
		m.summaries = map[string][]byte{}
	}
	m.summaries[login] = append([]byte(nil), payload...)
	return nil
}

type mockBronzeHelix struct {
	vods []ArchivedVOD
	err  error
}

func (m *mockBronzeHelix) Enabled() bool { return true }

func (m *mockBronzeHelix) ArchivedStreamHistory(context.Context, string, int) ([]ArchivedVOD, error) {
	return m.vods, m.err
}

func TestBronzePickCandidates(t *testing.T) {
	now := time.Date(2026, 6, 20, 12, 0, 0, 0, time.UTC)
	stale := now.Add(-25 * time.Hour)
	fresh := now.Add(-1 * time.Hour)
	roster := []string{"alpha", "beta", "gamma", "delta"}
	states := map[string]*BronzeIndexState{
		"alpha": {Login: "alpha", LastHelixAt: &fresh, LastSummaryAt: &fresh},
		"beta":  {Login: "beta", LastHelixAt: &stale, LastSummaryAt: &fresh},
		"gamma": nil,
		"delta": {Login: "delta", LastHelixAt: &stale, LastSummaryAt: &stale},
	}
	got := pickBronzeCandidates(roster, states, 2, now.Add(-24*time.Hour))
	if len(got) != 2 {
		t.Fatalf("expected 2 candidates, got %v", got)
	}
	if got[0] != "gamma" {
		t.Fatalf("never-indexed login should sort first, got %v", got)
	}
	for _, login := range got {
		if login == "alpha" {
			t.Fatalf("fresh login should be excluded, got %v", got)
		}
	}
}

func TestBronzeExportTTSummary(t *testing.T) {
	summary := `{"rank":1,"avg_viewers":1000}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/channels/summary/ohnepixel") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(summary))
	}))
	defer srv.Close()

	writer := &mockBronzeWriter{}
	indexer := NewBronzeIndexer(nil, &mockBronzeHelix{}, "", srv.URL, "test-agent", 500, nil, 2, 4).
		WithWriter(writer)

	uri, err := indexer.exportTTSummary(context.Background(), "ohnepixel")
	if err != nil {
		t.Fatal(err)
	}
	if uri != "channels/summary/ohnepixel.json" {
		t.Fatalf("uri = %q", uri)
	}
	if string(writer.summaries["ohnepixel"]) != summary {
		t.Fatalf("summary payload mismatch: %q", writer.summaries["ohnepixel"])
	}
}

func TestBronzeExportHelixIndex(t *testing.T) {
	writer := &mockBronzeWriter{}
	start := time.Date(2026, 6, 1, 10, 0, 0, 0, time.UTC)
	end := start.Add(2 * time.Hour)
	helix := &mockBronzeHelix{
		vods: []ArchivedVOD{{
			StreamID: "317014684259", VideoID: "v1", Title: "Test",
			StartedAt: start, EndedAt: end, DurationMinutes: 120,
		}},
	}
	indexer := NewBronzeIndexer(nil, helix, "", "", "", 500, nil, 2, 4).WithWriter(writer)
	rows, uri, err := indexer.exportHelixIndex(context.Background(), "ohnepixel")
	if err != nil {
		t.Fatal(err)
	}
	if rows != 1 {
		t.Fatalf("rows = %d", rows)
	}
	if uri != "channels/vod_index/ohnepixel.jsonl.gz" {
		t.Fatalf("uri = %q", uri)
	}
	var decoded ArchivedVOD
	if err := json.Unmarshal(writer.vodIndex["ohnepixel"][:len(writer.vodIndex["ohnepixel"])-1], &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.StreamID != "317014684259" {
		t.Fatalf("stream id = %q", decoded.StreamID)
	}
}

func TestBronzeHelixVideoTimes(t *testing.T) {
	start, end, mins := helixVideoTimes("2026-06-01T10:00:00Z", "2h30m")
	if start.IsZero() || end.IsZero() {
		t.Fatal("expected parsed times")
	}
	if mins != 150 {
		t.Fatalf("duration minutes = %d", mins)
	}
}

func TestBronzeChannelsPerTickScaling(t *testing.T) {
	idx := NewBronzeIndexer(nil, nil, "", "", "", 500, nil, 2, 4)
	if idx.channelsPerTick < 1 || idx.channelsPerTick > 4 {
		t.Fatalf("channelsPerTick = %d", idx.channelsPerTick)
	}
}
