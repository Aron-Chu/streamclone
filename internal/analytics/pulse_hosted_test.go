package analytics

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPulseBetaKeyMiddleware(t *testing.T) {
	h := &Handler{
		pulseHosted: PulseHostedConfig{
			Hosted:   true,
			BetaKeys: []string{"secret-one"},
		},
	}
	called := false
	mw := h.pulseBetaKeyMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/v1/extension/pulse/channels/xqc", nil)
	rec := httptest.NewRecorder()
	mw.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without key, got %d", rec.Code)
	}
	if called {
		t.Fatal("handler should not run without key")
	}

	req2 := httptest.NewRequest(http.MethodGet, "/v1/extension/pulse/channels/xqc", nil)
	req2.Header.Set("X-Streamclone-Beta-Key", "secret-one")
	rec2 := httptest.NewRecorder()
	mw.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusOK {
		t.Fatalf("expected 200 with key, got %d", rec2.Code)
	}
	if !called {
		t.Fatal("handler should run with valid key")
	}
}

func TestPulseHostedAuthorized(t *testing.T) {
	const validKey = "secret-one"
	want401 := map[string]string{
		"error": "unauthorized",
		"hint":  "Set X-Streamclone-Beta-Key header (Pulse extension options)",
	}

	tests := []struct {
		name       string
		hosted     bool
		betaKeys   []string
		headerKey  string
		wantStatus int
		want401    bool
	}{
		{
			name:       "hosted off",
			hosted:     false,
			betaKeys:   []string{validKey},
			headerKey:  "",
			wantStatus: http.StatusOK,
		},
		{
			name:       "hosted on no keys configured",
			hosted:     true,
			betaKeys:   nil,
			headerKey:  "",
			wantStatus: http.StatusUnauthorized,
			want401:    true,
		},
		{
			name:       "hosted on keys missing header",
			hosted:     true,
			betaKeys:   []string{validKey},
			headerKey:  "",
			wantStatus: http.StatusUnauthorized,
			want401:    true,
		},
		{
			name:       "hosted on keys wrong header",
			hosted:     true,
			betaKeys:   []string{validKey},
			headerKey:  "wrong-key",
			wantStatus: http.StatusUnauthorized,
			want401:    true,
		},
		{
			name:       "hosted on keys valid header",
			hosted:     true,
			betaKeys:   []string{validKey},
			headerKey:  validKey,
			wantStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := &Handler{
				pulseHosted: PulseHostedConfig{
					Hosted:   tt.hosted,
					BetaKeys: tt.betaKeys,
				},
			}
			called := false
			mw := h.pulseBetaKeyMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				called = true
				w.WriteHeader(http.StatusOK)
			}))

			req := httptest.NewRequest(http.MethodGet, "/v1/extension/pulse/channels/xqc", nil)
			if tt.headerKey != "" {
				req.Header.Set("X-Streamclone-Beta-Key", tt.headerKey)
			}
			rec := httptest.NewRecorder()
			mw.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
			if tt.wantStatus == http.StatusOK && !called {
				t.Fatal("handler should run when authorized")
			}
			if tt.wantStatus == http.StatusUnauthorized && called {
				t.Fatal("handler should not run when unauthorized")
			}
			if tt.want401 {
				var got map[string]string
				if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
					t.Fatalf("decode 401 body: %v", err)
				}
				if got["error"] != want401["error"] || got["hint"] != want401["hint"] {
					t.Fatalf("401 body = %#v, want %#v", got, want401)
				}
			}
		})
	}
}

func TestParsePulseBetaKeys(t *testing.T) {
	got := ParsePulseBetaKeys(" a , b , ")
	if len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Fatalf("ParsePulseBetaKeys() = %#v", got)
	}
}
