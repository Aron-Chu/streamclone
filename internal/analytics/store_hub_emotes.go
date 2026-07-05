package analytics

import (
	"context"
	"strings"
	"time"
)

// AggregateRollupTopEmotesBucketsSince sums emote uses from emotes_json for live IRC
// rollups across the corpus, keyed by bucket start (Unix milliseconds). Each bucket
// map holds at most topN emote keys by total count — read server-side only.
func (s *Store) AggregateRollupTopEmotesBucketsSince(ctx context.Context, since time.Time, bucketMinutes, limit, topN int) (map[int64]map[string]int, error) {
	out := map[int64]map[string]int{}
	if s == nil || s.db == nil {
		return out, nil
	}
	if bucketMinutes <= 0 {
		bucketMinutes = 1
	}
	if limit <= 0 || limit > 240 {
		limit = 240
	}
	if topN <= 0 {
		topN = 10
	}
	rows, err := s.db.Query(ctx, `
		WITH bucketed AS (
			SELECT
				to_timestamp(floor(extract(epoch from minute_ts) / ($2::double precision * 60)) * ($2::double precision * 60)) AS bucket_ts,
				emotes_json,
				chat_count,
				chat_source,
				source_confidence,
				viewer_samples
			FROM analytics_minute_rollups
			WHERE minute_ts >= $1
		),
		expanded AS (
			SELECT
				b.bucket_ts,
				e.key AS emote_key,
				e.value::int AS cnt
			FROM bucketed b,
			LATERAL jsonb_each_text(COALESCE(b.emotes_json, '{}'::jsonb)) AS e(key, value)
			WHERE `+sqlPublicLiveChatMinutePredicate+`
				AND e.value::int > 0
		),
		aggregated AS (
			SELECT bucket_ts, emote_key, SUM(cnt)::int AS total
			FROM expanded
			GROUP BY bucket_ts, emote_key
		),
		ranked AS (
			SELECT
				bucket_ts,
				emote_key,
				total,
				ROW_NUMBER() OVER (PARTITION BY bucket_ts ORDER BY total DESC, emote_key ASC) AS rn
			FROM aggregated
		),
		recent_buckets AS (
			SELECT DISTINCT bucket_ts
			FROM aggregated
			ORDER BY bucket_ts DESC
			LIMIT $3
		)
		SELECT
			(extract(epoch FROM r.bucket_ts) * 1000)::bigint AS bucket_ms,
			r.emote_key,
			r.total
		FROM ranked r
		INNER JOIN recent_buckets rb ON rb.bucket_ts = r.bucket_ts
		WHERE r.rn <= $4
		ORDER BY r.bucket_ts ASC, r.total DESC`, since.UTC(), bucketMinutes, limit, topN)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var bucketMS int64
		var emoteKey string
		var total int
		if err := rows.Scan(&bucketMS, &emoteKey, &total); err != nil {
			return out, err
		}
		emoteKey = strings.TrimSpace(emoteKey)
		if emoteKey == "" || total <= 0 {
			continue
		}
		if out[bucketMS] == nil {
			out[bucketMS] = map[string]int{}
		}
		out[bucketMS][emoteKey] = total
	}
	return out, rows.Err()
}
