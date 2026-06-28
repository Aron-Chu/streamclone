package analytics

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net"
	"net/http"
	"os"
	"strings"
	"time"
)

const pulseBetaKeyHeader = "X-Streamclone-Beta-Key"

type pulsePrincipalCtxKey struct{}

// PulsePrincipal identifies a hosted beta user or future device/user identity.
type PulsePrincipal struct {
	ID   string
	Kind string
}

// PulseHostedConfig gates public extension endpoints behind a beta key.
type PulseHostedConfig struct {
	Hosted                  bool
	BetaKeys                []string
	MaxActiveChannels       int
	MaxChannelsPerPrincipal int
	WatchRatePerMin         int
	BackfillRatePerHour     int
	IdleTTL                 time.Duration
}

func PulseHostedConfigFromEnv() PulseHostedConfig {
	hosted := strings.EqualFold(strings.TrimSpace(os.Getenv("PULSE_HOSTED_MODE")), "true") ||
		os.Getenv("PULSE_HOSTED_MODE") == "1"
	idleTTL := 15 * time.Minute
	if raw := strings.TrimSpace(os.Getenv("PULSE_IDLE_TTL")); raw != "" {
		if d, err := time.ParseDuration(raw); err == nil && d > 0 {
			idleTTL = d
		}
	}
	return PulseHostedConfig{
		Hosted:                  hosted,
		BetaKeys:                ParsePulseBetaKeys(os.Getenv("PULSE_BETA_KEYS")),
		MaxActiveChannels:       envIntDefault("PULSE_MAX_ACTIVE_CHANNELS", 0),
		MaxChannelsPerPrincipal: envIntDefault("PULSE_MAX_CHANNELS_PER_PRINCIPAL", 0),
		WatchRatePerMin:         envIntDefault("PULSE_WATCH_RATE_PER_MIN", 0),
		BackfillRatePerHour:     envIntDefault("PULSE_BACKFILL_RATE_PER_HOUR", 0),
		IdleTTL:                 idleTTL,
	}
}

func envIntDefault(key string, fallback int) int {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	n := 0
	for _, ch := range raw {
		if ch < '0' || ch > '9' {
			return fallback
		}
		n = n*10 + int(ch-'0')
	}
	return n
}

func ParsePulseBetaKeys(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	keys := make([]string, 0, len(parts))
	for _, part := range parts {
		if k := strings.TrimSpace(part); k != "" {
			keys = append(keys, k)
		}
	}
	return keys
}

// BetaKeyRequired is false — hosted Pulse uses guest principals when no beta key is sent.
func (c PulseHostedConfig) BetaKeyRequired() bool {
	return false
}

func (c PulseHostedConfig) authorized(r *http.Request) bool {
	if !c.Hosted {
		return true
	}
	if len(c.BetaKeys) == 0 {
		return false
	}
	got := strings.TrimSpace(r.Header.Get(pulseBetaKeyHeader))
	for _, want := range c.BetaKeys {
		if got != "" && got == want {
			return true
		}
	}
	return false
}

func hashPulseBetaKey(key string) string {
	sum := sha256.Sum256([]byte(key))
	return hex.EncodeToString(sum[:])
}

func principalFromRequest(r *http.Request, cfg PulseHostedConfig) (id string, kind string, ok bool) {
	if !cfg.Hosted {
		return "", "", false
	}
	got := strings.TrimSpace(r.Header.Get(pulseBetaKeyHeader))
	if got == "" {
		return "", "", false
	}
	for _, want := range cfg.BetaKeys {
		if got == want {
			return hashPulseBetaKey(want), "beta", true
		}
	}
	return "", "", false
}

func pulsePrincipalFromContext(ctx context.Context) (PulsePrincipal, bool) {
	p, ok := ctx.Value(pulsePrincipalCtxKey{}).(PulsePrincipal)
	return p, ok && p.ID != ""
}

func pulseClientIP(r *http.Request) string {
	if xff := strings.TrimSpace(r.Header.Get("X-Forwarded-For")); xff != "" {
		if i := strings.Index(xff, ","); i >= 0 {
			return strings.TrimSpace(xff[:i])
		}
		return xff
	}
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return ip
}

func guestPulsePrincipal(r *http.Request) PulsePrincipal {
	return PulsePrincipal{
		ID:   hashPulseBetaKey("guest:" + pulseClientIP(r)),
		Kind: "guest",
	}
}

func (h *Handler) pulseBetaKeyMiddleware(next http.Handler) http.Handler {
	return h.pulseHostedAuthMiddleware(next)
}

func (h *Handler) pulsePrincipalMiddleware(next http.Handler) http.Handler {
	return h.pulseHostedAuthMiddleware(next)
}

func (h *Handler) WithPulseHosted(cfg PulseHostedConfig) *Handler {
	h.pulseHosted = cfg
	if h.collector != nil && cfg.IdleTTL > 0 {
		h.collector.WithIdleTTL(cfg.IdleTTL)
	}
	if cfg.MaxActiveChannels > 0 && h.collector != nil {
		h.collector.WithMaxTracked(cfg.MaxActiveChannels)
	}
	return h
}

func (h *Handler) WithCDNPublicBase(base string) *Handler {
	h.cdnPublicBase = strings.TrimSpace(base)
	return h
}
