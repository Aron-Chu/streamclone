import assert from 'node:assert/strict'
import test from 'node:test'
import { clipsEmptyState, type ClipsRollup } from '../src/utils/clipsEmptyState.ts'

const chatRollup: ClipsRollup = { chatCount: 12, totalEmoteCount: 0 }
const emoteRollup: ClipsRollup = { chatCount: 0, totalEmoteCount: 5 }
const emptyRollup: ClipsRollup = { chatCount: 0, totalEmoteCount: 0 }

test('35.1 sync-first: zero chat/emote rollups instructs to sync first', () => {
  const content = clipsEmptyState({ rollups: [], clipJobCount: 0 })
  assert.ok(content)
  assert.equal(content.variant, 'sync-first')
  assert.equal(content.showSyncAction, true)
  assert.match(content.body, /sync/i)
  // Must not direct the user to an empty graph.
  assert.doesNotMatch(content.body, /click the graph/i)
})

test('35.1 sync-first: rollups present but all empty/missing still syncs first', () => {
  const content = clipsEmptyState({
    rollups: [emptyRollup, { missing: true, chatCount: 99 }],
    clipJobCount: 0,
  })
  assert.ok(content)
  assert.equal(content.variant, 'sync-first')
})

test('35.2 use-moments: chat rollups exist but no clip jobs references moments/peaks', () => {
  const content = clipsEmptyState({ rollups: [chatRollup], clipJobCount: 0 })
  assert.ok(content)
  assert.equal(content.variant, 'use-moments')
  assert.equal(content.showSyncAction, false)
  assert.match(content.body, /moments|heatmap peak/i)
  assert.doesNotMatch(content.body, /click the graph/i)
})

test('35.2 use-moments: emote-only rollups also count as rollup data', () => {
  const content = clipsEmptyState({ rollups: [emoteRollup], clipJobCount: 0 })
  assert.ok(content)
  assert.equal(content.variant, 'use-moments')
})

test('no empty state when clip jobs already exist', () => {
  assert.equal(clipsEmptyState({ rollups: [], clipJobCount: 1 }), null)
  assert.equal(clipsEmptyState({ rollups: [chatRollup], clipJobCount: 3 }), null)
})
