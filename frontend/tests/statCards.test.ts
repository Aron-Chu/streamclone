import assert from 'node:assert/strict'
import test from 'node:test'
import {
  classifyStatCards,
  isPlaceholder,
  type StatCardRollup,
} from '../src/utils/statCards.ts'

const sampled: StatCardRollup = { viewerSamples: 50, chatCount: 10, totalEmoteCount: 4 }
const emptyRollup: StatCardRollup = { viewerSamples: 0, chatCount: 0, totalEmoteCount: 0 }

test('6.1 stats only: tracker averages but no chat/viewer rollups', () => {
  const cards = classifyStatCards({
    state: 'historical',
    avgViewers: 1200,
    peakViewers: 3000,
    rollups: [],
  })
  assert.equal(cards.chat.placeholder, 'Stats only')
  assert.equal(cards.emoteUses.placeholder, 'Stats only')
  assert.equal(cards.chat.muted, true)
  // Average, Peak, Duration stay numeric (tracker values).
  assert.equal(cards.average.placeholder, null)
  assert.equal(cards.peak.placeholder, null)
  assert.equal(cards.duration.placeholder, null)
})

test('6.2 needs sync: not_collected with no averages and no rollups', () => {
  const cards = classifyStatCards({
    state: 'not_collected',
    avgViewers: 0,
    peakViewers: 0,
    rollups: [],
  })
  for (const key of ['current', 'average', 'peak', 'chat', 'emoteUses'] as const) {
    assert.equal(cards[key].placeholder, 'Needs sync')
    assert.equal(cards[key].muted, true)
  }
})

test('6.3 collecting: live with fewer than 2 sampled rollups', () => {
  const cards = classifyStatCards({
    state: 'live',
    avgViewers: 500,
    peakViewers: 800,
    rollups: [sampled, { missing: true }],
  })
  assert.equal(cards.chat.placeholder, 'Collecting')
  assert.equal(cards.emoteUses.placeholder, 'Collecting')
  // Live viewer cards stay numeric.
  assert.equal(cards.current.placeholder, null)
  assert.equal(cards.average.placeholder, null)
  assert.equal(cards.peak.placeholder, null)
})

test('6.4 collecting clears once 2+ sampled rollups exist', () => {
  const cards = classifyStatCards({
    state: 'live',
    avgViewers: 500,
    peakViewers: 800,
    rollups: [sampled, sampled],
  })
  assert.equal(cards.chat.placeholder, null)
  assert.equal(cards.emoteUses.placeholder, null)
})

test('numeric default when rollups carry chat/viewer data', () => {
  const cards = classifyStatCards({
    state: 'historical',
    avgViewers: 1000,
    peakViewers: 2000,
    rollups: [sampled, sampled],
  })
  for (const key of ['current', 'average', 'peak', 'chat', 'emoteUses', 'duration'] as const) {
    assert.equal(cards[key].placeholder, null)
    assert.equal(isPlaceholder(cards[key]), false)
  }
})

test('not_collected with tracker averages is stats only, not needs sync', () => {
  const cards = classifyStatCards({
    state: 'not_collected',
    avgViewers: 900,
    peakViewers: 1500,
    rollups: [emptyRollup],
  })
  assert.equal(cards.chat.placeholder, 'Stats only')
  assert.equal(cards.current.placeholder, null)
})
