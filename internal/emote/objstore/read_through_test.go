package objstore

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"
	"time"
)

type fakeObjectStore struct {
	render               []byte
	source               []byte
	readErr              error
	putErr               error
	deleteErr            error
	puts                 int
	sourcePuts           int
	sourcePutContentType string
	deletes              int
	ensureCalls          int
	putDone              chan struct{}
}

func (f *fakeObjectStore) Get(context.Context, string, string) ([]byte, string, error) {
	if f.readErr != nil {
		return nil, "", f.readErr
	}
	return append([]byte(nil), f.render...), "image/webp", nil
}
func (f *fakeObjectStore) Open(context.Context, string, string) (io.ReadCloser, ObjectInfo, error) {
	if f.readErr != nil {
		return nil, ObjectInfo{}, f.readErr
	}
	return io.NopCloser(bytes.NewReader(f.render)), ObjectInfo{
		Size:         int64(len(f.render)),
		ContentType:  "image/webp",
		ETag:         "fake-etag",
		LastModified: time.Unix(1, 0),
	}, nil
}
func (f *fakeObjectStore) Stat(context.Context, string, string) (ObjectInfo, error) {
	if f.readErr != nil {
		return ObjectInfo{}, f.readErr
	}
	return ObjectInfo{Size: int64(len(f.render)), ContentType: "image/webp"}, nil
}
func (f *fakeObjectStore) Exists(context.Context, string, string) (bool, error) {
	if f.readErr != nil {
		return false, f.readErr
	}
	return len(f.render) > 0, nil
}
func (f *fakeObjectStore) Put(_ context.Context, _ string, _ string, data []byte) error {
	f.puts++
	if f.putErr != nil {
		if f.putDone != nil {
			select {
			case f.putDone <- struct{}{}:
			default:
			}
		}
		return f.putErr
	}
	f.render = append([]byte(nil), data...)
	if f.putDone != nil {
		select {
		case f.putDone <- struct{}{}:
		default:
		}
	}
	return nil
}
func (f *fakeObjectStore) PutSrc(_ context.Context, _ string, data []byte, contentType string) error {
	f.sourcePuts++
	f.sourcePutContentType = contentType
	if f.putErr != nil {
		return f.putErr
	}
	f.source = append([]byte(nil), data...)
	return nil
}

func TestReadThroughStorePromotionFailureStillServesFallback(t *testing.T) {
	local := &fakeObjectStore{render: []byte("render")}
	secondary := &fakeObjectStore{
		readErr: errors.New("missing"),
		putErr:  errors.New("write unavailable"),
	}
	store, err := NewReadThroughStore(local, secondary, ReadThroughOptions{
		PrimarySecondary: true,
		ReadThrough:      true,
	})
	if err != nil {
		t.Fatal(err)
	}
	rc, _, err := store.Open(context.Background(), "emote", "1x")
	if err != nil {
		t.Fatalf("fallback should remain available when promotion fails: %v", err)
	}
	defer rc.Close()
	data, err := io.ReadAll(rc)
	if err != nil || string(data) != "render" {
		t.Fatalf("fallback data = %q, err %v", data, err)
	}
}
func (f *fakeObjectStore) GetSrc(context.Context, string) ([]byte, error) {
	if f.readErr != nil {
		return nil, f.readErr
	}
	return append([]byte(nil), f.source...), nil
}

func (f *fakeObjectStore) GetSrcWithContentType(ctx context.Context, id string) ([]byte, string, error) {
	data, err := f.GetSrc(ctx, id)
	return data, "image/gif", err
}
func (f *fakeObjectStore) Delete(context.Context, string, string) error {
	f.deletes++
	return f.deleteErr
}

func TestReadThroughStoreDeletesFallbackWhenPrimaryDeleteFails(t *testing.T) {
	local := &fakeObjectStore{}
	secondary := &fakeObjectStore{deleteErr: errors.New("primary unavailable")}
	store, err := NewReadThroughStore(local, secondary, ReadThroughOptions{PrimarySecondary: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Delete(context.Background(), "emote", "1x"); err == nil {
		t.Fatal("expected primary delete error")
	}
	if secondary.deletes != 1 || local.deletes != 1 {
		t.Fatalf("deletes = secondary %d local %d", secondary.deletes, local.deletes)
	}
}
func (f *fakeObjectStore) EnsureBucket(context.Context, bool) error {
	f.ensureCalls++
	return nil
}

func TestReadThroughStoreFallbackPromotesAndDualWrites(t *testing.T) {
	local := &fakeObjectStore{source: []byte("source"), render: []byte("render")}
	secondary := &fakeObjectStore{readErr: errors.New("missing")}
	store, err := NewReadThroughStore(local, secondary, ReadThroughOptions{
		PrimarySecondary: true,
		DualWrite:        true,
		ReadThrough:      true,
	})
	if err != nil {
		t.Fatal(err)
	}

	data, err := store.GetSrc(context.Background(), "emote")
	if err != nil || string(data) != "source" {
		t.Fatalf("fallback source = %q, err %v", data, err)
	}
	if secondary.sourcePuts != 1 || string(secondary.source) != "source" {
		t.Fatalf("source promotion = puts %d data %q", secondary.sourcePuts, secondary.source)
	}
	if secondary.sourcePutContentType != "image/gif" {
		t.Fatalf("source promotion content type = %q, want image/gif", secondary.sourcePutContentType)
	}
	secondary.readErr = nil

	if err := store.Put(context.Background(), "emote", "1x", []byte("next")); err != nil {
		t.Fatal(err)
	}
	if secondary.puts != 1 || local.puts != 1 {
		t.Fatalf("dual writes = secondary %d local %d", secondary.puts, local.puts)
	}
	if err := store.Delete(context.Background(), "emote", "1x"); err != nil {
		t.Fatal(err)
	}
	if secondary.deletes != 1 || local.deletes != 1 {
		t.Fatalf("deletes = secondary %d local %d", secondary.deletes, local.deletes)
	}
}

func TestReadThroughStoreStreamFallbackPromotesWithoutLosingMetadata(t *testing.T) {
	local := &fakeObjectStore{render: []byte("render")}
	secondary := &fakeObjectStore{
		readErr: errors.New("missing"),
		putDone: make(chan struct{}, 1),
	}
	store, err := NewReadThroughStore(local, secondary, ReadThroughOptions{
		PrimarySecondary: true,
		ReadThrough:      true,
	})
	if err != nil {
		t.Fatal(err)
	}
	rc, info, err := store.Open(context.Background(), "emote", "1x")
	if err != nil {
		t.Fatal(err)
	}
	defer rc.Close()
	if secondary.puts != 0 {
		t.Fatal("stream fallback must not read or promote before the caller consumes it")
	}
	data, err := io.ReadAll(rc)
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-secondary.putDone:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for background promotion")
	}
	if string(data) != "render" || info.ContentType != "image/webp" || info.ETag != "fake-etag" {
		t.Fatalf("stream fallback = %q %+v", data, info)
	}
	if secondary.puts != 1 || string(secondary.render) != "render" {
		t.Fatalf("stream promotion = puts %d data %q", secondary.puts, secondary.render)
	}
}

func TestReadThroughStoreRejectsMissingPrimary(t *testing.T) {
	if _, err := NewReadThroughStore(nil, nil, ReadThroughOptions{}); err == nil {
		t.Fatal("expected missing local store error")
	}
	if _, err := NewReadThroughStore(&fakeObjectStore{}, nil, ReadThroughOptions{PrimarySecondary: true}); err == nil {
		t.Fatal("expected missing secondary primary error")
	}
}

func TestReadThroughFalseDisablesFallbackButKeepsDualWrite(t *testing.T) {
	local := &fakeObjectStore{readErr: errors.New("local unavailable")}
	secondary := &fakeObjectStore{render: []byte("stale-secondary")}
	store, err := NewReadThroughStore(local, secondary, ReadThroughOptions{
		DualWrite:   true,
		ReadThrough: false,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.Get(context.Background(), "emote", "1x"); err == nil {
		t.Fatal("read-through disabled must not serve the secondary fallback")
	}
	local.readErr = nil
	if err := store.Put(context.Background(), "emote", "1x", []byte("next")); err != nil {
		t.Fatal(err)
	}
	if local.puts != 1 || secondary.puts != 1 {
		t.Fatalf("dual writes = local %d secondary %d, want 1 each", local.puts, secondary.puts)
	}
}
