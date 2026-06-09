package usher

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"streamclone/internal/upstream"
)

var ErrChannelOffline = errors.New("channel offline")

type Rendition struct {
	Name      string  `json:"name"`
	Group     string  `json:"group"`
	Width     int     `json:"width,omitempty"`
	Height    int     `json:"height,omitempty"`
	FrameRate float64 `json:"frameRate,omitempty"`
	Bandwidth int     `json:"bandwidth,omitempty"`
	URL       string  `json:"-"`
}

type Client struct {
	http     *http.Client
	base     string
	clientID string
	ua       string
}

func New(e upstream.Endpoints) *Client {
	return &Client{
		http:     &http.Client{Timeout: 10 * time.Second},
		base:     e.TwitchUsherURL,
		clientID: e.TwitchClientID,
		ua:       e.UserAgent,
	}
}

func (c *Client) Discover(ctx context.Context, login, tokenValue, signature string) ([]Rendition, error) {
	q := url.Values{}
	q.Set("client_id", c.clientID)
	q.Set("token", tokenValue)
	q.Set("sig", signature)
	q.Set("allow_source", "true")
	q.Set("allow_audio_only", "true")
	q.Set("fast_bread", "true")
	q.Set("playlist_include_framerate", "true")
	q.Set("player_backend", "mediaplayer")
	q.Set("p", strconv.Itoa(rand.Intn(9_000_000)+1_000_000))

	u := fmt.Sprintf("%s/api/channel/hls/%s.m3u8?%s", c.base, url.PathEscape(login), q.Encode())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", c.ua)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, ErrChannelOffline
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("usher status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	return ParseMaster(string(body))
}

func ParseMaster(s string) ([]Rendition, error) {
	if !strings.Contains(s, "#EXTM3U") {
		return nil, upstream.ErrUpstreamSchema
	}
	lines := strings.Split(s, "\n")
	names := map[string]string{}
	var out []Rendition
	for i := 0; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		switch {
		case strings.HasPrefix(line, "#EXT-X-MEDIA:"):
			a := ParseAttrs(strings.TrimPrefix(line, "#EXT-X-MEDIA:"))
			if a["TYPE"] == "VIDEO" && a["GROUP-ID"] != "" {
				names[a["GROUP-ID"]] = a["NAME"]
			}
		case strings.HasPrefix(line, "#EXT-X-STREAM-INF:"):
			a := ParseAttrs(strings.TrimPrefix(line, "#EXT-X-STREAM-INF:"))
			r := Rendition{Group: a["VIDEO"], Name: names[a["VIDEO"]]}
			if r.Name == "" {
				r.Name = a["VIDEO"]
			}
			if res := strings.SplitN(a["RESOLUTION"], "x", 2); len(res) == 2 {
				r.Width, _ = strconv.Atoi(res[0])
				r.Height, _ = strconv.Atoi(res[1])
			}
			r.FrameRate, _ = strconv.ParseFloat(a["FRAME-RATE"], 64)
			r.Bandwidth, _ = strconv.Atoi(a["BANDWIDTH"])
			for j := i + 1; j < len(lines); j++ {
				next := strings.TrimSpace(lines[j])
				if next == "" || strings.HasPrefix(next, "#") {
					continue
				}
				r.URL = next
				i = j
				break
			}
			out = append(out, r)
		}
	}
	return out, nil
}

func ParseAttrs(s string) map[string]string {
	m := map[string]string{}
	var key, val strings.Builder
	inKey, inQuote := true, false
	flush := func() {
		k := strings.TrimSpace(key.String())
		if k != "" {
			m[k] = strings.Trim(strings.TrimSpace(val.String()), `"`)
		}
		key.Reset()
		val.Reset()
		inKey = true
	}
	for _, r := range s {
		switch {
		case r == '"':
			inQuote = !inQuote
			val.WriteRune(r)
		case r == '=' && inKey && !inQuote:
			inKey = false
		case r == ',' && !inQuote:
			flush()
		default:
			if inKey {
				key.WriteRune(r)
			} else {
				val.WriteRune(r)
			}
		}
	}
	flush()
	return m
}
