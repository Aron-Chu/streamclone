package api

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"streamclone/internal/emote/objstore"
)

// serveEmoteAssetOpen exercises the streaming + exact ETag path without MinIO.
func serveEmoteAssetOpen(
	w http.ResponseWriter,
	r *http.Request,
	open func(context.Context, string, string) (io.ReadCloser, objstore.ObjectInfo, error),
) {
	id := strings.TrimSpace(chi.URLParam(r, "id"))
	scale := strings.TrimSpace(chi.URLParam(r, "scale"))
	if id == "" || scale == "" {
		http.NotFound(w, r)
		return
	}
	rc, info, err := open(r.Context(), id, scale)
	if err != nil {
		http.Error(w, "missing", http.StatusNotFound)
		return
	}
	defer rc.Close()
	w.Header().Set("Content-Type", info.ContentType)
	if info.Size > 0 {
		w.Header().Set("Content-Length", strconv.FormatInt(info.Size, 10))
	}
	if etag := strings.Trim(info.ETag, `"`); etag != "" {
		w.Header().Set("ETag", `"`+etag+`"`)
		if etagMatches(r.Header.Get("If-None-Match"), etag) {
			w.WriteHeader(http.StatusNotModified)
			return
		}
	}
	_, _ = io.Copy(w, rc)
}

func TestServeEmoteAssetOpenStreamsAndHonorsExactETag(t *testing.T) {
	body := []byte("webp-bytes")
	open := func(ctx context.Context, id, scale string) (io.ReadCloser, objstore.ObjectInfo, error) {
		return io.NopCloser(bytes.NewReader(body)), objstore.ObjectInfo{
			Size:        int64(len(body)),
			ContentType: "image/webp",
			ETag:        "deadbeef",
		}, nil
	}

	r := chi.NewRouter()
	r.Get("/emotes/{id}/{scale}", func(w http.ResponseWriter, req *http.Request) {
		serveEmoteAssetOpen(w, req, open)
	})

	req := httptest.NewRequest(http.MethodGet, "/emotes/abc/1x", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d", rec.Code)
	}
	if rec.Body.String() != string(body) {
		t.Fatalf("body=%q", rec.Body.String())
	}
	if rec.Header().Get("ETag") != `"deadbeef"` {
		t.Fatalf("etag=%q", rec.Header().Get("ETag"))
	}

	req304 := httptest.NewRequest(http.MethodGet, "/emotes/abc/1x", nil)
	req304.Header.Set("If-None-Match", `"deadbeef"`)
	rec304 := httptest.NewRecorder()
	r.ServeHTTP(rec304, req304)
	if rec304.Code != http.StatusNotModified {
		t.Fatalf("304 status=%d", rec304.Code)
	}
	if rec304.Body.Len() != 0 {
		t.Fatalf("304 body should be empty")
	}

	reqMiss := httptest.NewRequest(http.MethodGet, "/emotes/abc/1x", nil)
	reqMiss.Header.Set("If-None-Match", `"deadbee"`) // substring must NOT 304
	recMiss := httptest.NewRecorder()
	r.ServeHTTP(recMiss, reqMiss)
	if recMiss.Code != http.StatusOK {
		t.Fatalf("substring etag should not 304: status=%d", recMiss.Code)
	}
}

func TestServeEmoteAssetOpenMissingObject(t *testing.T) {
	open := func(ctx context.Context, id, scale string) (io.ReadCloser, objstore.ObjectInfo, error) {
		return nil, objstore.ObjectInfo{}, errors.New("NoSuchKey")
	}
	r := chi.NewRouter()
	r.Get("/emotes/{id}/{scale}", func(w http.ResponseWriter, req *http.Request) {
		serveEmoteAssetOpen(w, req, open)
	})
	req := httptest.NewRequest(http.MethodGet, "/emotes/abc/1x", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status=%d", rec.Code)
	}
}
