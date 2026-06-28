package analytics

import (
	"encoding/json"
	"log/slog"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"
)

func TestParseTwitchTrackerHTML_Metadata(t *testing.T) {
	// Let's create a dummy SyncService with a default logger
	s := &SyncService{
		log: slog.Default(),
	}

	// Real HTML containing the actual meta ecs content
	htmlContent := `<html>
<head>
<meta id="ecs" content="MzE2OTEzNDk3OTU0!ZmFsc2U!ZmFsc2U!ZmFsc2U!#1siMjAyNi0wNi0wNyAxMzoyNTo0NyIsMCxud#xsLG51bGxdLFsiMjAyNi0wNi0wNyAxMzozMDowMCIsMTYzOTIsMCwwXSxbIjIwMjYtMDYtMDcgMTM6NDA6MDAiLDIzMDMwLDUxLDBdLFsiMjAyNi0wNi0wNyAxMzo1MDowMCIsMjg1MjgsNjEsMF0s#yIyMDI2LTA2LTA3IDE0OjAwOjAwIiwzMDY3MSw2MSwwXSxbIjIwMjYtMDYtMDcgMTQ6MTA6MDAiLDM0NTEyLDYxLDBdLFsiMjAyNi0wNi0wNyAxNDoyMDowMCIsMzY1NjAsNTYsMF0s#yIyMDI2LTA2LTA3IDE0OjMwOjAwIiwzMjIxMiw1MCwwXSxbIjIwMjYtMDYtMDcgMTQ6NDA6MDAiLDM2MDE5LDUyLDBdLFsiMjAyNi0wNi0wNyAxNDo1MDowMCIsMzk2NTEsNzEsMF0s#yIyMDI2LTA2LTA3IDE1OjAwOjAwIiwzNTc0NSw2MiwwXSxbIjIwMjYtMDYtMDcgMTU6MTA6MDAiLDMyMzk3LDU2LDBdLFsiMjAyNi0wNi0wNyAxNToyMDowMCIsMzc1ODQsNDIsMF0s#yIyMDI2LTA2LTA3IDE1OjMwOjAwIiwzNzU5OCw2MiwwXSxbIjIwMjYtMDYtMDcgMTU6NDA6MDAiLDMzNzUzLDU5LDBdLFsiMjAyNi0wNi0wNyAxNTo1MDowMCIsMzkwNzcsNDcsMF0s#yIyMDI2LTA2LTA3IDE2OjAwOjAwIiwzODE5Niw2OSwwXSxbIjIwMjYtMDYtMDcgMTY6MTA6MDAiLDMyMjYwLDUxLDBdLFsiMjAyNi0wNi0wNyAxNjoyMDowMCIsMzMwODQsNTUsMF0s#yIyMDI2LTA2LTA3IDE2OjMwOjAwIiwzMzgwMCw3MSwwXSxbIjIwMjYtMDYtMDcgMTY6NDA6MDAiLDM1MjMzLDY3LDBdLFsiMjAyNi0wNi0wNyAxNjo1MDowMCIsMzc1NzEsODgsMF0s#yIyMDI2LTA2LTA3IDE3OjAwOjAwIiwzOTM1MSw4NCwwXSxbIjIwMjYtMDYtMDcgMTc6MTA6MDAiLDQyOTcxLDQ4LDBdLFsiMjAyNi0wNi0wNyAxNzoyMDowMCIsNTE0ODEsNjQsMF0s#yIyMDI2LTA2LTA3IDE3OjMwOjAwIiw0OTUzMyw2MywwXSxbIjIwMjYtMDYtMDcgMTc6NDA6MDAiLDQyODgwLDY0LDBdLFsiMjAyNi0wNi0wNyAxNzo1MDowMCIsNDQzNzEsNTAsMF0s#yIyMDI2LTA2LTA3IDE4OjAwOjAwIiw0NDI0MCw2MiwwXSxbIjIwMjYtMDYtMDcgMTg6MTA6MDAiLDUyMDM5LDUwLDBdLFsiMjAyNi0wNi0wNyAxODoyMDowMCIsNDg0NjAsNjUsMF0s#yIyMDI2LTA2LTA3IDE4OjMwOjAwIiw0Mjk3OSw0MTIsMF1d!IjIwMjYtMDYtMDdUMTM6MjU6NDcuMDAwMDAw#iI!IjIwMjYtMDYtMDdUMTg6MzU6MDAuMDAwMDAw#iI!Mzc0OTA!NTIwMzk!#3siY3JlYXRlZF9hdCI6IjIwMjYtMDYtMDdUMTM6MjU6NDcuMDAwMDAw#iIsInRpdGxlIjoiXHVkODNkXHVkZDM0Q0FOIERPTksgUVVBTElG#SBGT1IgTUFKT1IgUExB#U9GRlM/XHVkODNkXHVkZDM0IiwicmVsYXRpdmVfYXQiOjB9LHsiY3JlYXRlZF9hdCI6IjIwMjYtMDYtMDcgMTM6NDA6MDAiLCJ0aXRsZSI6Ilx1ZDgzZFx1ZGQzNEJJRyB2cyBNSUJSIHwgSUVNIENPTE9HTkUgTUFKT1IgMjAyNlx1ZDgzZFx1ZGQzNCIsInJlbGF0aXZlX2F0IjoxNH0seyJjcmVhdGVkX2F0IjoiMjAyNi0wNi0wNyAxNDozMDowMCIsInRpdGxlIjoiXHVkODNkXHVkZDM0TTgwIHZzIEJFVEJPT00gfCBJRU0gQ09MT0dORSBNQUpPUlx1ZDgzZFx1ZGQzNCIsInJlbGF0aXZlX2F0Ijo2NH0seyJjcmVhdGVkX2F0IjoiMjAyNi0wNi0wNyAxNToyMDowMCIsInRpdGxlIjoiXHVkODNkXHVkZDM0RE9OSyBQTEFZSU5HIE1BSk9SIFFVQUxJRklFUlx1ZDgzZFx1ZGQzNCIsInJlbGF0aXZlX2F0IjoxMTR9LHsiY3JlYXRlZF9hdCI6IjIwMjYtMDYtMDcgMTY6MzA6MDAiLCJ0aXRsZSI6Ilx1ZDgzZFx1ZGQzNERPTksgUExB#UlORyBNQUpPUiBRVUFMSUZJRVJcd#Q4M2Rcd#RkMzRXQVRDSElORyBET05LIEFGVEVSXHVkODNkXHVkZDM0IiwicmVsYXRpdmVfYXQiOjE4NH0seyJjcmVhdGVkX2F0IjoiMjAyNi0wNi0wNyAxNjo0MDowMCIsInRpdGxlIjoiXHVkODNkXHVkZDM0QkVTVCBXSEVFTCBQTEFZRVIgV0lOUyBFTElNSU5BVE9SXHVkODNkXHVkZDM0V0FUQ0hJTkcgRE9OSyBBRlRFUlx1ZDgzZFx1ZGQzNCIsInJlbGF0aXZlX2F0IjoxOTR9LHsiY3JlYXRlZF9hdCI6IjIwMjYtMDYtMDcgMTc6MDA6MDAiLCJ0aXRsZSI6Ilx1ZDgzZFx1ZGQzNFNQSVJJVCB2cyA5#iB8IElFTSBDT0xPR05FIE1BSk9SXHVkODNkXHVkZDM0IiwicmVsYXRpdmVfYXQiOjIxNH1d!#3siZGF0ZSI6IjIwMjYtMDYtMDcgMTM6MjU6NDciLCJpZCI6MzIzOTksIm5hb#UiOiJDb3VudGVyLVN0cmlrZSJ9LHsiZGF0ZSI6IjIwMjYtMDYtMDcgMTY6MzA6MDAiLCJpZCI6MTk5MTExNTMxMCwibmFtZSI6IkZvcnphIEhvcml6b24gNiJ9LHsiZGF0ZSI6IjIwMjYtMDYtMDcgMTc6MDA6MDAiLCJpZCI6MzIzOTksIm5hb#UiOiJDb3VudGVyLVN0cmlrZSJ9XQ!#yJpZCIsImxpdmUiLCJoYXN#a#V3cyIsImhhc0NoYXR0ZXJzIiwiY2hhcnQiLCJjcmVhdGVkX2F0IiwiZmluaXNoZ#RfYXQiLCJhdmdfdmlld2VycyIsIm1heF92a#V3ZXJzIiwidGl0bGVzIiwiZ2FtZXMiXQ">
</head>
<body></body>
</html>`

	startedAt := time.Now()
	// Let's print the parts of meta#ecs content to debug
	var content string
	if match := regexp.MustCompile(`(?i)<meta\s+id="ecs"\s+content="([^"]+)"`).FindStringSubmatch(htmlContent); len(match) > 1 {
		content = match[1]
	}
	parts := strings.Split(content, "!")
	t.Logf("Number of parts: %d", len(parts))
	if len(parts) > 0 {
		keysB64 := parts[len(parts)-1]
		keysBytes, err := decodeBase64(keysB64)
		if err != nil {
			t.Logf("Failed to decode keys: %v", err)
		} else {
			t.Logf("Decoded keys: %s", string(keysBytes))
		}

		// Map key to index
		var keys []string
		_ = json.Unmarshal(keysBytes, &keys)
		keyToIndex := make(map[string]int)
		for idx, key := range keys {
			keyToIndex[key] = idx
		}

		getPartBytes := func(key string) ([]byte, bool) {
			idx, ok := keyToIndex[key]
			if !ok || idx >= len(parts)-1 {
				return nil, false
			}
			decoded, err := decodeBase64(parts[idx])
			if err != nil {
				t.Logf("Failed to decode field %s: %v", key, err)
				return nil, false
			}
			return decoded, true
		}

		if b, ok := getPartBytes("created_at"); ok {
			t.Logf("Raw created_at bytes: %s", string(b))
			var s string
			err := json.Unmarshal(b, &s)
			t.Logf("Unmarshalled created_at: '%s', err: %v", s, err)
		}
		if b, ok := getPartBytes("chart"); ok {
			var cp [][]any
			err := json.Unmarshal(b, &cp)
			t.Logf("Unmarshalled chartPoints count: %d, err: %v", len(cp), err)
			if len(cp) > 0 {
				t.Logf("Sample chart point 0: %+v", cp[0])
			}
		}
		if b, ok := getPartBytes("games"); ok {
			t.Logf("Raw games bytes: %s", string(b))
		}
	}

	parsed, err := s.parseTwitchTrackerMetadata(htmlContent, startedAt)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if parsed.DurationMinutes != 309 {
		t.Errorf("Expected duration 309 minutes, got %d", parsed.DurationMinutes)
	}

	if parsed.PeakViewers != 52039 {
		t.Errorf("Expected peak viewers 52039, got %d", parsed.PeakViewers)
	}

	if len(parsed.Games) != 3 {
		t.Errorf("Expected 3 games, got %d", len(parsed.Games))
	} else {
		if parsed.Games[0].Title != "Counter-Strike" {
			t.Errorf("Expected game 0 title Counter-Strike, got %s", parsed.Games[0].Title)
		}
		if parsed.Games[0].BoxArt != "https://static-cdn.jtvnw.net/ttv-boxart/32399-210x280.jpg" {
			t.Errorf("Expected game 0 box art link, got %s", parsed.Games[0].BoxArt)
		}
		if parsed.Games[1].Title != "Forza Horizon 6" {
			t.Errorf("Expected game 1 title Forza Horizon 6, got %s", parsed.Games[1].Title)
		}
	}

	if len(parsed.ViewerPoints) == 0 {
		t.Errorf("Expected parsed viewer points, got 0")
	} else {
		t.Logf("Parsed %d viewer points successfully.", len(parsed.ViewerPoints))
	}
}

func TestBuildGameSegmentsEqualSplit(t *testing.T) {
	games := []scrapedGame{
		{Title: "Just Chatting", DurationMinutes: 0},
		{Title: "Left 4 Dead 2", DurationMinutes: 0},
		{Title: "Ultrapool", DurationMinutes: 0},
	}
	segments := buildGameSegments(games, 7*3600+6*60)
	if len(segments) != 3 {
		t.Fatalf("expected 3 segments, got %d", len(segments))
	}
	if segments[0].OffsetSeconds != 0 || segments[0].DurationSeconds <= 0 {
		t.Fatalf("unexpected first segment: %+v", segments[0])
	}
	if segments[1].OffsetSeconds != segments[0].DurationSeconds {
		t.Fatalf("expected sequential offsets, got %+v then %+v", segments[0], segments[1])
	}
}

func TestHasCompleteViewerChart(t *testing.T) {
	if hasCompleteViewerChart(nil, 3600) {
		t.Fatal("expected nil points to be rejected")
	}
	flat := []parsedViewerPoint{
		{OffsetSeconds: 0, Viewers: 69800},
		{OffsetSeconds: 3600, Viewers: 69800},
	}
	if hasCompleteViewerChart(flat, 3600) {
		t.Fatal("expected flat peak-only synthesis to be rejected")
	}
	partialTail := []parsedViewerPoint{
		{OffsetSeconds: 0, Viewers: 1000},
		{OffsetSeconds: 600, Viewers: 5000},
		{OffsetSeconds: 1200, Viewers: 3000},
		{OffsetSeconds: 1800, Viewers: 2900},
		{OffsetSeconds: 2400, Viewers: 2800},
		{OffsetSeconds: 3000, Viewers: 2800},
	}
	if hasCompleteViewerChart(partialTail, 8*3600) {
		t.Fatal("expected spike+short-tail chart to be rejected as incomplete")
	}
	fullLength := []parsedViewerPoint{
		{OffsetSeconds: 0, Viewers: 1200},
		{OffsetSeconds: 300, Viewers: 1500},
		{OffsetSeconds: 600, Viewers: 1800},
		{OffsetSeconds: 900, Viewers: 2100},
		{OffsetSeconds: 1200, Viewers: 1900},
		{OffsetSeconds: 1500, Viewers: 2200},
		{OffsetSeconds: 1800, Viewers: 2600},
		{OffsetSeconds: 2100, Viewers: 2400},
		{OffsetSeconds: 2400, Viewers: 2300},
		{OffsetSeconds: 2700, Viewers: 2500},
		{OffsetSeconds: 3000, Viewers: 2800},
		{OffsetSeconds: 3300, Viewers: 2600},
	}
	if !hasCompleteViewerChart(fullLength, 3600) {
		t.Fatal("expected full-length varied chart to be accepted")
	}
}

func TestMain(m *testing.M) {
	os.Exit(m.Run())
}
