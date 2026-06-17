import { Fragment, createElement, type ReactNode } from 'react'

export const URL_PATTERN =
  /\b(?:https?:\/\/|www\.)\S+|\b[a-z0-9](?:[a-z0-9-]*[a-z0-9])?(?:\.[a-z0-9-]+)+\.(?:com|net|org|gg|tv|io|co|me|xyz|live|stream|link|app|dev)(?:\/\S*)?/gi

const TRAILING_PUNCT = /[.,;:!?)]+$/g

export type LinkifyPart = { type: 'text' | 'url'; value: string; suffix?: string }

export function splitLinkifyParts(text: string): LinkifyPart[] {
  if (!text) return []

  const parts: LinkifyPart[] = []
  let lastIndex = 0

  for (const match of text.matchAll(URL_PATTERN)) {
    const start = match.index ?? 0
    if (start > lastIndex) {
      parts.push({ type: 'text', value: text.slice(lastIndex, start) })
    }

    const raw = match[0]
    const suffixMatch = raw.match(TRAILING_PUNCT)
    const suffix = suffixMatch?.[0] ?? ''
    const url = suffix ? raw.slice(0, raw.length - suffix.length) : raw
    parts.push({ type: 'url', value: url, suffix: suffix || undefined })
    lastIndex = start + raw.length
  }

  if (lastIndex < text.length) {
    parts.push({ type: 'text', value: text.slice(lastIndex) })
  }

  return parts
}

export function hrefForLink(raw: string): string {
  if (/^https?:\/\//i.test(raw)) return raw
  if (/^www\./i.test(raw)) return `https://${raw}`
  return `https://${raw}`
}

export function linkifyText(text: string, keyPrefix = 'txt'): ReactNode[] {
  const parts = splitLinkifyParts(text)
  if (!parts.length) return []

  const nodes: ReactNode[] = []
  parts.forEach((part, index) => {
    if (part.type === 'text') {
      nodes.push(createElement(Fragment, { key: `${keyPrefix}-t-${index}` }, part.value))
      return
    }
    nodes.push(
      createElement(
        'a',
        {
          key: `${keyPrefix}-l-${index}`,
          href: hrefForLink(part.value),
          target: '_blank',
          rel: 'noopener noreferrer',
          className: 'break-all text-violet-300 underline decoration-violet-400/60 underline-offset-2 hover:text-violet-200',
        },
        part.value,
      ),
    )
    if (part.suffix) {
      nodes.push(createElement(Fragment, { key: `${keyPrefix}-s-${index}` }, part.suffix))
    }
  })

  return nodes.length ? nodes : [text]
}
