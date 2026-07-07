import type { ChatModEventFrame, Message } from './chatStore.ts'
import { messageMatchesUser } from './utils/chatUserCard.ts'

function matchesTarget(msg: Message, targetLogin: string): boolean {
  if (msg.kind === 'mod_event' || msg.kind === 'notice') return false
  return messageMatchesUser(msg, targetLogin, targetLogin)
}

export function applyChatModEvents(messages: Message[], events: ChatModEventFrame[]): Message[] {
  if (!events.length) return messages

  let rows = [...messages]
  for (const ev of events) {
    switch (ev.kind) {
      case 'clear_chat':
        rows = []
        break
      case 'timeout':
      case 'ban': {
        const targetLogin = ev.targetLogin?.trim().toLowerCase()
        if (!targetLogin) break
        rows = rows.map(msg => {
          if (!matchesTarget(msg, targetLogin)) return msg
          return {
            ...msg,
            moderation: ev.kind === 'timeout' ? 'timeout' : 'ban',
            moderationDurationSec: ev.durationSec,
          }
        })
        break
      }
      case 'delete_message':
        if (ev.messageId) {
          rows = rows.map(msg =>
            msg.id === ev.messageId
              ? { ...msg, deleted: true, moderation: 'deleted' as const }
              : msg,
          )
        }
        break
      case 'notice':
        rows.push({
          id: `notice-${ev.ts}-${ev.noticeMsgId ?? ev.summaryText ?? Math.random().toString(36).slice(2)}`,
          user: 'system',
          color: '#a78bfa',
          badges: [],
          ts: ev.ts,
          fragments: [{ t: 'text', c: ev.displayText || ev.summaryText || ev.kind }],
          source: 'remote',
          kind: 'notice',
          modText: ev.displayText || ev.summaryText || ev.kind,
        })
        break
      default:
        break
    }
  }
  return rows
}
