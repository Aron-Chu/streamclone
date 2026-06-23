package canonicalstream

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5"
)

// Querier is the minimal pgx pool/transaction shape needed for stream alias resolution.
type Querier interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

// Resolution describes a transitive canonical stream lookup.
type Resolution struct {
	CanonicalID   string
	Path          []string
	Cycle         []string
	CycleDetected bool
}

// Resolve follows analytics_stream_aliases / analytics_streams canonical links.
func Resolve(ctx context.Context, q Querier, streamID string) (Resolution, error) {
	streamID = strings.TrimSpace(streamID)
	if streamID == "" {
		return Resolution{}, nil
	}
	seen := map[string]int{}
	path := []string{}
	current := streamID
	for depth := 0; depth < 64; depth++ {
		if idx, ok := seen[current]; ok {
			cycle := append([]string{}, path[idx:]...)
			return Resolution{
				CanonicalID:   StableID(cycle),
				Path:          path,
				Cycle:         cycle,
				CycleDetected: true,
			}, nil
		}
		seen[current] = len(path)
		path = append(path, current)

		var next string
		err := q.QueryRow(ctx, `
			SELECT COALESCE(
				(SELECT NULLIF(canonical_stream_id, '') FROM analytics_stream_aliases WHERE alias_stream_id = $1),
				(SELECT NULLIF(canonical_stream_id, '') FROM analytics_streams WHERE stream_id = $1),
				''
			)`, current).Scan(&next)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return Resolution{CanonicalID: current, Path: path}, nil
			}
			return Resolution{}, err
		}
		next = strings.TrimSpace(next)
		if next == "" || next == current {
			return Resolution{CanonicalID: current, Path: path}, nil
		}
		current = next
	}
	return Resolution{}, fmt.Errorf("canonical stream alias chain exceeded 64 hops from %s", streamID)
}

// StableID returns a deterministic ID from a set of candidates.
func StableID(ids []string) string {
	ids = Unique(ids)
	sort.Strings(ids)
	if len(ids) == 0 {
		return ""
	}
	return ids[0]
}

// Unique trims, de-duplicates, and preserves first-seen order.
func Unique(ids []string) []string {
	seen := make(map[string]struct{}, len(ids))
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}
