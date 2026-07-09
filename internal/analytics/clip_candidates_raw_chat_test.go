package analytics

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"pgregory.net/rapid"

	"streamclone/internal/analytics/recap"
)

// RF-P5-011 (Requirement 6.7) — Streamclone public client responses derived
// from moment or clip data must expose only aggregate chat signals
// (counts/rates/top-emotes), never raw per-message chat text or chatter
// usernames. These tests lock the serialized shape of the client-facing
// clip-candidate listing and the moment_context payload so a raw-chat field can
// never be reintroduced without tripping CI.

// rawChatFieldMarkers are case-insensitive substrings that would indicate a raw
// chat message body or chatter identity leaking into a serialized public
// payload. They are intentionally specific: legitimate aggregate/diagnostic
// keys like errorMessage, reason, or notes must not trip the guard, but any
// per-message chat text or username field must.
var rawChatFieldMarkers = []string{
	"chatmessage",
	"chat_message",
	"chatmessages",
	"chat_messages",
	"messagetext",
	"message_text",
	"messagebody",
	"message_body",
	"rawchat",
	"raw_chat",
	"chatlog",
	"chat_log",
	"chatline",
	"chat_line",
	"chattext",
	"chat_text",
	"username",
	"user_name",
	"displayname",
	"display_name",
	"chatter",
	"commenter",
	"chattername",
	"sender_login",
	"senderlogin",
	"nickname",
}

// aggregateSignalKeys is the closed allow-list for the ClipCandidate.Signals
// map — only server-computed aggregates are permitted.
var aggregateSignalKeys = map[string]struct{}{
	"chatCount":   {},
	"emoteCount":  {},
	"viewerCount": {},
	"confidence":  {},
}

// momentContextAggregateKeys is the closed allow-list of keys the outbound
// moment_context payload may carry (all aggregate signals, timing, and stream
// metadata — never raw chat). Mirrors BuildReplayForgeTriggerFromCandidate.
var momentContextAggregateKeys = map[string]struct{}{
	"candidate_id":       {},
	"stream_id":          {},
	"vod_id":             {},
	"vod_offset_seconds": {},
	"clip_start_seconds": {},
	"clip_end_seconds":   {},
	"moment_score":       {},
	"confidence":         {},
	"pick_reason":        {},
	"chat_per_min":       {},
	"emote_count":        {},
	"viewer_count":       {},
	"source_kind":        {},
	"source_status":      {},
	"minute_ts":          {},
	"category":           {},
	"top_emotes":         {},
}

// collectJSONKeys walks an already-decoded JSON value (map/slice/scalar tree)
// and records every object key it encounters. Only keys are collected — values
// (e.g. a stream title that happens to contain a username) are allowed, because
// Requirement 6.7 forbids raw-chat *fields*, not metadata that mentions names.
func collectJSONKeys(v interface{}, into map[string]struct{}) {
	switch t := v.(type) {
	case map[string]interface{}:
		for k, child := range t {
			into[k] = struct{}{}
			collectJSONKeys(child, into)
		}
	case []interface{}:
		for _, child := range t {
			collectJSONKeys(child, into)
		}
	}
}

// testingT is the minimal failing/helper surface shared by *testing.T and
// *rapid.T, so the assertion helpers work from both example and property tests.
type testingT interface {
	Helper()
	Fatalf(format string, args ...interface{})
}

// serializedKeys marshals v as it would be sent to a client and returns the set
// of all object keys in the payload.
func serializedKeys(t testingT, v interface{}) map[string]struct{} {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	var decoded interface{}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	keys := map[string]struct{}{}
	collectJSONKeys(decoded, keys)
	return keys
}

// assertNoRawChatKeys fails if any collected key matches a raw-chat marker.
func assertNoRawChatKeys(t testingT, keys map[string]struct{}, where string) {
	t.Helper()
	for key := range keys {
		lower := strings.ToLower(key)
		for _, marker := range rawChatFieldMarkers {
			if strings.Contains(lower, marker) {
				t.Fatalf("%s: serialized payload exposes raw-chat field %q (matched marker %q)", where, key, marker)
			}
		}
	}
}

// buildRawChatProbeStream builds a server-owned StreamRecord/StreamRecap pair
// whose metadata values are deliberately seeded with username- and
// message-looking strings. If the serializer ever surfaced raw chat, an
// adversarial value like this is exactly what would leak; the aggregate build
// must keep them confined to allowed metadata fields (title/category/emote
// name) and never emit them as a raw-chat field.
func buildRawChatProbeStream(login, title, category string, moments []recap.Moment) (*StreamRecord, recap.StreamRecap) {
	stream := &StreamRecord{
		StreamID:  "stream-rawchat",
		Login:     login,
		Title:     title,
		Category:  category,
		StartedAt: time.Date(2026, 7, 4, 19, 0, 0, 0, time.UTC),
		VodID:     "vod-rawchat",
	}
	rec := recap.StreamRecap{
		StreamID:        "stream-rawchat",
		Login:           login,
		DurationSeconds: len(moments)*600 + 600,
		ClipCandidates:  moments,
	}
	return stream, rec
}

// TestPropPublicClipResponsesExcludeRawChat is the Property 18 guard: for any
// server-built clip-candidate listing (including the nested job/moment_context
// tree), the serialized public payload exposes no raw chat message text or
// chatter username field, and the Signals map carries only aggregate keys.
//
// Feature: auto-clipper-replayforge-productization, Property 18: Public client
// responses exclude raw chat content.
//
// **Validates: Requirements 6.7**
func TestPropPublicClipResponsesExcludeRawChat(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		n := rapid.IntRange(1, 20).Draw(t, "momentCount")
		moments := make([]recap.Moment, n)
		for i := 0; i < n; i++ {
			emoteCount := rapid.IntRange(0, 5).Draw(t, fmt.Sprintf("emoteN%d", i))
			emotes := make([]recap.Emote, emoteCount)
			for j := 0; j < emoteCount; j++ {
				// Adversarial emote codes: raw-chat-looking values must remain
				// values, never keys.
				emotes[j] = recap.Emote{
					Code:     rapid.SampledFrom([]string{"KEKW", "user123: lol", "PogChamp", "<msg>hi</msg>"}).Draw(t, fmt.Sprintf("code%d_%d", i, j)),
					Count:    rapid.IntRange(1, 1000).Draw(t, fmt.Sprintf("ecount%d_%d", i, j)),
					Provider: rapid.SampledFrom([]string{"7tv", "bttv", "ffz", "twitch"}).Draw(t, fmt.Sprintf("prov%d_%d", i, j)),
				}
			}
			moments[i] = recap.Moment{
				OffsetSeconds: i * 600,
				Score:         rapid.IntRange(0, 100).Draw(t, fmt.Sprintf("score%d", i)),
				Confidence:    rapid.Float64Range(0, 1).Draw(t, fmt.Sprintf("conf%d", i)),
				Reasons:       []string{rapid.SampledFrom([]string{"chat_spike", "viewer_peak", "emote_spike", "manual"}).Draw(t, fmt.Sprintf("reason%d", i))},
				TopEmotes:     emotes,
				ChatCount:     rapid.IntRange(0, 100000).Draw(t, fmt.Sprintf("chat%d", i)),
				EmoteCount:    rapid.IntRange(0, 100000).Draw(t, fmt.Sprintf("emote%d", i)),
				ViewerCount:   rapid.IntRange(0, 500000).Draw(t, fmt.Sprintf("viewer%d", i)),
			}
		}

		login := rapid.SampledFrom([]string{"chan", "xqc", "user_name_streamer"}).Draw(t, "login")
		title := rapid.SampledFrom([]string{"good bit", "chatter said username: hi", "raw chat log dump"}).Draw(t, "title")
		category := rapid.SampledFrom([]string{"Just Chatting", "Games"}).Draw(t, "category")

		stream, rec := buildRawChatProbeStream(login, title, category, moments)
		candidates := BuildClipCandidatesFromRecap(stream, rec, ClipCandidateBuildOptions{MaxCandidates: maxClipCandidateLimit})
		if len(candidates) != n {
			t.Fatalf("len(candidates) = %d, want %d", len(candidates), n)
		}

		// Attach a job/moment_context subtree to at least one candidate so the
		// nested outbound contract is included in the serialized public shape.
		candidates[0].Job = &ClipCandidateJob{
			ID:               "job-1",
			CandidateID:      candidates[0].ID,
			Status:           "queued",
			ReplayForgeJobID: "rf-1",
			ReplayForgeState: "queued",
			Request:          BuildReplayForgeTriggerFromCandidate(candidates[0], ClipCandidateState{Status: ClipCandidateStatusSaved}),
			Response:         map[string]interface{}{"status": "queued", "job_id": "rf-1"},
		}

		resp := ClipCandidateListResponse{Items: candidates}
		keys := serializedKeys(t, resp)
		assertNoRawChatKeys(t, keys, "clip-candidate list response")

		// Signals must be aggregate-only for every candidate.
		for i, c := range candidates {
			for key := range c.Signals {
				if _, ok := aggregateSignalKeys[key]; !ok {
					t.Fatalf("candidate %d Signals key %q is not an aggregate signal (allowed: chatCount, emoteCount, viewerCount, confidence)", i, key)
				}
			}
		}

		// The outbound moment_context payload must likewise be aggregate-only.
		req := BuildReplayForgeTriggerFromCandidate(candidates[0], ClipCandidateState{Status: ClipCandidateStatusSaved})
		mcKeys := serializedKeys(t, req.MomentContext)
		assertNoRawChatKeys(t, mcKeys, "moment_context")
		for key := range req.MomentContext {
			if _, ok := momentContextAggregateKeys[key]; !ok {
				t.Fatalf("moment_context key %q is outside the aggregate allow-list", key)
			}
		}
	})
}

// TestClipCandidateResponseExcludesRawChatExample is an explicit example
// alongside the property: a fully-populated candidate (with a nested job and
// moment_context) serializes to a payload containing the aggregate signal keys
// and none of the known raw-chat field names.
func TestClipCandidateResponseExcludesRawChatExample(t *testing.T) {
	moments := []recap.Moment{{
		OffsetSeconds: 1200,
		Score:         88,
		Confidence:    0.77,
		Reasons:       []string{"chat_spike"},
		TopEmotes:     []recap.Emote{{Code: "KEKW", Count: 42, Provider: "7tv"}},
		ChatCount:     640,
		EmoteCount:    210,
		ViewerCount:   15300,
	}}
	stream, rec := buildRawChatProbeStream("chan", "chatter username: hello", "Just Chatting", moments)
	candidates := BuildClipCandidatesFromRecap(stream, rec, ClipCandidateBuildOptions{MaxCandidates: maxClipCandidateLimit})
	if len(candidates) != 1 {
		t.Fatalf("len(candidates) = %d, want 1", len(candidates))
	}
	candidates[0].Job = &ClipCandidateJob{
		ID:          "job-1",
		CandidateID: candidates[0].ID,
		Status:      "queued",
		Request:     BuildReplayForgeTriggerFromCandidate(candidates[0], ClipCandidateState{Status: ClipCandidateStatusSaved}),
	}

	resp := ClipCandidateListResponse{Items: candidates}
	keys := serializedKeys(t, resp)
	assertNoRawChatKeys(t, keys, "clip-candidate list response")

	// Aggregate signal keys are present and exclusively aggregate.
	for _, want := range []string{"chatCount", "emoteCount", "viewerCount"} {
		if _, ok := keys[want]; !ok {
			t.Fatalf("expected aggregate signal key %q in serialized response", want)
		}
	}
	for key := range candidates[0].Signals {
		if _, ok := aggregateSignalKeys[key]; !ok {
			t.Fatalf("Signals key %q is not an aggregate signal", key)
		}
	}
}

// TestRawChatFieldMarkersDetectLeak is a meta-test proving the guard actually
// catches a raw-chat field: a payload carrying chat message text and a chatter
// username must trip assertNoRawChatKeys. Without this, a too-lax marker set
// could let the primary tests pass vacuously.
func TestRawChatFieldMarkersDetectLeak(t *testing.T) {
	leaky := map[string]interface{}{
		"chatCount": 10,
		"chatMessages": []map[string]interface{}{
			{"username": "someviewer", "messageText": "hello world"},
		},
	}
	keys := serializedKeys(t, leaky)

	tripped := false
	for key := range keys {
		lower := strings.ToLower(key)
		for _, marker := range rawChatFieldMarkers {
			if strings.Contains(lower, marker) {
				tripped = true
			}
		}
	}
	if !tripped {
		t.Fatal("rawChatFieldMarkers failed to detect a payload with chatMessages/username/messageText fields")
	}
}
