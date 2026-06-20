package api

import (
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

var allowedThumbHosts = map[string]struct{}{
	"clips-media-assets2.twitch.tv": {},
	"clips-media-assets.twitch.tv":  {},
	"static-cdn.jtvnw.net":          {},
	"preview.redd.it":               {},
	"external-preview.redd.it":      {},
	"i.redd.it":                     {},
	"b.thumbs.redditmedia.com":      {},
	"a.thumbs.redditmedia.com":      {},
	"styles.redditmedia.com":        {},
}

var thumbHTTP = &http.Client{Timeout: 8 * time.Second}

func (h *Handler) mediaThumb(w http.ResponseWriter, r *http.Request) {
	raw := strings.TrimSpace(r.URL.Query().Get("u"))
	if raw == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "missing_url",
			"hint":  "pass u=https://… to proxy an allowlisted media thumbnail",
		})
		return
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_url"})
		return
	}
	host := strings.ToLower(parsed.Hostname())
	if !isAllowedThumbHost(host) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "host_not_allowed"})
		return
	}

	if h.streamImageURL(w, r, parsed.String()) {
		return
	}
	w.Header().Set("Cache-Control", "public, max-age=60")
	writeJSON(w, http.StatusBadGateway, map[string]string{"error": "upstream_status", "status": "403 Forbidden"})
}

func isAllowedThumbHost(host string) bool {
	if _, ok := allowedThumbHosts[host]; ok {
		return true
	}
	return strings.HasSuffix(host, ".redd.it")
}

func (h *Handler) streamImageURL(w http.ResponseWriter, r *http.Request, imageURL string) bool {
	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, imageURL, nil)
	if err != nil {
		return false
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "image/avif,image/webp,image/apng,image/*,*/*;q=0.8")
	lower := strings.ToLower(imageURL)
	if strings.Contains(lower, "twitch.tv") || strings.Contains(lower, "jtvnw.net") {
		req.Header.Set("Referer", "https://www.twitch.tv/")
	}

	resp, err := thumbHTTP.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return false
	}

	contentType := strings.TrimSpace(resp.Header.Get("Content-Type"))
	if contentType == "" || !strings.HasPrefix(strings.ToLower(contentType), "image/") {
		return false
	}

	w.Header().Set("Cache-Control", "public, max-age=3600, stale-while-revalidate=86400")
	w.Header().Set("Content-Type", contentType)
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, io.LimitReader(resp.Body, 4<<20))
	return true
}
