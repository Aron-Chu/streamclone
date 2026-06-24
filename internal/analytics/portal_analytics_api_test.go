package analytics

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestPortalStreamDetailOmitsRollups(t *testing.T) {
	// Shape-only test — no DB required (handler integration covered in CI with postgres).
	detail := PortalStreamDetail{
		Channel: "xqc",
		State:   "historical",
		Sources: []SourceStatus{{Source: "analytics_db", State: "ready"}},
	}
	body, err := json.Marshal(detail)
	if err != nil {
		t.Fatal(err)
	}
	raw := strings.ToLower(string(body))
	for _, forbidden := range []string{"rollups", "messages", "operator", "gql", "corpus", "archive"} {
		if strings.Contains(raw, forbidden) {
			t.Fatalf("portal detail must not contain %q", forbidden)
		}
	}
}

func TestPortalAnalyticsJSONForbiddenKeys(t *testing.T) {
	detail := PortalStreamDetail{
		Channel: "xqc",
		State:   "historical",
		Sources: []SourceStatus{{Source: "analytics_db", State: "ready"}},
	}
	body, err := json.Marshal(detail)
	if err != nil {
		t.Fatal(err)
	}
	raw := strings.ToLower(string(body))
	for _, forbidden := range []string{"rollups", "messages", "operator", "gql", "corpus", "archive"} {
		if strings.Contains(raw, forbidden) {
			t.Fatalf("portal detail must not contain %q", forbidden)
		}
	}
}

func TestPortalSyncStatusSanitizedShape(t *testing.T) {
	st := PortalSyncStatus{Phase: "completed", Message: "Done"}
	body, err := json.Marshal(st)
	if err != nil {
		t.Fatal(err)
	}
	raw := string(body)
	for _, forbidden := range []string{"network", "tracker", "commentsFetched"} {
		if strings.Contains(raw, forbidden) {
			t.Fatalf("sanitized sync status leaked %q", forbidden)
		}
	}
}

func TestAllowPortalSummaryRateLimitFailOpen(t *testing.T) {
	rl := NewPulseRateLimiter(nil, 10, 5)
	ok, _ := rl.AllowPortalSummary(t.Context(), "principal-a")
	if !ok {
		t.Fatal("expected fail-open when redis nil")
	}
}
