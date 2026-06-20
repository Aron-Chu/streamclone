package develop

import (
	"context"
	"fmt"

	"streamclone/internal/storygraph/store"
)

// Service handles Developing queue confirmation.
type Service struct {
	store *store.Store
}

func New(st *store.Store) *Service {
	return &Service{store: st}
}

// Confirm promotes, merges, or rejects a developing story.
func (s *Service) Confirm(ctx context.Context, clusterID int64, action string) error {
	switch action {
	case "confirm", "merge":
		return s.store.UpdateClusterState(ctx, clusterID, "published")
	case "reject":
		return s.store.UpdateClusterState(ctx, clusterID, "settled")
	default:
		return fmt.Errorf("unknown action %q", action)
	}
}
