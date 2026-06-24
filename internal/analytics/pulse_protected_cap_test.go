package analytics

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestProtectedCapReachedResponseShape(t *testing.T) {
	rec := httptest.NewRecorder()
	writeProtectedCapReached(rec)
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409", rec.Code)
	}
	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["error"] != "protected_cap_reached" || body["scope"] != "protected_pool" {
		t.Fatalf("body = %#v", body)
	}
}

func TestCheckProtectedCapAllowsExistingLogin(t *testing.T) {
	if err := evaluateProtectedCap(1, 1, true); err != nil {
		t.Fatalf("existing protected login should pass: %v", err)
	}
}

func TestCheckProtectedCapRejectsAtLimit(t *testing.T) {
	err := evaluateProtectedCap(1, 1, false)
	if err == nil {
		t.Fatal("expected cap error at limit")
	}
	if !isProtectedCapError(err) {
		t.Fatalf("expected protected cap error, got %v", err)
	}
}

func TestCheckProtectedCapRejectsZeroLimit(t *testing.T) {
	err := evaluateProtectedCap(0, 0, false)
	if err == nil {
		t.Fatal("expected cap error when limit is 0")
	}
}

func TestCheckProtectedCapAllowsUnprotect(t *testing.T) {
	if err := evaluateProtectedCap(5, 5, true); err != nil {
		t.Fatalf("update existing should pass: %v", err)
	}
}

func TestIsProtectedCapError(t *testing.T) {
	if !isProtectedCapError(&protectedCapError{scope: "protected_pool"}) {
		t.Fatal("expected protected cap error")
	}
	if isProtectedCapError(nil) {
		t.Fatal("nil should not match")
	}
}
