package orchestrator

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
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
type PlaylistProxyClient struct {
	Resolver HostResolver
	// DialContext is optional; when nil a secure dialer is built from Resolver.
	DialContext func(ctx context.Context, network, address string) (net.Conn, error)
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
	if _, err := allowedProxyURL(urlRaw); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrProxyDenied, err)
	}

	client, err := cfg.buildHTTPClient()
	if err != nil {
		return nil, fmt.Errorf("%w", ErrProxyDenied)
	}

	reqCtx := ctx
	var cancel context.CancelFunc
	if cfg.Timeout > 0 {
		reqCtx, cancel = context.WithTimeout(ctx, cfg.Timeout)
		defer cancel()
	}

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, urlRaw, nil)
	if err != nil {
		return nil, fmt.Errorf("%w", ErrProxyDenied)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")

	resp, err := client.Do(req)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return nil, err
		}
		// Collapse transport/policy failures to a non-leaky denial or generic upstream error.
		if errors.Is(err, ErrProxyDenied) || strings.Contains(err.Error(), ErrProxyDenied.Error()) {
			return nil, ErrProxyDenied
		}
		return nil, fmt.Errorf("upstream fetch failed")
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("upstream status %d", resp.StatusCode)
	}

	limited := io.LimitReader(resp.Body, cfg.MaxBytes+1)
	body, err := io.ReadAll(limited)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return nil, err
		}
		return nil, fmt.Errorf("upstream read failed")
	}
	if int64(len(body)) > cfg.MaxBytes {
		return nil, ErrProxyOversized
	}
	return body, nil
}

func (c *PlaylistProxyClient) buildHTTPClient() (*http.Client, error) {
	dial := c.DialContext
	if dial == nil {
		dial = c.secureDialContext
	}
	transport := &http.Transport{
		Proxy:                 func(*http.Request) (*url.URL, error) { return nil, nil },
		DialContext:           dial,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          4,
		IdleConnTimeout:       30 * time.Second,
		TLSHandshakeTimeout:   5 * time.Second,
		ResponseHeaderTimeout: 5 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		TLSClientConfig: &tls.Config{
			MinVersion: tls.VersionTLS12,
		},
	}
	client := &http.Client{
		Transport: transport,
		Timeout:   c.Timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= c.MaxRedirect {
				return fmt.Errorf("%w: too many redirects", ErrProxyDenied)
			}
			if _, err := allowedProxyURL(req.URL.String()); err != nil {
				return fmt.Errorf("%w", ErrProxyDenied)
			}
			// Force DialContext to re-validate the next hop IP.
			return nil
		},
	}
	return client, nil
}

func (c *PlaylistProxyClient) secureDialContext(ctx context.Context, network, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, fmt.Errorf("%w", ErrProxyDenied)
	}
	if err := validateProxyPort(port); err != nil {
		return nil, err
	}

	var ips []net.IP
	if parsedIP := net.ParseIP(host); parsedIP != nil {
		ips = []net.IP{parsedIP}
	} else {
		ips, err = c.Resolver(ctx, host)
		if err != nil || len(ips) == 0 {
			return nil, fmt.Errorf("%w", ErrProxyDenied)
		}
	}

	var lastErr error
	for _, ip := range ips {
		if isBlockedProxyIP(ip) {
			lastErr = fmt.Errorf("%w", ErrProxyDenied)
			continue
		}
		addrPort, err := netip.ParseAddrPort(net.JoinHostPort(ip.String(), port))
		if err != nil {
			lastErr = fmt.Errorf("%w", ErrProxyDenied)
			continue
		}
		d := net.Dialer{Timeout: 5 * time.Second, KeepAlive: 30 * time.Second}
		conn, err := d.DialContext(ctx, network, addrPort.String())
		if err != nil {
			lastErr = fmt.Errorf("upstream dial failed")
			continue
		}
		return conn, nil
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("%w", ErrProxyDenied)
	}
	return nil, lastErr
}

func validateProxyPort(port string) error {
	n, err := strconv.Atoi(port)
	if err != nil {
		return fmt.Errorf("%w", ErrProxyDenied)
	}
	if n != 80 && n != 443 {
		return fmt.Errorf("%w", ErrProxyDenied)
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
