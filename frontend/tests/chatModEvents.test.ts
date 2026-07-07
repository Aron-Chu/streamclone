import assert from 'node:assert/strict'
import { it } from 'node:test'

import { applyChatModEvents } from '../src/chatModEvents.ts'
import type { ChatModEventFrame, Message } from '../src/chatStore.ts'

function message(partial: Partial<Message> & Pick<Message, 'id' | 'user'>): Message {
  return {
    color: '#fff',
    badges: [],
    ts: Date.now(),
    fragments: [{ t: 'text', c: 'hello chat' }],
    ...partial,
  }
}

function modEvent(partial: Partial<ChatModEventFrame> & Pick<ChatModEventFrame, 'kind' | 'ts'>): ChatModEventFrame {
  return {
    ...partial,
  }
}

it('timeout marks matching user messages without appending mod_event rows', () => {
  const rows = [
    message({ id: '1', user: 'fn11', login: 'fn11' }),
    message({ id: '2', user: 'other', login: 'other' }),
  ]
  const next = applyChatModEvents(rows, [
    modEvent({ kind: 'timeout', ts: 1000, targetLogin: 'fn11', durationSec: 30 }),
  ])

  assert.equal(next.length, 2)
  assert.equal(next[0].moderation, 'timeout')
  assert.equal(next[0].moderationDurationSec, 30)
  assert.equal(next[1].moderation, undefined)
  assert.equal(next.some(row => row.kind === 'mod_event'), false)
})

it('delete_message marks one message without duplicate summary row', () => {
  const rows = [message({ id: 'msg-1', user: 'viewer', login: 'viewer' })]
  const next = applyChatModEvents(rows, [
    modEvent({ kind: 'delete_message', ts: 2000, messageId: 'msg-1' }),
  ])

  assert.equal(next.length, 1)
  assert.equal(next[0].deleted, true)
  assert.equal(next[0].moderation, 'deleted')
  assert.equal(next.some(row => row.kind === 'mod_event'), false)
})

it('clear_chat empties the visible buffer', () => {
  const rows = [
    message({ id: '1', user: 'a' }),
    message({ id: '2', user: 'b' }),
  ]
  const next = applyChatModEvents(rows, [modEvent({ kind: 'clear_chat', ts: 3000 })])
  assert.deepEqual(next, [])
})

it('notice events append compact system rows', () => {
  const rows = [message({ id: '1', user: 'viewer' })]
  const next = applyChatModEvents(rows, [
    modEvent({ kind: 'notice', ts: 4000, displayText: 'User123 subscribed at Tier 1.' }),
  ])

  assert.equal(next.length, 2)
  assert.equal(next[1].kind, 'notice')
  assert.equal(next[1].modText, 'User123 subscribed at Tier 1.')
})

it('ban marks all messages from the target login', () => {
  const rows = [
    message({ id: '1', user: 'Spammer', login: 'spammer' }),
    message({ id: '2', user: 'Spammer', login: 'spammer' }),
  ]
  const next = applyChatModEvents(rows, [
    modEvent({ kind: 'ban', ts: 5000, targetLogin: 'spammer' }),
  ])

  assert.equal(next.every(row => row.moderation === 'ban'), true)
  assert.equal(next.some(row => row.kind === 'mod_event'), false)
})
