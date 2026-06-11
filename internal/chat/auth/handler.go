package auth

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
)

const (
	cookieName  = "streamclone_session"
	devClaimTTL = 10 * time.Minute
	sessionTTL  = 30 * 24 * time.Hour
	deviceCodeGrantType = "urn:ietf:params:oauth:grant-type:device_code"
)

type Config struct {
	ClientID              string
	ClientSecret          string
	RedirectURL           string
	FrontendURL           string
	AuthURL               string
	TokenURL              string
	DeviceURL             string
	ValidateURL           string
	APIURL                string
	Scopes                string
	DevTokenImportEnabled bool
	CookieSecret          string
	CookieSameSite        string
}

type Handler struct {
	store  Store
	client *http.Client
	cfg    Config
	log    *slog.Logger
}

type User struct {
	ID              string `json:"id"`
	Login           string `json:"login"`
	DisplayName     string `json:"display_name"`
	ProfileImageURL string `json:"profile_image_url"`
}

type FollowedChannel struct {
	ID           string `json:"id"`
	Login        string `json:"login"`
	DisplayName  string `json:"displayName"`
	ProfileImage string `json:"profileImage,omitempty"`
	IsLive       bool   `json:"isLive"`
	Title        string `json:"title,omitempty"`
	Category     string `json:"category,omitempty"`
	Viewers      int    `json:"viewers,omitempty"`
	ThumbnailURL string `json:"thumbnailUrl,omitempty"`
}

type tokenResponse struct {
	AccessToken  string   `json:"access_token"`
	RefreshToken string   `json:"refresh_token"`
	ExpiresIn    int      `json:"expires_in"`
	Scopes       []string `json:"scope"`
	TokenType    string   `json:"token_type"`
}

type tokenValidation struct {
	ClientID  string   `json:"client_id"`
	Login     string   `json:"login"`
	Scopes    []string `json:"scopes"`
	UserID    string   `json:"user_id"`
	ExpiresIn int      `json:"expires_in"`
}

type deviceCodeStartResponse struct {
	DeviceCode              string `json:"device_code"`
	ExpiresIn               int    `json:"expires_in"`
	Interval                int    `json:"interval"`
	UserCode                string `json:"user_code"`
	VerificationURI         string `json:"verification_uri"`
	VerificationURIComplete string `json:"verification_uri_complete"`
}

type deviceCodeTokenError struct {
	Status  int    `json:"status"`
	Message string `json:"message"`
}

type deviceCodePollRequest struct {
	RequestID string `json:"request_id"`
}

type importTokenReq struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
}

type importedSession struct {
	Session Session
	User    User
}

func New(store Store, cfg Config, logger *slog.Logger) *Handler {
	if cfg.FrontendURL == "" {
		cfg.FrontendURL = "http://localhost:5174"
	}
	if cfg.AuthURL == "" {
		cfg.AuthURL = "https://id.twitch.tv/oauth2/authorize"
	}
	if cfg.TokenURL == "" {
		cfg.TokenURL = "https://id.twitch.tv/oauth2/token"
	}
	if cfg.DeviceURL == "" {
		cfg.DeviceURL = "https://id.twitch.tv/oauth2/device"
	}
	if cfg.ValidateURL == "" {
		cfg.ValidateURL = "https://id.twitch.tv/oauth2/validate"
	}
	if cfg.APIURL == "" {
		cfg.APIURL = "https://api.twitch.tv/helix"
	}
	if cfg.Scopes == "" {
		cfg.Scopes = "chat:read chat:edit user:read:follows"
	}
	if cfg.CookieSecret == "" {
		cfg.CookieSecret = "dev-insecure-cookie-secret"
	}
	if cfg.CookieSameSite == "" {
		cfg.CookieSameSite = "lax"
	}
	return &Handler{
		store:  store,
		client: &http.Client{Timeout: 10 * time.Second},
		cfg:    cfg,
		log:    logger,
	}
}

func (h *Handler) SetHTTPClient(client *http.Client) {
	if client != nil {
		h.client = client
	}
}

func (h *Handler) Routes(r chi.Router) {
	r.Get("/v1/auth/debug", h.debug)
	r.Post("/v1/auth/dev/import", h.importToken)
	r.Post("/v1/auth/dev/prepare", h.prepareToken)
	r.Get("/v1/auth/dev/claim", h.claimPreparedTokenRedirect)
	r.Post("/v1/auth/dev/claim", h.claimPreparedToken)
	r.Post("/v1/auth/dev/device/start", h.startDeviceAuth)
	r.Post("/v1/auth/dev/device/poll", h.pollDeviceAuth)
	r.Get("/v1/me", h.me)
	r.Post("/v1/logout", h.logout)
	r.Get("/v1/followed", h.followed)
}

func (h *Handler) SessionIDFromRequest(r *http.Request) (string, bool) {
	cookie, err := r.Cookie(cookieName)
	if err != nil {
		return "", false
	}
	return h.verifyCookie(cookie.Value)
}

func (h *Handler) Session(ctx context.Context, id string) (Session, error) {
	session, err := h.store.GetSession(ctx, id)
	if err != nil {
		return Session{}, err
	}
	if time.Until(time.Unix(session.ExpiresAt, 0)) > 2*time.Minute {
		return session, nil
	}
	if session.RefreshToken == "" {
		return Session{}, errors.New("session expired")
	}
	refreshed, err := h.refresh(ctx, session.RefreshToken)
	if err != nil {
		return Session{}, err
	}
	session.AccessToken = refreshed.AccessToken
	if refreshed.RefreshToken != "" {
		session.RefreshToken = refreshed.RefreshToken
	}
	if len(refreshed.Scopes) > 0 {
		session.Scopes = refreshed.Scopes
	}
	session.ExpiresAt = time.Now().Add(time.Duration(refreshed.ExpiresIn) * time.Second).Unix()
	if err := h.store.SaveSession(ctx, session, sessionTTL); err != nil {
		return Session{}, err
	}
	return session, nil
}

func (h *Handler) importToken(w http.ResponseWriter, r *http.Request) {
	if !h.allowDevTokenImport(r) {
		http.NotFound(w, r)
		return
	}
	var req importTokenReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	imported, err := h.createImportedSession(r.Context(), req, sessionTTL)
	if err != nil {
		h.writeImportError(w, err)
		return
	}
	h.setSessionCookie(w, r, imported.Session.ID)
	h.writeAuthenticatedImport(w, imported)
}

func (h *Handler) prepareToken(w http.ResponseWriter, r *http.Request) {
	if !h.allowDevTokenImport(r) {
		http.NotFound(w, r)
		return
	}
	var req importTokenReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	imported, err := h.createImportedSession(r.Context(), req, devClaimTTL)
	if err != nil {
		h.writeImportError(w, err)
		return
	}
	claimID, err := randomID()
	if err != nil {
		http.Error(w, "claim generation failed", http.StatusInternalServerError)
		return
	}
	if err := h.store.SetDevClaim(r.Context(), claimID, imported.Session.ID, devClaimTTL); err != nil {
		http.Error(w, "claim save failed", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"claimUrl":         h.claimURL(r, claimID),
		"expiresInSeconds": int(devClaimTTL.Seconds()),
		"user":             imported.User,
	})
}

func (h *Handler) claimPreparedTokenRedirect(w http.ResponseWriter, r *http.Request) {
	if !h.allowDevTokenImport(r) {
		http.NotFound(w, r)
		return
	}
	claimID := strings.TrimSpace(r.URL.Query().Get("code"))
	if claimID == "" {
		h.redirectAuthStatus(w, r, "error", "missing_local_claim", "Local token claim code was missing. Run make twitch-local-auth again.")
		return
	}
	imported, err := h.claimPreparedSession(r.Context(), claimID)
	if err != nil {
		h.redirectAuthStatus(w, r, "error", "local_claim_failed", "The prepared local token was missing or expired. Run make twitch-local-auth again.")
		return
	}
	h.setSessionCookie(w, r, imported.Session.ID)
	h.redirectAuthStatus(w, r, "success", "connected", "Twitch connected with local token.")
}

func (h *Handler) claimPreparedToken(w http.ResponseWriter, r *http.Request) {
	if !h.allowDevTokenImport(r) {
		http.NotFound(w, r)
		return
	}
	imported, err := h.claimLatestPreparedSession(r.Context())
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "no prepared local token is waiting"})
		return
	}
	h.setSessionCookie(w, r, imported.Session.ID)
	h.writeAuthenticatedImport(w, imported)
}

func (h *Handler) startDeviceAuth(w http.ResponseWriter, r *http.Request) {
	if !h.allowDevTokenImport(r) {
		http.NotFound(w, r)
		return
	}
	if strings.TrimSpace(h.cfg.ClientID) == "" {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "twitch oauth client id is not configured"})
		return
	}
	started, err := h.beginDeviceAuth(r.Context())
	if err != nil {
		h.log.Warn("twitch device auth start failed", "err", err)
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "could not start Twitch device authorization"})
		return
	}
	requestID, err := randomID()
	if err != nil {
		http.Error(w, "request generation failed", http.StatusInternalServerError)
		return
	}
	ttl := time.Duration(started.ExpiresIn) * time.Second
	if ttl <= 0 {
		ttl = 30 * time.Minute
	}
	pollInterval := started.Interval
	if pollInterval <= 0 {
		pollInterval = 5
	}
	verificationURI := deviceVerificationURL(started.VerificationURI, started.UserCode, started.VerificationURIComplete)
	deviceAuth := DeviceAuth{
		DeviceCode:          started.DeviceCode,
		UserCode:            started.UserCode,
		VerificationURI:     verificationURI,
		PollIntervalSeconds: pollInterval,
		ExpiresAt:           time.Now().Add(ttl).Unix(),
	}
	if err := h.store.SaveDeviceAuth(r.Context(), requestID, deviceAuth, ttl); err != nil {
		http.Error(w, "device auth save failed", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"requestId":           requestID,
		"userCode":            deviceAuth.UserCode,
		"verificationUri":     deviceAuth.VerificationURI,
		"expiresInSeconds":    started.ExpiresIn,
		"pollIntervalSeconds": pollInterval,
	})
}

func (h *Handler) pollDeviceAuth(w http.ResponseWriter, r *http.Request) {
	if !h.allowDevTokenImport(r) {
		http.NotFound(w, r)
		return
	}
	var req deviceCodePollRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	req.RequestID = strings.TrimSpace(req.RequestID)
	if req.RequestID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "request_id is required"})
		return
	}
	deviceAuth, err := h.store.GetDeviceAuth(r.Context(), req.RequestID)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "device authorization session was missing or expired", "code": "expired"})
		return
	}
	status, tok, err := h.exchangeDeviceCode(r.Context(), deviceAuth.DeviceCode)
	if err != nil {
		h.log.Warn("twitch device auth poll failed", "err", err)
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "could not complete Twitch device authorization"})
		return
	}
	switch status {
	case "pending":
		writeJSON(w, http.StatusOK, map[string]any{"status": "pending"})
		return
	case "denied":
		_ = h.store.DeleteDeviceAuth(r.Context(), req.RequestID)
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "twitch authorization was denied", "code": "access_denied"})
		return
	case "expired":
		_ = h.store.DeleteDeviceAuth(r.Context(), req.RequestID)
		writeJSON(w, http.StatusGone, map[string]string{"error": "device authorization expired", "code": "expired"})
		return
	case "complete":
		_ = h.store.DeleteDeviceAuth(r.Context(), req.RequestID)
		imported, err := h.createImportedSession(r.Context(), importTokenReq{
			AccessToken:  tok.AccessToken,
			RefreshToken: tok.RefreshToken,
		}, sessionTTL)
		if err != nil {
			h.writeImportError(w, err)
			return
		}
		h.setSessionCookie(w, r, imported.Session.ID)
		writeJSON(w, http.StatusOK, map[string]any{
			"status":        "complete",
			"authenticated": true,
			"user":          imported.User,
			"scopes":        imported.Session.Scopes,
		})
		return
	default:
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "unexpected Twitch device authorization state"})
	}
}

func (h *Handler) writeAuthenticatedImport(w http.ResponseWriter, imported importedSession) {
	writeJSON(w, http.StatusOK, map[string]any{
		"authenticated": true,
		"user":          imported.User,
		"scopes":        imported.Session.Scopes,
	})
}

func (h *Handler) createImportedSession(ctx context.Context, req importTokenReq, ttl time.Duration) (importedSession, error) {
	if strings.TrimSpace(h.cfg.ClientID) == "" {
		return importedSession{}, errors.New("twitch oauth client id is not configured")
	}
	req.AccessToken = strings.TrimSpace(req.AccessToken)
	req.RefreshToken = strings.TrimSpace(req.RefreshToken)
	if req.AccessToken == "" {
		return importedSession{}, errors.New("access_token is required")
	}
	validated, err := h.validateToken(ctx, req.AccessToken)
	if err != nil {
		h.log.Warn("twitch token validate failed", "err", err)
		return importedSession{}, errors.New("token validation failed")
	}
	if validated.ClientID != h.cfg.ClientID {
		return importedSession{}, errors.New("token client_id does not match configured Twitch app")
	}
	user, err := h.user(ctx, req.AccessToken)
	if err != nil {
		h.log.Warn("twitch user lookup failed", "err", err)
		return importedSession{}, errors.New("twitch user lookup failed")
	}
	if validated.UserID != "" && user.ID != "" && validated.UserID != user.ID {
		return importedSession{}, errors.New("token validation did not match Twitch user lookup")
	}
	id, err := randomID()
	if err != nil {
		return importedSession{}, err
	}
	session := Session{
		ID:              id,
		UserID:          user.ID,
		Login:           user.Login,
		DisplayName:     user.DisplayName,
		ProfileImageURL: user.ProfileImageURL,
		AccessToken:     req.AccessToken,
		RefreshToken:    req.RefreshToken,
		Scopes:          validated.Scopes,
		ExpiresAt:       time.Now().Add(time.Duration(validated.ExpiresIn) * time.Second).Unix(),
	}
	if err := h.store.SaveSession(ctx, session, ttl); err != nil {
		return importedSession{}, err
	}
	return importedSession{Session: session, User: user}, nil
}

func (h *Handler) claimPreparedSession(ctx context.Context, claimID string) (importedSession, error) {
	sessionID, ok, err := h.store.TakeDevClaim(ctx, claimID)
	if err != nil {
		return importedSession{}, err
	}
	if !ok {
		return importedSession{}, ErrNotFound
	}
	return h.sessionForClaim(ctx, sessionID)
}

func (h *Handler) claimLatestPreparedSession(ctx context.Context) (importedSession, error) {
	sessionID, ok, err := h.store.TakeLatestDevClaim(ctx)
	if err != nil {
		return importedSession{}, err
	}
	if !ok {
		return importedSession{}, ErrNotFound
	}
	return h.sessionForClaim(ctx, sessionID)
}

func (h *Handler) sessionForClaim(ctx context.Context, sessionID string) (importedSession, error) {
	session, err := h.store.GetSession(ctx, sessionID)
	if err != nil {
		return importedSession{}, err
	}
	if err := h.store.SaveSession(ctx, session, sessionTTL); err != nil {
		return importedSession{}, err
	}
	return importedSession{
		Session: session,
		User: User{
			ID:              session.UserID,
			Login:           session.Login,
			DisplayName:     session.DisplayName,
			ProfileImageURL: session.ProfileImageURL,
		},
	}, nil
}

func (h *Handler) writeImportError(w http.ResponseWriter, err error) {
	switch err.Error() {
	case "twitch oauth client id is not configured":
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": err.Error()})
	case "access_token is required":
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
	case "token validation failed":
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": err.Error()})
	case "twitch user lookup failed":
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
	default:
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
	}
}

func (h *Handler) redirectAuthStatus(w http.ResponseWriter, r *http.Request, status, code, message string) {
	u, err := url.Parse(h.cfg.FrontendURL)
	if err != nil || u.Scheme == "" || u.Host == "" {
		u = &url.URL{Scheme: "http", Host: "localhost:5174"}
	}
	q := u.Query()
	q.Set("auth", status)
	if code != "" {
		q.Set("auth_code", code)
	}
	if message != "" {
		q.Set("auth_message", message)
	}
	u.RawQuery = q.Encode()
	http.Redirect(w, r, u.String(), http.StatusFound)
}

func (h *Handler) claimURL(r *http.Request, claimID string) string {
	base := requestOrigin(r)
	if base == "" {
		base = strings.TrimRight(h.cfg.FrontendURL, "/")
	}
	return strings.TrimRight(base, "/") + "/v1/auth/dev/claim?code=" + url.QueryEscape(claimID)
}

func (h *Handler) me(w http.ResponseWriter, r *http.Request) {
	canImportLocalToken := h.allowDevTokenImport(r)
	id, ok := h.SessionIDFromRequest(r)
	if !ok {
		writeJSON(w, http.StatusOK, map[string]any{
			"authenticated":       false,
			"canImportLocalToken": canImportLocalToken,
		})
		return
	}
	session, err := h.Session(r.Context(), id)
	if err != nil {
		h.clearSessionCookie(w)
		writeJSON(w, http.StatusOK, map[string]any{
			"authenticated":       false,
			"canImportLocalToken": canImportLocalToken,
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"authenticated":       true,
		"canImportLocalToken": canImportLocalToken,
		"user": User{
			ID:              session.UserID,
			Login:           session.Login,
			DisplayName:     session.DisplayName,
			ProfileImageURL: session.ProfileImageURL,
		},
		"scopes": session.Scopes,
	})
}

func (h *Handler) logout(w http.ResponseWriter, r *http.Request) {
	if id, ok := h.SessionIDFromRequest(r); ok {
		_ = h.store.DeleteSession(r.Context(), id)
	}
	h.clearSessionCookie(w)
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) followed(w http.ResponseWriter, r *http.Request) {
	id, ok := h.SessionIDFromRequest(r)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	session, err := h.Session(r.Context(), id)
	if err != nil {
		h.clearSessionCookie(w)
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	channels, err := h.followedChannels(r.Context(), session)
	if err != nil {
		h.log.Warn("followed channels failed", "err", err)
		http.Error(w, "followed channels failed", http.StatusBadGateway)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"channels": channels})
}

type debugResp struct {
	Ready                    bool     `json:"ready"`
	ClientIDConfigured       bool     `json:"clientIdConfigured"`
	ClientSecretConfigured   bool     `json:"clientSecretConfigured"`
	RedirectURL              string   `json:"redirectUrl"`
	FrontendURL              string   `json:"frontendUrl"`
	APIOrigin                string   `json:"apiOrigin"`
	RequestOrigin            string   `json:"requestOrigin,omitempty"`
	RedirectOrigin           string   `json:"redirectOrigin,omitempty"`
	FrontendOrigin           string   `json:"frontendOrigin,omitempty"`
	CookieName               string   `json:"cookieName"`
	CookieSameSite           string   `json:"cookieSameSite"`
	CookieSecureOnThisOrigin bool     `json:"cookieSecureOnThisOrigin"`
	CallbackMatchesAPI       bool     `json:"callbackMatchesApi"`
	FrontendMatchesRequest   bool     `json:"frontendMatchesRequest"`
	Warnings                 []string `json:"warnings"`
}

func (h *Handler) debug(w http.ResponseWriter, r *http.Request) {
	apiOrigin := requestOrigin(r)
	redirectOrigin := originOf(h.cfg.RedirectURL)
	frontendOrigin := originOf(h.cfg.FrontendURL)
	requestOriginHeader := strings.TrimSpace(r.Header.Get("Origin"))
	callbackMatchesAPI := redirectOrigin == "" || apiOrigin == "" || sameHostOrigin(redirectOrigin, apiOrigin)
	frontendMatchesRequest := requestOriginHeader == "" || frontendOrigin == "" || sameHostOrigin(frontendOrigin, requestOriginHeader)
	warnings := h.authWarnings(apiOrigin, requestOriginHeader, frontendOrigin)
	writeJSON(w, http.StatusOK, debugResp{
		Ready:                    h.ready(),
		ClientIDConfigured:       h.cfg.ClientID != "",
		ClientSecretConfigured:   h.cfg.ClientSecret != "",
		RedirectURL:              h.cfg.RedirectURL,
		FrontendURL:              h.cfg.FrontendURL,
		APIOrigin:                apiOrigin,
		RequestOrigin:            requestOriginHeader,
		RedirectOrigin:           redirectOrigin,
		FrontendOrigin:           frontendOrigin,
		CookieName:               cookieName,
		CookieSameSite:           strings.ToLower(strings.TrimSpace(h.cfg.CookieSameSite)),
		CookieSecureOnThisOrigin: isHTTPSOrigin(apiOrigin),
		CallbackMatchesAPI:       callbackMatchesAPI,
		FrontendMatchesRequest:   frontendMatchesRequest,
		Warnings:                 warnings,
	})
}

func (h *Handler) authWarnings(apiOrigin, requestOriginHeader, frontendOrigin string) []string {
	warnings := []string{}
	if !h.ready() {
		warnings = append(warnings, "Twitch app credentials are missing a client ID or client secret.")
	}
	if requestOriginHeader != "" && frontendOrigin != "" && !sameHostOrigin(requestOriginHeader, frontendOrigin) {
		warnings = append(warnings, "FRONTEND_ORIGIN does not match the browser origin making this request; credentialed auth calls may be blocked.")
	}
	if strings.EqualFold(strings.TrimSpace(h.cfg.CookieSameSite), "none") && !isHTTPSOrigin(apiOrigin) {
		warnings = append(warnings, "SameSite=None cookies require HTTPS in modern browsers; use SameSite=Lax for localhost development.")
	}
	return warnings
}

func (h *Handler) allowDevTokenImport(r *http.Request) bool {
	if !h.cfg.DevTokenImportEnabled {
		return false
	}
	host := strings.TrimSpace(r.Header.Get("X-Forwarded-Host"))
	if host == "" {
		host = r.Host
	}
	if isLoopbackHost(host) {
		return true
	}
	return isLoopbackOrigin(strings.TrimSpace(r.Header.Get("Origin")))
}

func (h *Handler) ready() bool {
	return h.cfg.ClientID != "" && h.cfg.ClientSecret != ""
}

func deviceVerificationURL(uri, userCode, completeURI string) string {
	if completeURI != "" {
		return completeURI
	}
	if strings.Contains(uri, "device-code=") {
		return uri
	}
	if uri == "" || userCode == "" {
		return uri
	}
	sep := "?"
	if strings.Contains(uri, "?") {
		sep = "&"
	}
	return uri + sep + "device-code=" + url.QueryEscape(userCode)
}

func (h *Handler) beginDeviceAuth(ctx context.Context) (deviceCodeStartResponse, error) {
	form := url.Values{}
	form.Set("client_id", h.cfg.ClientID)
	form.Set("scopes", h.cfg.Scopes)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, h.cfg.DeviceURL, strings.NewReader(form.Encode()))
	if err != nil {
		return deviceCodeStartResponse{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := h.client.Do(req)
	if err != nil {
		return deviceCodeStartResponse{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return deviceCodeStartResponse{}, errors.New("device endpoint returned " + resp.Status + ": " + string(bytes.TrimSpace(body)))
	}
	var out deviceCodeStartResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return deviceCodeStartResponse{}, err
	}
	if out.DeviceCode == "" || out.UserCode == "" || out.VerificationURI == "" || out.ExpiresIn <= 0 {
		return deviceCodeStartResponse{}, errors.New("device endpoint returned incomplete device code response")
	}
	return out, nil
}

func (h *Handler) exchangeDeviceCode(ctx context.Context, deviceCode string) (string, tokenResponse, error) {
	form := url.Values{}
	form.Set("client_id", h.cfg.ClientID)
	form.Set("scopes", h.cfg.Scopes)
	form.Set("device_code", deviceCode)
	form.Set("grant_type", deviceCodeGrantType)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, h.cfg.TokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", tokenResponse{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := h.client.Do(req)
	if err != nil {
		return "", tokenResponse{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		var out tokenResponse
		if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
			return "", tokenResponse{}, err
		}
		if out.AccessToken == "" || out.ExpiresIn <= 0 {
			return "", tokenResponse{}, errors.New("token endpoint returned incomplete token")
		}
		return "complete", out, nil
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
	var deviceErr deviceCodeTokenError
	if err := json.Unmarshal(body, &deviceErr); err == nil {
		switch strings.ToLower(strings.TrimSpace(deviceErr.Message)) {
		case "authorization_pending":
			return "pending", tokenResponse{}, nil
		case "access_denied":
			return "denied", tokenResponse{}, nil
		case "invalid device code":
			return "expired", tokenResponse{}, nil
		case "slow_down":
			return "pending", tokenResponse{}, nil
		}
	}
	return "", tokenResponse{}, errors.New("token endpoint returned " + resp.Status + ": " + string(bytes.TrimSpace(body)))
}

func (h *Handler) validateToken(ctx context.Context, accessToken string) (tokenValidation, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, h.cfg.ValidateURL, nil)
	if err != nil {
		return tokenValidation{}, err
	}
	req.Header.Set("Authorization", "OAuth "+accessToken)
	resp, err := h.client.Do(req)
	if err != nil {
		return tokenValidation{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return tokenValidation{}, errors.New("validate endpoint returned " + resp.Status + ": " + string(bytes.TrimSpace(body)))
	}
	var out tokenValidation
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return tokenValidation{}, err
	}
	if out.ClientID == "" || out.UserID == "" || out.ExpiresIn <= 0 {
		return tokenValidation{}, errors.New("validate endpoint returned incomplete token")
	}
	return out, nil
}

func (h *Handler) refresh(ctx context.Context, refreshToken string) (tokenResponse, error) {
	form := url.Values{}
	form.Set("client_id", h.cfg.ClientID)
	form.Set("client_secret", h.cfg.ClientSecret)
	form.Set("grant_type", "refresh_token")
	form.Set("refresh_token", refreshToken)
	return h.postToken(ctx, form)
}

func (h *Handler) postToken(ctx context.Context, form url.Values) (tokenResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, h.cfg.TokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return tokenResponse{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := h.client.Do(req)
	if err != nil {
		return tokenResponse{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return tokenResponse{}, errors.New("token endpoint returned " + resp.Status + ": " + string(bytes.TrimSpace(body)))
	}
	var out tokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return tokenResponse{}, err
	}
	if out.AccessToken == "" || out.ExpiresIn <= 0 {
		return tokenResponse{}, errors.New("token endpoint returned incomplete token")
	}
	return out, nil
}

func (h *Handler) user(ctx context.Context, accessToken string) (User, error) {
	var resp struct {
		Data []User `json:"data"`
	}
	if err := h.getHelix(ctx, accessToken, "/users", &resp); err != nil {
		return User{}, err
	}
	if len(resp.Data) == 0 {
		return User{}, errors.New("twitch returned no user")
	}
	return resp.Data[0], nil
}

func (h *Handler) followedChannels(ctx context.Context, session Session) ([]FollowedChannel, error) {
	byLogin := map[string]FollowedChannel{}
	order := make([]string, 0, 64)

	var live struct {
		Data []struct {
			UserID       string `json:"user_id"`
			UserLogin    string `json:"user_login"`
			UserName     string `json:"user_name"`
			GameName     string `json:"game_name"`
			Title        string `json:"title"`
			ViewerCount  int    `json:"viewer_count"`
			ThumbnailURL string `json:"thumbnail_url"`
		} `json:"data"`
	}
	if err := h.getHelix(ctx, session.AccessToken, "/streams/followed?user_id="+url.QueryEscape(session.UserID)+"&first=50", &live); err != nil {
		return nil, err
	}
	for _, item := range live.Data {
		login := strings.ToLower(item.UserLogin)
		if login == "" {
			continue
		}
		byLogin[login] = FollowedChannel{
			ID:           item.UserID,
			Login:        login,
			DisplayName:  item.UserName,
			IsLive:       true,
			Title:        item.Title,
			Category:     item.GameName,
			Viewers:      item.ViewerCount,
			ThumbnailURL: item.ThumbnailURL,
		}
		order = append(order, login)
	}

	var followed struct {
		Data []struct {
			BroadcasterID    string `json:"broadcaster_id"`
			BroadcasterLogin string `json:"broadcaster_login"`
			BroadcasterName  string `json:"broadcaster_name"`
		} `json:"data"`
	}
	if err := h.getHelix(ctx, session.AccessToken, "/channels/followed?user_id="+url.QueryEscape(session.UserID)+"&first=50", &followed); err != nil {
		return nil, err
	}
	for _, item := range followed.Data {
		login := strings.ToLower(item.BroadcasterLogin)
		if login == "" {
			continue
		}
		if _, exists := byLogin[login]; !exists {
			order = append(order, login)
			byLogin[login] = FollowedChannel{
				ID:          item.BroadcasterID,
				Login:       login,
				DisplayName: item.BroadcasterName,
			}
		}
	}

	h.enrichFollowedProfiles(ctx, session.AccessToken, byLogin)

	out := make([]FollowedChannel, 0, len(order))
	seen := map[string]struct{}{}
	for _, login := range order {
		if _, exists := seen[login]; exists {
			continue
		}
		seen[login] = struct{}{}
		out = append(out, byLogin[login])
	}
	return out, nil
}

func (h *Handler) enrichFollowedProfiles(ctx context.Context, accessToken string, byLogin map[string]FollowedChannel) {
	ids := make([]string, 0, len(byLogin))
	for _, channel := range byLogin {
		if channel.ProfileImage != "" || channel.ID == "" {
			continue
		}
		ids = append(ids, channel.ID)
	}
	for start := 0; start < len(ids); start += 100 {
		end := start + 100
		if end > len(ids) {
			end = len(ids)
		}
		values := url.Values{}
		for _, id := range ids[start:end] {
			values.Add("id", id)
		}
		var users struct {
			Data []struct {
				ID              string `json:"id"`
				Login           string `json:"login"`
				DisplayName     string `json:"display_name"`
				ProfileImageURL string `json:"profile_image_url"`
			} `json:"data"`
		}
		if err := h.getHelix(ctx, accessToken, "/users?"+values.Encode(), &users); err != nil {
			h.log.Debug("followed profile enrichment failed", "err", err)
			continue
		}
		for _, user := range users.Data {
			login := strings.ToLower(user.Login)
			if login == "" {
				continue
			}
			channel, ok := byLogin[login]
			if !ok {
				continue
			}
			if channel.DisplayName == "" {
				channel.DisplayName = user.DisplayName
			}
			channel.ProfileImage = user.ProfileImageURL
			byLogin[login] = channel
		}
	}
}

func (h *Handler) getHelix(ctx context.Context, accessToken, path string, dst any) error {
	base := strings.TrimRight(h.cfg.APIURL, "/")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Client-Id", h.cfg.ClientID)
	resp, err := h.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return errors.New("helix returned " + resp.Status + ": " + string(bytes.TrimSpace(body)))
	}
	return json.NewDecoder(resp.Body).Decode(dst)
}

func (h *Handler) setSessionCookie(w http.ResponseWriter, r *http.Request, id string) {
	http.SetCookie(w, &http.Cookie{
		Name:     cookieName,
		Value:    id + "." + h.sign(id),
		Path:     "/",
		MaxAge:   int(sessionTTL.Seconds()),
		HttpOnly: true,
		SameSite: h.sameSiteMode(),
		Secure:   r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https",
	})
}

func (h *Handler) clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     cookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: h.sameSiteMode(),
	})
}

func (h *Handler) sameSiteMode() http.SameSite {
	switch strings.ToLower(strings.TrimSpace(h.cfg.CookieSameSite)) {
	case "none":
		return http.SameSiteNoneMode
	case "strict":
		return http.SameSiteStrictMode
	default:
		return http.SameSiteLaxMode
	}
}

func (h *Handler) sign(value string) string {
	mac := hmac.New(sha256.New, []byte(h.cfg.CookieSecret))
	mac.Write([]byte(value))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func (h *Handler) verifyCookie(value string) (string, bool) {
	id, sig, ok := strings.Cut(value, ".")
	if !ok || id == "" || sig == "" {
		return "", false
	}
	expected := h.sign(id)
	if !hmac.Equal([]byte(sig), []byte(expected)) {
		return "", false
	}
	return id, true
}

func requestOrigin(r *http.Request) string {
	proto := strings.TrimSpace(r.Header.Get("X-Forwarded-Proto"))
	if proto == "" {
		if r.TLS != nil {
			proto = "https"
		} else {
			proto = "http"
		}
	}
	host := strings.TrimSpace(r.Header.Get("X-Forwarded-Host"))
	if host == "" {
		host = r.Host
	}
	if host == "" {
		return ""
	}
	return strings.ToLower(proto + "://" + host)
}

func isLoopbackOrigin(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	return isLoopbackHost(u.Host)
}

func isLoopbackHost(host string) bool {
	host = strings.TrimSpace(host)
	if host == "" {
		return false
	}
	if strings.Contains(host, "://") {
		u, err := url.Parse(host)
		if err == nil {
			host = u.Host
		}
	}
	if parsedHost, _, err := net.SplitHostPort(host); err == nil {
		host = parsedHost
	}
	host = strings.Trim(host, "[]")
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func originOf(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return ""
	}
	return strings.ToLower(u.Scheme + "://" + u.Host)
}

func sameHostOrigin(a, b string) bool {
	au, errA := url.Parse(a)
	bu, errB := url.Parse(b)
	if errA != nil || errB != nil {
		return false
	}
	return strings.EqualFold(au.Host, bu.Host) && strings.EqualFold(au.Scheme, bu.Scheme)
}

func isHTTPSOrigin(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && strings.EqualFold(u.Scheme, "https")
}

func randomID() (string, error) {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b[:]), nil
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
