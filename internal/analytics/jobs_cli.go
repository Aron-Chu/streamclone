package analytics

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"streamclone/internal/archive/jobtracker"
)

// JobsCLI provides operator commands for archive_jobs progress.
type JobsCLI struct {
	tracker *jobtracker.Tracker
}

func NewJobsCLI(pool *pgxpool.Pool, heartbeat, stale time.Duration, eventLog bool) *JobsCLI {
	return &JobsCLI{tracker: jobtracker.NewTracker(pool, heartbeat, stale, eventLog)}
}

type jobListRow struct {
	ID             string    `json:"id"`
	JobType        string    `json:"jobType"`
	Tier           string    `json:"tier"`
	Status         string    `json:"status"`
	TriggerSource  string    `json:"triggerSource"`
	TotalItems     int       `json:"totalItems"`
	CompletedItems int       `json:"completedItems"`
	FailedItems    int       `json:"failedItems"`
	SkippedItems   int       `json:"skippedItems"`
	ProgressRatio  float64   `json:"progressRatio"`
	UpdatedAt      time.Time `json:"updatedAt"`
}

func (c *JobsCLI) List(ctx context.Context, status string, limit int) ([]jobListRow, error) {
	jobs, err := c.tracker.ListJobs(ctx, status, limit)
	if err != nil {
		return nil, err
	}
	out := make([]jobListRow, 0, len(jobs))
	for _, j := range jobs {
		out = append(out, jobListRow{
			ID:             j.ID,
			JobType:        j.JobType,
			Tier:           j.Tier,
			Status:         j.Status,
			TriggerSource:  j.TriggerSource,
			TotalItems:     j.TotalItems,
			CompletedItems: j.CompletedItems,
			FailedItems:    j.FailedItems,
			SkippedItems:   j.SkippedItems,
			ProgressRatio:  progressRatio(j.TotalItems, j.CompletedItems, j.FailedItems, j.SkippedItems),
			UpdatedAt:      j.UpdatedAt,
		})
	}
	return out, nil
}

type jobShowResponse struct {
	Job   jobListRow   `json:"job"`
	Items []jobItemRow `json:"items"`
}

type jobItemRow struct {
	ID            string `json:"id"`
	ItemKey       string `json:"itemKey"`
	ChannelLogin  string `json:"channelLogin,omitempty"`
	StreamID      string `json:"streamId,omitempty"`
	Status        string `json:"status"`
	Attempts      int    `json:"attempts"`
	Error         string `json:"error,omitempty"`
	OutputURI     string `json:"outputUri,omitempty"`
	BackfillJobID *int64 `json:"backfillJobId,omitempty"`
}

func (c *JobsCLI) Show(ctx context.Context, jobID string) (*jobShowResponse, error) {
	j, items, err := c.tracker.GetJob(ctx, jobID)
	if err != nil {
		return nil, err
	}
	resp := &jobShowResponse{
		Job: jobListRow{
			ID:             j.ID,
			JobType:        j.JobType,
			Tier:           j.Tier,
			Status:         j.Status,
			TriggerSource:  j.TriggerSource,
			TotalItems:     j.TotalItems,
			CompletedItems: j.CompletedItems,
			FailedItems:    j.FailedItems,
			SkippedItems:   j.SkippedItems,
			ProgressRatio:  progressRatio(j.TotalItems, j.CompletedItems, j.FailedItems, j.SkippedItems),
			UpdatedAt:      j.UpdatedAt,
		},
	}
	for _, it := range items {
		resp.Items = append(resp.Items, jobItemRow{
			ID:            it.ID,
			ItemKey:       it.ItemKey,
			ChannelLogin:  it.ChannelLogin,
			StreamID:      it.StreamID,
			Status:        it.Status,
			Attempts:      it.Attempts,
			Error:         it.Error,
			OutputURI:     it.OutputURI,
			BackfillJobID: it.BackfillJobID,
		})
	}
	return resp, nil
}

func (c *JobsCLI) RetryFailed(ctx context.Context, jobID string) error {
	return c.tracker.RetryFailedItems(ctx, jobID)
}

func (c *JobsCLI) Resume(ctx context.Context, jobID string) error {
	return c.tracker.ResumeJob(ctx, jobID)
}

func (c *JobsCLI) Cancel(ctx context.Context, jobID string) error {
	return c.tracker.CancelJob(ctx, jobID)
}

func progressRatio(total, completed, failed, skipped int) float64 {
	if total <= 0 {
		return 0
	}
	return float64(completed+failed+skipped) / float64(total)
}

func PrintJobsJSON(v any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func JobsStatusFromArgs(args []string) string {
	for _, arg := range args {
		if strings.HasPrefix(arg, "--status=") {
			return strings.TrimSpace(strings.TrimPrefix(arg, "--status="))
		}
	}
	return ""
}

func JobsJobIDFromArgs(args []string) string {
	for _, arg := range args {
		if strings.HasPrefix(arg, "--job-id=") {
			return strings.TrimSpace(strings.TrimPrefix(arg, "--job-id="))
		}
	}
	return ""
}

func JobsLimitFromArgs(args []string) int {
	limit := 50
	for _, arg := range args {
		if strings.HasPrefix(arg, "--limit=") {
			if _, err := fmt.Sscanf(strings.TrimPrefix(arg, "--limit="), "%d", &limit); err != nil {
				return 50
			}
		}
	}
	return limit
}
