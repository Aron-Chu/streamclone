package helix

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

func TestUserIDsByLoginBatchesAndNormalizes(t *testing.T) {
	t.Parallel()

	var mu sync.Mutex
	var requestSizes []int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/users" {
			http.NotFound(w, r)
			return
		}
		logins := r.URL.Query()["login"]
		if len(logins) > usersByLoginBatchSize {
			t.Errorf("batch size %d exceeds %d", len(logins), usersByLoginBatchSize)
		}
		mu.Lock()
		requestSizes = append(requestSizes, len(logins))
		mu.Unlock()

		users := make([]map[string]string, 0, len(logins))
		for _, login := range logins {
			users = append(users, map[string]string{
				"id":    "id-" + login,
				"login": login,
			})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"data": users})
	}))
	defer server.Close()

	client := New(server.URL, server.URL+"/token", "client", "secret", "test")
	client.token = "test-token"
	client.expiresAt = time.Now().Add(time.Hour)

	logins := make([]string, 0, 207)
	for i := 0; i < 205; i++ {
		logins = append(logins, fmt.Sprintf("Channel%03d", i))
	}
	logins = append(logins, " channel000 ", "CHANNEL001")

	got, err := client.UserIDsByLogin(context.Background(), logins)
	if err != nil {
		t.Fatalf("UserIDsByLogin: %v", err)
	}
	if len(got) != 205 {
		t.Fatalf("resolved %d users, want 205", len(got))
	}
	for i := 0; i < 205; i++ {
		login := fmt.Sprintf("channel%03d", i)
		if got[login] != "id-"+login {
			t.Fatalf("resolved[%s]=%q", login, got[login])
		}
	}

	mu.Lock()
	defer mu.Unlock()
	want := []int{100, 100, 5}
	if fmt.Sprint(requestSizes) != fmt.Sprint(want) {
		t.Fatalf("request sizes %v, want %v", requestSizes, want)
	}
	if len(requestSizes) != 3 {
		t.Fatalf("requests=%d, want 3", len(requestSizes))
	}
}
