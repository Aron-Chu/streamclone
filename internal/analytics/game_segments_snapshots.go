package analytics

import (
	"context"
	"sort"
	"strings"
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
	if stream.EndedAt != nil && len(stored) >= 2 && distinctGameCategoryCount(stored) >= 2 {
		return false
	}
	if stream.EndedAt == nil && (len(stored) <= 1 || distinctGameCategoryCount(stored) <= 1) {
		return true
	}
	return len(stored) == 0
}

func meaningfulGameSegments(segments []GameSegment) bool {
	return distinctGameCategoryCount(segments) > 0
}

func filterMeaningfulStoredSegments(segments []GameSegment) []GameSegment {
	if len(segments) == 0 {
		return segments
	}
	out := make([]GameSegment, 0, len(segments))
	for _, seg := range segments {
		if normalizeSyncGameCategory(seg.GameName) == "" {
			continue
		}
		out = append(out, seg)
	}
	return out
}

func distinctGameCategoryCount(segments []GameSegment) int {
	if len(segments) == 0 {
		return 0
	}
	seen := make(map[string]struct{}, len(segments))
	for _, seg := range segments {
		name := normalizeSyncGameCategory(seg.GameName)
		if name == "" {
			continue
		}
		seen[strings.ToLower(name)] = struct{}{}
	}
	return len(seen)
}

func preferGameSegments(stored, snapshot []GameSegment) []GameSegment {
	if len(snapshot) == 0 {
		return stored
	}
	if len(stored) == 0 {
		return snapshot
	}
	snapDistinct := distinctGameCategoryCount(snapshot)
	storedDistinct := distinctGameCategoryCount(stored)
	if snapDistinct > storedDistinct {
		return snapshot
	}
	if snapDistinct == storedDistinct && len(snapshot) > len(stored) {
		return snapshot
	}
	return stored
}

func assignGameSegmentSource(segments []GameSegment, source string) []GameSegment {
	if source == "" || len(segments) == 0 {
		return segments
	}
	out := make([]GameSegment, len(segments))
	copy(out, segments)
	for i := range out {
		if out[i].Source == "" {
			out[i].Source = source
		}
	}
	return out
}

func distinctCategoryCount(samples []categoryTimelineSample) int {
	if len(samples) == 0 {
		return 0
	}
	seen := make(map[string]struct{}, len(samples))
	for _, sample := range samples {
		name := normalizeSyncGameCategory(sample.Category)
		if name == "" {
			continue
		}
		seen[strings.ToLower(name)] = struct{}{}
	}
	return len(seen)
}

func mergeCategoryTimelineSamples(primary, extra []categoryTimelineSample) []categoryTimelineSample {
	if len(extra) == 0 {
		return primary
	}
	if len(primary) == 0 {
		return extra
	}
	merged := append(append([]categoryTimelineSample{}, primary...), extra...)
	sort.Slice(merged, func(i, j int) bool {
		if merged[i].At.Equal(merged[j].At) {
			return merged[i].Category < merged[j].Category
		}
		return merged[i].At.Before(merged[j].At)
	})
	out := merged[:0]
	var prevAt time.Time
	var prevName string
	for _, sample := range merged {
		name := normalizeSyncGameCategory(sample.Category)
		if name == "" {
			continue
		}
		if !prevAt.IsZero() && sample.At.Equal(prevAt) && strings.EqualFold(name, prevName) {
			continue
		}
		out = append(out, sample)
		prevAt = sample.At
		prevName = name
	}
	return out
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
			Source:          "snapshot",
		})
	}
	return out
}

func (s *Store) loadCategoryTimelineSamplesByStreamID(
	ctx context.Context,
	streamID string,
	startedAt, endAt time.Time,
) ([]categoryTimelineSample, error) {
	if s == nil || s.db == nil || streamID == "" {
		return nil, nil
	}
	rows, err := s.db.Query(ctx, `
		SELECT category_name, sample_tick_at
		FROM top500_live_snapshots
		WHERE stream_id = $1
		  AND sample_tick_at >= $2
		  AND sample_tick_at <= $3
		  AND btrim(category_name) <> ''
		ORDER BY sample_tick_at ASC`, streamID, startedAt.UTC(), endAt)
	if err != nil {
		if isUndefinedTableError(err) {
			return nil, nil
		}
		return nil, err
	}
	defer rows.Close()
	return scanCategoryTimelineSamples(rows)
}

func (s *Store) loadCategoryTimelineSamplesByChannelID(
	ctx context.Context,
	channelID string,
	startedAt, endAt time.Time,
) ([]categoryTimelineSample, error) {
	if s == nil || s.db == nil || strings.TrimSpace(channelID) == "" {
		return nil, nil
	}
	rows, err := s.db.Query(ctx, `
		SELECT category_name, sample_tick_at
		FROM top500_live_snapshots
		WHERE channel_id = $1
		  AND sample_tick_at >= $2
		  AND sample_tick_at <= $3
		  AND btrim(category_name) <> ''
		ORDER BY sample_tick_at ASC`, strings.TrimSpace(channelID), startedAt.UTC(), endAt)
	if err != nil {
		if isUndefinedTableError(err) {
			return nil, nil
		}
		return nil, err
	}
	defer rows.Close()
	return scanCategoryTimelineSamples(rows)
}

type categoryTimelineRowScanner interface {
	Next() bool
	Scan(dest ...any) error
	Err() error
}

func scanCategoryTimelineSamples(rows categoryTimelineRowScanner) ([]categoryTimelineSample, error) {
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
	return samples, nil
}

// GameSegmentsFromTop500Snapshots builds chart game segments from metadata sample ticks.
func (s *Store) GameSegmentsFromTop500Snapshots(
	ctx context.Context,
	streamID string,
	channelID string,
	startedAt time.Time,
	endedAt *time.Time,
) ([]GameSegment, error) {
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

	samples, err := s.loadCategoryTimelineSamplesByStreamID(ctx, canonicalID, startedAt, endAt)
	if err != nil {
		return nil, err
	}
	if distinctCategoryCount(samples) < 2 && strings.TrimSpace(channelID) != "" {
		channelSamples, channelErr := s.loadCategoryTimelineSamplesByChannelID(ctx, channelID, startedAt, endAt)
		if channelErr != nil {
			return nil, channelErr
		}
		samples = mergeCategoryTimelineSamples(samples, channelSamples)
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
	segments = filterMeaningfulStoredSegments(segments)
	storedSegments := assignGameSegmentSource(segments, "stored")
	stream, streamErr := h.store.StreamByID(ctx, streamID)
	if streamErr != nil {
		if !meaningfulGameSegments(storedSegments) {
			return []GameSegment{}, nil
		}
		return storedSegments, nil
	}
	if shouldTrySnapshotGameSegments(storedSegments, stream) {
		snapshot, snapErr := h.store.GameSegmentsFromTop500Snapshots(
			ctx,
			streamID,
			stream.BroadcasterID,
			stream.StartedAt,
			stream.EndedAt,
		)
		if snapErr != nil {
			return nil, snapErr
		}
		snapshot = assignGameSegmentSource(snapshot, "snapshot")
		segments = preferGameSegments(storedSegments, snapshot)
	} else {
		segments = storedSegments
	}
	if !meaningfulGameSegments(segments) {
		segments = assignGameSegmentSource(fallbackGameSegmentsForStream(stream), "category_fallback")
	}
	if segments == nil {
		segments = []GameSegment{}
	}
	return segments, nil
}
