package store

import (
	"encoding/json"
	"net/url"
	"testing"
)

func TestMergeSocialMetricsPreservesThumbnailFields(t *testing.T) {
	existing := json.RawMessage(`{
		"score": 42,
		"thumbnail_url": "https://static-cdn.jtvnw.net/thumb.jpg",
		"thumbnail_source": "helix",
		"thumbnail_status": "ready"
	}`)
	incoming := json.RawMessage(`{"score": 50, "comments": 3}`)
	got := MergeSocialMetrics(existing, incoming)
	var metrics map[string]any
	if err := json.Unmarshal(got, &metrics); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if metrics["score"] != float64(50) {
		t.Fatalf("score = %v, want 50", metrics["score"])
	}
	if metrics["comments"] != float64(3) {
		t.Fatalf("comments = %v, want 3", metrics["comments"])
	}
	if metrics["thumbnail_url"] != "https://static-cdn.jtvnw.net/thumb.jpg" {
		t.Fatalf("thumbnail_url = %v", metrics["thumbnail_url"])
	}
	if metrics["thumbnail_source"] != "helix" {
		t.Fatalf("thumbnail_source = %v", metrics["thumbnail_source"])
	}
	if metrics["thumbnail_status"] != "ready" {
		t.Fatalf("thumbnail_status = %v", metrics["thumbnail_status"])
	}
}

func TestMergeSocialMetricsIncomingThumbWins(t *testing.T) {
	existing := json.RawMessage(`{
		"thumbnail_url": "https://clips-media-assets2.twitch.tv/old-preview.jpg",
		"thumbnail_source": "synthetic",
		"thumbnail_status": "pending"
	}`)
	incoming := json.RawMessage(`{
		"thumbnail_url": "https://static-cdn.jtvnw.net/new.jpg",
		"thumbnail_source": "helix",
		"thumbnail_status": "ready"
	}`)
	got := MergeSocialMetrics(existing, incoming)
	var metrics map[string]any
	if err := json.Unmarshal(got, &metrics); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if metrics["thumbnail_url"] != "https://static-cdn.jtvnw.net/new.jpg" {
		t.Fatalf("thumbnail_url = %v", metrics["thumbnail_url"])
	}
	if metrics["thumbnail_source"] != "helix" {
		t.Fatalf("thumbnail_source = %v", metrics["thumbnail_source"])
	}
}

func TestDisplayThumbnailURLPrefersHelixMetrics(t *testing.T) {
	evidence := "https://clips-media-assets2.twitch.tv/FurrySpinelessDogCoolCat-preview-480x272.jpg"
	helix := "https://static-cdn.jtvnw.net/twitch-video-assets/thumb-480x272.jpg"
	got := displayThumbnailURL(evidence, helix)
	want := "/v1/pulse-wire/thumb?u=" + url.QueryEscape(helix)
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}
