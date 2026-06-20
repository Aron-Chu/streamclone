import assert from 'node:assert/strict'
import test from 'node:test'

import type { AnalyticsStreamDetail } from '../src/api.ts'
import {
  defaultAnalyticsVodSidebarTab,
  isEmbedAnalyticsVodReview,
  resolveVodBannerTotalSec,
  resolveVodDetailDurationSec,
  resolveVodTotalDurationSec,
} from '../src/utils/vodReviewLayout.ts'

test('isEmbedAnalyticsVodReview is true only for analytics Twitch embed review', () => {
  assert.equal(isEmbedAnalyticsVodReview(true, true), true)
  assert.equal(isEmbedAnalyticsVodReview(true, false), false)
  assert.equal(isEmbedAnalyticsVodReview(false, true), false)
})

test('defaultAnalyticsVodSidebarTab opens Pulse for analytics streams with sid', () => {
  assert.equal(defaultAnalyticsVodSidebarTab(true, 'abc123'), 'pulse')
  assert.equal(defaultAnalyticsVodSidebarTab(true, ''), 'chat')
  assert.equal(defaultAnalyticsVodSidebarTab(false, 'abc123'), 'chat')
})

test('resolveVodDetailDurationSec prefers vodDurationSec then rollup minutes', () => {
  assert.equal(
    resolveVodDetailDurationSec({ vodDurationSec: 5400, rollups: [] } as AnalyticsStreamDetail),
    5400,
  )
  assert.equal(
    resolveVodDetailDurationSec({ rollups: Array.from({ length: 90 }) } as AnalyticsStreamDetail),
    5400,
  )
  assert.equal(resolveVodDetailDurationSec(null), null)
})

test('resolveVodBannerTotalSec prefers rollups for embed analytics review', () => {
  assert.equal(
    resolveVodBannerTotalSec({
      preferRollupDuration: true,
      rollupDurationSec: 7200,
      embedDurationSec: null,
      vodDetailDurationSec: 7000,
      relaySeekableEndSec: 3600,
    }),
    7200,
  )
  assert.equal(
    resolveVodBannerTotalSec({
      preferRollupDuration: true,
      rollupDurationSec: null,
      embedDurationSec: null,
      vodDetailDurationSec: 5400,
      relaySeekableEndSec: null,
    }),
    5400,
  )
})

test('resolveVodBannerTotalSec uses relay seekable end for HLS VOD', () => {
  assert.equal(
    resolveVodBannerTotalSec({
      preferRollupDuration: false,
      rollupDurationSec: null,
      embedDurationSec: null,
      vodDetailDurationSec: null,
      relaySeekableEndSec: 4100,
    }),
    4100,
  )
})

test('resolveVodTotalDurationSec picks max when rollup is shorter than vod metadata', () => {
  assert.equal(
    resolveVodTotalDurationSec({
      rollupDurationSec: 5400,
      embedDurationSec: null,
      vodDetailDurationSec: 7200,
      relaySeekableEndSec: 3600,
    }),
    7200,
  )
  assert.equal(
    resolveVodTotalDurationSec({
      rollupDurationSec: 1800,
      embedDurationSec: 5400,
      vodDetailDurationSec: 7200,
      relaySeekableEndSec: null,
    }),
    7200,
  )
})

test('resolveVodTotalDurationSec returns null when no positive duration sources exist', () => {
  assert.equal(
    resolveVodTotalDurationSec({
      rollupDurationSec: null,
      embedDurationSec: null,
      vodDetailDurationSec: null,
      relaySeekableEndSec: null,
    }),
    null,
  )
})
