package orchestrator

import (
	"bufio"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// ErrProxyDenied is returned for policy failures (host, IP, redirect, size).
// Public handlers must map this to a bounded message without internal detail.
var ErrProxyDenied = errors.New("proxy request denied")

// ErrProxyOversized is returned when the playlist exceeds maxProxyPlaylistBytes.
var ErrProxyOversized = errors.New("playlist response too large")

// HostResolver resolves a hostname for the playlist proxy. Tests inject fakes.
type HostResolver func(ctx context.Context, host string) ([]net.IP, error)

// PlaylistProxyClient fetches Twitch HLS playlists with redirect-safe SSRF controls.
// It never passes a user-controlled URL to net/http.Client; redirects are followed
// manually after re-validating each hop and dialing a validated resolved address.
type PlaylistProxyClient struct {
	Resolver HostResolver
	// DialContext is optional; when set, tests may inject a dialer. Production uses
	// secure dial to a previously validated IP while TLS ServerName stays the hostname.
	DialContext func(ctx context.Context, network, address string) (net.Conn, error)
	// TLSConfig optionally customizes TLS (tests may set InsecureSkipVerify for httptest).
	TLSConfig   *tls.Config
	Timeout     time.Duration
	MaxBytes    int64
	MaxRedirect int
}

func defaultPlaylistProxyClient() *PlaylistProxyClient {
	return &PlaylistProxyClient{
		Resolver: func(ctx context.Context, host string) ([]net.IP, error) {
			return net.DefaultResolver.LookupIP(ctx, "ip", host)
		},
		Timeout:     12 * time.Second,
		MaxBytes:    maxProxyPlaylistBytes,
		MaxRedirect: maxProxyRedirects,
	}
}

func (c *PlaylistProxyClient) effective() *PlaylistProxyClient {
	out := *c
	if out.Resolver == nil {
		out.Resolver = defaultPlaylistProxyClient().Resolver
	}
	if out.Timeout <= 0 {
		out.Timeout = 12 * time.Second
	}
	if out.MaxBytes <= 0 {
		out.MaxBytes = maxProxyPlaylistBytes
	}
	if out.MaxRedirect <= 0 {
		out.MaxRedirect = maxProxyRedirects
	}
	return &out
}

// FetchPlaylist validates urlRaw, follows only approved redirects, and returns body bytes.
func (c *PlaylistProxyClient) FetchPlaylist(ctx context.Context, urlRaw string) ([]byte, error) {
	cfg := c.effective()
	current, err := allowedProxyURL(urlRaw)
	if err != nil {
		return nil, fmt.Errorf("%w", ErrProxyDenied)
	}

	reqCtx := ctx
	var cancel context.CancelFunc
	if cfg.Timeout > 0 {
		reqCtx, cancel = context.WithTimeout(ctx, cfg.Timeout)
		defer cancel()
	}

	for redirects := 0; ; redirects++ {
		if err := reqCtx.Err(); err != nil {
			return nil, err
		}
		status, body, location, err := cfg.fetchOne(reqCtx, current)
		if err != nil {
			return nil, err
		}
		if status >= 300 && status < 400 {
			if redirects >= cfg.MaxRedirect {
				return nil, ErrProxyDenied
			}
			if location == "" {
				return nil, ErrProxyDenied
			}
			next, err := current.Parse(location)
			if err != nil {
				return nil, ErrProxyDenied
			}
			current, err = allowedProxyURL(next.String())
			if err != nil {
				return nil, ErrProxyDenied
			}
			continue
		}
		if status != http.StatusOK {
			return nil, fmt.Errorf("upstream status %d", status)
		}
		return body, nil
	}
}

func (c *PlaylistProxyClient) fetchOne(ctx context.Context, u *url.URL) (status int, body []byte, location string, err error) {
	host := u.Hostname()
	if !isTwitchUsherHost(host) {
		return 0, nil, "", ErrProxyDenied
	}
	port := u.Port()
	if port == "" {
		if u.Scheme == "http" {
			port = "80"
		} else {
			port = "443"
		}
	}
	if err := validateProxyPort(port); err != nil {
		return 0, nil, "", err
	}

	ip, err := c.resolvePublicIP(ctx, host)
	if err != nil {
		return 0, nil, "", err
	}

	dial := c.DialContext
	if dial == nil {
		dial = func(ctx context.Context, network, address string) (net.Conn, error) {
			d := net.Dialer{Timeout: 5 * time.Second, KeepAlive: 30 * time.Second}
			return d.DialContext(ctx, network, address)
		}
	}
	rawConn, err := dial(ctx, "tcp", net.JoinHostPort(ip.String(), port))
	if err != nil {
		if errors.Is(err, ErrProxyDenied) {
			return 0, nil, "", ErrProxyDenied
		}
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return 0, nil, "", err
		}
		return 0, nil, "", fmt.Errorf("upstream dial failed")
	}

	var conn net.Conn = rawConn
	if u.Scheme == "https" {
		tlsCfg := &tls.Config{
			ServerName: host,
			MinVersion: tls.VersionTLS12,
		}
		if c.TLSConfig != nil {
			tlsCfg = c.TLSConfig.Clone()
			if tlsCfg.ServerName == "" {
				tlsCfg.ServerName = host
			}
			if tlsCfg.MinVersion == 0 {
				tlsCfg.MinVersion = tls.VersionTLS12
			}
		}
		tlsConn := tls.Client(rawConn, tlsCfg)
		if err := tlsConn.HandshakeContext(ctx); err != nil {
			_ = rawConn.Close()
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return 0, nil, "", err
			}
			return 0, nil, "", fmt.Errorf("upstream fetch failed")
		}
		conn = tlsConn
	}
	defer conn.Close()

	_ = conn.SetDeadline(time.Now().Add(c.Timeout))
	if dl, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(dl)
	}

	reqPath := u.RequestURI()
	if reqPath == "" {
		reqPath = "/"
	}
	// Write a fixed-shape request; Host is the already-allowlisted hostname.
	payload := fmt.Sprintf(
		"GET %s HTTP/1.1\r\nHost: %s\r\nUser-Agent: Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36\r\nAccept: */*\r\nConnection: close\r\n\r\n",
		reqPath,
		host,
	)
	if _, err := io.WriteString(conn, payload); err != nil {
		return 0, nil, "", fmt.Errorf("upstream fetch failed")
	}

	br := bufio.NewReader(conn)
	resp, err := http.ReadResponse(br, &http.Request{Method: http.MethodGet, URL: u})
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return 0, nil, "", err
		}
		return 0, nil, "", fmt.Errorf("upstream fetch failed")
	}
	defer resp.Body.Close()

	loc := resp.Header.Get("Location")
	limited := io.LimitReader(resp.Body, c.MaxBytes+1)
	body, err = io.ReadAll(limited)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return 0, nil, "", err
		}
		return 0, nil, "", fmt.Errorf("upstream read failed")
	}
	if int64(len(body)) > c.MaxBytes {
		return 0, nil, "", ErrProxyOversized
	}
	return resp.StatusCode, body, loc, nil
}

func (c *PlaylistProxyClient) resolvePublicIP(ctx context.Context, host string) (net.IP, error) {
	if parsed := net.ParseIP(host); parsed != nil {
		if isBlockedProxyIP(parsed) {
			return nil, ErrProxyDenied
		}
		return parsed, nil
	}
	ips, err := c.Resolver(ctx, host)
	if err != nil || len(ips) == 0 {
		return nil, ErrProxyDenied
	}
	for _, ip := range ips {
		if !isBlockedProxyIP(ip) {
			return ip, nil
		}
	}
	return nil, ErrProxyDenied
}

func validateProxyPort(port string) error {
	n, err := strconv.Atoi(port)
	if err != nil {
		return ErrProxyDenied
	}
	if n != 80 && n != 443 {
		return ErrProxyDenied
	}
	return nil
}

// publicProxyError maps internal errors to safe client-facing messages and status codes.
func publicProxyError(err error) (msg string, status int) {
	if err == nil {
		return "proxy error", http.StatusBadGateway
	}
	if errors.Is(err, context.Canceled) {
		return "request canceled", 499
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "upstream timeout", http.StatusGatewayTimeout
	}
	if errors.Is(err, ErrProxyOversized) {
		return "playlist too large", http.StatusBadGateway
	}
	if errors.Is(err, ErrProxyDenied) {
		return "url not allowed", http.StatusBadRequest
	}
	if strings.HasPrefix(err.Error(), "upstream status ") {
		return err.Error(), http.StatusBadGateway
	}
	return "upstream fetch failed", http.StatusBadGateway
}
