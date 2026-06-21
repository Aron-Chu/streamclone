package jobtracker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	JobStatusQueued     = "queued"
	JobStatusRunning    = "running"
	JobStatusSucceeded  = "succeeded"
	JobStatusPartial    = "partial"
	JobStatusFailed     = "failed"
	JobStatusCancelled  = "cancelled"
	JobStatusStale      = "stale"

	ItemStatusQueued    = "queued"
	ItemStatusRunning   = "running"
	ItemStatusSucceeded = "succeeded"
	ItemStatusSkipped   = "skipped"
	ItemStatusFailed    = "failed"
	ItemStatusRetrying  = "retrying"
	ItemStatusCancelled = "cancelled"

	JobTypeBronzeRoster   = "bronze_roster"
	JobTypeBronzeChannel  = "bronze_channel"
	JobTypeSilverRollup   = "silver_viewer_rollup"
	JobTypeEmoteSnapshot  = "emote_snapshot"
	JobTypeCoverageReport = "coverage_report"
	JobTypeBlobVerify     = "blob_verify"
)

type Job struct {
	ID              string
	JobType         string
	Tier            string
	Status          string
	TriggerSource   string
	TotalItems      int
	CompletedItems  int
	FailedItems     int
	SkippedItems    int
	RetriedItems    int
	StartedAt       *time.Time
	UpdatedAt       time.Time
	FinishedAt      *time.Time
	HeartbeatAt     *time.Time
	Error           string
	Metadata        map[string]any
}

type Item struct {
	ID            string
	JobID         string
	ItemKey       string
	ChannelLogin  string
	ChannelID     string
	StreamID      string
	Status        string
	Attempts      int
	Error         string
	OutputURI     string
	OutputSHA256  string
	BackfillJobID *int64
	Metadata      map[string]any
}

type Tracker struct {
	db              *pgxpool.Pool
	heartbeatEvery  time.Duration
	staleAfter      time.Duration
	eventLogEnabled bool
}

func NewTracker(db *pgxpool.Pool, heartbeatEvery, staleAfter time.Duration, eventLog bool) *Tracker {
	if heartbeatEvery <= 0 {
		heartbeatEvery = 15 * time.Second
	}
	if staleAfter <= 0 {
		staleAfter = 10 * time.Minute
	}
	return &Tracker{db: db, heartbeatEvery: heartbeatEvery, staleAfter: staleAfter, eventLogEnabled: eventLog}
}

func (t *Tracker) CreateJob(ctx context.Context, jobType, tier, trigger string, metadata map[string]any) (string, error) {
	if t == nil || t.db == nil {
		return "", errors.New("jobtracker: unavailable")
	}
	metaJSON, err := json.Marshal(metadataOrEmpty(metadata))
	if err != nil {
		return "", err
	}
	id := uuid.NewString()
	now := time.Now().UTC()
	_, err = t.db.Exec(ctx, `
		INSERT INTO archive_jobs (
			id, job_type, tier, status, trigger_source, started_at, updated_at, heartbeat_at, metadata
		) VALUES ($1,$2,$3,$4,$5,$6,$6,$6,$7)`,
		id, jobType, tier, JobStatusRunning, trigger, now, metaJSON,
	)
	if err != nil {
		return "", err
	}
	t.logEvent(ctx, id, "", "info", "job_started", "job created", nil)
	return id, nil
}

func (t *Tracker) SetTotalItems(ctx context.Context, jobID string, total int) error {
	_, err := t.db.Exec(ctx, `
		UPDATE archive_jobs SET total_items=$2, updated_at=now() WHERE id=$1`, jobID, total)
	return err
}

func (t *Tracker) Heartbeat(ctx context.Context, jobID string) error {
	_, err := t.db.Exec(ctx, `
		UPDATE archive_jobs SET heartbeat_at=now(), updated_at=now() WHERE id=$1`, jobID)
	return err
}

func (t *Tracker) FinishJob(ctx context.Context, jobID string, status string, errMsg string) error {
	status = strings.TrimSpace(status)
	if status == "" {
		status = JobStatusSucceeded
	}
	_, err := t.db.Exec(ctx, `
		UPDATE archive_jobs
		SET status=$2, error=NULLIF($3,''), finished_at=now(), updated_at=now()
		WHERE id=$1`, jobID, status, errMsg)
	if err == nil {
		t.logEvent(ctx, jobID, "", "info", "job_finished", status, map[string]any{"error": errMsg})
	}
	return err
}

func (t *Tracker) UpsertItem(ctx context.Context, jobID, itemKey string, login, channelID, streamID, artifactType string) (string, error) {
	id := uuid.NewString()
	_, err := t.db.Exec(ctx, `
		INSERT INTO archive_job_items (
			id, job_id, item_key, channel_login, channel_id, stream_id, artifact_type, status, updated_at
		) VALUES ($1,$2,$3,NULLIF($4,''),NULLIF($5,''),NULLIF($6,''),NULLIF($7,''),$8,now())
		ON CONFLICT (job_id, item_key) DO UPDATE SET
			channel_login = COALESCE(NULLIF(EXCLUDED.channel_login,''), archive_job_items.channel_login),
			channel_id = COALESCE(NULLIF(EXCLUDED.channel_id,''), archive_job_items.channel_id),
			stream_id = COALESCE(NULLIF(EXCLUDED.stream_id,''), archive_job_items.stream_id),
			artifact_type = COALESCE(NULLIF(EXCLUDED.artifact_type,''), archive_job_items.artifact_type),
			updated_at = now()`,
		id, jobID, itemKey, login, channelID, streamID, artifactType, ItemStatusQueued,
	)
	if err != nil {
		return "", err
	}
	var itemID string
	err = t.db.QueryRow(ctx, `
		SELECT id FROM archive_job_items WHERE job_id=$1 AND item_key=$2`, jobID, itemKey).Scan(&itemID)
	return itemID, err
}

func (t *Tracker) StartItem(ctx context.Context, itemID string) error {
	_, err := t.db.Exec(ctx, `
		UPDATE archive_job_items
		SET status=$2, attempts=attempts+1, started_at=now(), updated_at=now()
		WHERE id=$1`, itemID, ItemStatusRunning)
	return err
}

func (t *Tracker) SucceedItem(ctx context.Context, jobID, itemID, outputURI, sha256 string) error {
	_, err := t.db.Exec(ctx, `
		UPDATE archive_job_items
		SET status=$2, output_uri=NULLIF($3,''), output_sha256=NULLIF($4,''),
			finished_at=now(), updated_at=now(), error=NULL
		WHERE id=$1`, itemID, ItemStatusSucceeded, outputURI, sha256)
	if err != nil {
		return err
	}
	return t.bumpCounters(ctx, jobID, ItemStatusSucceeded)
}

func (t *Tracker) FailItem(ctx context.Context, jobID, itemID, errMsg string) error {
	_, err := t.db.Exec(ctx, `
		UPDATE archive_job_items
		SET status=$2, error=NULLIF($3,''), finished_at=now(), updated_at=now()
		WHERE id=$1`, itemID, ItemStatusFailed, errMsg)
	if err != nil {
		return err
	}
	return t.bumpCounters(ctx, jobID, ItemStatusFailed)
}

func (t *Tracker) SkipItem(ctx context.Context, jobID, itemID string) error {
	_, err := t.db.Exec(ctx, `
		UPDATE archive_job_items
		SET status=$2, finished_at=now(), updated_at=now()
		WHERE id=$1`, itemID, ItemStatusSkipped)
	if err != nil {
		return err
	}
	return t.bumpCounters(ctx, jobID, ItemStatusSkipped)
}

func (t *Tracker) LinkBackfillJob(ctx context.Context, itemID string, backfillJobID int64) error {
	_, err := t.db.Exec(ctx, `
		UPDATE archive_job_items SET backfill_job_id=$2, updated_at=now() WHERE id=$1`,
		itemID, backfillJobID)
	return err
}

func (t *Tracker) LinkBackfillJobOnEnqueue(ctx context.Context, archiveJobID string, backfillJobID int64) error {
	_, err := t.db.Exec(ctx, `
		UPDATE backfill_jobs SET archive_job_id=$2, updated_at=now() WHERE id=$1`,
		backfillJobID, archiveJobID)
	return err
}

func (t *Tracker) RetryFailedItems(ctx context.Context, jobID string) error {
	_, err := t.db.Exec(ctx, `
		UPDATE archive_job_items
		SET status=$2, updated_at=now(), finished_at=NULL, error=NULL
		WHERE job_id=$1 AND status=$3`, jobID, ItemStatusQueued, ItemStatusFailed)
	return err
}

func (t *Tracker) ResumeJob(ctx context.Context, jobID string) error {
	_, err := t.db.Exec(ctx, `
		UPDATE archive_jobs
		SET status=$2, heartbeat_at=now(), updated_at=now(), finished_at=NULL, error=NULL
		WHERE id=$1`, jobID, JobStatusRunning)
	if err != nil {
		return err
	}
	_, err = t.db.Exec(ctx, `
		UPDATE archive_job_items
		SET status=$2, updated_at=now()
		WHERE job_id=$1 AND status IN ($3,$4,$5)`,
		jobID, ItemStatusQueued, ItemStatusFailed, JobStatusStale, ItemStatusRunning)
	return err
}

func (t *Tracker) CancelJob(ctx context.Context, jobID string) error {
	_, err := t.db.Exec(ctx, `
		UPDATE archive_jobs SET status=$2, finished_at=now(), updated_at=now() WHERE id=$1`,
		jobID, JobStatusCancelled)
	return err
}

func (t *Tracker) UpdateItemFromBackfill(ctx context.Context, backfillJobID int64, status, outputURI, errMsg string) error {
	itemStatus := ItemStatusSucceeded
	switch status {
	case "failed":
		itemStatus = ItemStatusFailed
	case "skipped":
		itemStatus = ItemStatusSkipped
	case "done":
		itemStatus = ItemStatusSucceeded
	default:
		if errMsg != "" {
			itemStatus = ItemStatusFailed
		}
	}
	_, err := t.db.Exec(ctx, `
		UPDATE archive_job_items
		SET status=$2, output_uri=NULLIF($3,''), error=NULLIF($4,''),
			finished_at=now(), updated_at=now()
		WHERE backfill_job_id=$1`, backfillJobID, itemStatus, outputURI, errMsg)
	if err != nil {
		return err
	}
	var jobID string
	err = t.db.QueryRow(ctx, `
		SELECT job_id FROM archive_job_items WHERE backfill_job_id=$1 LIMIT 1`, backfillJobID).Scan(&jobID)
	if err != nil || jobID == "" {
		return nil
	}
	return t.bumpCounters(ctx, jobID, itemStatus)
}

func (t *Tracker) bumpCounters(ctx context.Context, jobID, itemStatus string) error {
	col := "completed_items"
	switch itemStatus {
	case ItemStatusFailed:
		col = "failed_items"
	case ItemStatusSkipped:
		col = "skipped_items"
	}
	_, err := t.db.Exec(ctx, fmt.Sprintf(`
		UPDATE archive_jobs SET %s = %s + 1, updated_at=now(), heartbeat_at=now() WHERE id=$1`, col, col), jobID)
	return err
}

func (t *Tracker) MarkStaleJobs(ctx context.Context) (int64, error) {
	if t == nil || t.db == nil {
		return 0, nil
	}
	cutoff := time.Now().UTC().Add(-t.staleAfter)
	tag, err := t.db.Exec(ctx, `
		UPDATE archive_jobs
		SET status=$1, updated_at=now()
		WHERE status=$2 AND heartbeat_at IS NOT NULL AND heartbeat_at < $3`,
		JobStatusStale, JobStatusRunning, cutoff)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

func (t *Tracker) LogOperatorEvent(ctx context.Context, jobID, eventType, message string) error {
	t.logEvent(ctx, jobID, "", "info", eventType, message, nil)
	return nil
}

func (t *Tracker) ListJobs(ctx context.Context, status string, limit int) ([]Job, error) {
	if limit <= 0 {
		limit = 50
	}
	q := `
		SELECT id, job_type, COALESCE(tier,''), status, COALESCE(trigger_source,''),
			total_items, completed_items, failed_items, skipped_items, retried_items,
			started_at, updated_at, finished_at, heartbeat_at, COALESCE(error,''), metadata
		FROM archive_jobs`
	args := []any{}
	if status != "" {
		q += " WHERE status=$1"
		args = append(args, status)
	}
	q += " ORDER BY updated_at DESC LIMIT " + fmt.Sprintf("%d", limit)
	rows, err := t.db.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Job
	for rows.Next() {
		var j Job
		var meta []byte
		if err := rows.Scan(
			&j.ID, &j.JobType, &j.Tier, &j.Status, &j.TriggerSource,
			&j.TotalItems, &j.CompletedItems, &j.FailedItems, &j.SkippedItems, &j.RetriedItems,
			&j.StartedAt, &j.UpdatedAt, &j.FinishedAt, &j.HeartbeatAt, &j.Error, &meta,
		); err != nil {
			return nil, err
		}
		json.Unmarshal(meta, &j.Metadata)
		out = append(out, j)
	}
	return out, rows.Err()
}

func (t *Tracker) GetJob(ctx context.Context, jobID string) (*Job, []Item, error) {
	var j Job
	var meta []byte
	err := t.db.QueryRow(ctx, `
		SELECT id, job_type, COALESCE(tier,''), status, COALESCE(trigger_source,''),
			total_items, completed_items, failed_items, skipped_items, retried_items,
			started_at, updated_at, finished_at, heartbeat_at, COALESCE(error,''), metadata
		FROM archive_jobs WHERE id=$1`, jobID,
	).Scan(
		&j.ID, &j.JobType, &j.Tier, &j.Status, &j.TriggerSource,
		&j.TotalItems, &j.CompletedItems, &j.FailedItems, &j.SkippedItems, &j.RetriedItems,
		&j.StartedAt, &j.UpdatedAt, &j.FinishedAt, &j.HeartbeatAt, &j.Error, &meta,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil, fmt.Errorf("job not found")
		}
		return nil, nil, err
	}
	json.Unmarshal(meta, &j.Metadata)
	itemRows, err := t.db.Query(ctx, `
		SELECT id, job_id, item_key, COALESCE(channel_login,''), COALESCE(channel_id,''),
			COALESCE(stream_id,''), status, attempts, COALESCE(error,''),
			COALESCE(output_uri,''), COALESCE(output_sha256,''), backfill_job_id, metadata
		FROM archive_job_items WHERE job_id=$1 ORDER BY item_key`, jobID)
	if err != nil {
		return &j, nil, err
	}
	defer itemRows.Close()
	var items []Item
	for itemRows.Next() {
		var it Item
		var itemMeta []byte
		var backfillID *int64
		if err := itemRows.Scan(
			&it.ID, &it.JobID, &it.ItemKey, &it.ChannelLogin, &it.ChannelID,
			&it.StreamID, &it.Status, &it.Attempts, &it.Error,
			&it.OutputURI, &it.OutputSHA256, &backfillID, &itemMeta,
		); err != nil {
			return &j, items, err
		}
		it.BackfillJobID = backfillID
		json.Unmarshal(itemMeta, &it.Metadata)
		items = append(items, it)
	}
	return &j, items, itemRows.Err()
}

func (t *Tracker) logEvent(ctx context.Context, jobID, itemID, level, eventType, message string, meta map[string]any) {
	if !t.eventLogEnabled || t.db == nil {
		return
	}
	metaJSON, _ := json.Marshal(metadataOrEmpty(meta))
	var item any
	if itemID != "" {
		item = itemID
	}
	_, _ = t.db.Exec(ctx, `
		INSERT INTO archive_job_events (job_id, item_id, level, event_type, message, metadata)
		VALUES ($1,$2,$3,$4,$5,$6)`, jobID, item, level, eventType, message, metaJSON)
}

func metadataOrEmpty(m map[string]any) map[string]any {
	if m == nil {
		return map[string]any{}
	}
	return m
}
