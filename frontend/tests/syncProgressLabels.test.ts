import assert from 'node:assert/strict'
import { describe, it } from 'node:test'

import {
  chatFetchCleanupLabel,
  chatFetchDetailLabel,
  viewerStatusShowsExistingChart,
  viewerStepBadge,
  viewerTrackerStepProgress,
} from '../src/utils/syncProgressLabels.ts'

describe('syncProgressLabels', () => {
  it('chatFetchCleanupLabel reports parallel cleanup remaining segments', () => {
    const label = chatFetchCleanupLabel(
      { cleanupPhase: 'parallel_cleanup', segmentsIncomplete: 3 },
      139,
      142,
      { timelinePct: 99, segmentCleanup: true },
    )
    assert.equal(label, 'Cleanup: 3 segments remaining')
  })

  it('chatFetchDetailLabel prefers cleanup over timeline percent', () => {
    const label = chatFetchDetailLabel(
      {
        streamId: '1',
        phase: 'fetching_comments',
        startedAt: new Date().toISOString(),
        updatedAt: new Date().toISOString(),
        chat: { cleanupPhase: 'serial_retry', segmentsDone: 140, segmentsTotal: 142 },
      },
      false,
      true,
      false,
      99,
      140,
      142,
    )
    assert.match(label, /Serial retry/)
  })

  it('chatFetchDetailLabel switches to finalizing label once timeline is complete', () => {
    const label = chatFetchDetailLabel(
      {
        streamId: '1',
        phase: 'fetching_comments',
        startedAt: new Date().toISOString(),
        updatedAt: new Date().toISOString(),
        chat: {
          cleanupPhase: 'parallel_cleanup',
          throttled: true,
          segmentsIncomplete: 3,
        },
      },
      false,
      true,
      true,
      100,
      486,
      489,
    )
    assert.equal(label, 'Finalizing chat index')
  })

  it('chatFetchCleanupLabel shows remaining segments during finalization tail', () => {
    const label = chatFetchCleanupLabel(
      { cleanupPhase: 'parallel_cleanup', segmentsIncomplete: 3 },
      486,
      489,
      { timelinePct: 100, segmentCleanup: true },
    )
    assert.equal(label, '3 segments remaining')
  })

  it('chatFetchDetailLabel uses writing label once chat fetch reaches rollup phase', () => {
    const label = chatFetchDetailLabel(
      {
        streamId: '1',
        phase: 'writing_rollups',
        startedAt: new Date().toISOString(),
        updatedAt: new Date().toISOString(),
        chat: {
          indexPhase: 'writing',
          cleanupPhase: 'parallel_cleanup',
        },
      },
      true,
      true,
      false,
      100,
      489,
      489,
    )
    assert.equal(label, 'Writing rollups & emotes')
  })

  it('chatFetchDetailLabel respects explicit finalizing phase', () => {
    const label = chatFetchDetailLabel(
      {
        streamId: '1',
        phase: 'fetching_comments',
        startedAt: new Date().toISOString(),
        updatedAt: new Date().toISOString(),
        chat: {
          indexPhase: 'finalizing',
          summaryRefreshDeferred: true,
        },
      },
      true,
      true,
      false,
      100,
      489,
      489,
    )
    assert.equal(label, 'Finalizing chat index')
  })

  it('viewerStatusShowsExistingChart treats pending_backfill with live rollups as chart-ready', () => {
    assert.equal(viewerStatusShowsExistingChart('pending_backfill', true), true)
    assert.equal(viewerStatusShowsExistingChart('pending_backfill', false), false)
  })

  it('viewerTrackerStepProgress ramps pct during browser scrape', () => {
    const startedAt = new Date(Date.now() - 20_000).toISOString()
    const progress = viewerTrackerStepProgress({
      streamId: '1',
      phase: 'scraping_tracker',
      startedAt,
      updatedAt: startedAt,
      tracker: {
        active: true,
        phase: 'browser',
        expectedMs: 45_000,
        elapsedMs: 20_000,
        message: 'Browser scrape (Camoufox) · 20s / ~45s',
      },
    })
    assert.ok(progress.pct > 35)
    assert.ok(progress.pct < 90)
    assert.match(progress.detail, /Browser scrape/)
  })

  it('viewerStepBadge shows Unavailable instead of Skipped on failure', () => {
    assert.equal(viewerStepBadge('failed', 0, 'failed'), 'Unavailable')
  })
})
