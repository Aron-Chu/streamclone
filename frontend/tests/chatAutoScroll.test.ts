import assert from 'node:assert/strict'
import { it } from 'node:test'

import {
  CHAT_AT_BOTTOM_THRESHOLD_PX,
  isChatAtBottom,
  isChatScrollUp,
  shouldLatchPauseOnScrollUp,
  shouldPauseAutoFollowOnScroll,
  shouldPauseAutoFollowOnWheel,
  shouldResumeAutoFollowAtBottom,
} from '../src/utils/chatAutoScroll.ts'

it('isChatAtBottom treats small distance-from-bottom as pinned', () => {
  assert.equal(isChatAtBottom(968, 1000, 32, CHAT_AT_BOTTOM_THRESHOLD_PX), true)
  assert.equal(isChatAtBottom(900, 1000, 32, CHAT_AT_BOTTOM_THRESHOLD_PX), false)
})

it('isChatScrollUp detects upward movement above noise threshold', () => {
  assert.equal(isChatScrollUp(100, 103), true)
  assert.equal(isChatScrollUp(102, 103), false)
})

it('shouldPauseAutoFollowOnWheel pauses on upward wheel intent', () => {
  assert.equal(shouldPauseAutoFollowOnWheel(-1), true)
  assert.equal(shouldPauseAutoFollowOnWheel(0), false)
  assert.equal(shouldPauseAutoFollowOnWheel(12), false)
})

it('shouldPauseAutoFollowOnScroll pauses only on scroll-up', () => {
  assert.equal(shouldPauseAutoFollowOnScroll(100, 120), true)
  assert.equal(shouldPauseAutoFollowOnScroll(120, 100), false)
  assert.equal(shouldPauseAutoFollowOnScroll(100, 100), false)
})

it('latched pause never auto-resumes at bottom', () => {
  assert.equal(shouldResumeAutoFollowAtBottom(true), false)
  assert.equal(shouldResumeAutoFollowAtBottom(false), false)
})

it('shouldLatchPauseOnScrollUp mirrors scroll-up intent', () => {
  assert.equal(shouldLatchPauseOnScrollUp(true), true)
  assert.equal(shouldLatchPauseOnScrollUp(false), false)
})
