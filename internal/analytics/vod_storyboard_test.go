package analytics

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
)

func TestResolveStoryboardThumb(t *testing.T) {
	meta, ok := normalizeStoryboardRow(struct {
		URL      string `json:"url"`
		Width    int    `json:"width"`
		Height   int    `json:"height"`
		Count    int    `json:"count"`
		Duration int    `json:"duration"`
	}{
		URL:      "https://example.test/storyboards/{index}.jpg",
		Width:    1600,
		Height:   900,
		Count:    10,
		Duration: 30_000,
	})
	if !ok {
		t.Fatal("expected meta")
	}
	thumb, reason := resolveStoryboardThumb(meta, 120)
	if reason != "" {
		t.Fatalf("unexpected reason %q", reason)
	}
	if thumb == nil {
		t.Fatal("expected thumb")
	}
	if thumb.SheetURL != "https://example.test/storyboards/0.jpg" {
		t.Fatalf("sheet url=%q", thumb.SheetURL)
	}
	if thumb.CropW != 160 || thumb.CropH != 90 {
		t.Fatalf("crop=%dx%d", thumb.CropW, thumb.CropH)
	}
}

func TestResolveStoryboardThumbOffsetOutOfRange(t *testing.T) {
	meta, ok := normalizeStoryboardRow(struct {
		URL      string `json:"url"`
		Width    int    `json:"width"`
		Height   int    `json:"height"`
		Count    int    `json:"count"`
		Duration int    `json:"duration"`
	}{
		URL:      "https://example.test/{index}.jpg",
		Width:    160,
		Height:   90,
		Count:    1,
		Duration: 30_000,
	})
	if !ok {
		t.Fatal("expected meta")
	}
	thumb, reason := resolveStoryboardThumb(meta, 999_999)
	if thumb != nil || reason != "offset_out_of_range" {
		t.Fatalf("thumb=%v reason=%q", thumb, reason)
	}
}

func TestFetchVodStoryboardMeta(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode([]map[string]any{
			{
				"data": map[string]any{
					"video": map[string]any{
						"storyboards": []map[string]any{
							{
								"url":      "https://example.test/storyboards/{index}.jpg",
								"width":    1600,
								"height":   900,
								"count":    2,
								"duration": 30_000,
							},
						},
					},
				},
			},
		})
	}))
	defer srv.Close()

	svc := &SyncService{
		twitchGQLURL:   srv.URL,
		twitchClientID: "test-client",
		client:         &http.Client{},
	}
	meta, err := svc.fetchVodStoryboardMeta(context.Background(), "1234567890")
	if err != nil {
		t.Fatalf("fetchVodStoryboardMeta: %v", err)
	}
	if meta.SheetCount != 2 || meta.URLTemplate == "" {
		t.Fatalf("meta=%+v", meta)
	}
}

func TestVodStoryboardThumbHandler(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode([]map[string]any{
			{
				"data": map[string]any{
					"video": map[string]any{
						"storyboards": []map[string]any{
							{
								"url":      "https://example.test/storyboards/{index}.jpg",
								"width":    1600,
								"height":   900,
								"count":    2,
								"duration": 30_000,
							},
						},
					},
				},
			},
		})
	}))
	defer srv.Close()

	h := NewHandler(nil, nil, nil, &SyncService{
		twitchGQLURL:   srv.URL,
		twitchClientID: "test-client",
		client:         &http.Client{},
	})
	req := httptest.NewRequest(http.MethodGet, "/v1/analytics/vods/1234567890/storyboard-thumb?offsetSec=60", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("vodId", "1234567890")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	rec := httptest.NewRecorder()
	h.vodStoryboardThumb(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var thumb storyboardThumbResponse
	if err := json.NewDecoder(rec.Body).Decode(&thumb); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if thumb.SheetURL == "" || thumb.CropW != 160 {
		t.Fatalf("thumb=%+v", thumb)
	}
}
