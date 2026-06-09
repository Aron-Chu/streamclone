package auth

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"
)

type memoryStore struct {
	mu             sync.Mutex
	devClaims      map[string]string
	latestDevClaim string
	deviceAuths    map[string]DeviceAuth
	sessions       map[string]Session
}

func newMemoryStore() *memoryStore {
	return &memoryStore{
		devClaims:   map[string]string{},
		deviceAuths: map[string]DeviceAuth{},
		sessions:    map[string]Session{},
	}
}

func (s *memoryStore) SetDevClaim(_ context.Context, claimID string, sessionID string, _ time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.devClaims[claimID] = sessionID
	s.latestDevClaim = claimID
	return nil
}

func (s *memoryStore) TakeDevClaim(_ context.Context, claimID string) (string, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sessionID, ok := s.devClaims[claimID]
	if !ok {
		return "", false, nil
	}
	delete(s.devClaims, claimID)
	if s.latestDevClaim == claimID {
		s.latestDevClaim = ""
	}
	return sessionID, true, nil
}

func (s *memoryStore) TakeLatestDevClaim(_ context.Context) (string, bool, error) {
	s.mu.Lock()
	claimID := s.latestDevClaim
	s.latestDevClaim = ""
	if claimID == "" {
		s.mu.Unlock()
		return "", false, nil
	}
	sessionID, ok := s.devClaims[claimID]
	if ok {
		delete(s.devClaims, claimID)
	}
	s.mu.Unlock()
	return sessionID, ok, nil
}

func (s *memoryStore) SaveDeviceAuth(_ context.Context, requestID string, deviceAuth DeviceAuth, _ time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.deviceAuths[requestID] = deviceAuth
	return nil
}

func (s *memoryStore) GetDeviceAuth(_ context.Context, requestID string) (DeviceAuth, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	deviceAuth, ok := s.deviceAuths[requestID]
	if !ok {
		return DeviceAuth{}, ErrNotFound
	}
	return deviceAuth, nil
}

func (s *memoryStore) DeleteDeviceAuth(_ context.Context, requestID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.deviceAuths, requestID)
	return nil
}

func (s *memoryStore) SaveSession(_ context.Context, session Session, _ time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions[session.ID] = session
	return nil
}

func (s *memoryStore) GetSession(_ context.Context, id string) (Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	session, ok := s.sessions[id]
	if !ok {
		return Session{}, ErrNotFound
	}
	return session, nil
}

func (s *memoryStore) DeleteSession(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessions, id)
	return nil
}

func TestDebugReportsFrontendOriginMismatch(t *testing.T) {
	h := New(newMemoryStore(), Config{
		ClientID:       "cid",
		ClientSecret:   "secret",
		FrontendURL:    "http://localhost:5174",
		CookieSecret:   "cookie-secret",
		CookieSameSite: "lax",
	}, slog.Default())

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/auth/debug", nil)
	req.Host = "localhost:8083"
	req.Header.Set("Origin", "http://127.0.0.1:8090")
	h.debug(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status got %d", rec.Code)
	}
	var body debugResp
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if !body.Ready || len(body.Warnings) == 0 {
		t.Fatalf("expected frontend origin mismatch warning, got %+v", body)
	}
	if body.ClientIDConfigured != true || body.ClientSecretConfigured != true {
		t.Fatalf("expected configured flags, got %+v", body)
	}
}

func TestImportTokenCreatesSessionOnLoopback(t *testing.T) {
	store := newMemoryStore()
	twitch := newImportTwitchServer(t)
	defer twitch.Close()

	h := New(store, Config{
		ClientID:              "cid",
		ClientSecret:          "secret",
		ValidateURL:           twitch.URL + "/validate",
		APIURL:                twitch.URL,
		DevTokenImportEnabled: true,
		CookieSecret:          "cookie-secret",
	}, slog.Default())

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/auth/dev/import", strings.NewReader(`{"access_token":"access","refresh_token":"refresh"}`))
	req.Host = "localhost:8083"
	req.Header.Set("Content-Type", "application/json")
	h.importToken(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status got %d body %s", rec.Code, rec.Body.String())
	}
	cookies := rec.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatal("missing session cookie")
	}
	sessionID, ok := h.verifyCookie(cookies[0].Value)
	if !ok {
		t.Fatal("session cookie did not verify")
	}
	session, err := store.GetSession(context.Background(), sessionID)
	if err != nil {
		t.Fatal(err)
	}
	if session.Login != "viewer" || session.AccessToken != "access" || session.RefreshToken != "refresh" {
		t.Fatalf("unexpected session: %+v", session)
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if authenticated, _ := body["authenticated"].(bool); !authenticated {
		t.Fatalf("expected authenticated response, got %+v", body)
	}
}

func TestPrepareTokenClaimRedirectSetsBrowserSession(t *testing.T) {
	store := newMemoryStore()
	twitch := newImportTwitchServer(t)
	defer twitch.Close()

	h := New(store, Config{
		ClientID:              "cid",
		ClientSecret:          "secret",
		FrontendURL:           "http://localhost:8090",
		ValidateURL:           twitch.URL + "/validate",
		APIURL:                twitch.URL,
		DevTokenImportEnabled: true,
		CookieSecret:          "cookie-secret",
	}, slog.Default())

	prepareRec := httptest.NewRecorder()
	prepareReq := httptest.NewRequest(http.MethodPost, "/v1/auth/dev/prepare", strings.NewReader(`{"access_token":"access","refresh_token":"refresh"}`))
	prepareReq.Host = "localhost:8090"
	prepareReq.Header.Set("Content-Type", "application/json")
	h.prepareToken(prepareRec, prepareReq)

	if prepareRec.Code != http.StatusOK {
		t.Fatalf("prepare status got %d body %s", prepareRec.Code, prepareRec.Body.String())
	}
	var prepared struct {
		ClaimURL string `json:"claimUrl"`
	}
	if err := json.Unmarshal(prepareRec.Body.Bytes(), &prepared); err != nil {
		t.Fatal(err)
	}
	claimURL, err := url.Parse(prepared.ClaimURL)
	if err != nil {
		t.Fatal(err)
	}
	claimCode := claimURL.Query().Get("code")
	if claimURL.Path != "/v1/auth/dev/claim" || claimCode == "" {
		t.Fatalf("unexpected claim URL %q", prepared.ClaimURL)
	}

	claimRec := httptest.NewRecorder()
	claimReq := httptest.NewRequest(http.MethodGet, "/v1/auth/dev/claim?code="+url.QueryEscape(claimCode), nil)
	claimReq.Host = "localhost:8090"
	h.claimPreparedTokenRedirect(claimRec, claimReq)

	if claimRec.Code != http.StatusFound {
		t.Fatalf("claim status got %d body %s", claimRec.Code, claimRec.Body.String())
	}
	loc, err := url.Parse(claimRec.Header().Get("Location"))
	if err != nil {
		t.Fatal(err)
	}
	if loc.Query().Get("auth") != "success" {
		t.Fatalf("unexpected claim redirect: %q", claimRec.Header().Get("Location"))
	}
	cookies := claimRec.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatal("missing session cookie")
	}
	sessionID, ok := h.verifyCookie(cookies[0].Value)
	if !ok {
		t.Fatal("session cookie did not verify")
	}
	session, err := store.GetSession(context.Background(), sessionID)
	if err != nil {
		t.Fatal(err)
	}
	if session.Login != "viewer" || session.AccessToken != "access" || session.RefreshToken != "refresh" {
		t.Fatalf("unexpected session: %+v", session)
	}
}

func TestClaimPreparedTokenUsesLatestPreparedSession(t *testing.T) {
	store := newMemoryStore()
	twitch := newImportTwitchServer(t)
	defer twitch.Close()

	h := New(store, Config{
		ClientID:              "cid",
		ClientSecret:          "secret",
		ValidateURL:           twitch.URL + "/validate",
		APIURL:                twitch.URL,
		DevTokenImportEnabled: true,
		CookieSecret:          "cookie-secret",
	}, slog.Default())

	prepareRec := httptest.NewRecorder()
	prepareReq := httptest.NewRequest(http.MethodPost, "/v1/auth/dev/prepare", strings.NewReader(`{"access_token":"access","refresh_token":"refresh"}`))
	prepareReq.Host = "localhost:8090"
	prepareReq.Header.Set("Content-Type", "application/json")
	h.prepareToken(prepareRec, prepareReq)
	if prepareRec.Code != http.StatusOK {
		t.Fatalf("prepare status got %d body %s", prepareRec.Code, prepareRec.Body.String())
	}

	claimRec := httptest.NewRecorder()
	claimReq := httptest.NewRequest(http.MethodPost, "/v1/auth/dev/claim", nil)
	claimReq.Host = "localhost:8090"
	h.claimPreparedToken(claimRec, claimReq)

	if claimRec.Code != http.StatusOK {
		t.Fatalf("claim status got %d body %s", claimRec.Code, claimRec.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(claimRec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if authenticated, _ := body["authenticated"].(bool); !authenticated {
		t.Fatalf("expected authenticated response, got %+v", body)
	}
	if len(claimRec.Result().Cookies()) == 0 {
		t.Fatal("missing session cookie")
	}
}

func TestImportTokenRejectsNonLoopbackHost(t *testing.T) {
	h := New(newMemoryStore(), Config{
		ClientID:              "cid",
		DevTokenImportEnabled: true,
		CookieSecret:          "cookie-secret",
	}, slog.Default())

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/auth/dev/import", strings.NewReader(`{"access_token":"access"}`))
	req.Host = "example.com"
	req.Header.Set("Content-Type", "application/json")
	h.importToken(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected not found, got %d", rec.Code)
	}
}

func TestMeReportsLoopbackTokenImportCapability(t *testing.T) {
	h := New(newMemoryStore(), Config{
		DevTokenImportEnabled: true,
		CookieSecret:          "cookie-secret",
	}, slog.Default())

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/me", nil)
	req.Host = "localhost:8083"
	h.me(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status got %d", rec.Code)
	}
	var body struct {
		Authenticated       bool `json:"authenticated"`
		CanImportLocalToken bool `json:"canImportLocalToken"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Authenticated {
		t.Fatalf("expected unauthenticated response, got %+v", body)
	}
	if !body.CanImportLocalToken {
		t.Fatalf("expected loopback import capability, got %+v", body)
	}
}

func TestStartDeviceAuthReturnsVerificationData(t *testing.T) {
	store := newMemoryStore()
	twitch := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/device":
			if err := r.ParseForm(); err != nil {
				t.Fatal(err)
			}
			if r.Form.Get("client_id") != "cid" {
				t.Fatalf("unexpected device form: %+v", r.Form)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"device_code":      "device-code",
				"expires_in":       1800,
				"interval":         5,
				"user_code":        "ABCDEFGH",
				"verification_uri": "https://www.twitch.tv/activate?public=true&device-code=ABCDEFGH",
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer twitch.Close()

	h := New(store, Config{
		ClientID:              "cid",
		DeviceURL:             twitch.URL + "/device",
		DevTokenImportEnabled: true,
		CookieSecret:          "cookie-secret",
	}, slog.Default())

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/auth/dev/device/start", nil)
	req.Host = "localhost:8090"
	h.startDeviceAuth(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status got %d body %s", rec.Code, rec.Body.String())
	}
	var body struct {
		RequestID           string `json:"requestId"`
		UserCode            string `json:"userCode"`
		VerificationURI     string `json:"verificationUri"`
		ExpiresInSeconds    int    `json:"expiresInSeconds"`
		PollIntervalSeconds int    `json:"pollIntervalSeconds"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.RequestID == "" || body.UserCode != "ABCDEFGH" || body.PollIntervalSeconds != 5 {
		t.Fatalf("unexpected device auth body: %+v", body)
	}
	if _, err := store.GetDeviceAuth(context.Background(), body.RequestID); err != nil {
		t.Fatalf("device auth was not stored: %v", err)
	}
}

func TestPollDeviceAuthPendingThenCompleteSetsSession(t *testing.T) {
	store := newMemoryStore()
	pollCount := 0
	twitch := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/token":
			pollCount++
			if err := r.ParseForm(); err != nil {
				t.Fatal(err)
			}
			if r.Form.Get("device_code") != "device-code" || r.Form.Get("grant_type") != deviceCodeGrantType {
				t.Fatalf("unexpected token form: %+v", r.Form)
			}
			if pollCount == 1 {
				w.WriteHeader(http.StatusBadRequest)
				_ = json.NewEncoder(w).Encode(map[string]any{"status": 400, "message": "authorization_pending"})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"access_token":  "access",
				"refresh_token": "refresh",
				"expires_in":    3600,
				"scope":         []string{"chat:read", "chat:edit"},
			})
		case "/validate":
			if r.Header.Get("Authorization") != "OAuth access" {
				t.Fatalf("unexpected validate headers: %+v", r.Header)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"client_id":  "cid",
				"login":      "viewer",
				"scopes":     []string{"chat:read", "chat:edit"},
				"user_id":    "u1",
				"expires_in": 3600,
			})
		case "/users":
			if r.Header.Get("Authorization") != "Bearer access" || r.Header.Get("Client-Id") != "cid" {
				t.Fatalf("unexpected user headers: %+v", r.Header)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": []map[string]string{{
					"id":                "u1",
					"login":             "viewer",
					"display_name":      "Viewer",
					"profile_image_url": "http://img",
				}},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer twitch.Close()

	h := New(store, Config{
		ClientID:              "cid",
		DeviceURL:             twitch.URL + "/device",
		TokenURL:              twitch.URL + "/token",
		ValidateURL:           twitch.URL + "/validate",
		APIURL:                twitch.URL,
		DevTokenImportEnabled: true,
		CookieSecret:          "cookie-secret",
	}, slog.Default())
	if err := store.SaveDeviceAuth(context.Background(), "req-1", DeviceAuth{
		DeviceCode:          "device-code",
		UserCode:            "ABCDEFGH",
		VerificationURI:     "https://www.twitch.tv/activate?public=true&device-code=ABCDEFGH",
		PollIntervalSeconds: 5,
		ExpiresAt:           time.Now().Add(30 * time.Minute).Unix(),
	}, 30*time.Minute); err != nil {
		t.Fatal(err)
	}

	pendingRec := httptest.NewRecorder()
	pendingReq := httptest.NewRequest(http.MethodPost, "/v1/auth/dev/device/poll", strings.NewReader(`{"request_id":"req-1"}`))
	pendingReq.Host = "localhost:8090"
	pendingReq.Header.Set("Content-Type", "application/json")
	h.pollDeviceAuth(pendingRec, pendingReq)

	if pendingRec.Code != http.StatusOK || !strings.Contains(pendingRec.Body.String(), `"status":"pending"`) {
		t.Fatalf("unexpected pending response %d %s", pendingRec.Code, pendingRec.Body.String())
	}
	completeRec := httptest.NewRecorder()
	completeReq := httptest.NewRequest(http.MethodPost, "/v1/auth/dev/device/poll", strings.NewReader(`{"request_id":"req-1"}`))
	completeReq.Host = "localhost:8090"
	completeReq.Header.Set("Content-Type", "application/json")
	h.pollDeviceAuth(completeRec, completeReq)

	if completeRec.Code != http.StatusOK {
		t.Fatalf("unexpected complete response %d %s", completeRec.Code, completeRec.Body.String())
	}
	if len(completeRec.Result().Cookies()) == 0 {
		t.Fatal("missing session cookie")
	}
	var body map[string]any
	if err := json.Unmarshal(completeRec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["status"] != "complete" || body["authenticated"] != true {
		t.Fatalf("unexpected complete body: %+v", body)
	}
	if _, err := store.GetDeviceAuth(context.Background(), "req-1"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("device auth should be removed after completion, got %v", err)
	}
}

func newImportTwitchServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/validate":
			if r.Header.Get("Authorization") != "OAuth access" {
				t.Fatalf("unexpected validate headers: %+v", r.Header)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"client_id":  "cid",
				"login":      "viewer",
				"scopes":     []string{"chat:read", "chat:edit"},
				"user_id":    "u1",
				"expires_in": 3600,
			})
		case "/users":
			if r.Header.Get("Authorization") != "Bearer access" || r.Header.Get("Client-Id") != "cid" {
				t.Fatalf("unexpected user headers: %+v", r.Header)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": []map[string]string{{
					"id":                "u1",
					"login":             "viewer",
					"display_name":      "Viewer",
					"profile_image_url": "http://img",
				}},
			})
		default:
			http.NotFound(w, r)
		}
	}))
}

func TestFollowedChannelsEnrichesOfflineProfileImages(t *testing.T) {
	twitch := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/streams/followed":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": []map[string]any{{
					"user_id":       "live1",
					"user_login":    "livechan",
					"user_name":     "LiveChan",
					"viewer_count":  42,
					"thumbnail_url": "http://thumb/{width}x{height}",
				}},
			})
		case "/channels/followed":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": []map[string]string{{
					"broadcaster_id":    "offline1",
					"broadcaster_login": "offlinechan",
					"broadcaster_name":  "OfflineChan",
				}},
			})
		case "/users":
			if r.URL.Query().Get("id") == "" {
				t.Fatal("expected id query")
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": []map[string]string{
					{"id": "live1", "login": "livechan", "display_name": "LiveChan", "profile_image_url": "http://live/avatar.png"},
					{"id": "offline1", "login": "offlinechan", "display_name": "OfflineChan", "profile_image_url": "http://offline/avatar.png"},
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer twitch.Close()

	h := New(newMemoryStore(), Config{
		ClientID:     "cid",
		ClientSecret: "secret",
		RedirectURL:  "http://chat/callback",
		APIURL:       twitch.URL,
		CookieSecret: "cookie-secret",
	}, slog.Default())
	channels, err := h.followedChannels(context.Background(), Session{UserID: "viewer", AccessToken: "access"})
	if err != nil {
		t.Fatal(err)
	}
	if len(channels) != 2 {
		t.Fatalf("expected 2 channels, got %+v", channels)
	}
	if channels[1].Login != "offlinechan" || channels[1].ProfileImage != "http://offline/avatar.png" {
		t.Fatalf("offline profile image was not enriched: %+v", channels)
	}
}
