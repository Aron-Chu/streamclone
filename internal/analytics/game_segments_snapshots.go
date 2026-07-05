package analytics

import (
	"context"
	"time"
)

type categoryTimelineSample struct {
	At       time.Time
	Category string
}

func shouldTrySnapshotGameSegments(stored []GameSegment, stream *StreamRecord) bool {
	if len(stored) == 0 {
		return true
	}
	if stream == nil {
		return false
	}
	return stream.EndedAt == nil && len(stored) <= 1
}

func preferGameSegments(stored, snapshot []GameSegment) []GameSegment {
	if len(snapshot) == 0 {
		return stored
	}
	if len(stored) == 0 {
		return snapshot
	}
	if len(snapshot) > len(stored) {
		return snapshot
	}
	return stored
}

func buildGameSegmentsFromCategoryTimeline(streamID string, startedAt, endAt time.Time, samples []categoryTimelineSample) []GameSegment {
	if streamID == "" || startedAt.IsZero() || len(samples) == 0 {
		return nil
	}
	startedAt = startedAt.UTC()
	if endAt.IsZero() || !endAt.After(startedAt) {
		endAt = time.Now().UTC()
	} else {
		endAt = endAt.UTC()
	}

	type segmentStart struct {
		name  string
		start time.Time
	}
	collapsed := make([]segmentStart, 0, len(samples))
	for _, sample := range samples {
		at := sample.At.UTC()
		if at.Before(startedAt) || at.After(endAt) {
			continue
		}
		name := normalizeSyncGameCategory(sample.Category)
		if name == "" {
			continue
		}
		if len(collapsed) == 0 || collapsed[len(collapsed)-1].name != name {
			collapsed = append(collapsed, segmentStart{name: name, start: at})
		}
	}
	if len(collapsed) == 0 {
		return nil
	}

	out := make([]GameSegment, 0, len(collapsed))
	for i, cur := range collapsed {
		segStart := cur.start
		if i == 0 {
			segStart = startedAt
		}
		offset := int(segStart.Sub(startedAt).Seconds())
		if offset < 0 {
			offset = 0
		}
		nextStart := endAt
		if i+1 < len(collapsed) {
			nextStart = collapsed[i+1].start
		}
		duration := int(nextStart.Sub(segStart).Seconds())
		if duration <= 0 {
			duration = 60
		}
		out = append(out, GameSegment{
			StreamID:        streamID,
			GameName:        cur.name,
			OffsetSeconds:   offset,
			DurationSeconds: duration,
		})
	}
	return out
}

// GameSegmentsFromTop500Snapshots builds chart game segments from metadata sample ticks.
func (s *Store) GameSegmentsFromTop500Snapshots(ctx context.Context, streamID string, startedAt time.Time, endedAt *time.Time) ([]GameSegment, error) {
	if s == nil || s.db == nil || streamID == "" || startedAt.IsZero() {
		return nil, nil
	}
	canonicalID, err := s.ResolveCanonicalStreamID(ctx, streamID)
	if err != nil {
		return nil, err
	}
	endAt := time.Now().UTC()
	if endedAt != nil && !endedAt.IsZero() {
		endAt = endedAt.UTC()
	}

	rows, err := s.db.Query(ctx, `
		SELECT category_name, sampled_at
		FROM top500_live_snapshots
		WHERE stream_id = $1
		  AND sampled_at >= $2
		  AND sampled_at <= $3
		  AND btrim(category_name) <> ''
		ORDER BY sampled_at ASC`, canonicalID, startedAt.UTC(), endAt)
	if err != nil {
		if isUndefinedTableError(err) {
			return nil, nil
		}
		return nil, err
	}
	defer rows.Close()

	samples := make([]categoryTimelineSample, 0, 32)
	for rows.Next() {
		var name string
		var at time.Time
		if err := rows.Scan(&name, &at); err != nil {
			return nil, err
		}
		samples = append(samples, categoryTimelineSample{At: at, Category: name})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return buildGameSegmentsFromCategoryTimeline(canonicalID, startedAt, endAt, samples), nil
}

func (h *Handler) resolveStreamGameSegments(ctx context.Context, streamID string) ([]GameSegment, error) {
	if h == nil || h.store == nil {
		return nil, nil
	}
	segments, err := h.store.GetGameSegments(ctx, streamID)
	if err != nil {
		return nil, err
	}
	stream, streamErr := h.store.StreamByID(ctx, streamID)
	if streamErr != nil {
		if len(segments) == 0 {
			return []GameSegment{}, nil
		}
		return segments, nil
	}
	if shouldTrySnapshotGameSegments(segments, stream) {
		snapshot, snapErr := h.store.GameSegmentsFromTop500Snapshots(ctx, streamID, stream.StartedAt, stream.EndedAt)
		if snapErr != nil {
			return nil, snapErr
		}
		segments = preferGameSegments(segments, snapshot)
	}
	if len(segments) == 0 {
		segments = fallbackGameSegmentsForStream(stream)
	}
	if segments == nil {
		segments = []GameSegment{}
	}
	return segments, nil
}
