package analytics

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
)

func TestClassifyCorpusGoldGap(t *testing.T) {
	expired := time.Now().UTC().Add(-time.Minute)
	if got := classifyCorpusGoldGap("failed", 0, "", nil); got != CorpusGapKindFailed {
		t.Fatalf("failed = %q", got)
	}
	if got := classifyCorpusGoldGap("running", 0, "", &expired); got != CorpusGapKindStaleRunning {
		t.Fatalf("stale running = %q", got)
	}
	if got := classifyCorpusGoldGap("done", 0, "", nil); got != CorpusGapKindKnownEmpty {
		t.Fatalf("known empty = %q", got)
	}
}

func TestCorpusGoldGapsStoreRequeue(t *testing.T) {
	ctx, store := setupSessionStore(t)
	applyGoldVODSegmentMigration(t, ctx, store)

	plans := PlanGoldVODSegments("vod-gap", "stream-gap", "xqc", 600, 600, "")
	if _, err := store.UpsertGoldVODSegmentPlans(ctx, plans, nil, 2); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	claim, err := store.ClaimGoldVODSegment(ctx, "worker-a", time.Minute, 2)
	if err != nil || claim == nil {
		t.Fatalf("claim: %+v err=%v", claim, err)
	}
	if _, err := store.FailGoldVODSegment(ctx, claim.ID, "worker-a", "boom", time.Minute); err != nil {
		t.Fatalf("fail: %v", err)
	}

	n, err := store.RequeueCorpusGoldSegments(ctx, []string{plans[0].SegmentKey})
	if err != nil {
		t.Fatalf("requeue: %v", err)
	}
	if n != 1 {
		t.Fatalf("requeued = %d, want 1", n)
	}
	gaps, err := store.ListCorpusGoldGaps(ctx, 10, "vod-gap", "")
	if err != nil {
		t.Fatalf("list gaps: %v", err)
	}
	if len(gaps) != 1 || gaps[0].Status != "queued" {
		t.Fatalf("gaps after requeue = %+v", gaps)
	}
}

func TestCorpusGoldGapsHTTPRoutes(t *testing.T) {
	ctx, store := setupSessionStore(t)
	applyGoldVODSegmentMigration(t, ctx, store)
	h := NewHandler(store, nil, nil, nil)

	plans := PlanGoldVODSegments("vod-http", "stream-http", "xqc", 600, 600, "")
	if _, err := store.UpsertGoldVODSegmentPlans(ctx, plans, nil, 1); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	claim, err := store.ClaimGoldVODSegment(ctx, "worker-http", time.Minute, 1)
	if err != nil || claim == nil {
		t.Fatalf("claim: %v", err)
	}
	if _, err := store.FailGoldVODSegment(ctx, claim.ID, "worker-http", "err", time.Minute); err != nil {
		t.Fatalf("fail: %v", err)
	}

	r := chiRouterWithCorpus(h)
	req := httptest.NewRequest(http.MethodGet, "/v1/internal/corpus/gaps?vod_id=vod-http", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("GET gaps status=%d body=%s", rr.Code, rr.Body.String())
	}
	var list corpusGapsListResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &list); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if list.Count != 1 {
		t.Fatalf("count=%d body=%s", list.Count, rr.Body.String())
	}

	body, _ := json.Marshal(corpusGapsRequeueRequest{SegmentKeys: []string{plans[0].SegmentKey}})
	req = httptest.NewRequest(http.MethodPost, "/v1/internal/corpus/gaps/requeue", bytes.NewReader(body))
	rr = httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("POST requeue status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func chiRouterWithCorpus(h *Handler) http.Handler {
	r := chi.NewRouter()
	h.CorpusRoutes(r)
	return r
}
