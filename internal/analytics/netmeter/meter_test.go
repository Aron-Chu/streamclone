package netmeter

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestMeterRecordAndSnapshot(t *testing.T) {
	var recorded int64
	m := NewMeter(func(op string, n int64) {
		if op == OpGQL {
			recorded += n
		}
	})
	m.Record(OpTracker, 100)
	m.Record(OpGQL, 200)
	snap := m.Snapshot()
	if snap.TotalBytes != 300 || recorded != 200 {
		t.Fatalf("snap=%+v recorded=%d", snap, recorded)
	}
}

func TestMeterRate(t *testing.T) {
	m := NewMeter(nil)
	m.lastSnap = time.Now().Add(-time.Second)
	m.lastTotal = 0
	m.Record(OpGQL, 125_000)
	snap := m.Snapshot()
	if snap.LastRateBps <= 0 {
		t.Fatalf("rate=%f", snap.LastRateBps)
	}
}

func TestCountingTransport(t *testing.T) {
	m := NewMeter(nil)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("hello"))
	}))
	defer srv.Close()

	client := &http.Client{Transport: NewCountingTransport(http.DefaultTransport, m, OpGQL)}
	resp, err := client.Get(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	snap := m.Snapshot()
	if snap.GQLFetchBytes != 5 {
		t.Fatalf("bytes=%d", snap.GQLFetchBytes)
	}
}
