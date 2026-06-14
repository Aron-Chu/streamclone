import assert from 'node:assert/strict'
import { it } from 'node:test'
import type { AnalyticsStream } from '../src/api.ts'
import {
  isActiveLiveCollectorStream,
  isSyncPrefetchPlaceholder,
} from '../src/utils/analyticsStreamRow.ts'

function row(partial: Partial<AnalyticsStream> & Pick<AnalyticsStream, 'streamId'>): AnalyticsStream {
  return {
    streamId: partial.streamId,
    broadcasterId: partial.broadcasterId ?? '',
    login: 'ohnepixel',
    tags: [],
    startedAt: partial.startedAt ?? '',
    lastSeenAt: '',
    currentViewers: 0,
    avgViewers: 0,
    peakViewers: 0,
    viewerSamples: partial.viewerSamples ?? 0,
    chatMessages: partial.chatMessages ?? 0,
    totalEmoteUses: 0,
    seventvEmoteUses: 0,
    endedAt: partial.endedAt,
    title: partial.title,
  }
}

it('isSyncPrefetchPlaceholder stays true when tracker title overlays Syncing row', () => {
  const placeholder = row({
    streamId: '316986179940',
    broadcasterId: 'pending',
    title: 'FALCONS NIKO PLAYING MAJOR',
    startedAt: '2026-06-13T12:01:19Z',
  })
  assert.equal(isSyncPrefetchPlaceholder(placeholder), true)
  assert.equal(isActiveLiveCollectorStream(placeholder, 'live'), false)
})

it('isSyncPrefetchPlaceholder is false for the live collector stream', () => {
  const live = row({
    streamId: '317014684259',
    broadcasterId: '43683025',
    title: 'Live CS2',
    viewerSamples: 1050,
    chatMessages: 100000,
    startedAt: '2026-06-14T15:00:14Z',
  })
  assert.equal(isSyncPrefetchPlaceholder(live), false)
  assert.equal(isActiveLiveCollectorStream(live, 'live'), true)
})
