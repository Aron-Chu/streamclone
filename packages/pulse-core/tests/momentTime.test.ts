import assert from 'node:assert/strict'
import test from 'node:test'
import { formatMomentTimeLabel } from '../src/momentTime.ts'

test('formatMomentTimeLabel falls back to offset when start missing', () => {
  const label = formatMomentTimeLabel({ offsetSeconds: 3661 })
  assert.equal(label.primary, '01:01:01')
  assert.equal(label.secondary, undefined)
})

test('formatMomentTimeLabel uses local wall clock when start known', () => {
  const label = formatMomentTimeLabel({
    startedAtIso: '2026-01-15T20:00:00.000Z',
    offsetSeconds: 5400,
  })
  assert.match(label.primary, /\d/)
  assert.equal(label.secondary, '01:30:00 into stream')
})
