package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"streamclone/internal/analytics"
	"streamclone/internal/config"
)

// GoldShadowReconcileResult is stdout JSON for dev shadow→GQL→reconcile proofs.
type GoldShadowReconcileResult struct {
	StreamID                string                         `json:"stream_id"`
	Login                   string                         `json:"login"`
	IVR                     analytics.GoldIVRAttemptResult `json:"ivr"`
	GQLSyncOK               bool                           `json:"gql_sync_ok"`
	GQLSyncError            string                         `json:"gql_sync_error,omitempty"`
	ReconcileArtifactPath   string                         `json:"reconcile_artifact_path,omitempty"`
	GQLCanonicalMinutes     int                            `json:"gql_canonical_minutes"`
	IVRRollupRowsAfter      int                            `json:"ivr_rollup_rows_after"`
	StreamMetadataUnchanged bool                           `json:"stream_metadata_unchanged"`
}

func runGoldShadowReconcileOnce(
	ctx context.Context,
	cfg config.Config,
	pool *pgxpool.Pool,
	logger *slog.Logger,
	streamID, login string,
	noArchive bool,
) (GoldShadowReconcileResult, error) {
	out := GoldShadowReconcileResult{StreamID: streamID, Login: login}
	if !cfg.GoldIVREnabled {
		return out, fmt.Errorf("GOLD_IVR_ENABLED must be true")
	}
	if !cfg.GoldIVRShadowMode {
		return out, fmt.Errorf("GOLD_IVR_SHADOW_MODE must be true for shadow-reconcile proof")
	}
	streamID = strings.TrimSpace(streamID)
	login = strings.ToLower(strings.TrimSpace(login))
	if streamID == "" || login == "" {
		return out, fmt.Errorf("--stream-id and --login are required")
	}

	metaBefore, _ := analytics.NewStore(pool).GetStreamChatSourceMetadata(ctx, streamID)

	rdb, err := newBackfillRedis(cfg)
	if err != nil {
		return out, err
	}
	defer rdb.Close()

	withArchive := false // shadow-reconcile proofs never require Azure archive export
	syncService, err := newBackfillSyncService(cfg, pool, rdb, logger, withArchive)
	if err != nil {
		return out, err
	}

	syncCtx := ctx
	if cfg.GoldSyncTimeoutMS > 0 {
		var cancel context.CancelFunc
		syncCtx, cancel = context.WithTimeout(ctx, time.Duration(cfg.GoldSyncTimeoutMS)*time.Millisecond)
		defer cancel()
	}

	ivr := syncService.TryGoldIVRLiteBeforeGQL(ctx, streamID, login)
	out.IVR = ivr
	if !ivr.ShadowOnly {
		return out, fmt.Errorf("ivr shadow did not complete: reason=%q", ivr.Reason)
	}

	_, syncErr := syncService.SyncHistoricalStream(syncCtx, streamID, login, false, true, "")
	out.GQLSyncOK = syncErr == nil
	if syncErr != nil {
		out.GQLSyncError = syncErr.Error()
		return out, fmt.Errorf("gql sync failed: %w", syncErr)
	}

	reconcilePath, recErr := syncService.ReconcileGoldIVRShadowAfterGQL(ctx, streamID, login, ivr)
	if recErr != nil {
		return out, fmt.Errorf("reconcile failed: %w", recErr)
	}
	out.ReconcileArtifactPath = reconcilePath

	store := analytics.NewStore(pool)
	rollups, _ := store.RollupsByStream(ctx, streamID)
	for _, r := range rollups {
		if analytics.IsGQLCanonicalRollup(r) && r.ChatCount > 0 {
			out.GQLCanonicalMinutes++
		}
		if analytics.IsIVRProvisionalRollup(r) && r.ChatCount > 0 {
			out.IVRRollupRowsAfter++
		}
	}
	metaAfter, _ := store.GetStreamChatSourceMetadata(ctx, streamID)
	out.StreamMetadataUnchanged = streamChatMetaEqual(metaBefore, metaAfter)
	return out, nil
}

func runGoldShadowReconcileFixture(
	ctx context.Context,
	cfg config.Config,
	pool *pgxpool.Pool,
	logger *slog.Logger,
	channel, vodID, broadcasterID, startedAt, endedAt string,
	noArchive bool,
) (GoldShadowReconcileResult, error) {
	channel = strings.ToLower(strings.TrimSpace(channel))
	vodID = strings.TrimSpace(vodID)
	if channel == "" || vodID == "" {
		return GoldShadowReconcileResult{}, fmt.Errorf("--channel and --vod are required")
	}
	if broadcasterID == "" {
		broadcasterID = "40934651"
	}
	if startedAt == "" {
		startedAt = "2026-06-24 22:29:34+00"
	}
	if endedAt == "" {
		endedAt = "2026-06-25 02:48:17+00"
	}
	streamID := fmt.Sprintf("bench-ivr-%s-%s", channel, vodID)
	_, err := pool.Exec(ctx, `
		INSERT INTO analytics_streams (
			stream_id, login, broadcaster_id, vod_id, vod_source,
			started_at, ended_at, title, category, current_viewers, peak_viewers, updated_at
		) VALUES ($1,$2,$3,$4,'bench_fixture',$5,$6,'IVR shadow fixture','Just Chatting',0,0,now())
		ON CONFLICT (stream_id) DO UPDATE SET
			login=EXCLUDED.login,
			broadcaster_id=EXCLUDED.broadcaster_id,
			vod_id=EXCLUDED.vod_id,
			started_at=EXCLUDED.started_at,
			ended_at=EXCLUDED.ended_at,
			updated_at=now()`,
		streamID, channel, broadcasterID, vodID, startedAt, endedAt,
	)
	if err != nil {
		return GoldShadowReconcileResult{}, err
	}
	return runGoldShadowReconcileOnce(ctx, cfg, pool, logger, streamID, channel, noArchive)
}

func shadowReconcileArgsFromCLI(args []string) (streamID, login string, noArchive bool) {
	streamID, login = goldArgsFromCLI(args)
	noArchive = cliFlag(args, "--no-archive")
	return streamID, login, noArchive
}

func shadowReconcileFixtureArgsFromCLI(args []string) (channel, vodID, broadcasterID, startedAt, endedAt string, noArchive bool) {
	noArchive = cliFlag(args, "--no-archive")
	for _, arg := range args {
		switch {
		case strings.HasPrefix(arg, "--channel="):
			channel = strings.TrimSpace(strings.TrimPrefix(arg, "--channel="))
		case strings.HasPrefix(arg, "--vod="):
			vodID = strings.TrimSpace(strings.TrimPrefix(arg, "--vod="))
		case strings.HasPrefix(arg, "--broadcaster-id="):
			broadcasterID = strings.TrimSpace(strings.TrimPrefix(arg, "--broadcaster-id="))
		case strings.HasPrefix(arg, "--started-at="):
			startedAt = strings.TrimSpace(strings.TrimPrefix(arg, "--started-at="))
		case strings.HasPrefix(arg, "--ended-at="):
			endedAt = strings.TrimSpace(strings.TrimPrefix(arg, "--ended-at="))
		}
	}
	return channel, vodID, broadcasterID, startedAt, endedAt, noArchive
}

func streamChatMetaEqual(a, b *analytics.StreamChatSourceMetadata) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return a.ChatState == b.ChatState &&
		a.ChatSource == b.ChatSource &&
		a.SourceConfidence == b.SourceConfidence &&
		a.ChatSourceDetail == b.ChatSourceDetail &&
		a.IVRCoveragePct == b.IVRCoveragePct &&
		a.GQLCoveragePct == b.GQLCoveragePct
}

func printGoldShadowReconcileResult(result GoldShadowReconcileResult) {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	_ = enc.Encode(result)
}
