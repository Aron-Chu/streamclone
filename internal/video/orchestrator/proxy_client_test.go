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

func playlistTestClient(t *testing.T, srv *httptest.Server, resolve map[string][]net.IP) *PlaylistProxyClient {
	t.Helper()
	return &PlaylistProxyClient{
		Resolver: func(ctx context.Context, host string) ([]net.IP, error) {
			ips, ok := resolve[host]
			if !ok {
				return nil, fmt.Errorf("no mock dns for %s", host)
			}
			return ips, nil
		},
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
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
			d := net.Dialer{Timeout: time.Second}
			return d.DialContext(ctx, "tcp", srv.Listener.Addr().String())
		},
		TLSConfig:   &tls.Config{InsecureSkipVerify: true, MinVersion: tls.VersionTLS12},
		Timeout:     3 * time.Second,
		MaxBytes:    maxProxyPlaylistBytes,
		MaxRedirect: maxProxyRedirects,
	}
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

	publicIP := net.ParseIP("8.8.8.8")
	client := playlistTestClient(t, srv, map[string][]net.IP{
		"usher.ttvnw.net":                  {publicIP},
		"video-weaver.sfo01.hls.ttvnw.net": {publicIP},
	})
	body, err := client.FetchPlaylist(context.Background(), "https://usher.ttvnw.net/start")
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
	client := playlistTestClient(t, srv, map[string][]net.IP{
		"usher.ttvnw.net": {net.ParseIP("8.8.8.8")},
	})
	_, err := client.FetchPlaylist(context.Background(), "https://usher.ttvnw.net/start")
	if err == nil {
		t.Fatal("expected denial")
	}
}

func TestPlaylistProxy_redirectToRFC1918Host(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/start", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "https://internal.ttvnw.net/x", http.StatusFound)
	})
	srv := httptest.NewTLSServer(mux)
	t.Cleanup(srv.Close)
	client := playlistTestClient(t, srv, map[string][]net.IP{
		"usher.ttvnw.net":    {net.ParseIP("8.8.8.8")},
		"internal.ttvnw.net": {net.ParseIP("10.0.0.5")},
	})
	_, err := client.FetchPlaylist(context.Background(), "https://usher.ttvnw.net/start")
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
	client := playlistTestClient(t, srv, map[string][]net.IP{
		"usher.ttvnw.net": {net.ParseIP("8.8.8.8")},
		"loop6.ttvnw.net": {net.ParseIP("::1")},
	})
	_, err := client.FetchPlaylist(context.Background(), "https://usher.ttvnw.net/start")
	if err == nil {
		t.Fatal("expected denial")
	}
}

func TestPlaylistProxy_redirectToIPv6ULA(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/start", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "https://ula.ttvnw.net/x", http.StatusFound)
	})
	srv := httptest.NewTLSServer(mux)
	t.Cleanup(srv.Close)
	client := playlistTestClient(t, srv, map[string][]net.IP{
		"usher.ttvnw.net": {net.ParseIP("8.8.8.8")},
		"ula.ttvnw.net":   {net.ParseIP("fc00::1")},
	})
	_, err := client.FetchPlaylist(context.Background(), "https://usher.ttvnw.net/start")
	if err == nil {
		t.Fatal("expected denial for IPv6 ULA redirect")
	}
}

func TestPlaylistProxy_responseHeaderTimeout(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/slow-headers", func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(400 * time.Millisecond)
		_, _ = io.WriteString(w, "#EXTM3U\n")
	})
	srv := httptest.NewTLSServer(mux)
	t.Cleanup(srv.Close)
	client := playlistTestClient(t, srv, map[string][]net.IP{
		"usher.ttvnw.net": {net.ParseIP("8.8.8.8")},
	})
	client.Timeout = 2 * time.Second
	client.ResponseHeaderTimeout = 50 * time.Millisecond
	_, err := client.FetchPlaylist(context.Background(), "https://usher.ttvnw.net/slow-headers")
	if err == nil {
		t.Fatal("expected response-header timeout")
	}
}

func TestPlaylistProxy_redirectUnapprovedPublicHost(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/start", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "https://evil.example.com/x", http.StatusFound)
	})
	srv := httptest.NewTLSServer(mux)
	t.Cleanup(srv.Close)
	client := playlistTestClient(t, srv, map[string][]net.IP{
		"usher.ttvnw.net": {net.ParseIP("8.8.8.8")},
	})
	_, err := client.FetchPlaylist(context.Background(), "https://usher.ttvnw.net/start")
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
	client := playlistTestClient(t, srv, map[string][]net.IP{
		"usher.ttvnw.net": {net.ParseIP("8.8.8.8")},
	})
	client.MaxRedirect = 3
	_, err := client.FetchPlaylist(context.Background(), "https://usher.ttvnw.net/a")
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
	mux := http.NewServeMux()
	mux.HandleFunc("/a", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "https://b.ttvnw.net/end", http.StatusFound)
	})
	mux.HandleFunc("/end", func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "#EXTM3U\n")
	})
	srv := httptest.NewTLSServer(mux)
	t.Cleanup(srv.Close)

	var n atomic.Int32
	client := playlistTestClient(t, srv, map[string][]net.IP{})
	client.Resolver = func(ctx context.Context, host string) ([]net.IP, error) {
		i := n.Add(1)
		if i == 1 {
			return []net.IP{net.ParseIP("8.8.8.8")}, nil
		}
		return []net.IP{net.ParseIP("127.0.0.1")}, nil
	}
	_, err := client.FetchPlaylist(context.Background(), "https://usher.ttvnw.net/a")
	if err == nil {
		t.Fatal("expected second-hop private DNS to deny")
	}
}

func TestPlaylistProxy_oversized(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/big", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(strings.Repeat("A", 2048)))
	})
	srv := httptest.NewTLSServer(mux)
	t.Cleanup(srv.Close)
	client := playlistTestClient(t, srv, map[string][]net.IP{
		"usher.ttvnw.net": {net.ParseIP("8.8.8.8")},
	})
	client.MaxBytes = 1024
	_, err := client.FetchPlaylist(context.Background(), "https://usher.ttvnw.net/big")
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
	client := playlistTestClient(t, srv, map[string][]net.IP{
		"usher.ttvnw.net": {net.ParseIP("8.8.8.8")},
	})
	client.Timeout = 50 * time.Millisecond
	_, err := client.FetchPlaylist(context.Background(), "https://usher.ttvnw.net/slow")
	if err == nil {
		t.Fatal("expected timeout")
	}

	client.Timeout = 2 * time.Second
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = client.FetchPlaylist(ctx, "https://usher.ttvnw.net/slow")
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
