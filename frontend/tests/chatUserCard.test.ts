import assert from 'node:assert/strict'
import { it } from 'node:test'

import type { Message } from '../src/chatStore.ts'
import {
  messageMatchesUser,
  messagePlainText,
  normalizeChatUserLogin,
  selectRecentUserMessages,
} from '../src/utils/chatUserCard.ts'

function message(partial: Partial<Message> & Pick<Message, 'id' | 'user'>): Message {
  return {
    color: '#fff',
    badges: [],
    ts: Date.now(),
    fragments: [{ t: 'text', c: partial.user }],
    ...partial,
  }
}

it('normalizeChatUserLogin prefers explicit login', () => {
  assert.equal(normalizeChatUserLogin('Display Name', 'RealLogin'), 'reallogin')
})

it('normalizeChatUserLogin falls back to display name slug', () => {
  assert.equal(normalizeChatUserLogin('Cool Streamer'), 'coolstreamer')
})

it('messageMatchesUser matches login or display name', () => {
  const byLogin = message({ id: '1', user: 'CoolStreamer', login: 'coolstreamer' })
  const byDisplay = message({ id: '2', user: 'Cool Streamer' })
  assert.equal(messageMatchesUser(byLogin, 'Other', 'coolstreamer'), true)
  assert.equal(messageMatchesUser(byDisplay, 'Cool Streamer'), true)
  assert.equal(messageMatchesUser(byLogin, 'Someone Else', 'otherlogin'), false)
})

it('messageMatchesUser ignores deleted and mod events', () => {
  const deleted = message({ id: '3', user: 'mod', login: 'mod', deleted: true })
  const modEvent = message({ id: '4', user: 'system', kind: 'mod_event' })
  assert.equal(messageMatchesUser(deleted, 'mod', 'mod'), false)
  assert.equal(messageMatchesUser(modEvent, 'system'), false)
})

it('messagePlainText joins fragment text', () => {
  const row = message({
    id: '5',
    user: 'viewer',
    fragments: [{ t: 'text', c: 'hello ' }, { t: 'mention', c: '@mod' }],
  })
  assert.equal(messagePlainText(row), 'hello @mod')
})

it('selectRecentUserMessages returns newest matching rows up to limit', () => {
  const messages = [
    message({ id: 'a', user: 'Alpha', login: 'alpha', ts: 1, fragments: [{ t: 'text', c: 'one' }] }),
    message({ id: 'b', user: 'Beta', login: 'beta', ts: 2, fragments: [{ t: 'text', c: 'two' }] }),
    message({ id: 'c', user: 'Alpha', login: 'alpha', ts: 3, fragments: [{ t: 'text', c: 'three' }] }),
    message({ id: 'd', user: 'Alpha', login: 'alpha', ts: 4, fragments: [{ t: 'text', c: 'four' }] }),
    message({ id: 'e', user: 'Beta', login: 'beta', ts: 5, fragments: [{ t: 'text', c: 'five' }] }),
    message({ id: 'f', user: 'Alpha', login: 'alpha', ts: 6, fragments: [{ t: 'text', c: 'six' }] }),
  ]

  const recent = selectRecentUserMessages(messages, 'Alpha', 'alpha', 3)
  assert.deepEqual(recent.map(row => row.id), ['c', 'd', 'f'])
})

it('selectRecentUserMessages returns empty when no matches', () => {
  const messages = [message({ id: 'x', user: 'solo', login: 'solo' })]
  assert.deepEqual(selectRecentUserMessages(messages, 'missing', 'missing'), [])
})
