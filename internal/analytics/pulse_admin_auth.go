package analytics

import (
	"context"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"streamclone/internal/config"
)

const cfAccessJWTHeader = "Cf-Access-Jwt-Assertion"

type pulseOperatorCtxKey struct{}

// PulseOperator identifies an authenticated admin caller (Access JWT or break-glass token).
type PulseOperator struct {
	Email  string
	Source string // access_jwt | archive_token | local_bypass
}

type cfAccessCerts struct {
	Keys []cfAccessKey `json:"keys"`
}

type cfAccessKey struct {
	Kid string `json:"kid"`
	Kty string `json:"kty"`
	Alg string `json:"alg"`
	N   string `json:"n"`
	E   string `json:"e"`
}

var (
	cfCertCacheMu sync.Mutex
	cfCertCache   = map[string]cfAccessCerts{}
	cfCertFetched = map[string]time.Time{}
)

// cfAccessValidator allows tests to inject JWT validation without network.
var cfAccessValidator func(teamDomain, aud, token string) (email string, ok bool)

func pulseOperatorFromContext(ctx context.Context) (PulseOperator, bool) {
	op, ok := ctx.Value(pulseOperatorCtxKey{}).(PulseOperator)
	return op, ok && op.Source != ""
}

func newPulseAdminAuthConfig(cfg config.Config, hosted bool) pulseAdminAuthConfig {
	return pulseAdminAuthConfig{
		hosted:           hosted,
		teamDomain:       strings.TrimSpace(cfg.PulseCFAccessTeamDomain),
		audiences:        parseCSVEnv(cfg.PulseCFAccessAud),
		archiveToken:     loadAdminArchiveToken(cfg),
		requireArchive:   cfg.AdminArchiveRequireToken,
		localBypass:      cfg.PulseAdminLocalBypass && !hosted,
	}
}

type pulseAdminAuthConfig struct {
	hosted         bool
	teamDomain     string
	audiences      []string
	archiveToken   string
	requireArchive bool
	localBypass    bool
}

func (c pulseAdminAuthConfig) authorized(r *http.Request) (PulseOperator, bool) {
	if c.localBypass {
		return PulseOperator{Email: "local-dev", Source: "local_bypass"}, true
	}
	if token := strings.TrimSpace(r.Header.Get(adminArchiveHeader)); token != "" {
		if c.archiveToken != "" && token == c.archiveToken {
			return PulseOperator{Email: "archive-token", Source: "archive_token"}, true
		}
	}
	jwtRaw := strings.TrimSpace(r.Header.Get(cfAccessJWTHeader))
	if jwtRaw == "" || c.teamDomain == "" || len(c.audiences) == 0 {
		return PulseOperator{}, false
	}
	if cfAccessValidator != nil {
		if email, ok := cfAccessValidator(c.teamDomain, strings.Join(c.audiences, ","), jwtRaw); ok {
			if email == "" {
				email = "access-operator"
			}
			return PulseOperator{Email: email, Source: "access_jwt"}, true
		}
		return PulseOperator{}, false
	}
	email, err := validateCloudflareAccessJWT(r.Context(), c.teamDomain, c.audiences, jwtRaw)
	if err != nil {
		return PulseOperator{}, false
	}
	return PulseOperator{Email: email, Source: "access_jwt"}, true
}

// PulseAdminAuthMiddleware gates /v1/admin/pulse/* — fail closed; never partial operator payloads.
func PulseAdminAuthMiddleware(cfg config.Config, hosted bool, next http.Handler) http.Handler {
	authCfg := newPulseAdminAuthConfig(cfg, hosted)
	limiter := newAdminRateLimiter(30, time.Minute)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		op, ok := authCfg.authorized(r)
		if !ok {
			writeJSON(w, http.StatusUnauthorized, map[string]string{
				"error": "unauthorized",
				"hint":  "Cloudflare Access session or X-Admin-Archive-Token required",
			})
			return
		}
		key := op.Email
		if key == "" {
			key = op.Source
		}
		if !limiter.allow(key) {
			writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": "rate_limited"})
			return
		}
		ctx := context.WithValue(r.Context(), pulseOperatorCtxKey{}, op)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func parseCSVEnv(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if s := strings.TrimSpace(p); s != "" {
			out = append(out, s)
		}
	}
	return out
}

func validateCloudflareAccessJWT(ctx context.Context, teamDomain string, audiences []string, token string) (string, error) {
	teamDomain = strings.TrimSpace(strings.TrimSuffix(teamDomain, "/"))
	if teamDomain == "" || len(audiences) == 0 {
		return "", errors.New("access not configured")
	}
	parsed, err := jwt.Parse(token, func(t *jwt.Token) (any, error) {
		if t.Method.Alg() != jwt.SigningMethodRS256.Alg() {
			return nil, fmt.Errorf("unexpected alg %s", t.Method.Alg())
		}
		kid, _ := t.Header["kid"].(string)
		pub, err := cfAccessPublicKey(ctx, teamDomain, kid)
		if err != nil {
			return nil, err
		}
		return pub, nil
	}, jwt.WithValidMethods([]string{"RS256"}))
	if err != nil || !parsed.Valid {
		return "", errors.New("invalid jwt")
	}
	claims, ok := parsed.Claims.(jwt.MapClaims)
	if !ok {
		return "", errors.New("invalid claims")
	}
	audOK := false
	switch aud := claims["aud"].(type) {
	case string:
		audOK = audienceMatch(aud, audiences)
	case []any:
		for _, item := range aud {
			if s, ok := item.(string); ok && audienceMatch(s, audiences) {
				audOK = true
				break
			}
		}
	}
	if !audOK {
		return "", errors.New("aud mismatch")
	}
	email, _ := claims["email"].(string)
	if email == "" {
		email, _ = claims["sub"].(string)
	}
	return email, nil
}

func audienceMatch(got string, want []string) bool {
	for _, w := range want {
		if got == w {
			return true
		}
	}
	return false
}

func cfAccessPublicKey(ctx context.Context, teamDomain, kid string) (*rsa.PublicKey, error) {
	certs, err := fetchCFAccessCerts(ctx, teamDomain)
	if err != nil {
		return nil, err
	}
	for _, k := range certs.Keys {
		if k.Kid != kid || k.Kty != "RSA" {
			continue
		}
		return rsaPublicKeyFromJWK(k.N, k.E)
	}
	return nil, fmt.Errorf("kid %q not found", kid)
}

func fetchCFAccessCerts(ctx context.Context, teamDomain string) (cfAccessCerts, error) {
	cfCertCacheMu.Lock()
	if cached, ok := cfCertCache[teamDomain]; ok {
		if time.Since(cfCertFetched[teamDomain]) < 10*time.Minute {
			cfCertCacheMu.Unlock()
			return cached, nil
		}
	}
	cfCertCacheMu.Unlock()

	url := fmt.Sprintf("https://%s/cdn-cgi/access/certs", teamDomain)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return cfAccessCerts{}, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return cfAccessCerts{}, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return cfAccessCerts{}, err
	}
	if resp.StatusCode != http.StatusOK {
		return cfAccessCerts{}, fmt.Errorf("cf certs http %d", resp.StatusCode)
	}
	var certs cfAccessCerts
	if err := json.Unmarshal(body, &certs); err != nil {
		return cfAccessCerts{}, err
	}
	cfCertCacheMu.Lock()
	cfCertCache[teamDomain] = certs
	cfCertFetched[teamDomain] = time.Now()
	cfCertCacheMu.Unlock()
	return certs, nil
}

func rsaPublicKeyFromJWK(nB64, eB64 string) (*rsa.PublicKey, error) {
	nb, err := base64.RawURLEncoding.DecodeString(nB64)
	if err != nil {
		return nil, err
	}
	eb, err := base64.RawURLEncoding.DecodeString(eB64)
	if err != nil {
		return nil, err
	}
	eInt := 0
	for _, b := range eb {
		eInt = eInt<<8 + int(b)
	}
	if eInt == 0 {
		eInt = 65537
	}
	return &rsa.PublicKey{
		N: new(big.Int).SetBytes(nb),
		E: eInt,
	}, nil
}

func pulseAdminLocalBypassFromEnv() bool {
	v := strings.TrimSpace(os.Getenv("PULSE_ADMIN_LOCAL_BYPASS"))
	return v == "1" || strings.EqualFold(v, "true")
}
