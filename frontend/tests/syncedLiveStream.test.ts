import assert from 'node:assert/strict'
import { it } from 'node:test'
import type { AnalyticsStream } from '../src/api.ts'
import {
  analyticsStreamPathSlug,
  pickSyncedLiveStreamTarget,
  streamHasSyncedMinutes,
} from '../src/utils/syncedLiveStream.ts'

function stream(partial: Partial<AnalyticsStream> & Pick<AnalyticsStream, 'streamId'>): AnalyticsStream {
  return {
    streamId: partial.streamId,
    broadcasterId: '',
    login: 'xqc',
    tags: [],
    startedAt: partial.startedAt ?? '',
    lastSeenAt: '',
    currentViewers: 0,
    avgViewers: partial.avgViewers ?? 0,
    peakViewers: partial.peakViewers ?? 0,
    viewerSamples: partial.viewerSamples ?? 0,
    chatMessages: partial.chatMessages ?? 0,
    totalEmoteUses: 0,
    seventvEmoteUses: 0,
    endedAt: partial.endedAt,
    title: partial.title,
  }
}

it('streamHasSyncedMinutes detects viewer or chat minute data', () => {
  assert.equal(streamHasSyncedMinutes(stream({ streamId: '1' })), false)
  assert.equal(streamHasSyncedMinutes(stream({ streamId: '1', viewerSamples: 3 })), true)
  assert.equal(streamHasSyncedMinutes(stream({ streamId: '1', chatMessages: 1 })), true)
})

it('pickSyncedLiveStreamTarget prefers the live endpoint stream id when synced', () => {
  const rows = [
    stream({ streamId: '100', viewerSamples: 10, startedAt: '2026-06-13T14:07:00.000Z' }),
    stream({ streamId: '200', startedAt: '2026-06-13T20:00:00.000Z' }),
  ]
  const picked = pickSyncedLiveStreamTarget(rows, { liveStreamId: '100', channelLive: true })
  assert.equal(picked?.streamId, '100')
})

it('pickSyncedLiveStreamTarget prefers open synced session over stale collector row', () => {
  const rows = [
    stream({
      streamId: '200',
      startedAt: '2026-06-13T20:00:00.000Z',
    }),
    stream({
      streamId: '100',
      viewerSamples: 420,
      chatMessages: 9000,
      startedAt: '2026-06-13T14:07:00.000Z',
    }),
  ]
  const picked = pickSyncedLiveStreamTarget(rows, { liveStreamId: '200', channelLive: true })
  assert.equal(picked?.streamId, '100')
})

it('pickSyncedLiveStreamTarget redirects live channels to recent synced rows without endedAt', () => {
  const rows = [
    stream({
      streamId: '200',
      startedAt: '2026-06-13T20:00:00.000Z',
    }),
    stream({
      streamId: '100',
      viewerSamples: 420,
      chatMessages: 9000,
      startedAt: '2026-06-13T14:07:00.000Z',
      endedAt: '2026-06-13T21:00:00.000Z',
    }),
  ]
  const picked = pickSyncedLiveStreamTarget(rows, { liveStreamId: '200', channelLive: true })
  assert.equal(picked?.streamId, '100')
})

it('pickSyncedLiveStreamTarget does not redirect live channels to stale synced history', () => {
  const rows = [
    stream({
      streamId: '200',
      startedAt: '2026-06-13T20:00:00.000Z',
    }),
    stream({
      streamId: '99',
      viewerSamples: 50,
      startedAt: '2026-06-10T10:00:00.000Z',
      endedAt: '2026-06-10T18:00:00.000Z',
    }),
  ]
  assert.equal(
    pickSyncedLiveStreamTarget(rows, { liveStreamId: '200', channelLive: true }),
    undefined,
  )
})

it('pickSyncedLiveStreamTarget still picks ended-only sync when channel is offline', () => {
  const rows = [
    stream({
      streamId: '99',
      viewerSamples: 50,
      startedAt: '2026-06-12T10:00:00.000Z',
      endedAt: '2026-06-12T18:00:00.000Z',
    }),
  ]
  assert.equal(
    pickSyncedLiveStreamTarget(rows, { channelLive: false })?.streamId,
    '99',
  )
})

it('analyticsStreamPathSlug uses date slug when unique for the day', () => {
  const rows = [
    stream({ streamId: '100', startedAt: '2026-06-13T14:07:00.000Z' }),
    stream({ streamId: '99', startedAt: '2026-06-12T10:00:00.000Z' }),
  ]
  assert.equal(analyticsStreamPathSlug(rows[0], rows), '2026-06-13')
  assert.equal(analyticsStreamPathSlug(
    stream({ streamId: '101', startedAt: '2026-06-13T18:00:00.000Z' }),
    [...rows, stream({ streamId: '101', startedAt: '2026-06-13T18:00:00.000Z' })],
  ), '101')
})
