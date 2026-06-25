package analytics

import (
	"context"
	"database/sql"
	"fmt"
)

// StoreSilverCandidateReader loads gate candidates from top500_current (read-only).
type StoreSilverCandidateReader struct {
	Store *Store
}

func (r StoreSilverCandidateReader) ListCandidates(ctx context.Context, limit int) ([]SilverGateCandidate, error) {
	if r.Store == nil || r.Store.db == nil {
		return nil, nil
	}
	if limit <= 0 {
		limit = DefaultTop500SilverGateMaxCandidates
	}
	rows, err := r.Store.db.Query(ctx, `
		SELECT channel_id, login, rank, stream_id, viewer_count, is_live, started_at, sampled_at, stale_after
		FROM top500_current
		ORDER BY rank ASC, login ASC
		LIMIT $1`, limit)
	if err != nil {
		return nil, fmt.Errorf("list top500 silver gate candidates: %w", err)
	}
	defer rows.Close()

	out := make([]SilverGateCandidate, 0, limit)
	for rows.Next() {
		var cand SilverGateCandidate
		var streamID sql.NullString
		var startedAt sql.NullTime
		var viewerCount sql.NullInt64
		if err := rows.Scan(
			&cand.ChannelID, &cand.Login, &cand.Rank, &streamID, &viewerCount, &cand.IsLive,
			&startedAt, &cand.SampledAt, &cand.StaleAfter,
		); err != nil {
			return nil, fmt.Errorf("scan top500 silver gate candidate: %w", err)
		}
		if streamID.Valid {
			cand.StreamID = streamID.String
		}
		if startedAt.Valid {
			cand.StartedAt = startedAt.Time
		}
		if viewerCount.Valid {
			cand.ViewerCount = int(viewerCount.Int64)
		}
		cand.CandidateSource = "top500_current"
		cand.CandidateID = cand.ChannelID
		out = append(out, cand)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}
