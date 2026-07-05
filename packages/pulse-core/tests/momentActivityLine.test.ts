import assert from 'node:assert/strict'
import test from 'node:test'
import { formatMomentActivityLine } from '../src/momentActivityLine.ts'

test('formatMomentActivityLine defaults to chat and emotes', () => {
  assert.equal(
    formatMomentActivityLine({ reason: 'chat_spike', chatCount: 412, emoteCount: 88 }),
    '412 chat · 88 emotes',
  )
})

test('formatMomentActivityLine leads with viewers for viewer spike', () => {
  assert.equal(
    formatMomentActivityLine({
      reason: 'viewer_spike',
      chatCount: 0,
      emoteCount: 0,
      viewerCount: 18825,
    }),
    '18.8K viewers',
  )
})

test('formatMomentActivityLine shows viewer delta when set', () => {
  assert.equal(
    formatMomentActivityLine({
      reason: 'viewer_spike',
      chatCount: 0,
      emoteCount: 0,
      viewerCount: 19231,
      viewerDelta: 406,
    }),
    '+406 viewers',
  )
})

test('formatMomentActivityLine shows viewers when reaction counts are zero', () => {
  assert.equal(
    formatMomentActivityLine({
      reason: 'manual',
      chatCount: 0,
      emoteCount: 0,
      viewerCount: 42000,
    }),
    '42K viewers',
  )
})

test('formatMomentActivityLine combines viewers with partial reaction data', () => {
  assert.equal(
    formatMomentActivityLine({
      reason: 'viewer_spike',
      chatCount: 173,
      emoteCount: 139,
      viewerCount: 19231,
      viewerDelta: 406,
    }),
    '+406 viewers · 173 chat · 139 emotes',
  )
})

test('formatMomentActivityLine falls back when all counts are zero', () => {
  assert.equal(
    formatMomentActivityLine({ reason: 'chat_spike', chatCount: 0, emoteCount: 0 }),
    '0 chat · 0 emotes',
  )
})
