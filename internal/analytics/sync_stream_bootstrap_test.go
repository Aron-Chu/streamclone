package analytics

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestEnsureAnalyticsStreamRowNilService(t *testing.T) {
	var s *SyncService
	_, err := s.ensureAnalyticsStreamRow(context.Background(), "123", "bc", "login", "title", time.Now())
	if err == nil || !strings.Contains(err.Error(), "unavailable") {
		t.Fatalf("nil service err = %v, want unavailable", err)
	}
}

func TestEnsureAnalyticsStreamRowNilStore(t *testing.T) {
	s := &SyncService{}
	_, err := s.ensureAnalyticsStreamRow(context.Background(), "123", "bc", "login", "title", time.Now())
	if err == nil || !strings.Contains(err.Error(), "unavailable") {
		t.Fatalf("nil store err = %v, want unavailable", err)
	}
}

func TestEnsureAnalyticsStreamRowMissingStreamID(t *testing.T) {
	s := &SyncService{store: &Store{}}
	_, err := s.ensureAnalyticsStreamRow(context.Background(), "", "bc", "login", "title", time.Now())
	if err == nil || !strings.Contains(err.Error(), "stream id required") {
		t.Fatalf("empty stream id err = %v", err)
	}
}

func TestEnsureAnalyticsStreamRowMissingLogin(t *testing.T) {
	s := &SyncService{store: &Store{}}
	_, err := s.ensureAnalyticsStreamRow(context.Background(), "123", "bc", "", "title", time.Now())
	if err == nil || !strings.Contains(err.Error(), "login required") {
		t.Fatalf("empty login err = %v", err)
	}
}
