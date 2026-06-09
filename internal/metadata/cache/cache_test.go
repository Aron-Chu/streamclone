package cache

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

type memStore struct {
	mu   sync.Mutex
	data map[string][]byte
}

func newMemStore() *memStore { return &memStore{data: make(map[string][]byte)} }

func (m *memStore) Get(_ context.Context, key string) ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	v, ok := m.data[key]
	if !ok {
		return nil, ErrNotFound
	}
	return v, nil
}

func (m *memStore) Set(_ context.Context, key string, val []byte, _ time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.data[key] = val
	return nil
}

func (m *memStore) delete(key string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.data, key)
}

func TestGetFresh(t *testing.T) {
	s := newMemStore()
	c := New(s, time.Minute, time.Hour)
	_ = c.Set(context.Background(), "k", []byte("v"))
	r, err := c.Get(context.Background(), "k")
	if err != nil || r.Stale || string(r.Data) != "v" {
		t.Fatalf("unexpected: %v %v", r, err)
	}
}

func TestStaleAfterExpiry(t *testing.T) {
	s := newMemStore()
	c := New(s, time.Minute, time.Hour)
	_ = c.Set(context.Background(), "k", []byte("stale-v"))
	s.delete("k")
	r, err := c.Get(context.Background(), "k")
	if err != nil || !r.Stale || string(r.Data) != "stale-v" {
		t.Fatalf("expected stale fallback: %v %v", r, err)
	}
}

func TestNotFound(t *testing.T) {
	s := newMemStore()
	c := New(s, time.Minute, time.Hour)
	_, err := c.Get(context.Background(), "missing")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestDegradeWhenStoreUnavailable(t *testing.T) {
	s := newMemStore()
	_ = s.Set(context.Background(), "k", []byte("old"), time.Hour)
	_ = s.Set(context.Background(), staleKey("k"), []byte("old"), time.Hour)

	c := New(s, time.Minute, time.Hour)
	r, err := c.Get(context.Background(), "k")
	if err != nil || string(r.Data) != "old" {
		t.Fatalf("unexpected: %v %v", r, err)
	}
}
