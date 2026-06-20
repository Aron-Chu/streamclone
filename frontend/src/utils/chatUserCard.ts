import type { Message } from '../chatStore'

export function normalizeChatUserLogin(displayName: string, login?: string): string | undefined {
  const trimmed = login?.trim()
  if (trimmed) return trimmed.toLowerCase()
  const guess = displayName.trim().toLowerCase().replace(/\s+/g, '')
  return guess || undefined
}

export function messageMatchesUser(msg: Message, displayName: string, login?: string): boolean {
  if (msg.deleted || msg.kind === 'mod_event') return false
  const targetLogin = normalizeChatUserLogin(displayName, login)
  const msgLogin = msg.login?.trim().toLowerCase()
  if (targetLogin && msgLogin && msgLogin === targetLogin) return true
  const msgUser = msg.user.trim().toLowerCase().replace(/\s+/g, '')
  const targetDisplay = displayName.trim().toLowerCase().replace(/\s+/g, '')
  if (targetLogin && msgUser === targetLogin) return true
  return msgUser === targetDisplay
}

export function messagePlainText(msg: Message): string {
  return msg.fragments.map(fragment => fragment.c).join('').trim()
}

export function selectRecentUserMessages(
  messages: Message[],
  displayName: string,
  login?: string,
  limit = 5,
): Message[] {
  if (!messages.length || limit <= 0) return []
  const rows: Message[] = []
  for (let index = messages.length - 1; index >= 0 && rows.length < limit; index -= 1) {
    const msg = messages[index]
    if (messageMatchesUser(msg, displayName, login)) {
      rows.unshift(msg)
    }
  }
  return rows
}
