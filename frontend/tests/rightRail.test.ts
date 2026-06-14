import assert from 'node:assert/strict'
import test from 'node:test'
import {
  RIGHT_RAIL_TABS,
  RIGHT_RAIL_DEFAULT_TAB,
  type RightRailTabId,
} from '../src/components/analytics/rightRailTabs.ts'

// Feature: moment-timeline — unit tests for Right Rail tab order and default
// selection (task 5.8).
//
// Testing scope / limitation:
//   The Node test runner loads tests via `node --experimental-strip-types`,
//   which cannot parse `RightRail.tsx` (JSX is not stripped and `.tsx` is not a
//   recognized module type → ERR_UNKNOWN_FILE_EXTENSION). There is no React
//   Testing Library / jsdom in this repo, so the rail cannot be mounted in a
//   runtime DOM here.
//
//   Therefore these tests assert the PURE, JSX-free tab contract exported from
//   the sibling `rightRailTabs.ts` module:
//     - Requirement 3.1: tab order Moments | Emotes | Clips | Sync, with labels.
//     - Requirement 3.2: default/active tab is 'moments'.
//
//   The render-dependent behaviors are NOT exercised at runtime here:
//     - Requirement 3.3 (selection retention until stream change / reload) and
//     - Requirement 3.4 (Moments empty state when no rollup data)
//   are implemented in `RightRail.tsx` and are covered at the type level by
//   `npm run build` (tsc), which compiles the component against this contract.
//   The reset-to-Moments-on-stream-change logic (3.2/3.3) is driven by
//   RIGHT_RAIL_DEFAULT_TAB, which is asserted below.

test('3.1 tab order is exactly Moments | Emotes | Clips | Sync', () => {
  const ids = RIGHT_RAIL_TABS.map(t => t.id)
  assert.deepEqual(ids, ['moments', 'emotes', 'clips', 'sync'])
})

test('3.1 tab labels match their ids', () => {
  const labels = RIGHT_RAIL_TABS.map(t => t.label)
  assert.deepEqual(labels, ['Moments', 'Emotes', 'Clips', 'Sync'])
})

test('3.1 Moments is first so it surfaces strongest moments on open', () => {
  assert.equal(RIGHT_RAIL_TABS[0].id, 'moments')
  assert.equal(RIGHT_RAIL_TABS[0].label, 'Moments')
})

test('3.1 Sync is last (future tabs may be appended after it)', () => {
  assert.equal(RIGHT_RAIL_TABS[RIGHT_RAIL_TABS.length - 1].id, 'sync')
})

test('3.1 tab ids are unique', () => {
  const ids = RIGHT_RAIL_TABS.map(t => t.id)
  assert.equal(new Set(ids).size, ids.length)
})

test('3.2 default tab is Moments', () => {
  assert.equal(RIGHT_RAIL_DEFAULT_TAB, 'moments')
})

test('3.2 default tab is a member of the tab set', () => {
  const ids = RIGHT_RAIL_TABS.map(t => t.id)
  assert.ok(ids.includes(RIGHT_RAIL_DEFAULT_TAB))
})

test('3.1/3.2 every tab id is a valid RightRailTabId', () => {
  const valid: RightRailTabId[] = ['moments', 'emotes', 'clips', 'sync']
  for (const tab of RIGHT_RAIL_TABS) {
    assert.ok(valid.includes(tab.id), `unexpected tab id: ${tab.id}`)
  }
})
