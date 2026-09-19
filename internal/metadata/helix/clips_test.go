package helix

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"streamclone/internal/metadata/model"
)

func TestClipsPreservesVODOffsets(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"id":"a","video_id":"123","vod_offset":0},{"id":"b","video_id":"123","vod_offset":125.5},{"id":"c","vod_offset":null}]}`))
	}))
	defer server.Close()
	client := New(server.URL, server.URL, "test", "test", "test")
	client.token = "test"
	client.expiresAt = time.Now().Add(time.Hour)
	result, err := client.Clips(context.Background(), "channel", model.ClipQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Items) != 3 {
		t.Fatalf("items: %v", result.Items)
	}
	if result.Items[0].VideoID != "123" || result.Items[0].VODOffsetSeconds == nil || *result.Items[0].VODOffsetSeconds != 0 {
		t.Fatal("zero offset lost")
	}
	if result.Items[1].VODOffsetSeconds == nil || *result.Items[1].VODOffsetSeconds != 125.5 {
		t.Fatal("offset lost")
	}
	if result.Items[2].VODOffsetSeconds != nil {
		t.Fatal("unknown offset became zero")
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	var wire struct {
		Items []map[string]any `json:"items"`
	}
	if err := json.Unmarshal(encoded, &wire); err != nil {
		t.Fatal(err)
	}
	if wire.Items[0]["vodOffsetSeconds"] != float64(0) || wire.Items[0]["videoId"] != "123" {
		t.Fatal("zero offset or VOD identity missing from response")
	}
	if _, exists := wire.Items[2]["vodOffsetSeconds"]; exists {
		t.Fatal("unknown offset must be omitted from response")
	}
}
