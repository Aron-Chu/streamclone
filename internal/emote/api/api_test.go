package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	"streamclone/internal/emote/seeder"
	"streamclone/internal/emote/store"
)

func newTestHandler(token string) *Handler {
	return &Handler{token: token}
}

func postEnsure(h *Handler, login, body string) *httptest.ResponseRecorder {
	r := chi.NewRouter()
	h.Routes(r)
	req := httptest.NewRequest(http.MethodPost, "/v1/channels/"+login+"/emotes/ensure", bytes.NewBufferString(body))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

func TestBearerAuthMissingHeader(t *testing.T) {
	h := newTestHandler("secret")
	r := chi.NewRouter()
	r.Use(h.bearerAuth)
	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestBearerAuthWrongToken(t *testing.T) {
	h := newTestHandler("secret")
	r := chi.NewRouter()
	r.Use(h.bearerAuth)
	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer wrongtoken")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestBearerAuthCorrectToken(t *testing.T) {
	h := newTestHandler("secret")
	r := chi.NewRouter()
	r.Use(h.bearerAuth)
	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer secret")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestEnsureChannelEmotesAlreadyLoaded(t *testing.T) {
	h := &Handler{ensure: func(_ context.Context, login, twitchID string, providers []seeder.Provider) (ensureResponse, int, error) {
		if login != "leeonbeeon" || twitchID != "670388028" {
			t.Fatalf("unexpected args: %q %q", login, twitchID)
		}
		if len(providers) != 1 || providers[0] != seeder.ProviderSevenTV {
			t.Fatalf("unexpected providers: %+v", providers)
		}
		return makeEnsureResponse("ready", 12, 0, providers), http.StatusOK, nil
	}}

	rec := postEnsure(h, "leeonbeeon", `{"twitch_id":"670388028"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var body ensureResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.State != "ready" || body.Count != 12 || body.Pending != 0 {
		t.Fatalf("unexpected body: %+v", body)
	}
}

func TestEnsureChannelEmotesStartsSeed(t *testing.T) {
	h := &Handler{ensure: func(_ context.Context, _ string, _ string, providers []seeder.Provider) (ensureResponse, int, error) {
		if len(providers) != 2 || providers[0] != seeder.ProviderSevenTV || providers[1] != seeder.ProviderFFZ {
			t.Fatalf("unexpected providers: %+v", providers)
		}
		return makeEnsureResponse("processing", 0, 33, providers), http.StatusAccepted, nil
	}}

	rec := postEnsure(h, "leeonbeeon", `{"twitch_id":"670388028","providers":["7tv","ffz"]}`)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d", rec.Code)
	}
	var body ensureResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.State != "processing" || body.Pending != 33 {
		t.Fatalf("unexpected body: %+v", body)
	}
	if len(body.Providers) != 2 || body.Providers[1].Provider != "ffz" {
		t.Fatalf("unexpected providers body: %+v", body.Providers)
	}
}

func TestEnsureChannelEmotesBadTwitchID(t *testing.T) {
	h := &Handler{ensure: func(context.Context, string, string, []seeder.Provider) (ensureResponse, int, error) {
		t.Fatal("ensure must not be called for bad request")
		return ensureResponse{}, 0, nil
	}}

	rec := postEnsure(h, "leeonbeeon", `{"twitch_id":""}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestEnsureChannelEmotesBadProvider(t *testing.T) {
	h := &Handler{ensure: func(context.Context, string, string, []seeder.Provider) (ensureResponse, int, error) {
		t.Fatal("ensure must not be called for bad provider")
		return ensureResponse{}, 0, nil
	}}

	rec := postEnsure(h, "leeonbeeon", `{"twitch_id":"670388028","providers":["foo"]}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestEnsureChannelEmotesAcceptsBTTV(t *testing.T) {
	h := &Handler{ensure: func(_ context.Context, _ string, _ string, providers []seeder.Provider) (ensureResponse, int, error) {
		if len(providers) != 1 || providers[0] != seeder.ProviderBTTV {
			t.Fatalf("unexpected providers: %+v", providers)
		}
		return makeEnsureResponse("processing", 0, 10, providers), http.StatusAccepted, nil
	}}

	rec := postEnsure(h, "leeonbeeon", `{"twitch_id":"670388028","providers":["bttv"]}`)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d", rec.Code)
	}
}

func TestEnsureChannelEmotesSeedFailure(t *testing.T) {
	h := &Handler{ensure: func(context.Context, string, string, []seeder.Provider) (ensureResponse, int, error) {
		return ensureResponse{}, http.StatusBadGateway, errors.New("7tv returned 404")
	}}

	rec := postEnsure(h, "leeonbeeon", `{"twitch_id":"670388028"}`)
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("expected 502, got %d", rec.Code)
	}
}

func TestApplyProviderSummaryUsesCatalogTargetWhileAssetsPending(t *testing.T) {
	resp := makeEnsureResponse("processing", 100, 851, []seeder.Provider{seeder.ProviderSevenTV})
	applyProviderSummary(&resp, map[string]store.ProviderEmoteSummary{
		"seventv": {Provider: "seventv", Ready: 100, Pending: 0, Failed: 0},
	}, map[string]store.ChannelProviderLoad{
		"seventv": {Provider: "seventv", State: "ready", Count: 951},
	})

	if len(resp.Providers) != 1 {
		t.Fatalf("expected one provider, got %+v", resp.Providers)
	}
	provider := resp.Providers[0]
	if provider.State != "processing" {
		t.Fatalf("expected processing state, got %+v", provider)
	}
	if provider.Count != 100 || provider.Total != 951 || provider.Percent != 10 {
		t.Fatalf("unexpected progress: %+v", provider)
	}
}

func TestApplyProviderSummaryBootstrapsFromCatalogBeforeSummaryExists(t *testing.T) {
	resp := makeEnsureResponse("processing", 0, 1, []seeder.Provider{seeder.ProviderSevenTV})
	applyProviderSummary(&resp, map[string]store.ProviderEmoteSummary{}, map[string]store.ChannelProviderLoad{
		"seventv": {Provider: "seventv", State: "ready", Count: 951},
	})

	provider := resp.Providers[0]
	if provider.State != "processing" || provider.Total != 951 || provider.Percent != 0 || provider.Pending != 951 {
		t.Fatalf("unexpected bootstrap progress: %+v", provider)
	}
}

func TestProviderSnapshotNeedsRefresh(t *testing.T) {
	tests := []struct {
		name       string
		localFound bool
		localSetID string
		localHash  string
		localCount int
		remoteSet  string
		remoteHash string
		remoteCnt  int
		want       bool
	}{
		{name: "missing local snapshot", localFound: false, remoteSet: "set-1", remoteCnt: 10, remoteHash: "abc", want: true},
		{name: "count mismatch", localFound: true, localSetID: "set-1", localHash: "abc", localCount: 96, remoteSet: "set-1", remoteHash: "abc", remoteCnt: 954, want: true},
		{name: "hash mismatch", localFound: true, localSetID: "set-1", localHash: "abc", localCount: 10, remoteSet: "set-1", remoteHash: "def", remoteCnt: 10, want: true},
		{name: "set mismatch", localFound: true, localSetID: "set-1", localHash: "abc", localCount: 10, remoteSet: "set-2", remoteHash: "abc", remoteCnt: 10, want: true},
		{name: "matching snapshot", localFound: true, localSetID: "set-1", localHash: "abc", localCount: 10, remoteSet: "set-1", remoteHash: "abc", remoteCnt: 10, want: false},
		{name: "no remote set", localFound: true, localSetID: "set-1", localHash: "abc", localCount: 10, remoteSet: "", remoteHash: "", remoteCnt: 0, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := providerSnapshotNeedsRefresh(tt.localFound, tt.localSetID, tt.localHash, tt.localCount, tt.remoteSet, tt.remoteHash, tt.remoteCnt)
			if got != tt.want {
				t.Fatalf("providerSnapshotNeedsRefresh() = %v, want %v", got, tt.want)
			}
		})
	}
}
