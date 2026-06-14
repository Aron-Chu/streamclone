package analytics

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"pgregory.net/rapid"
)

// Feature: moment-timeline, Property 24: Window Parameter Validation
// Validates: Requirements 8.4

func TestPropWindowParamValidation_InvalidReturns400(t *testing.T) {
	handler := &Handler{}

	r := chi.NewRouter()
	r.Get("/v1/analytics/streams/{streamID}/replay-heatmap", handler.replayHeatmap)

	rapid.Check(t, func(rt *rapid.T) {
		windowStr := rapid.OneOf(
			// Non-integer strings
			rapid.Map(rapid.StringMatching(`[a-zA-Z]+`), func(s string) string { return s }),
			// Float-like strings
			rapid.Map(rapid.Float64Range(-1000, 1000), func(f float64) string { return fmt.Sprintf("%.2f", f) }),
			// Integers below the minimum (< 10)
			rapid.Map(rapid.IntRange(-10000, 9), func(n int) string { return fmt.Sprintf("%d", n) }),
			// Integers above the maximum (> 600)
			rapid.Map(rapid.IntRange(601, 100000), func(n int) string { return fmt.Sprintf("%d", n) }),
			// Empty string after trim or whitespace-only won't trigger (empty is valid = default)
			// Mixed alphanumeric
			rapid.Map(rapid.StringMatching(`\d+[a-z]+`), func(s string) string { return s }),
		).Draw(rt, "invalidWindow")

		req := httptest.NewRequest("GET",
			"/v1/analytics/streams/test-stream-123/replay-heatmap?window="+windowStr, nil)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		if rec.Code != http.StatusBadRequest {
			rt.Fatalf("expected 400 for invalid window %q, got %d", windowStr, rec.Code)
		}
	})
}

func TestPropWindowParamValidation_ValidNotReturns400(t *testing.T) {
	handler := &Handler{}

	r := chi.NewRouter()
	r.Get("/v1/analytics/streams/{streamID}/replay-heatmap", handler.replayHeatmap)

	rapid.Check(t, func(rt *rapid.T) {
		window := rapid.IntRange(10, 600).Draw(rt, "validWindow")
		windowStr := fmt.Sprintf("%d", window)

		req := httptest.NewRequest("GET",
			"/v1/analytics/streams/test-stream-123/replay-heatmap?window="+windowStr, nil)
		rec := httptest.NewRecorder()

		func() {
			defer func() {
				if rv := recover(); rv != nil {
					// Panic from nil store is expected for valid windows
					// since handler proceeds past validation to DB calls.
					// The key assertion: it did NOT return 400.
				}
			}()
			r.ServeHTTP(rec, req)
		}()

		// If we didn't panic, check we didn't get 400
		if rec.Code == http.StatusBadRequest {
			rt.Fatalf("expected non-400 for valid window %d, got 400", window)
		}
	})
}

func TestPropWindowParamValidation_MissingDefaultsTo60(t *testing.T) {
	handler := &Handler{}

	r := chi.NewRouter()
	r.Get("/v1/analytics/streams/{streamID}/replay-heatmap", handler.replayHeatmap)

	// No window param → should not return 400 (defaults to 60)
	req := httptest.NewRequest("GET",
		"/v1/analytics/streams/test-stream-123/replay-heatmap", nil)
	rec := httptest.NewRecorder()

	func() {
		defer func() {
			if rv := recover(); rv != nil {
				// Panic from nil store is expected since it passes validation
			}
		}()
		r.ServeHTTP(rec, req)
	}()

	if rec.Code == http.StatusBadRequest {
		t.Fatalf("expected non-400 when window is omitted (default 60), got 400")
	}
}
