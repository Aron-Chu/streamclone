package analytics

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/redis/go-redis/v9"
)

type mockWatchlistSummaryStore struct {
	watchlists      map[string][]PulseWatchlistEntry
	streams         map[string]*StreamRecord
	bookmarks       map[string][]PulseBookmark
	listErr         error
	lastBatchLogins []string
}

func (m *mockWatchlistSummaryStore) ListPulseWatchlist(_ context.Context, principalID string) ([]PulseWatchlistEntry, error) {
	if m.listErr != nil {
		return nil, m.listErr
	}
	if m.watchlists == nil {
		return nil, nil
	}
	return m.watchlists[principalID], nil
}

func (m *mockWatchlistSummaryStore) LatestStreamsByLogins(_ context.Context, logins []string) (map[string]*StreamRecord, error) {
	m.lastBatchLogins = append([]string(nil), logins...)
	out := make(map[string]*StreamRecord, len(logins))
	for _, login := range logins {
		if m.streams != nil {
			if rec, ok := m.streams[login]; ok {
				out[login] = rec
			}
		}
	}
	return out, nil
}

func (m *mockWatchlistSummaryStore) ListPulseBookmarks(_ context.Context, filter ListPulseBookmarksFilter) ([]PulseBookmark, string, error) {
	if m.bookmarks == nil {
		return nil, "", nil
	}
	items := m.bookmarks[filter.PrincipalID]
	if len(items) > filter.Limit && filter.Limit > 0 {
		items = items[:filter.Limit]
	}
	return items, "", nil
}

func hostedSummaryRouter(h *Handler) chi.Router {
	r := chi.NewRouter()
	h.PulseRoutes(r)
	return r
}

func TestWatchlistSummaryUnauthorizedWithoutKey(t *testing.T) {
	h := &Handler{
		pulseHosted: PulseHostedConfig{Hosted: true, BetaKeys: []string{"secret-one"}},
	}
	r := hostedSummaryRouter(h)

	req := httptest.NewRequest(http.MethodGet, "/v1/pulse/watchlist/summary", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestWatchlistSummaryInvalidLogin(t *testing.T) {
	h := &Handler{
		pulseHosted: PulseHostedConfig{Hosted: true, BetaKeys: []string{"secret-one"}},
	}
	r := hostedSummaryRouter(h)

	req := httptest.NewRequest(http.MethodGet, "/v1/pulse/watchlist/summary?login=!!!", nil)
	req.Header.Set("X-Streamclone-Beta-Key", "secret-one")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
	var body map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["error"] != "invalid_login" {
		t.Fatalf("error = %q, want invalid_login", body["error"])
	}
}

func TestWatchlistSummaryEmptyWatchlist(t *testing.T) {
	principalID := hashPulseBetaKey("secret-one")
	h := &Handler{
		pulseHosted: PulseHostedConfig{Hosted: true, BetaKeys: []string{"secret-one"}},
		store:       &Store{},
	}
	h.store = nil
	summary, err := h.buildPulseWatchlistSummary(context.Background(), &mockWatchlistSummaryStore{
		watchlists: map[string][]PulseWatchlistEntry{principalID: {}},
	}, principalID, "", watchlistSummaryContext{})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if summary.LiveCount != 0 || summary.ProtectedCount != 0 {
		t.Fatalf("counts = live %d protected %d, want 0/0", summary.LiveCount, summary.ProtectedCount)
	}
	if len(summary.LiveNow) != 0 || len(summary.Moments) != 0 || len(summary.Recaps) != 0 || len(summary.Attention) != 0 {
		t.Fatalf("expected empty arrays, got liveNow=%d moments=%d recaps=%d attention=%d",
			len(summary.LiveNow), len(summary.Moments), len(summary.Recaps), len(summary.Attention))
	}
}

func TestWatchlistSummaryPrincipalIsolation(t *testing.T) {
	principalA := hashPulseBetaKey("alpha")
	principalB := hashPulseBetaKey("beta")
	store := &mockWatchlistSummaryStore{
		watchlists: map[string][]PulseWatchlistEntry{
			principalA: {{Login: "chan_a", AlwaysTrack: true}},
			principalB: {{Login: "chan_b", AlwaysTrack: false}},
		},
	}
	h := &Handler{}
	summaryA, err := h.buildPulseWatchlistSummary(context.Background(), store, principalA, "", watchlistSummaryContext{})
	if err != nil {
		t.Fatalf("build A: %v", err)
	}
	summaryB, err := h.buildPulseWatchlistSummary(context.Background(), store, principalB, "", watchlistSummaryContext{})
	if err != nil {
		t.Fatalf("build B: %v", err)
	}
	if summaryA.ProtectedCount != 1 || summaryB.ProtectedCount != 0 {
		t.Fatalf("protected counts = %d / %d", summaryA.ProtectedCount, summaryB.ProtectedCount)
	}
}

func TestWatchlistSummaryWatchlistCap(t *testing.T) {
	principalID := "principal-cap"
	items := make([]PulseWatchlistEntry, 0, 12)
	for i := 0; i < 12; i++ {
		items = append(items, PulseWatchlistEntry{Login: "chan" + strconvItoaTest(i)})
	}
	store := &mockWatchlistSummaryStore{watchlists: map[string][]PulseWatchlistEntry{principalID: items}}
	h := &Handler{}
	summary, err := h.buildPulseWatchlistSummary(context.Background(), store, principalID, "", watchlistSummaryContext{})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if len(store.lastBatchLogins) > pulseSummaryWatchlistCap {
		t.Fatalf("batch logins = %d, want <= %d", len(store.lastBatchLogins), pulseSummaryWatchlistCap)
	}
	_ = summary
}

func strconvItoaTest(i int) string {
	const digits = "0123456789"
	if i == 0 {
		return "0"
	}
	var b [20]byte
	pos := len(b)
	for i > 0 {
		pos--
		b[pos] = digits[i%10]
		i /= 10
	}
	return string(b[pos:])
}

func TestWatchlistSummaryCappedArrays(t *testing.T) {
	now := time.Now().UTC()
	ended := now.Add(-time.Hour)
	items := make([]PulseWatchlistEntry, pulseSummaryWatchlistCap)
	streams := make(map[string]*StreamRecord, pulseSummaryWatchlistCap)
	for i := 0; i < pulseSummaryWatchlistCap; i++ {
		login := "live" + strconvItoaTest(i)
		items[i] = PulseWatchlistEntry{Login: login}
		streams[login] = &StreamRecord{
			StreamID:       "s-" + login,
			Login:          login,
			DisplayName:    login,
			StartedAt:      now.Add(-30 * time.Minute),
			LastSeenAt:     now,
			CurrentViewers: 100 + i,
		}
	}
	for i := 0; i < pulseSummaryArrayCap+2; i++ {
		login := "ended" + strconvItoaTest(i)
		items = append(items, PulseWatchlistEntry{Login: login})
		streams[login] = &StreamRecord{
			StreamID:       "e-" + login,
			Login:          login,
			StartedAt:      now.Add(-2 * time.Hour),
			EndedAt:        &ended,
			LastSeenAt:     ended,
			ChatMessages:   10,
			ViewerSamples:  5,
		}
	}
	bookmarks := make([]PulseBookmark, pulseSummaryArrayCap+2)
	for i := range bookmarks {
		id := "bm-" + strconvItoaTest(i)
		bookmarks[i] = PulseBookmark{ID: id, Login: "chan", Label: "moment " + id}
	}
	store := &mockWatchlistSummaryStore{
		watchlists: map[string][]PulseWatchlistEntry{"p1": items},
		streams:    streams,
		bookmarks:  map[string][]PulseBookmark{"p1": bookmarks},
	}
	h := &Handler{}
	summary, err := h.buildPulseWatchlistSummary(context.Background(), store, "p1", "", watchlistSummaryContext{})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if len(summary.LiveNow) > pulseSummaryArrayCap {
		t.Fatalf("liveNow len = %d, cap = %d", len(summary.LiveNow), pulseSummaryArrayCap)
	}
	if len(summary.Recaps) > pulseSummaryArrayCap {
		t.Fatalf("recaps len = %d, cap = %d", len(summary.Recaps), pulseSummaryArrayCap)
	}
	if len(summary.Moments) > pulseSummaryArrayCap {
		t.Fatalf("moments len = %d, cap = %d", len(summary.Moments), pulseSummaryArrayCap)
	}
	if len(summary.Attention) > pulseSummaryArrayCap {
		t.Fatalf("attention len = %d, cap = %d", len(summary.Attention), pulseSummaryArrayCap)
	}
}

func TestWatchlistSummaryRateLimit429(t *testing.T) {
	h := &Handler{
		pulseHosted: PulseHostedConfig{Hosted: true, BetaKeys: []string{"secret-one"}},
		store:       &Store{},
	}
	h.rateLimiter = &PulseRateLimiter{summaryPerMin: 1}
	h.rateLimiter.testAllowFn = func(_ string, limit int) int64 { return int64(limit + 1) }
	r := hostedSummaryRouter(h)

	req := httptest.NewRequest(http.MethodGet, "/v1/pulse/watchlist/summary", nil)
	req.Header.Set("X-Streamclone-Beta-Key", "secret-one")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want 429", rec.Code)
	}
	var body map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["scope"] != "summary" {
		t.Fatalf("scope = %v, want summary", body["scope"])
	}
}

func TestWatchlistSummaryCacheFailOpen(t *testing.T) {
	h := &Handler{
		rdb: redis.NewClient(&redis.Options{
			Addr:        "127.0.0.1:1",
			DialTimeout: 50 * time.Millisecond,
		}),
	}
	payload := PulseWatchlistSummary{
		LiveCount: 0,
		LiveNow:   []PulseWatchlistSummaryLiveNow{},
		Recaps:    []PulseWatchlistSummaryRecap{},
		Moments:   []PulseWatchlistSummaryMoment{},
		Attention: []PulseWatchlistSummaryAttention{},
	}
	key := pulseSummaryCacheKey("principal-1", "")
	h.savePulseWatchlistSummaryCache(context.Background(), key, payload)
	if _, hit := h.loadPulseWatchlistSummaryCache(context.Background(), key); hit {
		t.Fatal("cache get failure must miss, not fail request path")
	}
}

func TestWatchlistSummaryForbiddenKeys(t *testing.T) {
	principalID := hashPulseBetaKey("secret-one")
	now := time.Now().UTC()
	store := &mockWatchlistSummaryStore{
		watchlists: map[string][]PulseWatchlistEntry{
			principalID: {{Login: "xqc", AlwaysTrack: true}},
		},
		streams: map[string]*StreamRecord{
			"xqc": {
				StreamID: "1", Login: "xqc", DisplayName: "xQc",
				StartedAt: now, LastSeenAt: now, CurrentViewers: 500,
			},
		},
	}
	h := &Handler{collector: NewCollector(nil, fakeProvider{}, &fakeJoiner{}, nil, nilLogger(), 5, time.Second, time.Hour, 10)}
	summary, err := h.buildPulseWatchlistSummary(context.Background(), store, principalID, "xqc", h.watchlistSummaryContext())
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	raw, err := json.Marshal(summary)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := assertNoForbiddenSummaryKeys(t, raw); err != nil {
		t.Fatal(err)
	}
}

func TestWatchlistSummaryLoginProtectedHonesty(t *testing.T) {
	principalID := "p1"
	store := &mockWatchlistSummaryStore{
		watchlists: map[string][]PulseWatchlistEntry{
			principalID: {{Login: "tracked", AlwaysTrack: false}},
		},
	}
	h := &Handler{}
	summary, err := h.buildPulseWatchlistSummary(context.Background(), store, principalID, "otherlogin", watchlistSummaryContext{})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if summary.CurrentChannel == nil {
		t.Fatal("expected currentChannel")
	}
	if summary.CurrentChannel.Protected {
		t.Fatal("login not on watchlist must not be protected")
	}
	if summary.CurrentChannel.Login != "otherlogin" {
		t.Fatalf("login = %q, want otherlogin", summary.CurrentChannel.Login)
	}
}

func TestWatchlistSummaryCoreFailure503(t *testing.T) {
	h := &Handler{
		pulseHosted: PulseHostedConfig{Hosted: true, BetaKeys: []string{"secret-one"}},
	}
	r := hostedSummaryRouter(h)
	req := httptest.NewRequest(http.MethodGet, "/v1/pulse/watchlist/summary", nil)
	req.Header.Set("X-Streamclone-Beta-Key", "secret-one")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503 when store list fails", rec.Code)
	}
	var body map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["error"] != "summary_unavailable" {
		t.Fatalf("error = %q", body["error"])
	}
	if strings.Contains(rec.Body.String(), "pgx") || strings.Contains(rec.Body.String(), "sql") {
		t.Fatal("response leaked internal error text")
	}
}

func TestWatchlistSummaryShapeKeys(t *testing.T) {
	principalID := hashPulseBetaKey("secret-one")
	h := &Handler{}
	summary, err := h.buildPulseWatchlistSummary(context.Background(), &mockWatchlistSummaryStore{
		watchlists: map[string][]PulseWatchlistEntry{principalID: {}},
	}, principalID, "", watchlistSummaryContext{})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	raw, err := json.Marshal(summary)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	for _, key := range []string{"liveCount", "recapReadyCount", "attentionCount", "protectedCount", "liveNow", "recaps", "moments", "attention"} {
		if _, ok := obj[key]; !ok {
			t.Fatalf("missing top-level key %q", key)
		}
	}
}

func TestWatchlistSummaryListFailure503(t *testing.T) {
	h := &Handler{
		pulseHosted: PulseHostedConfig{Hosted: true, BetaKeys: []string{"secret-one"}},
	}
	// Use mock via replacing store is not possible on Handler; test build path directly.
	_, err := h.buildPulseWatchlistSummary(context.Background(), &mockWatchlistSummaryStore{
		listErr: errors.New("db down"),
	}, "p1", "", watchlistSummaryContext{})
	if !errors.Is(err, errSummaryUnavailable) {
		t.Fatalf("err = %v, want summary_unavailable", err)
	}
}

func TestCapWatchlistEntries(t *testing.T) {
	items := make([]PulseWatchlistEntry, 15)
	for i := range items {
		items[i] = PulseWatchlistEntry{Login: "c" + strconvItoaTest(i)}
	}
	capped := capWatchlistEntries(items, pulseSummaryWatchlistCap)
	if len(capped) != pulseSummaryWatchlistCap {
		t.Fatalf("len = %d, want %d", len(capped), pulseSummaryWatchlistCap)
	}
}

var forbiddenSummaryKeySubstrings = []string{
	"messages", "rollups", "operator", "stack", "sql", "err", "password", "token",
	"chatMessages", "rawChat", "internal", "hostname",
}

func assertNoForbiddenSummaryKeys(t *testing.T, raw []byte) error {
	t.Helper()
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return err
	}
	return walkForbiddenKeys(t, v, "")
}

func walkForbiddenKeys(t *testing.T, v any, path string) error {
	t.Helper()
	switch node := v.(type) {
	case map[string]any:
		for k, child := range node {
			full := k
			if path != "" {
				full = path + "." + k
			}
			lower := strings.ToLower(k)
			for _, forbidden := range forbiddenSummaryKeySubstrings {
				if strings.Contains(lower, strings.ToLower(forbidden)) {
					t.Fatalf("forbidden key %q at path %q", k, full)
				}
			}
			if err := walkForbiddenKeys(t, child, full); err != nil {
				return err
			}
		}
	case []any:
		for _, child := range node {
			if err := walkForbiddenKeys(t, child, path+"[]"); err != nil {
				return err
			}
		}
	}
	return nil
}

func TestPulseSummaryCacheKey(t *testing.T) {
	key := pulseSummaryCacheKey("principal-1", "xqc")
	if !strings.HasPrefix(key, pulseSummaryCacheKeyPrefix) {
		t.Fatalf("key = %q", key)
	}
	if !strings.HasSuffix(key, ":xqc") {
		t.Fatalf("login suffix missing: %q", key)
	}
}

func TestAllowSummaryRateLimitFailOpen(t *testing.T) {
	l := &PulseRateLimiter{rdb: redis.NewClient(&redis.Options{Addr: "127.0.0.1:1", DialTimeout: 50 * time.Millisecond})}
	allowed, _ := l.AllowSummary(context.Background(), "p1")
	if !allowed {
		t.Fatal("redis failure must fail-open and allow request")
	}
}
