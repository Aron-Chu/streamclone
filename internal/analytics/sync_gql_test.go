package analytics

import (
	"strings"
	"testing"
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
