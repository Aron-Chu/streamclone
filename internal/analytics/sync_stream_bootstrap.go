package analytics

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// ensureAnalyticsStreamRow resolves the canonical write stream id and upserts a
// parent analytics_streams row before child writes (game segments, rollups).
func (s *SyncService) ensureAnalyticsStreamRow(
	ctx context.Context,
	streamID, broadcasterID, login, title string,
	startedAt time.Time,
) (string, error) {
	if s == nil || s.store == nil {
		return "", fmt.Errorf("analytics store unavailable")
	}
	streamID = strings.TrimSpace(streamID)
	login = normalizeLogin(login)
	if streamID == "" {
		return "", fmt.Errorf("stream id required")
	}
	if login == "" {
		return "", fmt.Errorf("login required for stream bootstrap")
	}

	writeID, err := s.store.ResolveStreamIDForWrite(ctx, streamID)
	if err != nil {
		return "", fmt.Errorf("resolve stream id for write: %w", err)
	}
	if err := s.store.UpsertStreamPlaceholder(ctx, writeID, broadcasterID, login, title, startedAt); err != nil {
		return "", fmt.Errorf("upsert stream placeholder: %w", err)
	}
	canon, err := s.store.ResolveStreamIDForWrite(ctx, writeID)
	if err != nil {
		return "", fmt.Errorf("resolve canonical stream id after upsert: %w", err)
	}
	if canon == "" {
		return writeID, nil
	}
	return canon, nil
}
