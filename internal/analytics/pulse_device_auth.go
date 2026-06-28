package analytics

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

const (
	pulseDeviceTokenPrefix = "spdev_"
	pulseDeviceIDPrefix    = "dev_"
	defaultDeviceTokenTTL  = 90 * 24 * time.Hour
)

type PulseDeviceTokenRecord struct {
	DeviceID        string
	BetaPrincipalID string
	Label           string
	ExpiresAt       time.Time
}

type extensionAuthDeviceRequest struct {
	Label string `json:"label"`
}

type extensionAuthDeviceResponse struct {
	Token         string    `json:"token"`
	DeviceID      string    `json:"deviceId"`
	ExpiresAt     time.Time `json:"expiresAt"`
	PrincipalKind string    `json:"principalKind"`
}

type extensionMeResponse struct {
	PrincipalID    string          `json:"principalId"`
	PrincipalKind  string          `json:"principalKind"`
	WatchlistCount int             `json:"watchlistCount"`
	Caps           extensionMeCaps `json:"caps"`
}

type extensionMeCaps struct {
	MaxActiveChannels       int `json:"maxActiveChannels"`
	MaxChannelsPerPrincipal int `json:"maxChannelsPerPrincipal"`
	WatchRatePerMin         int `json:"watchRatePerMin"`
	BackfillRatePerHour     int `json:"backfillRatePerHour"`
}

func deviceTokenTTLFromEnv() time.Duration {
	raw := strings.TrimSpace(os.Getenv("PULSE_DEVICE_TOKEN_TTL"))
	if raw == "" {
		return defaultDeviceTokenTTL
	}
	d, err := time.ParseDuration(raw)
	if err != nil || d <= 0 {
		return defaultDeviceTokenTTL
	}
	return d
}

func hashPulseDeviceToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

func bearerTokenFromRequest(r *http.Request) string {
	auth := strings.TrimSpace(r.Header.Get("Authorization"))
	if auth == "" {
		return ""
	}
	const prefix = "Bearer "
	if !strings.HasPrefix(auth, prefix) {
		return ""
	}
	return strings.TrimSpace(strings.TrimPrefix(auth, prefix))
}

func (s *Store) MintPulseDeviceToken(ctx context.Context, betaPrincipalID, label string, ttl time.Duration) (rawToken string, record PulseDeviceTokenRecord, err error) {
	if s == nil || s.db == nil {
		return "", PulseDeviceTokenRecord{}, errors.New("store unavailable")
	}
	betaPrincipalID = strings.TrimSpace(betaPrincipalID)
	if betaPrincipalID == "" {
		return "", PulseDeviceTokenRecord{}, errors.New("beta principal required")
	}
	if ttl <= 0 {
		ttl = defaultDeviceTokenTTL
	}
	label = strings.TrimSpace(label)
	if len(label) > 120 {
		label = label[:120]
	}

	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", PulseDeviceTokenRecord{}, err
	}
	rawToken = pulseDeviceTokenPrefix + hex.EncodeToString(raw)

	deviceRaw := make([]byte, 16)
	if _, err := rand.Read(deviceRaw); err != nil {
		return "", PulseDeviceTokenRecord{}, err
	}
	deviceID := pulseDeviceIDPrefix + hex.EncodeToString(deviceRaw)
	expiresAt := time.Now().UTC().Add(ttl)

	_, err = s.db.Exec(ctx, `
		INSERT INTO pulse_device_tokens (device_id, token_hash, beta_principal_id, label, expires_at)
		VALUES ($1, $2, $3, $4, $5)
	`, deviceID, hashPulseDeviceToken(rawToken), betaPrincipalID, label, expiresAt)
	if err != nil {
		return "", PulseDeviceTokenRecord{}, err
	}
	return rawToken, PulseDeviceTokenRecord{
		DeviceID:        deviceID,
		BetaPrincipalID: betaPrincipalID,
		Label:           label,
		ExpiresAt:       expiresAt,
	}, nil
}

func (s *Store) ResolvePulseDeviceToken(ctx context.Context, rawToken string) (PulsePrincipal, bool) {
	if s == nil || s.db == nil {
		return PulsePrincipal{}, false
	}
	rawToken = strings.TrimSpace(rawToken)
	if rawToken == "" {
		return PulsePrincipal{}, false
	}
	var deviceID string
	var expiresAt time.Time
	err := s.db.QueryRow(ctx, `
		SELECT device_id, expires_at
		FROM pulse_device_tokens
		WHERE token_hash = $1
		  AND revoked_at IS NULL
		  AND expires_at > NOW()
	`, hashPulseDeviceToken(rawToken)).Scan(&deviceID, &expiresAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return PulsePrincipal{}, false
		}
		return PulsePrincipal{}, false
	}
	_, _ = s.db.Exec(ctx, `UPDATE pulse_device_tokens SET last_seen_at = NOW() WHERE device_id = $1`, deviceID)
	return PulsePrincipal{ID: deviceID, Kind: "device"}, true
}

func (h *Handler) pulseHostedAuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !h.pulseHosted.Hosted {
			next.ServeHTTP(w, r)
			return
		}
		if token := bearerTokenFromRequest(r); token != "" && h.store != nil {
			if principal, ok := h.store.ResolvePulseDeviceToken(r.Context(), token); ok {
				ctx := context.WithValue(r.Context(), pulsePrincipalCtxKey{}, principal)
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}
		}
		if id, kind, ok := principalFromRequest(r, h.pulseHosted); ok {
			ctx := context.WithValue(r.Context(), pulsePrincipalCtxKey{}, PulsePrincipal{ID: id, Kind: kind})
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}
		principal := guestPulsePrincipal(r)
		ctx := context.WithValue(r.Context(), pulsePrincipalCtxKey{}, principal)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (h *Handler) extensionAuthDevice(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "store_unavailable"})
		return
	}
	principal, ok := pulsePrincipalFromContext(r.Context())
	if !ok || strings.TrimSpace(principal.ID) == "" {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	if h.pulseHosted.Hosted && principal.Kind == "guest" {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	var req extensionAuthDeviceRequest
	if r.Body != nil {
		dec := json.NewDecoder(r.Body)
		dec.DisallowUnknownFields()
		if err := dec.Decode(&req); err != nil && !errors.Is(err, io.EOF) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_json"})
			return
		}
	}
	rawToken, record, err := h.store.MintPulseDeviceToken(r.Context(), principal.ID, req.Label, deviceTokenTTLFromEnv())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "mint_failed"})
		return
	}
	writeJSON(w, http.StatusCreated, extensionAuthDeviceResponse{
		Token:         rawToken,
		DeviceID:      record.DeviceID,
		ExpiresAt:     record.ExpiresAt,
		PrincipalKind: "device",
	})
}

func (h *Handler) extensionMe(w http.ResponseWriter, r *http.Request) {
	principal, ok := pulsePrincipalFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	cfg := h.pulseHosted
	watchlistCount := 0
	if h.store != nil {
		if n, err := h.store.CountPulseWatchlist(r.Context(), principal.ID); err == nil {
			watchlistCount = n
		}
	}
	writeJSON(w, http.StatusOK, extensionMeResponse{
		PrincipalID:    principal.ID,
		PrincipalKind:  principal.Kind,
		WatchlistCount: watchlistCount,
		Caps: extensionMeCaps{
			MaxActiveChannels:       cfg.MaxActiveChannels,
			MaxChannelsPerPrincipal: cfg.MaxChannelsPerPrincipal,
			WatchRatePerMin:         cfg.WatchRatePerMin,
			BackfillRatePerHour:     cfg.BackfillRatePerHour,
		},
	})
}
