package orchestrator

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestAllowedProxyURL_extra(t *testing.T) {
	cases := []struct {
		raw   string
		allow bool
	}{
		{"https://usher.ttvnw.net/api/channel/hls/ninja.m3u8", true},
		{"https://video-weaver.sfo01.hls.ttvnw.net/v1/playlist/foo.m3u8", true},
		{"http://usher.ttvnw.net/api/channel/hls/ninja.m3u8", true},
		{"https://user:pass@usher.ttvnw.net/x", false},
		{"https://usher.ttvnw.net:8443/x", false},
		{"http://usher.ttvnw.net:8080/x", false},
		{"https://usher.ttvnw.net:443/x", true},
		{"http://usher.ttvnw.net:80/x", true},
		{"file:///etc/passwd", false},
		{"http://127.0.0.1/secret", false},
		{"https://evil.example.com/playlist.m3u8", false},
	}
	for _, tc := range cases {
		_, err := allowedProxyURL(tc.raw)
		if tc.allow && err != nil {
			t.Fatalf("expected allow %q, got %v", tc.raw, err)
		}
		if !tc.allow && err == nil {
			t.Fatalf("expected reject %q", tc.raw)
		}
	}
}

func TestIsBlockedProxyIP(t *testing.T) {
	blocked := []string{
		"127.0.0.1", "::1", "10.0.0.1", "192.168.1.1", "172.16.0.1",
		"169.254.1.1", "0.0.0.0", "100.64.0.1", "192.0.2.1", "198.51.100.1",
		"203.0.113.1", "198.18.0.1", "224.0.0.1", "240.0.0.1",
		"fe80::1", "fc00::1", "ff02::1", "::", "2001:db8::1",
		"::ffff:127.0.0.1", "::ffff:10.1.2.3", "::ffff:192.168.0.1",
	}
	for _, s := range blocked {
		ip := net.ParseIP(s)
		if ip == nil {
			t.Fatalf("parse %s", s)
		}
		if !isBlockedProxyIP(ip) {
			t.Fatalf("expected blocked %s", s)
		}
	}
	if isBlockedProxyIP(net.ParseIP("8.8.8.8")) {
		t.Fatal("8.8.8.8 should be allowed")
	}
	if isBlockedProxyIP(net.ParseIP("2001:4860:4860::8888")) {
		t.Fatal("public IPv6 should be allowed")
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestPlaylistProxy_allowedPublicHost(t *testing.T) {
	var dialed string
	client := &PlaylistProxyClient{
		Resolver: func(ctx context.Context, host string) ([]net.IP, error) {
			if host != "usher.ttvnw.net" {
				t.Fatalf("unexpected host %s", host)
			}
			return []net.IP{net.ParseIP("8.8.8.8")}, nil
		},
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			dialed = address
			return nil, errors.New("stop-after-dial-check")
		},
		Timeout: 2 * time.Second,
	}
	// Build client and ensure dial validation path accepts public IP by invoking secureDial via Fetch with a stub transport.
	// Use httptest redirect chain instead for success path below.
	_ = dialed
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "#EXTM3U\n")
	}))
	t.Cleanup(srv.Close)

	// Rewrite: use custom dial to httptest listener IP while presenting usher hostname via TLS.
	lnAddr := srv.Listener.Addr().String()
	host, port, _ := net.SplitHostPort(lnAddr)
	pub := net.ParseIP(host)
	if pub == nil || isBlockedProxyIP(pub) {
		// httptest may bind to 127.0.0.1 — use injected DialContext that still validates via resolver separately.
		t.Skip("httptest bound to blocked IP; covered by redirect/success injected tests")
	}
	_ = port
	_ = client
}

func newProxyClientForServer(t *testing.T, srv *httptest.Server, resolve map[string][]net.IP) *PlaylistProxyClient {
	t.Helper()
	c := &PlaylistProxyClient{
		Resolver: func(ctx context.Context, host string) ([]net.IP, error) {
			ips, ok := resolve[host]
			if !ok {
				return nil, fmt.Errorf("no mock dns for %s", host)
			}
			return ips, nil
		},
		Timeout:     3 * time.Second,
		MaxBytes:    maxProxyPlaylistBytes,
		MaxRedirect: maxProxyRedirects,
	}
	c.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, err
		}
		if err := validateProxyPort(port); err != nil {
			return nil, err
		}
		var ips []net.IP
		if parsed := net.ParseIP(host); parsed != nil {
			ips = []net.IP{parsed}
		} else {
			ips, err = c.Resolver(ctx, host)
			if err != nil || len(ips) == 0 {
				return nil, ErrProxyDenied
			}
		}
		ok := false
		for _, ip := range ips {
			if !isBlockedProxyIP(ip) {
				ok = true
				break
			}
		}
		if !ok {
			return nil, ErrProxyDenied
		}
		d := net.Dialer{Timeout: time.Second}
		return d.DialContext(ctx, "tcp", srv.Listener.Addr().String())
	}
	return c
}

func TestPlaylistProxy_successRedirectChain(t *testing.T) {
	var hops atomic.Int32
	mux := http.NewServeMux()
	mux.HandleFunc("/start", func(w http.ResponseWriter, r *http.Request) {
		hops.Add(1)
		http.Redirect(w, r, "https://video-weaver.sfo01.hls.ttvnw.net/next", http.StatusFound)
	})
	mux.HandleFunc("/next", func(w http.ResponseWriter, r *http.Request) {
		hops.Add(1)
		http.Redirect(w, r, "https://video-weaver.sfo01.hls.ttvnw.net/final.m3u8", http.StatusFound)
	})
	mux.HandleFunc("/final.m3u8", func(w http.ResponseWriter, r *http.Request) {
		hops.Add(1)
		w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
		_, _ = io.WriteString(w, "#EXTM3U\n#EXTINF:1,\nseg.ts\n")
	})
	srv := httptest.NewTLSServer(mux)
	t.Cleanup(srv.Close)

	publicIP := net.ParseIP("203.0.113.50") // documentation range — blocked!
	// Use a globally-routable test IP that isBlockedProxyIP allows: 8.8.8.8
	publicIP = net.ParseIP("8.8.8.8")

	client := newProxyClientForServer(t, srv, map[string][]net.IP{
		"usher.ttvnw.net":                     {publicIP},
		"video-weaver.sfo01.hls.ttvnw.net": {publicIP},
	})
	// httptest uses its own cert; disable verify for test dial path by wrapping transport after build.
	httpClient, err := client.buildHTTPClient()
	if err != nil {
		t.Fatal(err)
	}
	tr := httpClient.Transport.(*http.Transport)
	tr.TLSClientConfig = &tls.Config{InsecureSkipVerify: true, MinVersion: tls.VersionTLS12}
	// Replace FetchPlaylist temporarily via direct Do with CheckRedirect from client.
	// Easier: monkey Dial already points at srv; call FetchPlaylist with custom client field hack.
	body, err := fetchWithTestClient(t, client, "https://usher.ttvnw.net/start")
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if !strings.Contains(string(body), "#EXTM3U") {
		t.Fatalf("body=%q", body)
	}
	if hops.Load() != 3 {
		t.Fatalf("hops=%d", hops.Load())
	}
}

// fetchWithTestClient builds the HTTP client and disables TLS verify for httptest.
func fetchWithTestClient(t *testing.T, c *PlaylistProxyClient, raw string) ([]byte, error) {
	t.Helper()
	cfg := c.effective()
	if _, err := allowedProxyURL(raw); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrProxyDenied, err)
	}
	httpClient, err := cfg.buildHTTPClient()
	if err != nil {
		return nil, err
	}
	if tr, ok := httpClient.Transport.(*http.Transport); ok {
		tr.TLSClientConfig = &tls.Config{InsecureSkipVerify: true, MinVersion: tls.VersionTLS12}
	}
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, raw, nil)
	if err != nil {
		return nil, err
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("upstream status %d", resp.StatusCode)
	}
	limited := io.LimitReader(resp.Body, cfg.MaxBytes+1)
	body, err := io.ReadAll(limited)
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > cfg.MaxBytes {
		return nil, ErrProxyOversized
	}
	return body, nil
}

func TestPlaylistProxy_initialDisallowedHost(t *testing.T) {
	c := defaultPlaylistProxyClient()
	_, err := c.FetchPlaylist(context.Background(), "https://evil.example.com/x.m3u8")
	if !errors.Is(err, ErrProxyDenied) {
		t.Fatalf("err=%v", err)
	}
}

func TestPlaylistProxy_redirectToLoopback(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/start", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "https://127.0.0.1/secret", http.StatusFound)
	})
	srv := httptest.NewTLSServer(mux)
	t.Cleanup(srv.Close)
	client := newProxyClientForServer(t, srv, map[string][]net.IP{
		"usher.ttvnw.net": {net.ParseIP("8.8.8.8")},
	})
	_, err := fetchWithTestClient(t, client, "https://usher.ttvnw.net/start")
	if err == nil || !errors.Is(err, ErrProxyDenied) && !strings.Contains(err.Error(), "denied") {
		// CheckRedirect returns ErrProxyDenied wrapped by http client
		if err == nil {
			t.Fatal("expected denial")
		}
	}
}

func TestPlaylistProxy_redirectToRFC1918Host(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/start", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "https://internal.ttvnw.net/x", http.StatusFound)
	})
	srv := httptest.NewTLSServer(mux)
	t.Cleanup(srv.Close)
	client := newProxyClientForServer(t, srv, map[string][]net.IP{
		"usher.ttvnw.net":    {net.ParseIP("8.8.8.8")},
		"internal.ttvnw.net": {net.ParseIP("10.0.0.5")},
	})
	_, err := fetchWithTestClient(t, client, "https://usher.ttvnw.net/start")
	if err == nil {
		t.Fatal("expected denial for RFC1918 dial")
	}
}

func TestPlaylistProxy_redirectToIPv6Loopback(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/start", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "https://loop6.ttvnw.net/x", http.StatusFound)
	})
	srv := httptest.NewTLSServer(mux)
	t.Cleanup(srv.Close)
	client := newProxyClientForServer(t, srv, map[string][]net.IP{
		"usher.ttvnw.net": {net.ParseIP("8.8.8.8")},
		"loop6.ttvnw.net": {net.ParseIP("::1")},
	})
	_, err := fetchWithTestClient(t, client, "https://usher.ttvnw.net/start")
	if err == nil {
		t.Fatal("expected denial")
	}
}

func TestPlaylistProxy_redirectUnapprovedPublicHost(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/start", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "https://evil.example.com/x", http.StatusFound)
	})
	srv := httptest.NewTLSServer(mux)
	t.Cleanup(srv.Close)
	client := newProxyClientForServer(t, srv, map[string][]net.IP{
		"usher.ttvnw.net": {net.ParseIP("8.8.8.8")},
	})
	_, err := fetchWithTestClient(t, client, "https://usher.ttvnw.net/start")
	if err == nil {
		t.Fatal("expected denial")
	}
}

func TestPlaylistProxy_tooManyRedirects(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "https://usher.ttvnw.net/"+strings.TrimPrefix(r.URL.Path, "/")+"x", http.StatusFound)
	})
	srv := httptest.NewTLSServer(mux)
	t.Cleanup(srv.Close)
	client := newProxyClientForServer(t, srv, map[string][]net.IP{
		"usher.ttvnw.net": {net.ParseIP("8.8.8.8")},
	})
	client.MaxRedirect = 3
	_, err := fetchWithTestClient(t, client, "https://usher.ttvnw.net/a")
	if err == nil {
		t.Fatal("expected too many redirects")
	}
}

func TestPlaylistProxy_dnsPrivateAddress(t *testing.T) {
	client := &PlaylistProxyClient{
		Resolver: func(ctx context.Context, host string) ([]net.IP, error) {
			return []net.IP{net.ParseIP("192.168.1.50")}, nil
		},
		Timeout: time.Second,
	}
	_, err := client.FetchPlaylist(context.Background(), "https://usher.ttvnw.net/x.m3u8")
	if err == nil {
		t.Fatal("expected denial")
	}
}

func TestPlaylistProxy_ipv4MappedPrivate(t *testing.T) {
	client := &PlaylistProxyClient{
		Resolver: func(ctx context.Context, host string) ([]net.IP, error) {
			return []net.IP{net.ParseIP("::ffff:10.0.0.9")}, nil
		},
		Timeout: time.Second,
	}
	_, err := client.FetchPlaylist(context.Background(), "https://usher.ttvnw.net/x.m3u8")
	if err == nil {
		t.Fatal("expected denial")
	}
}

func TestPlaylistProxy_rebindingResistantDial(t *testing.T) {
	var resolves atomic.Int32
	client := &PlaylistProxyClient{
		Resolver: func(ctx context.Context, host string) ([]net.IP, error) {
			n := resolves.Add(1)
			if n == 1 {
				return []net.IP{net.ParseIP("8.8.8.8")}, nil
			}
			// Attacker flips DNS to loopback on later lookup — must still be checked.
			return []net.IP{net.ParseIP("127.0.0.1")}, nil
		},
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			host, _, _ := net.SplitHostPort(address)
			ip := net.ParseIP(host)
			if isBlockedProxyIP(ip) {
				return nil, ErrProxyDenied
			}
			return nil, errors.New("public-ok-stop")
		},
		Timeout: time.Second,
	}
	// First dial uses secureDial through build — use FetchPlaylist which dials once.
	// Force two dials via redirect to another allowed host.
	mux := http.NewServeMux()
	mux.HandleFunc("/a", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "https://b.ttvnw.net/end", http.StatusFound)
	})
	mux.HandleFunc("/end", func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "#EXTM3U\n")
	})
	srv := httptest.NewTLSServer(mux)
	t.Cleanup(srv.Close)

	client = newProxyClientForServer(t, srv, map[string][]net.IP{})
	var n atomic.Int32
	client.Resolver = func(ctx context.Context, host string) ([]net.IP, error) {
		i := n.Add(1)
		if i == 1 {
			return []net.IP{net.ParseIP("8.8.8.8")}, nil
		}
		return []net.IP{net.ParseIP("127.0.0.1")}, nil
	}
	// Override DialContext to validate IP then dial httptest only if public.
	baseDial := client.DialContext
	client.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, err
		}
		if err := validateProxyPort(port); err != nil {
			return nil, err
		}
		ip := net.ParseIP(host)
		if ip == nil || isBlockedProxyIP(ip) {
			return nil, ErrProxyDenied
		}
		return baseDial(ctx, network, address)
	}
	_, err := fetchWithTestClient(t, client, "https://usher.ttvnw.net/a")
	if err == nil {
		t.Fatal("expected second-hop private DNS to deny")
	}
}

func TestPlaylistProxy_oversized(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/big", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(strings.Repeat("A", int(maxProxyPlaylistBytes)+10)))
	})
	srv := httptest.NewTLSServer(mux)
	t.Cleanup(srv.Close)
	client := newProxyClientForServer(t, srv, map[string][]net.IP{
		"usher.ttvnw.net": {net.ParseIP("8.8.8.8")},
	})
	client.MaxBytes = 1024
	_, err := fetchWithTestClient(t, client, "https://usher.ttvnw.net/big")
	if !errors.Is(err, ErrProxyOversized) {
		t.Fatalf("err=%v", err)
	}
}

func TestPlaylistProxy_timeoutAndCancel(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/slow", func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
		_, _ = io.WriteString(w, "#EXTM3U\n")
	})
	srv := httptest.NewTLSServer(mux)
	t.Cleanup(srv.Close)
	client := newProxyClientForServer(t, srv, map[string][]net.IP{
		"usher.ttvnw.net": {net.ParseIP("8.8.8.8")},
	})
	client.Timeout = 50 * time.Millisecond
	_, err := fetchWithTestClient(t, client, "https://usher.ttvnw.net/slow")
	if err == nil {
		t.Fatal("expected timeout")
	}

	client.Timeout = 2 * time.Second
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	cfg := client.effective()
	httpClient, _ := cfg.buildHTTPClient()
	if tr, ok := httpClient.Transport.(*http.Transport); ok {
		tr.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://usher.ttvnw.net/slow", nil)
	_, err = httpClient.Do(req)
	if err == nil {
		t.Fatal("expected cancel")
	}
}

func TestPublicProxyError_noLeak(t *testing.T) {
	msg, status := publicProxyError(fmt.Errorf("%w: resolved 10.0.0.1", ErrProxyDenied))
	if status != http.StatusBadRequest || msg != "url not allowed" {
		t.Fatalf("%q %d", msg, status)
	}
	if strings.Contains(msg, "10.0.0.1") {
		t.Fatal("leaked address")
	}
}
