package analytics

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"streamclone/internal/analytics/netmeter"
)

func TestListActiveSyncsReturnsRegistryJobs(t *testing.T) {
	svc := &SyncService{
		activeSyncRegistry: map[string]*activeSyncState{
			"stream-1": {channel: "sodapoppin", phase: SyncPhaseFetchingComments},
		},
	}
	svc.syncMeters.Store("stream-1", netmeter.NewMeter(func(string, int64) {}))
	v, ok := svc.syncMeters.Load("stream-1")
	if !ok {
		t.Fatal("meter missing")
	}
	meter := v.(*netmeter.Meter)
	meter.Record(netmeter.OpGQL, 2048)

	jobs := svc.ListActiveSyncs(context.Background())
	if len(jobs) != 1 {
		t.Fatalf("jobs = %d, want 1", len(jobs))
	}
	if jobs[0].StreamID != "stream-1" || jobs[0].Channel != "sodapoppin" {
		t.Fatalf("job = %+v", jobs[0])
	}
	if jobs[0].Phase != SyncPhaseFetchingComments {
		t.Fatalf("phase = %q", jobs[0].Phase)
	}

	status := &SyncStatus{StreamID: "stream-1", Channel: "sodapoppin"}
	svc.updateNetworkUsage(status)
	if status.Network == nil || status.Network.GQLFetchBytes != 2048 {
		t.Fatalf("network = %+v", status.Network)
	}
}

func TestListActiveSyncsHandler(t *testing.T) {
	svc := &SyncService{
		activeSyncRegistry: map[string]*activeSyncState{
			"abc": {channel: "ninja", phase: SyncPhaseScrapingTracker},
		},
	}
	h := &Handler{syncService: svc}
	req := httptest.NewRequest(http.MethodGet, "/v1/analytics/sync/active", nil)
	rec := httptest.NewRecorder()
	h.listActiveSyncs(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var payload struct {
		Syncs []ActiveSyncItem `json:"syncs"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Syncs) != 1 || payload.Syncs[0].Channel != "ninja" {
		t.Fatalf("syncs = %+v", payload.Syncs)
	}
}

func TestTrackingSnapshotHandler(t *testing.T) {
	collector := NewCollector(nil, nil, nil, nil, nil, 5, time.Second, time.Hour, 10)
	collector.mu.Lock()
	collector.tracked = map[string]*trackedChannel{
		"sodapoppin": {login: "sodapoppin", addedAt: time.Now().UTC()},
	}
	collector.mu.Unlock()

	h := &Handler{collector: collector}
	req := httptest.NewRequest(http.MethodGet, "/v1/analytics/tracking/snapshot", nil)
	rec := httptest.NewRecorder()
	h.trackingSnapshot(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var snap TrackingSnapshot
	if err := json.Unmarshal(rec.Body.Bytes(), &snap); err != nil {
		t.Fatal(err)
	}
	if len(snap.TrackedChannels) != 1 || snap.TrackedChannels[0] != "sodapoppin" {
		t.Fatalf("tracked = %+v", snap.TrackedChannels)
	}
}

func TestSyncNetRecordThroughContext(t *testing.T) {
	svc := &SyncService{
		activeSyncRegistry: map[string]*activeSyncState{
			"s1": {channel: "test"},
		},
	}
	ctx := svc.withSyncNetwork(context.Background(), "s1")
	syncNetRecord(ctx, netmeter.OpTracker, 512)
	v, ok := svc.syncMeters.Load("s1")
	if !ok {
		t.Fatal("meter missing")
	}
	snap := v.(*netmeter.Meter).Snapshot()
	if snap.TrackerScrapeBytes != 512 {
		t.Fatalf("bytes = %d", snap.TrackerScrapeBytes)
	}
}
