package analytics

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestGoldIVRPreflightHit(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/list" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`["2026-06-25"]`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	g := NewGoldIVRService(GoldIVRConfig{
		Enabled:     true,
		LiteEnabled: true,
		BaseURL:     srv.URL,
	}, nil, srv.Client(), nil)

	hit, reason := g.preflightCoverage(context.Background(), "40934651", time.Date(2026, 6, 25, 0, 0, 0, 0, time.UTC))
	if !hit || !strings.Contains(reason, "hit") {
		t.Fatalf("expected hit, got hit=%v reason=%q", hit, reason)
	}

	hit2, _ := g.preflightCoverage(context.Background(), "40934651", time.Date(2026, 6, 25, 0, 0, 0, 0, time.UTC))
	if !hit2 {
		t.Fatal("expected cache hit")
	}
}

func TestGoldIVRPreflightMiss(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte("Not found"))
	}))
	defer srv.Close()

	g := NewGoldIVRService(GoldIVRConfig{Enabled: true, LiteEnabled: true, BaseURL: srv.URL}, nil, srv.Client(), nil)
	hit, reason := g.preflightCoverage(context.Background(), "411377640", time.Now().UTC())
	if hit || !strings.Contains(reason, "miss") {
		t.Fatalf("expected miss, got hit=%v reason=%q", hit, reason)
	}
}

func TestGoldIVRParseNDJSON(t *testing.T) {
	body := strings.Join([]string{
		`{"id":"1","text":"hello","displayName":"u","username":"u","timestamp":"2026-06-25T00:00:01Z"}`,
		`{"id":"2","text":":emote:","displayName":"u","username":"u","timestamp":"2026-06-25T00:00:02Z"}`,
		"not-json",
	}, "\n")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-ndjson")
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	g := NewGoldIVRService(GoldIVRConfig{
		Enabled:            true,
		LiteEnabled:        true,
		BaseURL:            srv.URL,
		MaxMessagesPerJob:  100,
		ParserErrorRateMax: 0.5,
	}, nil, srv.Client(), nil)

	from := time.Date(2026, 6, 25, 0, 0, 0, 0, time.UTC)
	to := from.Add(time.Minute)
	stats, err := g.importWindow(context.Background(), "40934651", from, to, nil)
	if err != nil {
		t.Fatalf("importWindow: %v", err)
	}
	if stats.messages != 2 {
		t.Fatalf("messages=%d", stats.messages)
	}
	if stats.parserErrors != 1 {
		t.Fatalf("parserErrors=%d", stats.parserErrors)
	}
	if err := g.qualityCheck(stats); err != nil {
		t.Fatalf("qualityCheck: %v", err)
	}
	rollups := stats.minuteRollups(from)
	if len(rollups) != 1 || rollups[0].ChatCount != 2 {
		t.Fatalf("rollups=%+v", rollups)
	}
}

func TestGoldIVRQualityFailure(t *testing.T) {
	g := NewGoldIVRService(GoldIVRConfig{ParserErrorRateMax: 0.001}, nil, nil, nil)
	stats := &ivrImportStats{messages: 0, parserErrors: 10}
	if err := g.qualityCheck(stats); err == nil {
		t.Fatal("expected quality failure")
	}
	stats = &ivrImportStats{messages: 100, parserErrors: 1}
	if err := g.qualityCheck(stats); err == nil {
		t.Fatalf("expected parser rate failure, got nil")
	}
}

func TestGoldIVRDisabledByDefault(t *testing.T) {
	g := NewGoldIVRService(GoldIVRConfig{}, nil, nil, nil)
	out := g.TryAccelerator(context.Background(), "s1", "ludwig")
	if out.Attempted {
		t.Fatal("disabled service should not attempt")
	}
}

func TestGoldIVRAllowlist(t *testing.T) {
	g := NewGoldIVRService(GoldIVRConfig{
		Enabled:     true,
		LiteEnabled: true,
		Allowlist:   ParseGoldIVRAllowlist("ludwig"),
	}, nil, nil, nil)
	ok, _ := g.allowed("ludwig", "")
	if !ok {
		t.Fatal("ludwig should be allowed")
	}
	ok, _ = g.allowed("jynxzi", "")
	if ok {
		t.Fatal("jynxzi should be blocked by allowlist")
	}
}

func TestCanIVROverwriteRollupIntegration(t *testing.T) {
	if canIVROverwriteRollup(SourceConfidenceCanonical) {
		t.Fatal("ivr must not overwrite gql canonical")
	}
	if !canIVROverwriteRollup(SourceConfidenceProvisional) {
		t.Fatal("ivr may fill provisional gaps")
	}
}

func TestCoveragePctForMinutes(t *testing.T) {
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	end := start.Add(10 * time.Minute)
	pct := coveragePctForMinutes(5, start, end)
	if pct < 49.9 || pct > 50.1 {
		t.Fatalf("pct=%f", pct)
	}
}

func TestExistingChatMinutesSkipsImport(t *testing.T) {
	from := time.Date(2026, 6, 25, 0, 0, 0, 0, time.UTC)
	existing := []MinuteRollup{{MinuteTS: from, ChatCount: 50}}
	line := fmt.Sprintf(`{"id":"1","text":"x","displayName":"u","username":"u","timestamp":"%s"}`, from.Add(5*time.Second).Format(time.RFC3339))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(line))
	}))
	defer srv.Close()
	g := NewGoldIVRService(GoldIVRConfig{Enabled: true, LiteEnabled: true, BaseURL: srv.URL, MinMessagesPerWindow: 0}, nil, srv.Client(), nil)
	stats, err := g.importWindow(context.Background(), "1", from, from.Add(time.Minute), existing)
	if err != nil {
		t.Fatal(err)
	}
	if stats.messages != 0 {
		t.Fatalf("expected skip covered minute, got %d messages", stats.messages)
	}
}

func TestGoldIVRShadowModeAllowedWithoutLite(t *testing.T) {
	g := NewGoldIVRService(GoldIVRConfig{
		Enabled:     true,
		LiteEnabled: false,
		ShadowMode:  true,
		Allowlist:   ParseGoldIVRAllowlist("ludwig"),
	}, nil, nil, nil)
	ok, reason := g.allowed("ludwig", "")
	if !ok || reason == "" {
		t.Fatalf("shadow should allow ludwig without lite: ok=%v reason=%q", ok, reason)
	}
	ok, _ = g.allowed("jynxzi", "")
	if ok {
		t.Fatal("shadow still respects allowlist")
	}
}

func TestGoldIVRShadowModeBlocksWithoutAllowlist(t *testing.T) {
	g := NewGoldIVRService(GoldIVRConfig{
		Enabled:     true,
		LiteEnabled: false,
		ShadowMode:  true,
	}, nil, nil, nil)
	ok, reason := g.allowed("ludwig", "")
	if ok || reason != "allowlist_empty" {
		t.Fatalf("empty allowlist should deny: ok=%v reason=%q", ok, reason)
	}
}

func TestShadowCompareRollups(t *testing.T) {
	start := time.Date(2026, 6, 25, 0, 0, 0, 0, time.UTC)
	existing := []MinuteRollup{{
		MinuteTS: start, ChatCount: 100,
		ChatSource: RollupChatSourceGQL, SourceConfidence: SourceConfidenceCanonical,
	}}
	ivr := []MinuteRollup{{MinuteTS: start, ChatCount: 98}}
	score, rec := shadowCompareRollups(existing, ivr)
	if score < 95 || rec != "experimental_lite_chat_peaks" {
		t.Fatalf("score=%f rec=%q", score, rec)
	}
	ivrLow := []MinuteRollup{{MinuteTS: start, ChatCount: 40}}
	score, rec = shadowCompareRollups(existing, ivrLow)
	if rec != "reject_lite_for_now" && rec != "hold" {
		t.Fatalf("low overlap rec=%q score=%f", rec, score)
	}
}
