package archive

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func DirectorySampleBlobKey(date, hour string) string {
	h, _ := strconv.Atoi(strings.TrimSpace(hour))
	return fmt.Sprintf("directory/samples/date=%s/hour=%02d/part-000.jsonl.gz", date, h)
}

func ChatModEventsBlobKey(login, date string) string {
	login = strings.ToLower(strings.TrimSpace(login))
	return fmt.Sprintf("chat_mod_events/login=%s/date=%s/events.jsonl.gz", login, date)
}

func chatModEventNaturalKey(channel, kind, messageID string, ts time.Time, targetLogin string) string {
	return fmt.Sprintf("%s:%s:%s:%d:%s",
		strings.ToLower(strings.TrimSpace(channel)),
		strings.TrimSpace(kind),
		strings.TrimSpace(messageID),
		ts.UTC().Unix(),
		strings.TrimSpace(targetLogin),
	)
}

func ModEventNaturalKey(channel, kind, messageID string, ts time.Time, targetLogin string) string {
	return chatModEventNaturalKey(channel, kind, messageID, ts, targetLogin)
}

// ExportDirectorySamples uploads one UTC hour partition of directory_samples rows.
func (w *Writer) ExportDirectorySamples(ctx context.Context, db *pgxpool.Pool, date, hour string) error {
	if w == nil || w.blob == nil || db == nil {
		return fmt.Errorf("archive writer is not configured")
	}
	date = strings.TrimSpace(date)
	hourInt, err := strconv.Atoi(strings.TrimSpace(hour))
	if err != nil || hourInt < 0 || hourInt > 23 {
		return fmt.Errorf("archive export: hour must be 0-23")
	}
	if date == "" {
		date = time.Now().UTC().Format("2006-01-02")
	}
	dayStart, err := time.ParseInLocation("2006-01-02", date, time.UTC)
	if err != nil {
		return fmt.Errorf("archive export: invalid date: %w", err)
	}
	windowStart := dayStart.Add(time.Duration(hourInt) * time.Hour)
	windowEnd := windowStart.Add(time.Hour)
	type sampleRow struct {
		line      DirectorySampleExportLine
		naturalKey string
	}
	rows, err := db.Query(ctx, `
		SELECT twitch_login, COALESCE(twitch_id,''), COALESCE(display_name,''), COALESCE(category,''),
			viewers, rank, is_live, sample_run_id, sampled_at
		FROM directory_samples
		WHERE sampled_at >= $1 AND sampled_at < $2
		ORDER BY rank ASC, twitch_login ASC`, windowStart, windowEnd)
	if err != nil {
		return err
	}
	defer rows.Close()
	var samples []sampleRow
	var buf strings.Builder
	for rows.Next() {
		var line DirectorySampleExportLine
		if err := rows.Scan(
			&line.TwitchLogin, &line.TwitchID, &line.DisplayName, &line.Category,
			&line.Viewers, &line.Rank, &line.IsLive, &line.SampleRunID, &line.SampledAt,
		); err != nil {
			return err
		}
		raw, err := json.Marshal(line)
		if err != nil {
			return err
		}
		buf.Write(raw)
		buf.WriteByte('\n')
		samples = append(samples, sampleRow{
			line:       line,
			naturalKey: line.SampleRunID + ":" + strings.ToLower(line.TwitchLogin),
		})
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if len(samples) == 0 {
		return nil
	}
	res, err := w.putGzip(ctx, DirectorySampleBlobKey(date, hour), []byte(buf.String()))
	if err != nil {
		recordArchiveExportFailed(ArtifactDirectorySample)
		return fmt.Errorf("upload directory samples: %w", err)
	}
	res.RowCount = int64(len(samples))
	batchKey := fmt.Sprintf("directory:%s:%02d", date, hourInt)
	if err := w.confirmManifest(ctx, ArtifactDirectorySample, batchKey, res); err != nil {
		return err
	}
	for _, sample := range samples {
		rowRes := res
		rowRes.RowCount = 1
		if err := w.confirmManifest(ctx, ArtifactDirectorySample, sample.naturalKey, rowRes); err != nil {
			return err
		}
	}
	return nil
}

// DirectorySampleExportLine is one JSONL row for directory sample cold storage.
type DirectorySampleExportLine struct {
	TwitchLogin string    `json:"twitchLogin"`
	TwitchID    string    `json:"twitchId"`
	DisplayName string    `json:"displayName"`
	Category    string    `json:"category"`
	Viewers     int       `json:"viewers"`
	Rank        int       `json:"rank"`
	IsLive      bool      `json:"isLive"`
	SampleRunID string    `json:"sampleRunId"`
	SampledAt   time.Time `json:"sampledAt"`
}

// ExportChatModEvents uploads mod events for one login and UTC date.
func (w *Writer) ExportChatModEvents(ctx context.Context, db *pgxpool.Pool, login, date string) error {
	if w == nil || w.blob == nil || db == nil {
		return fmt.Errorf("archive writer is not configured")
	}
	login = strings.ToLower(strings.TrimSpace(login))
	date = strings.TrimSpace(date)
	if login == "" || date == "" {
		return fmt.Errorf("archive export: login and date are required")
	}
	dayStart, err := time.ParseInLocation("2006-01-02", date, time.UTC)
	if err != nil {
		return fmt.Errorf("archive export: invalid date: %w", err)
	}
	dayEnd := dayStart.Add(24 * time.Hour)
	rows, err := db.Query(ctx, `
		SELECT id, channel, kind, COALESCE(actor_login,''), COALESCE(target_login,''),
			COALESCE(duration_sec,0), COALESCE(reason,''), COALESCE(message_id,''), COALESCE(text_preview,''),
			ts, synced_at
		FROM chat_mod_events
		WHERE channel = $1 AND ts >= $2 AND ts < $3
		ORDER BY ts ASC, id ASC`, login, dayStart, dayEnd)
	if err != nil {
		return err
	}
	defer rows.Close()
	type modRow struct {
		line       map[string]any
		naturalKey string
	}
	var events []modRow
	var buf strings.Builder
	for rows.Next() {
		var id int64
		var channel, kind, actor, target, reason, messageID, preview string
		var duration int
		var ts, syncedAt time.Time
		if err := rows.Scan(&id, &channel, &kind, &actor, &target, &duration, &reason, &messageID, &preview, &ts, &syncedAt); err != nil {
			return err
		}
		line := map[string]any{
			"id": id, "channel": channel, "kind": kind, "ts": ts, "syncedAt": syncedAt,
		}
		if actor != "" {
			line["actorLogin"] = actor
		}
		if target != "" {
			line["targetLogin"] = target
		}
		if duration > 0 {
			line["durationSec"] = duration
		}
		if reason != "" {
			line["reason"] = reason
		}
		if messageID != "" {
			line["messageId"] = messageID
		}
		if preview != "" {
			line["textPreview"] = preview
		}
		raw, err := json.Marshal(line)
		if err != nil {
			return err
		}
		buf.Write(raw)
		buf.WriteByte('\n')
		events = append(events, modRow{
			line:       line,
			naturalKey: chatModEventNaturalKey(channel, kind, messageID, ts, target),
		})
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if len(events) == 0 {
		return nil
	}
	res, err := w.putGzip(ctx, ChatModEventsBlobKey(login, date), []byte(buf.String()))
	if err != nil {
		recordArchiveExportFailed(ArtifactChatModEvent)
		return fmt.Errorf("upload chat mod events: %w", err)
	}
	res.RowCount = int64(len(events))
	batchKey := fmt.Sprintf("chat_mod_events:%s:%s", login, date)
	if err := w.confirmManifest(ctx, ArtifactChatModEvent, batchKey, res); err != nil {
		return err
	}
	for _, ev := range events {
		rowRes := res
		rowRes.RowCount = 1
		if err := w.confirmManifest(ctx, ArtifactChatModEvent, ev.naturalKey, rowRes); err != nil {
			return err
		}
	}
	return nil
}

// ExportModEventsBeforePurge exports login/date partitions for rows older than cutoff.
func (w *Writer) ExportModEventsBeforePurge(ctx context.Context, db *pgxpool.Pool, cutoff time.Time) error {
	if w == nil || db == nil {
		return nil
	}
	rows, err := db.Query(ctx, `
		SELECT DISTINCT channel, to_char(ts AT TIME ZONE 'UTC', 'YYYY-MM-DD') AS day
		FROM chat_mod_events
		WHERE synced_at < $1
		ORDER BY channel, day`, cutoff.UTC())
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var login, day string
		if err := rows.Scan(&login, &day); err != nil {
			return err
		}
		if err := w.ExportChatModEvents(ctx, db, login, day); err != nil {
			return err
		}
	}
	return rows.Err()
}

// ModEventArchiveAdapter wires Writer export into chatreplay retention purge.
type ModEventArchiveAdapter struct {
	Writer *Writer
	DB     *pgxpool.Pool
}

func (a *ModEventArchiveAdapter) ExportPendingModEventsBeforeCutoff(ctx context.Context, cutoff time.Time) error {
	if a == nil || a.Writer == nil {
		return nil
	}
	return a.Writer.ExportModEventsBeforePurge(ctx, a.DB, cutoff)
}
