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
)

var ErrVodUnavailable = errors.New("vod unavailable")

func (c *Client) DiscoverVod(ctx context.Context, vodID, tokenValue, signature string) ([]Rendition, error) {
	q := url.Values{}
	q.Set("client_id", c.clientID)
	q.Set("token", tokenValue)
	q.Set("sig", signature)
	q.Set("allow_source", "true")
	q.Set("allow_audio_only", "true")
	q.Set("playlist_include_framerate", "true")
	q.Set("player_backend", "mediaplayer")
	q.Set("p", strconv.Itoa(rand.Intn(9_000_000)+1_000_000))

	u := fmt.Sprintf("%s/api/vod/%s.m3u8?%s", c.base, url.PathEscape(vodID), q.Encode())
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
		return nil, ErrVodUnavailable
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("usher vod status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	return ParseMaster(string(body))
}
