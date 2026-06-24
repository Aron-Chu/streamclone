package analytics

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestBearerTokenFromRequest(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer spdev_abc123")
	if got := bearerTokenFromRequest(req); got != "spdev_abc123" {
		t.Fatalf("token = %q", got)
	}
	req.Header.Set("Authorization", "Basic x")
	if got := bearerTokenFromRequest(req); got != "" {
		t.Fatalf("expected empty for non-Bearer, got %q", got)
	}
}

func TestHashPulseDeviceTokenStable(t *testing.T) {
	a := hashPulseDeviceToken("spdev_test")
	b := hashPulseDeviceToken("spdev_test")
	if a != b || a == "" {
		t.Fatalf("hash not stable: %q vs %q", a, b)
	}
}

func TestExtensionAuthDeviceInvalidBetaKey(t *testing.T) {
	h := &Handler{
		pulseHosted: PulseHostedConfig{Hosted: true, BetaKeys: []string{"secret-one"}},
		store:       &Store{},
	}
	r := chi.NewRouter()
	h.ExtensionRoutes(r)

	req := httptest.NewRequest(http.MethodPost, "/v1/extension/auth/device", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestExtensionAuthDeviceMintAndMe(t *testing.T) {
	ctx, store := setupDeviceAuthStore(t)
	const betaKey = "secret-one"
	h := &Handler{
		store: store,
		pulseHosted: PulseHostedConfig{
			Hosted:                  true,
			BetaKeys:                []string{betaKey},
			MaxActiveChannels:       10,
			MaxChannelsPerPrincipal: 5,
			WatchRatePerMin:         30,
			BackfillRatePerHour:     2,
		},
	}
	r := chi.NewRouter()
	h.ExtensionRoutes(r)

	mintReq := httptest.NewRequest(http.MethodPost, "/v1/extension/auth/device", nil)
	mintReq = mintReq.WithContext(ctx)
	mintReq.Header.Set("X-Streamclone-Beta-Key", betaKey)
	mintRec := httptest.NewRecorder()
	r.ServeHTTP(mintRec, mintReq)
	if mintRec.Code != http.StatusCreated {
		t.Fatalf("mint status = %d body=%s", mintRec.Code, mintRec.Body.String())
	}
	var mintBody extensionAuthDeviceResponse
	if err := json.Unmarshal(mintRec.Body.Bytes(), &mintBody); err != nil {
		t.Fatalf("decode mint: %v", err)
	}
	if mintBody.Token == "" || mintBody.DeviceID == "" || mintBody.PrincipalKind != "device" {
		t.Fatalf("mint body = %#v", mintBody)
	}

	meReq := httptest.NewRequest(http.MethodGet, "/v1/extension/me", nil)
	meReq = meReq.WithContext(ctx)
	meReq.Header.Set("Authorization", "Bearer "+mintBody.Token)
	meRec := httptest.NewRecorder()
	r.ServeHTTP(meRec, meReq)
	if meRec.Code != http.StatusOK {
		t.Fatalf("me status = %d body=%s", meRec.Code, meRec.Body.String())
	}
	var meBody extensionMeResponse
	if err := json.Unmarshal(meRec.Body.Bytes(), &meBody); err != nil {
		t.Fatalf("decode me: %v", err)
	}
	if meBody.PrincipalKind != "device" || meBody.PrincipalID != mintBody.DeviceID {
		t.Fatalf("me principal = %#v, want device %q", meBody, mintBody.DeviceID)
	}
	if meBody.Caps.MaxActiveChannels != 10 {
		t.Fatalf("caps = %#v", meBody.Caps)
	}
}

func TestExtensionMeBetaKeyPrincipal(t *testing.T) {
	ctx, store := setupDeviceAuthStore(t)
	const betaKey = "secret-one"
	h := &Handler{
		store:       store,
		pulseHosted: PulseHostedConfig{Hosted: true, BetaKeys: []string{betaKey}, MaxActiveChannels: 10},
	}
	r := chi.NewRouter()
	h.ExtensionRoutes(r)

	req := httptest.NewRequest(http.MethodGet, "/v1/extension/me", nil)
	req = req.WithContext(ctx)
	req.Header.Set("X-Streamclone-Beta-Key", betaKey)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	var body extensionMeResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.PrincipalKind != "beta" {
		t.Fatalf("kind = %q", body.PrincipalKind)
	}
}

func TestResolvePulseDeviceTokenExpired(t *testing.T) {
	ctx, store := setupDeviceAuthStore(t)
	betaID := hashPulseBetaKey("secret-one")
	raw, record, err := store.MintPulseDeviceToken(ctx, betaID, "test", time.Hour)
	if err != nil {
		t.Fatalf("mint: %v", err)
	}
	_, err = store.db.Exec(ctx, `UPDATE pulse_device_tokens SET expires_at = NOW() - INTERVAL '1 minute' WHERE device_id = $1`, record.DeviceID)
	if err != nil {
		t.Fatalf("expire: %v", err)
	}
	if _, ok := store.ResolvePulseDeviceToken(ctx, raw); ok {
		t.Fatal("expected expired token to fail")
	}
}

func setupDeviceAuthStore(t *testing.T) (context.Context, *Store) {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	if os.Getenv("INTEGRATION") == "" {
		t.Skip("set INTEGRATION=1 to run device auth integration tests")
	}
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://app:test@localhost:15432/emotes?sslmode=disable"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	admin, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("admin pgxpool.New: %v", err)
	}
	t.Cleanup(admin.Close)
	schema := fmt.Sprintf("pulse_device_auth_test_%d", time.Now().UnixNano())
	if _, err := admin.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatalf("create schema: %v", err)
	}
	t.Cleanup(func() {
		_, _ = admin.Exec(context.Background(), `DROP SCHEMA IF EXISTS `+schema+` CASCADE`)
	})

	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatalf("parse config: %v", err)
	}
	if cfg.ConnConfig.RuntimeParams == nil {
		cfg.ConnConfig.RuntimeParams = map[string]string{}
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatalf("test pgxpool.NewWithConfig: %v", err)
	}
	t.Cleanup(pool.Close)

	store := NewStore(pool)
	_, err = pool.Exec(ctx, `
		CREATE TABLE pulse_device_tokens (
			device_id TEXT PRIMARY KEY,
			token_hash TEXT NOT NULL UNIQUE,
			beta_principal_id TEXT NOT NULL,
			label TEXT NOT NULL DEFAULT '',
			expires_at TIMESTAMPTZ NOT NULL,
			revoked_at TIMESTAMPTZ,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			last_seen_at TIMESTAMPTZ
		)
	`)
	if err != nil {
		t.Fatalf("create table: %v", err)
	}
	return ctx, store
}
