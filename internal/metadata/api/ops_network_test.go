package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestOpsNetworkActiveStreams(t *testing.T) {
	video := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/stream/status":
			writeJSON(w, http.StatusOK, map[string]any{
				"sessions": []map[string]any{
					{"channel": "ninja", "listeners": 2, "quality": "720p60", "workerBackend": "streamlink"},
				},
			})
		case "/v1/stream/diagnostics":
			writeJSON(w, http.StatusOK, map[string]any{
				"liveEdge": 3,
				"hlsProbe": map[string]any{"durationMs": 42, "targetDuration": "2"},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(video.Close)

	origVideoStatusURL := videoStatusURL
	origVideoDiagnosticsBase := videoDiagnosticsBase
	videoStatusURL = video.URL + "/v1/stream/status"
	videoDiagnosticsBase = video.URL + "/v1/stream/diagnostics?channel="
	t.Cleanup(func() {
		videoStatusURL = origVideoStatusURL
		videoDiagnosticsBase = origVideoDiagnosticsBase
	})

	h := New(nil, nil).WithSetupWelcome(SetupWelcomeOptions{Profile: "core"})
	req := httptest.NewRequest(http.MethodGet, "/v1/ops/network", nil)
	rec := httptest.NewRecorder()
	started := time.Now()
	h.opsNetwork(rec, req)
	if elapsed := time.Since(started); elapsed > 4*time.Second {
		t.Fatalf("ops network took %s, want under 4s", elapsed)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}

	var resp opsNetworkResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Services.Metadata != "ready" {
		t.Fatalf("metadata = %q", resp.Services.Metadata)
	}
	if len(resp.ActiveStreams) != 1 || resp.ActiveStreams[0].Channel != "ninja" {
		t.Fatalf("activeStreams = %+v", resp.ActiveStreams)
	}
	if resp.ActiveStreams[0].LiveEdge != 3 {
		t.Fatalf("liveEdge = %d, want 3", resp.ActiveStreams[0].LiveEdge)
	}
}

func TestOpsNetworkWithoutPulse(t *testing.T) {
	origVideoStatusURL := videoStatusURL
	videoStatusURL = "http://127.0.0.1:1/v1/stream/status"
	t.Cleanup(func() { videoStatusURL = origVideoStatusURL })

	h := New(nil, nil).WithSetupWelcome(SetupWelcomeOptions{Profile: "core"})
	req := httptest.NewRequest(http.MethodGet, "/v1/ops/network", nil)
	rec := httptest.NewRecorder()
	h.opsNetwork(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var resp opsNetworkResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.PulseReady {
		t.Fatal("pulseReady = true, want false")
	}
	if resp.Prometheus != nil {
		t.Fatalf("prometheus = %+v, want nil when pulse offline", resp.Prometheus)
	}
}

func TestQueryPrometheusInstant(t *testing.T) {
	prom := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query().Get("query")
		var result []map[string]any
		switch query {
		case `sum(rate(http_requests_total[1m])) by (service)`:
			result = []map[string]any{
				{"metric": map[string]string{"service": "video"}, "value": []any{time.Now().Unix(), "12.5"}},
			}
		case "sum(chat_connections)":
			result = []map[string]any{
				{"metric": map[string]string{}, "value": []any{time.Now().Unix(), "4"}},
			}
		default:
			result = []map[string]any{}
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"status": "success",
			"data": map[string]any{
				"resultType": "vector",
				"result":     result,
			},
		})
	}))
	t.Cleanup(prom.Close)

	origPrometheusQueryURL := prometheusQueryURL
	prometheusQueryURL = prom.URL
	t.Cleanup(func() { prometheusQueryURL = origPrometheusQueryURL })

	h := New(nil, nil)
	seriesMetric := h.queryPrometheusInstant(context.Background(), `sum(rate(http_requests_total[1m])) by (service)`)
	if seriesMetric == nil || len(seriesMetric.Series) != 1 || seriesMetric.Series[0].Value != 12.5 {
		t.Fatalf("series = %+v", seriesMetric)
	}

	scalarMetric := h.queryPrometheusInstant(context.Background(), "sum(chat_connections)")
	if scalarMetric == nil || scalarMetric.Value == nil || *scalarMetric.Value != 4 {
		t.Fatalf("value = %+v, want 4", scalarMetric)
	}
}
