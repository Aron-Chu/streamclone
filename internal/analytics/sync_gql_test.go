package analytics

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestGQLCommentTextUsesFragmentsWhenBodyEmpty(t *testing.T) {
	msg := struct {
		Body      string `json:"body"`
		Fragments []struct {
			Text string `json:"text"`
		} `json:"fragments"`
	}{
		Fragments: []struct {
			Text string `json:"text"`
		}{
			{Text: "WW "},
			{Text: "Pog"},
		},
	}
	got := gqlCommentText(msg)
	if got != "WW Pog" {
		t.Fatalf("expected fragment join, got %q", got)
	}
}

func TestGQLCommentTextPrefersBody(t *testing.T) {
	msg := struct {
		Body      string `json:"body"`
		Fragments []struct {
			Text string `json:"text"`
		} `json:"fragments"`
	}{
		Body: "hello",
		Fragments: []struct {
			Text string `json:"text"`
		}{
			{Text: "ignored"},
		},
	}
	if got := gqlCommentText(msg); got != "hello" {
		t.Fatalf("expected body, got %q", got)
	}
}

func TestBuildVideoCommentsGQLRequestUsesCursor(t *testing.T) {
	req := buildVideoCommentsGQLRequest("123", "hash", true, 99, "cursor-abc")
	if req.Variables.Cursor == nil || *req.Variables.Cursor != "cursor-abc" {
		t.Fatalf("expected cursor cursor-abc, got %+v", req.Variables.Cursor)
	}
	if req.Variables.ContentOffsetSeconds != nil {
		t.Fatalf("cursor mode should not set offset")
	}
}

func TestBuildVideoCommentsGQLRequestUsesOffset(t *testing.T) {
	req := buildVideoCommentsGQLRequest("123", "hash", false, 42, "")
	if req.Variables.ContentOffsetSeconds == nil || *req.Variables.ContentOffsetSeconds != 42 {
		t.Fatalf("expected offset 42, got %+v", req.Variables.ContentOffsetSeconds)
	}
	if req.Variables.Cursor != nil {
		t.Fatalf("offset mode should not set cursor")
	}
}

func TestIsGQLIntegrityError(t *testing.T) {
	resp := GQLResponse{
		Errors: []struct {
			Message string `json:"message"`
		}{
			{Message: "failed integrity check"},
		},
	}
	if !isGQLIntegrityError(resp) {
		t.Fatal("expected integrity error")
	}
	resp.Errors[0].Message = "not found"
	if isGQLIntegrityError(resp) {
		t.Fatal("expected non-integrity error")
	}
}

func TestIsGQLIntegrityErrorCaseInsensitive(t *testing.T) {
	resp := GQLResponse{
		Errors: []struct {
			Message string `json:"message"`
		}{
			{Message: strings.ToUpper("failed integrity check")},
		},
	}
	if !isGQLIntegrityError(resp) {
		t.Fatal("expected case-insensitive integrity match")
	}
}

func TestParseRetryAfterSeconds(t *testing.T) {
	h := http.Header{}
	h.Set("Retry-After", "2")
	if got := parseRetryAfter(h); got != 2*time.Second {
		t.Fatalf("expected 2s, got %v", got)
	}
}

func TestParseRetryAfterMissing(t *testing.T) {
	if got := parseRetryAfter(http.Header{}); got != 0 {
		t.Fatalf("expected zero delay, got %v", got)
	}
}

func TestGQLBackoffDelayUsesRetryAfter(t *testing.T) {
	delay := gqlBackoffDelay(3, 1500*time.Millisecond)
	if delay != 1500*time.Millisecond {
		t.Fatalf("expected retry-after to win, got %v", delay)
	}
}

func TestPostGQLVideoCommentsRetries429(t *testing.T) {
	var attempts int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts == 1 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]GQLResponse{{
			Data: struct {
				Video *struct {
					Comments *struct {
						Edges    []GQLCommentEdge `json:"edges"`
						PageInfo struct {
							HasNextPage bool `json:"hasNextPage"`
						} `json:"pageInfo"`
					} `json:"comments"`
				} `json:"video"`
			}{
				Video: &struct {
					Comments *struct {
						Edges    []GQLCommentEdge `json:"edges"`
						PageInfo struct {
							HasNextPage bool `json:"hasNextPage"`
						} `json:"pageInfo"`
					} `json:"comments"`
				}{
					Comments: &struct {
						Edges    []GQLCommentEdge `json:"edges"`
						PageInfo struct {
							HasNextPage bool `json:"hasNextPage"`
						} `json:"pageInfo"`
					}{},
				},
			},
		}})
	}))
	defer srv.Close()

	svc := &SyncService{
		twitchGQLURL:   srv.URL,
		twitchClientID: "test-client",
		gqlClient:      srv.Client(),
		log:            slog.Default(),
	}
	req := buildVideoCommentsGQLRequest("123", "hash", false, 0, "")
	resp, err := svc.postGQLVideoComments(context.Background(), req, nil, nil)
	if err != nil {
		t.Fatalf("postGQLVideoComments: %v", err)
	}
	if resp.Data.Video == nil {
		t.Fatal("expected video payload")
	}
	if attempts != 2 {
		t.Fatalf("expected 2 attempts, got %d", attempts)
	}
}

func TestPostGQLVideoCommentsRejectsNonRetryableStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("bad request"))
	}))
	defer srv.Close()

	svc := &SyncService{
		twitchGQLURL:   srv.URL,
		twitchClientID: "test-client",
		gqlClient:      srv.Client(),
		log:            slog.Default(),
	}
	_, err := svc.postGQLVideoComments(context.Background(), buildVideoCommentsGQLRequest("123", "hash", false, 0, ""), nil, nil)
	if err == nil || !strings.Contains(err.Error(), "400") {
		t.Fatalf("expected status 400 error, got %v", err)
	}
}
