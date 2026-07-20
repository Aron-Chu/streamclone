package api

import "testing"

func TestEtagMatchesExactTokens(t *testing.T) {
	if !etagMatches(`"abc123"`, "abc123") {
		t.Fatal("quoted etag should match")
	}
	if !etagMatches(`W/"abc123"`, "abc123") {
		t.Fatal("weak etag should match bare value")
	}
	if !etagMatches(`"other", "abc123"`, "abc123") {
		t.Fatal("list member should match")
	}
	if etagMatches(`"abc12"`, "abc123") {
		t.Fatal("substring must not match")
	}
	if etagMatches(`"abc1234"`, "abc123") {
		t.Fatal("prefix/suffix must not match")
	}
	if etagMatches("", "abc123") {
		t.Fatal("empty If-None-Match must not match")
	}
	if !etagMatches("*", "anything") {
		t.Fatal("* should match any etag")
	}
}
