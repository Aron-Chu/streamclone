package archive

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// StreamExportData is the analytics stream row exported to blob storage.
type StreamExportData struct {
	StreamID          string     `json:"streamId"`
	BroadcasterID     string     `json:"broadcasterId"`
	Login             string     `json:"login"`
	DisplayName       string     `json:"displayName,omitempty"`
	Title             string     `json:"title,omitempty"`
	Category          string     `json:"category,omitempty"`
	StartedAt         time.Time  `json:"startedAt"`
	EndedAt           *time.Time `json:"endedAt,omitempty"`
	VodID             string     `json:"vodId,omitempty"`
	ViewerSource      string     `json:"viewerSource,omitempty"`
	CanonicalStreamID string     `json:"canonicalStreamId,omitempty"`
	AvgViewers        int        `json:"avgViewers"`
	PeakViewers       int        `json:"peakViewers"`
	ViewerSamples     int        `json:"viewerSamples"`
	ExportedAt        time.Time  `json:"exportedAt"`
}

// RollupExportLine is one JSONL row for minute rollups.
type RollupExportLine struct {
	MinuteTS          time.Time      `json:"minuteTs"`
	ViewerAvg         int            `json:"viewerAvg"`
	ViewerMax         int            `json:"viewerMax"`
	ViewerLatest      int            `json:"viewerLatest"`
	ViewerSamples     int            `json:"viewerSamples"`
	ChatCount         int            `json:"chatCount"`
	TotalEmoteCount   int            `json:"totalEmoteCount"`
	SevenTVEmoteCount int            `json:"seventvEmoteCount"`
	Emotes            map[string]int `json:"emotes,omitempty"`
}

// AnalyticsDB reads stream + rollup rows for export.
type AnalyticsDB interface {
	ExportStreamRow(ctx context.Context, streamID string) (*StreamExportData, error)
	ExportRollups(ctx context.Context, streamID string) ([]RollupExportLine, error)
}

// PgxAnalyticsDB implements AnalyticsDB against Postgres.
type PgxAnalyticsDB struct {
	db *pgxpool.Pool
}

func NewPgxAnalyticsDB(db *pgxpool.Pool) *PgxAnalyticsDB {
	return &PgxAnalyticsDB{db: db}
}

func (d *PgxAnalyticsDB) ExportStreamRow(ctx context.Context, streamID string) (*StreamExportData, error) {
	if d == nil || d.db == nil {
		return nil, fmt.Errorf("archive export: db unavailable")
	}
	var row StreamExportData
	var endedAt *time.Time
	var canonicalID, viewerSource string
	err := d.db.QueryRow(ctx, `
		SELECT stream_id, COALESCE(broadcaster_id,''), login, COALESCE(display_name,''),
			COALESCE(title,''), COALESCE(category,''), started_at, ended_at,
			COALESCE(vod_id,''), COALESCE(viewer_source,''),
			COALESCE(NULLIF(canonical_stream_id,''), stream_id),
			avg_viewers, peak_viewers, viewer_samples
		FROM analytics_streams
		WHERE stream_id = $1`, streamID,
	).Scan(
		&row.StreamID, &row.BroadcasterID, &row.Login, &row.DisplayName,
		&row.Title, &row.Category, &row.StartedAt, &endedAt,
		&row.VodID, &viewerSource, &canonicalID,
		&row.AvgViewers, &row.PeakViewers, &row.ViewerSamples,
	)
	if err != nil {
		return nil, err
	}
	row.EndedAt = endedAt
	row.ViewerSource = viewerSource
	row.CanonicalStreamID = canonicalID
	row.ExportedAt = time.Now().UTC()
	return &row, nil
}

func (d *PgxAnalyticsDB) ExportRollups(ctx context.Context, streamID string) ([]RollupExportLine, error) {
	if d == nil || d.db == nil {
		return nil, fmt.Errorf("archive export: db unavailable")
	}
	rows, err := d.db.Query(ctx, `
		SELECT minute_ts, viewer_avg, viewer_max, viewer_latest, viewer_samples,
			chat_count, total_emote_count, seventv_emote_count, emotes_json
		FROM analytics_minute_rollups
		WHERE stream_id = $1
		ORDER BY minute_ts ASC`, streamID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []RollupExportLine
	for rows.Next() {
		var line RollupExportLine
		var emotesRaw []byte
		if err := rows.Scan(
			&line.MinuteTS, &line.ViewerAvg, &line.ViewerMax, &line.ViewerLatest, &line.ViewerSamples,
			&line.ChatCount, &line.TotalEmoteCount, &line.SevenTVEmoteCount, &emotesRaw,
		); err != nil {
			return nil, err
		}
		if len(emotesRaw) > 0 {
			_ = json.Unmarshal(emotesRaw, &line.Emotes)
		}
		out = append(out, line)
	}
	return out, rows.Err()
}

// SyncExporter implements analytics.SyncArchiveExporter.
type SyncExporter struct {
	writer *Writer
	db     AnalyticsDB
}

func NewSyncExporter(writer *Writer, db AnalyticsDB) *SyncExporter {
	return &SyncExporter{writer: writer, db: db}
}

func (e *SyncExporter) ExportSync(ctx context.Context, streamID, channel, resultMessage string) error {
	if e == nil || e.writer == nil || e.db == nil {
		return fmt.Errorf("archive exporter is not configured")
	}
	streamID = strings.TrimSpace(streamID)
	if streamID == "" {
		return fmt.Errorf("archive export: stream id is required")
	}
	return e.writer.ExportStream(ctx, streamID, e.db)
}

func (w *Writer) ExportStream(ctx context.Context, streamID string, db AnalyticsDB) error {
	if w == nil || w.blob == nil {
		return fmt.Errorf("archive writer is not configured")
	}
	stream, err := db.ExportStreamRow(ctx, streamID)
	if err != nil {
		return fmt.Errorf("export stream row: %w", err)
	}
	sessionRaw, err := json.Marshal(stream)
	if err != nil {
		return err
	}
	sessionRes, err := w.putGzip(ctx, StreamSessionBlobKey(streamID), sessionRaw)
	if err != nil {
		return fmt.Errorf("upload stream session: %w", err)
	}
	sessionRes.RowCount = 1
	if err := w.confirmManifest(ctx, ArtifactAnalyticsStream, streamID, sessionRes); err != nil {
		return err
	}

	if login := strings.ToLower(strings.TrimSpace(stream.Login)); login != "" {
		channelRes, err := w.putGzip(ctx, StreamChannelBlobKey(login, streamID), sessionRaw)
		if err != nil {
			return fmt.Errorf("upload stream channel index: %w", err)
		}
		channelRes.RowCount = 1
		channelKey := fmt.Sprintf("streams:%s:%s", login, streamID)
		if err := w.confirmManifest(ctx, ArtifactAnalyticsStream, channelKey, channelRes); err != nil {
			return err
		}
	}

	rollups, err := db.ExportRollups(ctx, streamID)
	if err != nil {
		return fmt.Errorf("export rollups: %w", err)
	}
	var buf strings.Builder
	for _, line := range rollups {
		b, err := json.Marshal(line)
		if err != nil {
			return err
		}
		buf.Write(b)
		buf.WriteByte('\n')
	}
	rollupRes, err := w.putGzip(ctx, RollupsBlobKey(streamID), []byte(buf.String()))
	if err != nil {
		return fmt.Errorf("upload rollups: %w", err)
	}
	rollupRes.RowCount = int64(len(rollups))
	rollupKey := fmt.Sprintf("rollups:%s", streamID)
	return w.confirmManifest(ctx, ArtifactAnalyticsRollups, rollupKey, rollupRes)
}

// ExportTTDetail uploads optional TwitchTracker HTML (gap-fill / debug artifact).
func (w *Writer) ExportTTDetail(ctx context.Context, login, streamID string, html []byte) error {
	if w == nil || w.blob == nil {
		return fmt.Errorf("archive writer is not configured")
	}
	login = strings.ToLower(strings.TrimSpace(login))
	streamID = strings.TrimSpace(streamID)
	if login == "" || streamID == "" {
		return fmt.Errorf("archive export: login and stream id are required for tt-detail")
	}
	if len(html) == 0 {
		return nil
	}
	res, err := w.putGzip(ctx, TTDetailBlobKey(login, streamID), html)
	if err != nil {
		return fmt.Errorf("upload tt-detail: %w", err)
	}
	res.RowCount = 1
	naturalKey := fmt.Sprintf("tt-detail:%s:%s", login, streamID)
	return w.confirmManifest(ctx, ArtifactTTDetailHTML, naturalKey, res)
}

func (w *Writer) ExportPostgresDump(ctx context.Context, date string, sqlGzip []byte) error {
	if w == nil || w.blob == nil {
		return fmt.Errorf("archive writer is not configured")
	}
	if date == "" {
		date = time.Now().UTC().Format("2006-01-02")
	}
	res, err := w.blob.Put(ctx, PostgresNightlyBlobKey(date), sqlGzip, "application/gzip")
	if err != nil {
		return err
	}
	res.RowCount = 1
	return w.confirmManifest(ctx, "postgres_nightly", date, res)
}

func (w *Writer) ExportTopRoster(ctx context.Context, payload []byte) error {
	if w == nil || w.blob == nil {
		return fmt.Errorf("archive writer is not configured")
	}
	res, err := w.putGzip(ctx, TopRosterBlobKey(), payload)
	if err != nil {
		return err
	}
	res.RowCount = 1
	return w.confirmManifest(ctx, "tracked_roster", "top200", res)
}

// ExportTop500 uploads the reproducible Bronze channel list (top-N + always-tracked).
func (w *Writer) ExportTop500(ctx context.Context, payload []byte) error {
	if w == nil || w.blob == nil {
		return fmt.Errorf("archive writer is not configured")
	}
	res, err := w.putGzip(ctx, Top500BlobKey(), payload)
	if err != nil {
		return err
	}
	res.RowCount = 1
	return w.confirmManifest(ctx, ArtifactBronzeTop500, "top500", res)
}

// ExportChannelSummary uploads TwitchTracker summary JSON for one login.
func (w *Writer) ExportChannelSummary(ctx context.Context, login string, payload []byte) error {
	if w == nil || w.blob == nil {
		return fmt.Errorf("archive writer is not configured")
	}
	login = strings.ToLower(strings.TrimSpace(login))
	if login == "" {
		return fmt.Errorf("archive export: login is required for channel summary")
	}
	if len(payload) == 0 {
		return fmt.Errorf("archive export: empty summary payload for %q", login)
	}
	key := ChannelSummaryBlobKey(login)
	res, err := w.blob.Put(ctx, key, payload, "application/json")
	if err != nil {
		return fmt.Errorf("upload channel summary: %w", err)
	}
	res.RowCount = 1
	naturalKey := fmt.Sprintf("summary:%s", login)
	return w.confirmManifest(ctx, ArtifactBronzeChannelSummary, naturalKey, res)
}

// ExportVODIndex uploads Helix archive VOD list as JSONL.gz for one login.
func (w *Writer) ExportVODIndex(ctx context.Context, login string, lines []byte) error {
	if w == nil || w.blob == nil {
		return fmt.Errorf("archive writer is not configured")
	}
	login = strings.ToLower(strings.TrimSpace(login))
	if login == "" {
		return fmt.Errorf("archive export: login is required for vod index")
	}
	res, err := w.putGzip(ctx, VODIndexBlobKey(login), lines)
	if err != nil {
		return fmt.Errorf("upload vod index: %w", err)
	}
	rowCount := int64(bytes.Count(lines, []byte{'\n'}))
	if len(lines) > 0 && lines[len(lines)-1] != '\n' {
		rowCount++
	}
	if rowCount == 0 && len(lines) > 0 {
		rowCount = 1
	}
	res.RowCount = rowCount
	naturalKey := fmt.Sprintf("vod_index:%s", login)
	return w.confirmManifest(ctx, ArtifactBronzeVODIndex, naturalKey, res)
}
