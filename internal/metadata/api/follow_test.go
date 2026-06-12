package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
)

type mockFollowStore struct {
	logins []string
}

func (m *mockFollowStore) List(context.Context) ([]string, error) {
	out := make([]string, len(m.logins))
	copy(out, m.logins)
	return out, nil
}

func (m *mockFollowStore) Add(_ context.Context, login string) error {
	for _, existing := range m.logins {
		if existing == login {
			return nil
		}
	}
	m.logins = append([]string{login}, m.logins...)
	return nil
}

func (m *mockFollowStore) Remove(_ context.Context, login string) error {
	next := m.logins[:0]
	removed := false
	for _, existing := range m.logins {
		if existing == login {
			removed = true
			continue
		}
		next = append(next, existing)
	}
	if !removed {
		return pgx.ErrNoRows
	}
	m.logins = next
	return nil
}

func TestFollowHandlers(t *testing.T) {
	store := &mockFollowStore{}
	h := New(newTestCache(), &fakeGQL{}).WithFollowStore(store)
	r := chi.NewRouter()
	h.Mount(r)

	req := httptest.NewRequest(http.MethodPost, "/v1/channels/sodapoppin/follow", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("follow status=%d body=%s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/v1/followed", nil)
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("followed status=%d body=%s", rec.Code, rec.Body.String())
	}
	var payload struct {
		Channels []FollowedChannel `json:"channels"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode followed: %v", err)
	}
	if len(payload.Channels) != 1 || payload.Channels[0].Login != "sodapoppin" {
		t.Fatalf("unexpected followed payload: %+v", payload.Channels)
	}

	req = httptest.NewRequest(http.MethodDelete, "/v1/channels/sodapoppin/follow", nil)
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("unfollow status=%d body=%s", rec.Code, rec.Body.String())
	}
}
