package ingest

import (
	"encoding/json"
	"testing"

	"streamclone/internal/storygraph/store"
)

func TestOriginMatchConfidenceUsesQuotesAndHeatmapConfidence(t *testing.T) {
	quotes, _ := json.Marshal([]string{"streamer explains the contract leak on stream"})
	originConfidence := 0.9
	fp := store.MomentFingerprint{
		TranscriptKW:     quotes,
		ChatSpikeSummary: "chat spike around contract leak",
		OriginConfidence: &originConfidence,
	}

	got := originMatchConfidence("Streamer explains the contract leak on stream", fp)
	if got < 0.55 {
		t.Fatalf("expected linkable origin confidence, got %.2f", got)
	}
}

func TestOriginMatchConfidenceRejectsUnrelatedQuotes(t *testing.T) {
	quotes, _ := json.Marshal([]string{"chat laughs at a gameplay fail"})
	fp := store.MomentFingerprint{
		TranscriptKW:     quotes,
		ChatSpikeSummary: "funny fall guys round",
	}

	got := originMatchConfidence("Creator lawsuit response spreads", fp)
	if got >= 0.55 {
		t.Fatalf("expected unrelated origin to stay below link threshold, got %.2f", got)
	}
}
