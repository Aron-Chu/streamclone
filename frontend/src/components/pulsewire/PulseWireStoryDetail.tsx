import { FormEvent, useEffect, useMemo, useRef, useState } from 'react'
import { useLocation } from 'react-router-dom'
import { SETUP_CONTROL_TOKEN } from '../../config'
import {
  addEvidenceStory,
  effectiveScores,
  markPulseWireStory,
  storyReceipts,
  storyTimeline,
  storyUpdatedAt,
  type PulseWireEvidencePreview,
  type PulseWireMatchExplanation,
  type PulseWireOperatorAction,
  type PulseWireOperatorActionName,
  type PulseWireReceipt,
  type PulseWireSourceHealth,
  type PulseWireStory,
} from '../../pulseWireApi'
import { formatRelativeTime } from '../../utils/pulseWireFormat'
import { buildPulseWireOriginHref } from '../../utils/pulseWireOriginLink'
import { hasSearchedMissingOrigin, toWireStoryView } from '../../utils/pulseWireStoryView'
import { withTwitchEmbedParent } from '../../utils/twitchEmbed'
import OriginSpikeChart from './OriginSpikeChart'
import ReceiptsRow from './ReceiptsRow'
import ScoreBars from './ScoreBars'
import SpreadTimeline from './SpreadTimeline'

type Props = {
  story: PulseWireStory
  sourceHealth?: PulseWireSourceHealth
  analystMode?: boolean
  onAnalystModeChange?: (enabled: boolean) => void
  onAdded?: () => void
  onCollapse?: () => void
}

function sourceLabel(value: string) {
  return value.replace(/_/g, ' ')
}

function evidenceProviderState(item: PulseWireEvidencePreview) {
  const platform = item.platform.toLowerCase()
  const canonical = item.canonicalUrl.toLowerCase()
  if (platform === 'twitch' || platform === 'twitch_clip' || canonical.includes('clips.twitch.tv')) return 'Twitch clip'
  if (platform === 'reddit' || canonical.includes('reddit.com') || canonical.includes('redd.it')) return 'Reddit thread'
  if (platform === 'youtube' || canonical.includes('youtube.com') || canonical.includes('youtu.be')) {
    return canonical.includes('/shorts/') ? 'YouTube Short' : 'YouTube video'
  }
  if (platform === 'x' || platform === 'twitter' || canonical.includes('x.com') || canonical.includes('twitter.com')) return 'X linked post'
  if (platform === 'tiktok' || canonical.includes('tiktok.com')) return 'TikTok linked video'
  if (platform === 'streamerbans') return 'StreamerBans authority'
  return 'Generic link'
}

function hasBanSignal(value?: string) {
  return /ban|moderation|streamerbans/i.test(value ?? '')
}

function ModerationContext({
  story,
  receipts,
  gallery,
  matches,
  updatedAt,
}: {
  story: PulseWireStory
  receipts: PulseWireReceipt[]
  gallery: PulseWireEvidencePreview[]
  matches: PulseWireMatchExplanation[]
  updatedAt?: string
}) {
  const isBanStory = story.story.category === 'bans'
  const authorityReceipts = receipts.filter(receipt => hasBanSignal(receipt.sourceType) || hasBanSignal(receipt.label))
  const moderationPreviews = gallery.filter(item => (
    hasBanSignal(item.platform) ||
    hasBanSignal(item.providerName) ||
    hasBanSignal(item.matchKind) ||
    hasBanSignal(item.title)
  ))
  const authorityMatches = matches.filter(match => hasBanSignal(match.sourceType) || hasBanSignal(match.matchedBy))

  if (!isBanStory && !authorityReceipts.length && !moderationPreviews.length && !authorityMatches.length) {
    return null
  }

  const authority = authorityReceipts[0]
  const preview = moderationPreviews[0]
  const match = authorityMatches[0]
  const title = authority?.label || preview?.title || story.story.title || 'Moderation event'
  const source = authority?.sourceType || preview?.providerName || preview?.platform || match?.sourceType
  const confidence = authority?.pct ?? (match ? Math.round(match.confidence * 100) : undefined)
  const eventTime = authority?.occurredAt || preview?.createdAtSrc || updatedAt
  const sourceUrl = authority?.url || preview?.canonicalUrl || match?.sourceUrl

  return (
    <section className="rounded-2xl border border-[#3A2426] bg-[#160F11] p-4">
      <div className="mb-3 flex flex-wrap items-start justify-between gap-3">
        <div>
          <p className="text-[11px] font-bold uppercase tracking-[0.06em] text-[#FF8F8A]">Moderation context</p>
          <h3 className="text-base font-bold text-[#F7F7F8]">{title}</h3>
          <p className="mt-1 max-w-2xl text-sm text-[#ADADB8]">
            Ban-category stories stay tied to authority receipts and visible source URLs before they are treated as confirmed.
          </p>
        </div>
        {story.entity?.displayName || story.entity?.login ? (
          <span className="rounded-full border border-[#4A2F32] bg-[#201416] px-3 py-1 text-xs font-semibold text-[#FFD8D6]">
            {story.entity.displayName || story.entity.login}
          </span>
        ) : null}
      </div>
      <div className="grid gap-3 text-sm md:grid-cols-3">
        <div className="rounded-xl border border-[#2A2A2E] bg-[#121217] p-3">
          <p className="text-[11px] font-bold uppercase tracking-[0.06em] text-[#7A7A85]">Authority source</p>
          <p className="mt-1 font-semibold text-[#EFEFF1]">{source ? sourceLabel(source) : 'No authority receipt yet'}</p>
        </div>
        <div className="rounded-xl border border-[#2A2A2E] bg-[#121217] p-3">
          <p className="text-[11px] font-bold uppercase tracking-[0.06em] text-[#7A7A85]">Confidence</p>
          <p className="mt-1 font-semibold text-[#EFEFF1]">{confidence != null ? `${confidence}%` : 'Pending evidence'}</p>
        </div>
        <div className="rounded-xl border border-[#2A2A2E] bg-[#121217] p-3">
          <p className="text-[11px] font-bold uppercase tracking-[0.06em] text-[#7A7A85]">Event time</p>
          <p className="mt-1 font-semibold text-[#EFEFF1]">{eventTime ? formatRelativeTime(eventTime) : 'Unknown'}</p>
        </div>
      </div>
      <div className="mt-3 rounded-xl border border-[#2A2A2E] bg-[#121217] p-3 text-sm text-[#ADADB8]">
        {sourceUrl ? (
          <a href={sourceUrl} target="_blank" rel="noreferrer" className="font-semibold text-[#A970FF] hover:underline">
            Open moderation source
          </a>
        ) : (
          'No moderation source URL attached yet.'
        )}
        {!authorityReceipts.length ? (
          <p className="mt-2 text-xs text-[#7A7A85]">No StreamerBans authority receipt is attached to this story yet.</p>
        ) : null}
      </div>
    </section>
  )
}

function MissingEvidenceBlock({
  story,
  sourceHealth,
  analystMode = false,
  onAnalystModeChange,
  focusOnMount,
}: {
  story: PulseWireStory
  sourceHealth?: PulseWireSourceHealth
  analystMode?: boolean
  onAnalystModeChange?: (enabled: boolean) => void
  focusOnMount?: boolean
}) {
  const sectionRef = useRef<HTMLElement | null>(null)
  const [showAnalystGaps, setShowAnalystGaps] = useState(analystMode)
  const readerView = toWireStoryView(story, sourceHealth, { analystMode: false })
  const analystView = toWireStoryView(story, sourceHealth, { analystMode: true })
  const readerItems = readerView.missingEvidence
  const analystItems = analystView.missingEvidence.flatMap(item => (
    item === 'No Pulse or Twitch origin matched'
      ? ['No Pulse origin matched', 'No original Twitch clip found']
      : [item]
  ))
  const items = showAnalystGaps ? analystItems : readerItems

  useEffect(() => {
    setShowAnalystGaps(analystMode)
  }, [analystMode])

  useEffect(() => {
    if (!focusOnMount || typeof window === 'undefined') return
    const scrollIntoReadingPosition = (target: HTMLElement) => {
      target.scrollIntoView({ block: 'start' })
      const stickyHeaderOffset = 96
      const top = target.getBoundingClientRect().top + window.scrollY - stickyHeaderOffset
      window.scrollTo({ top: Math.max(0, top), behavior: 'auto' })

      let parent = target.parentElement
      while (parent && parent !== document.body) {
        const style = window.getComputedStyle(parent)
        const canScroll = /(auto|scroll|overlay)/.test(style.overflowY) && parent.scrollHeight > parent.clientHeight
        if (canScroll) {
          const parentRect = parent.getBoundingClientRect()
          const targetRect = target.getBoundingClientRect()
          parent.scrollTop += targetRect.top - parentRect.top - 16
          break
        }
        parent = parent.parentElement
      }
    }
    const focusSection = () => {
      if (!sectionRef.current) return
      scrollIntoReadingPosition(sectionRef.current)
      sectionRef.current.focus({ preventScroll: true })
    }
    const frame = window.requestAnimationFrame(() => {
      focusSection()
      window.setTimeout(focusSection, 100)
      window.setTimeout(focusSection, 300)
    })
    return () => window.cancelAnimationFrame(frame)
  }, [focusOnMount, story.story.id])

  if (!readerItems.length && !analystItems.length) {
    return null
  }

  const content = (
    <>
      <div className="mb-3 flex flex-wrap items-start justify-between gap-3">
        <div>
          <p className="text-[11px] font-bold uppercase tracking-[0.06em] text-[#FFE0A3]">Missing evidence</p>
          <h3 className="text-base font-bold text-[#F7F7F8]">
            {items.length ? `${items.length} gaps before stronger confirmation` : 'No missing evidence flagged'}
          </h3>
          <p className="mt-1 max-w-2xl text-sm text-[#ADADB8]">
            {showAnalystGaps
              ? 'Full analyst checklist for corroboration across configured sources.'
              : 'Partial spread gaps only — open analyst view for the full checklist.'}
          </p>
        </div>
        <div className="flex flex-wrap gap-2">
          {analystItems.length > readerItems.length ? (
            <button
              type="button"
              onClick={() => {
                const next = !showAnalystGaps
                setShowAnalystGaps(next)
                onAnalystModeChange?.(next)
              }}
              className="rounded-lg border border-[#4A3A18] bg-[#21190D] px-3 py-1.5 text-xs font-semibold text-[#FFE0A3] hover:border-[#FFE0A3]/60"
            >
              {showAnalystGaps ? 'Reader gaps only' : 'Show all analyst gaps'}
            </button>
          ) : null}
          {showAnalystGaps && SETUP_CONTROL_TOKEN ? (
            <a href="#pulse-wire-add-evidence" className="rounded-lg border border-[#4A3A18] bg-[#21190D] px-3 py-1.5 text-xs font-semibold text-[#FFE0A3] hover:border-[#FFE0A3]/60">
              Add evidence
            </a>
          ) : null}
        </div>
      </div>
      {items.length ? (
        <ul className="grid gap-2 md:grid-cols-2">
          {items.map(item => (
            <li key={item} className="rounded-xl border border-[#4A3A18] bg-[#21190D] p-3 text-sm text-[#FFE0A3]">
              {item}
            </li>
          ))}
        </ul>
      ) : (
        <p className="rounded-xl border border-[#2A2A2E] bg-[#121217] p-3 text-sm text-[#7A7A85]">
          Current receipts cover the expected sources for this story.
        </p>
      )}
      {!SETUP_CONTROL_TOKEN && showAnalystGaps ? (
        <p className="mt-3 text-xs text-[#7A7A85]">Operator add URL is hidden until a setup-control token is available.</p>
      ) : null}
    </>
  )

  if (!showAnalystGaps && readerItems.length) {
    return (
      <details className="scroll-mt-4 rounded-2xl border border-[#3A2A12] bg-[#151208] p-4">
        <summary className="cursor-pointer text-sm font-semibold text-[#FFE0A3] marker:text-[#FFE0A3]">
          Spread gaps ({readerItems.length})
        </summary>
        <div className="mt-3">{content}</div>
      </details>
    )
  }

  return (
    <section ref={sectionRef} id="missing-evidence" tabIndex={-1} className="scroll-mt-4 rounded-2xl border border-[#3A2A12] bg-[#151208] p-4 focus:outline-none focus-visible:ring-2 focus-visible:ring-[#FFE0A3]/60">
      {content}
    </section>
  )
}

function OriginBlock({ story, sourceHealth }: { story: PulseWireStory; sourceHealth?: PulseWireSourceHealth }) {
  const view = toWireStoryView(story, sourceHealth)
  const origin = story.origin
  const originHref = buildPulseWireOriginHref(story)
  const searchedMissingOrigin = hasSearchedMissingOrigin(story)
  const originLabel = view.hasPulseOrigin
    ? 'Pulse origin matched'
    : view.canCreateClip
      ? 'Twitch origin found'
      : searchedMissingOrigin
        ? 'No Twitch origin found'
        : 'Origin pending'
  const originCopy = view.hasPulseOrigin
    ? 'This story is attached to a Pulse moment and can be reviewed from the original stream context.'
    : view.canCreateClip
      ? 'A Twitch origin is attached, so clip workflows can use the stored stream timestamp.'
      : searchedMissingOrigin
        ? 'Analytics searched available Twitch moments for this story and did not find a matching origin yet.'
        : 'No Pulse or Twitch origin is matched yet. Treat the story as spread evidence until origin proof lands.'
  const topEmotes = (origin?.topEmotes ?? []).slice(0, 8)
  const originConfidence = origin?.originConfidence != null && Number.isFinite(origin.originConfidence)
    ? `${Math.round(Math.max(0, Math.min(1, origin.originConfidence)) * 100)}%`
    : 'Pending'

  return (
    <section id="origin" aria-labelledby="story-origin-heading" className="rounded-2xl border border-[#2A2A2E] bg-[#121217] p-4">
      <div className="mb-3 flex flex-wrap items-start justify-between gap-3">
        <div>
          <p className="text-[11px] font-bold uppercase tracking-[0.06em] text-[#7A7A85]">Origin</p>
          <h3 id="story-origin-heading" className="text-base font-bold text-[#F7F7F8]">{originLabel}</h3>
          <p className="mt-1 max-w-2xl text-sm text-[#ADADB8]">{originCopy}</p>
        </div>
        {origin?.streamId ? (
          <span className="rounded-full border border-[#2A2A2E] bg-[#1B1B1F] px-3 py-1 text-xs font-semibold text-[#ADADB8]">
            stream {origin.streamId}
          </span>
        ) : null}
      </div>
      <div className="grid gap-3 text-sm md:grid-cols-3">
        <div className="rounded-xl border border-[#2A2A2E] bg-[#161619] p-3">
          <p className="text-[11px] font-bold uppercase tracking-[0.06em] text-[#7A7A85]">Pulse match</p>
          <p className="mt-1 font-semibold text-[#EFEFF1]">{view.hasPulseOrigin ? 'Matched' : searchedMissingOrigin ? 'Searched' : 'Pending'}</p>
        </div>
        <div className="rounded-xl border border-[#2A2A2E] bg-[#161619] p-3">
          <p className="text-[11px] font-bold uppercase tracking-[0.06em] text-[#7A7A85]">Twitch timestamp</p>
          <p className="mt-1 font-semibold text-[#EFEFF1]">
            {origin?.vodOffsetS != null ? `${origin.vodOffsetS}s` : 'Not found'}
          </p>
        </div>
        <div className="rounded-xl border border-[#2A2A2E] bg-[#161619] p-3">
          <p className="text-[11px] font-bold uppercase tracking-[0.06em] text-[#7A7A85]">Clip flow</p>
          <p className="mt-1 font-semibold text-[#EFEFF1]">{view.canCreateClip ? 'Available' : 'Waiting for origin'}</p>
        </div>
      </div>
      {origin ? (
        <div className="mt-3 grid gap-3 text-sm md:grid-cols-[1fr_160px]">
          <div className="rounded-xl border border-[#2A2A2E] bg-[#161619] p-3">
            <p className="mb-1 text-[11px] font-bold uppercase tracking-[0.06em] text-[#7A7A85]">Chat spike summary</p>
            <p className="text-[#D6D6DE]">{origin.chatSpikeSummary || 'No chat spike summary stored yet.'}</p>
          </div>
          <div className="rounded-xl border border-[#2A2A2E] bg-[#161619] p-3">
            <p className="text-[11px] font-bold uppercase tracking-[0.06em] text-[#7A7A85]">Origin confidence</p>
            <p className="mt-1 font-semibold text-[#EFEFF1]">{originConfidence}</p>
          </div>
        </div>
      ) : null}
      <OriginSpikeChart origin={origin} />
      {topEmotes.length ? (
        <div className="mt-3 rounded-xl border border-[#2A2A2E] bg-[#161619] p-3">
          <p className="mb-2 text-[11px] font-bold uppercase tracking-[0.06em] text-[#7A7A85]">Top emotes near origin</p>
          <div className="flex flex-wrap gap-2">
            {topEmotes.map((emote, index) => (
              <span key={`${emote.id ?? emote.name}-${index}`} className="rounded-lg border border-[#33333A] bg-[#1B1B1F] px-2.5 py-1 text-xs font-semibold text-[#D6D6DE]">
                {emote.name}{emote.count ? ` x${emote.count}` : ''}
              </span>
            ))}
          </div>
        </div>
      ) : null}
      {origin?.quotes?.length ? (
        <div className="mt-3 rounded-xl border border-[#2A2A2E] bg-[#161619] p-3 text-sm text-[#D6D6DE]">
          <p className="mb-1 text-[11px] font-bold uppercase tracking-[0.06em] text-[#7A7A85]">Origin quote</p>
          {origin.quotes[0]}
        </div>
      ) : null}
      {originHref ? (
        <a
          href={originHref}
          className="mt-3 inline-flex rounded-lg border border-[#33333A] bg-[#1B1B21] px-3 py-1.5 text-xs font-semibold text-[#EFEFF1] transition hover:border-[#A970FF]/40"
        >
          View origin moment
        </a>
      ) : null}
    </section>
  )
}

const STATUS_STYLES: Record<string, string> = {
  ready: 'border-[#2A2A2E] bg-[#161619] text-[#EFEFF1]',
  fallback: 'border-amber-400/30 bg-amber-500/10 text-amber-100',
  error: 'border-red-400/30 bg-red-500/10 text-red-100',
  pending: 'border-[#33333A] bg-[#121217] text-[#ADADB8]',
}

const ALLOWED_EMBED_HOSTS = new Set([
  'www.youtube.com',
  'youtube.com',
  'youtu.be',
  'player.twitch.tv',
  'clips.twitch.tv',
])

function safeEmbedSrc(value?: string) {
  if (!value) return ''
  try {
    const url = new URL(value)
    if (!['http:', 'https:'].includes(url.protocol)) return ''
    if (!ALLOWED_EMBED_HOSTS.has(url.hostname.toLowerCase())) return ''
    return withTwitchEmbedParent(url.toString())
  } catch {
    return ''
  }
}

function EvidenceCard({ item }: { item: PulseWireEvidencePreview }) {
  const statusClass = STATUS_STYLES[item.previewStatus] ?? STATUS_STYLES.fallback
  const embedSrc = safeEmbedSrc(item.embedUrl)
  const providerState = evidenceProviderState(item)

  return (
    <article className={`pulse-wire-card-enter overflow-hidden rounded-xl border transition-shadow duration-300 hover:shadow-[0_8px_24px_-8px_rgba(145,71,255,0.35)] ${statusClass}`}>
      {item.previewStatus === 'pending' ? (
        <div className="grid aspect-video place-items-center bg-[#0C0C0F] px-4 text-center text-xs text-[#7A7A85]">
          Hydrating preview…
        </div>
      ) : embedSrc ? (
        <div className="aspect-video bg-black">
          <iframe
            src={embedSrc}
            title={item.title || item.canonicalUrl}
            className="h-full w-full"
            loading="lazy"
            sandbox="allow-scripts allow-same-origin allow-popups allow-popups-to-escape-sandbox"
            allow="accelerometer; autoplay; clipboard-write; encrypted-media; picture-in-picture"
          />
        </div>
      ) : item.thumbnailUrl ? (
        <img src={item.thumbnailUrl} alt="" className="aspect-video w-full object-cover" loading="lazy" />
      ) : (
        <div className="grid aspect-video place-items-center bg-[#0C0C0F] px-4 text-center text-xs text-[#7A7A85]">
          Link preview unavailable — open source below
        </div>
      )}
      <div className="space-y-2 p-3">
        <div className="flex flex-wrap items-center gap-2 text-[11px] font-bold uppercase tracking-[0.06em] text-[#7A7A85]">
          <span>{item.providerName || sourceLabel(item.platform)}</span>
          <span className="rounded-full border border-[#33333A] bg-[#0C0C0F] px-2 py-0.5 text-[#D6D6DE]">{providerState}</span>
          <span className="rounded-full bg-[#26262C] px-2 py-0.5">{item.previewStatus}</span>
          {item.matchKind ? (
            <span className="rounded-full bg-[#26262C] px-2 py-0.5">{sourceLabel(item.matchKind)}</span>
          ) : null}
        </div>
        <h4 className="line-clamp-2 text-sm font-semibold text-[#F7F7F8]">{item.title || item.canonicalUrl}</h4>
        {item.author ? <p className="text-xs text-[#ADADB8]">by {item.author}</p> : null}
        {item.note ? <p className="rounded-lg bg-[#1B1B1F] p-2 text-xs text-[#D6D6DE]">{item.note}</p> : null}
        <a
          href={item.canonicalUrl}
          target="_blank"
          rel="noreferrer"
          className="inline-flex text-xs font-semibold text-[#A970FF] hover:text-[#C8A8FF]"
        >
          Open source
        </a>
      </div>
    </article>
  )
}

function groupGallery(gallery: PulseWireEvidencePreview[]) {
  const groups = new Map<string, PulseWireEvidencePreview[]>()
  for (const item of gallery) {
    const key = item.platform || 'web'
    const list = groups.get(key) ?? []
    list.push(item)
    groups.set(key, list)
  }
  return [...groups.entries()].sort((a, b) => a[0].localeCompare(b[0]))
}

function ReceiptsTable({ receipts }: { receipts: PulseWireReceipt[] }) {
  return (
    <div className="overflow-hidden rounded-xl border border-[#2A2A2E]">
      <table className="w-full text-left text-sm">
        <thead className="bg-[#1B1B1F] text-xs uppercase tracking-[0.06em] text-[#7A7A85]">
          <tr>
            <th className="px-3 py-2">Source</th>
            <th className="px-3 py-2">Risk</th>
            <th className="px-3 py-2">Confidence</th>
            <th className="px-3 py-2">Preview</th>
            <th className="px-3 py-2">Link</th>
          </tr>
        </thead>
        <tbody>
          {receipts.map(receipt => (
            <tr key={receipt.sourceType} className="border-t border-[#2A2A2E] text-[#D6D6DE]">
              <td className="px-3 py-2 capitalize">{sourceLabel(receipt.sourceType)}</td>
              <td className="px-3 py-2">{receipt.risk ? sourceLabel(receipt.risk) : 'unknown'}</td>
              <td className="px-3 py-2">{receipt.pct}%</td>
              <td className="px-3 py-2">{receipt.previewStatus ? sourceLabel(receipt.previewStatus) : '—'}</td>
              <td className="px-3 py-2">
                {receipt.url ? (
                  <a href={receipt.url} target="_blank" rel="noreferrer" className="text-[#A970FF] hover:underline">
                    Open
                  </a>
                ) : (
                  '—'
                )}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  )
}

function MatchExplanationList({ matches }: { matches: PulseWireMatchExplanation[] }) {
  return (
    <div className="space-y-2">
      {matches.map((match, index) => (
        <div key={`${match.sourceType}-${match.evidenceId ?? index}`} className="rounded-xl bg-[#1B1B1F] p-3 text-sm text-[#D6D6DE]">
          <span className="font-semibold capitalize text-[#EFEFF1]">{sourceLabel(match.sourceType)}</span>
          {' matched by '}
          <span className="font-semibold text-[#A970FF]">{sourceLabel(match.matchedBy)}</span>
          {' at '}
          <span className="font-semibold">{Math.round(match.confidence * 100)}%</span>
          {match.previewStatus ? <span className="text-[#7A7A85]"> · preview {match.previewStatus}</span> : null}
          {match.sourceUrl ? (
            <p className="mt-1 truncate text-xs text-[#7A7A85]">
              <a href={match.sourceUrl} target="_blank" rel="noreferrer" className="text-[#A970FF] hover:underline">
                {match.sourceUrl}
              </a>
            </p>
          ) : null}
          {match.factors?.length ? (
            <p className="mt-1 text-xs text-[#7A7A85]">{match.factors.join(' · ')}</p>
          ) : null}
        </div>
      ))}
    </div>
  )
}

function AddEvidenceForm({ storyId, onAdded }: { storyId: number; onAdded?: () => void }) {
  const [url, setUrl] = useState('')
  const [note, setNote] = useState('')
  const [pending, setPending] = useState(false)
  const [message, setMessage] = useState('')

  async function submit(event: FormEvent) {
    event.preventDefault()
    if (!url.trim()) return
    try {
      setPending(true)
      setMessage('')
      const result = await addEvidenceStory(storyId, url.trim(), note.trim())
      setUrl('')
      setNote('')
      if (result.status === 'already_attached') {
        setMessage('That URL is already attached to this story.')
      } else {
        setMessage('Evidence added.')
      }
      onAdded?.()
    } catch (error) {
      setMessage(error instanceof Error ? error.message : 'Add evidence failed')
    } finally {
      setPending(false)
    }
  }

  if (!SETUP_CONTROL_TOKEN) {
    return (
      <p className="rounded-xl border border-[#2A2A2E] bg-[#121217] p-3 text-xs text-[#7A7A85]">
        Operator add URL is hidden until a setup-control token is available.
      </p>
    )
  }

  return (
    <form id="pulse-wire-add-evidence" onSubmit={event => void submit(event)} className="rounded-xl border border-[#2A2A2E] bg-[#121217] p-3">
      <p className="mb-2 text-[11px] font-bold uppercase tracking-[0.06em] text-[#7A7A85]">Add evidence URL (manual)</p>
      <div className="grid gap-2 md:grid-cols-[minmax(0,1fr)_180px_auto]">
        <input
          value={url}
          onChange={event => setUrl(event.target.value)}
          placeholder="https://..."
          className="rounded-lg border border-[#33333A] bg-[#0C0C0F] px-3 py-2 text-sm text-[#EFEFF1] outline-none focus:border-[#A970FF]/60"
        />
        <input
          value={note}
          onChange={event => setNote(event.target.value)}
          placeholder="optional note"
          className="rounded-lg border border-[#33333A] bg-[#0C0C0F] px-3 py-2 text-sm text-[#EFEFF1] outline-none focus:border-[#A970FF]/60"
        />
        <button
          type="submit"
          disabled={pending || !url.trim()}
          className="rounded-lg bg-[#9147FF] px-4 py-2 text-sm font-semibold text-white hover:bg-[#A970FF] disabled:cursor-not-allowed disabled:opacity-50"
        >
          {pending ? 'Adding…' : 'Add'}
        </button>
      </div>
      {message ? <p className="mt-2 text-xs text-[#ADADB8]">{message}</p> : null}
    </form>
  )
}

function operatorActionLabel(action: string) {
  switch (action) {
    case 'mark_not_news':
      return 'Marked not news'
    case 'mark_community_meta':
      return 'Marked community meta'
    case 'mark_debunked':
      return 'Marked debunked'
    case 'manual_suppress':
      return 'Manually suppressed'
    case 'confirm_streamer_entity':
      return 'Confirmed streamer entity'
    case 'confirm_origin_moment':
      return 'Confirmed origin moment'
    case 'merge_duplicate_story':
      return 'Merged duplicate story'
    case 'split_unrelated_evidence':
      return 'Split unrelated evidence'
    default:
      return sourceLabel(action)
  }
}

function auditDelta(action: PulseWireOperatorAction) {
  const before = action.beforeData ?? {}
  const after = action.afterData ?? {}
  const beforeState = typeof before.state === 'string' ? before.state : ''
  const afterState = typeof after.state === 'string' ? after.state : ''
  const beforeCategory = typeof before.category === 'string' ? before.category : ''
  const afterCategory = typeof after.category === 'string' ? after.category : ''
  const beforeClass = typeof before.storyClass === 'string' ? before.storyClass : ''
  const afterClass = typeof after.storyClass === 'string' ? after.storyClass : ''
  const beforeEntityID = typeof before.entityId === 'number' ? before.entityId : null
  const afterEntityID = typeof after.entityId === 'number' ? after.entityId : null
  const beforeEntity = typeof before.entityDisplayName === 'string' && before.entityDisplayName
    ? before.entityDisplayName
    : typeof before.entityLogin === 'string'
      ? before.entityLogin
      : beforeEntityID
        ? `entity ${beforeEntityID}`
        : ''
  const afterEntity = typeof after.entityDisplayName === 'string' && after.entityDisplayName
    ? after.entityDisplayName
    : typeof after.entityLogin === 'string'
      ? after.entityLogin
      : afterEntityID
        ? `entity ${afterEntityID}`
        : ''
  const beforeMomentID = typeof before.momentFpId === 'number' ? before.momentFpId : null
  const afterMomentID = typeof after.momentFpId === 'number' ? after.momentFpId : null
  const beforeOrigin = originAuditLabel(before)
  const afterOrigin = originAuditLabel(after)
  const targetClusterID = typeof after.targetClusterId === 'number' ? after.targetClusterId : null
  const newClusterID = typeof after.newClusterId === 'number' ? after.newClusterId : null
  const movedEvidence = typeof after.movedEvidence === 'number'
    ? after.movedEvidence
    : Array.isArray(after.evidenceIds)
      ? after.evidenceIds.length
      : null
  const parts = []
  if (beforeCategory !== afterCategory) parts.push(`${beforeCategory || 'uncategorized'} -> ${afterCategory || 'uncategorized'}`)
  if (beforeClass !== afterClass) parts.push(`class ${beforeClass || 'unset'} -> ${afterClass || 'unset'}`)
  if (beforeState !== afterState) parts.push(`${beforeState || 'unknown'} -> ${afterState || 'unknown'}`)
  if (beforeEntity !== afterEntity) parts.push(`entity ${beforeEntity || 'unset'} -> ${afterEntity || 'unset'}`)
  if (beforeMomentID !== afterMomentID) parts.push(`origin ${beforeOrigin || 'unset'} -> ${afterOrigin || 'unset'}`)
  if (targetClusterID) parts.push(`${movedEvidence ?? 'selected'} evidence -> story ${targetClusterID}`)
  if (newClusterID) parts.push(`${movedEvidence ?? 'selected'} evidence -> new story ${newClusterID}`)
  if (!parts.length && afterEntity) parts.push(`entity ${afterEntity} confirmed`)
  if (!parts.length && afterOrigin) parts.push(`origin ${afterOrigin} confirmed`)
  return parts.join(' / ')
}

function originAuditLabel(data: Record<string, unknown>) {
  const streamID = typeof data.streamId === 'string' ? data.streamId : ''
  const vodID = typeof data.vodId === 'string' ? data.vodId : ''
  const offset = typeof data.vodOffsetS === 'number' ? data.vodOffsetS : null
  const momentID = typeof data.momentFpId === 'number' ? data.momentFpId : null
  if (streamID && vodID && offset !== null) return `${streamID} / ${vodID} @ ${offset}s`
  if (streamID && offset !== null) return `${streamID} @ ${offset}s`
  if (momentID) return `moment ${momentID}`
  return ''
}

function OperatorMarkActions({ story, onAdded }: { story: PulseWireStory; onAdded?: () => void }) {
  const [pendingAction, setPendingAction] = useState('')
  const [message, setMessage] = useState('')
  const entityID = story.entity?.id
  const entityLabel = story.entity?.displayName || story.entity?.login || (entityID ? `entity ${entityID}` : '')
  const originID = story.origin?.id
  const originLabel = story.origin?.streamId
    ? `${story.origin.streamId}${story.origin.vodId ? ` / ${story.origin.vodId}` : ''} @ ${story.origin.vodOffsetS}s`
    : ''
  const actions: Array<{ id: PulseWireOperatorActionName; label: string; note: string; entityId?: number; momentFpId?: number }> = [
    { id: 'mark_not_news', label: 'Not news', note: 'Operator marked story as not news.' },
    { id: 'mark_community_meta', label: 'Community meta', note: 'Operator marked story as community meta.' },
    { id: 'mark_debunked', label: 'Debunked', note: 'Operator marked story as debunked.' },
    { id: 'manual_suppress', label: 'Suppress', note: 'Operator manually suppressed story from the default Wire feed.' },
  ]
  if (entityID) {
    actions.push({
      id: 'confirm_streamer_entity',
      label: 'Confirm streamer',
      note: `Operator confirmed ${entityLabel || 'the current streamer'} as this story's entity.`,
      entityId: entityID,
    })
  }
  if (originID) {
    actions.push({
      id: 'confirm_origin_moment',
      label: 'Confirm origin',
      note: `Operator confirmed ${originLabel || 'the current Pulse origin'} as this story's origin moment.`,
      momentFpId: originID,
    })
  }

  async function submit(action: PulseWireOperatorActionName, note: string, entityId?: number, momentFpId?: number) {
    try {
      setPendingAction(action)
      setMessage('')
      await markPulseWireStory(story.story.id, action, note, { entityId, momentFpId })
      setMessage(`${operatorActionLabel(action)}.`)
      onAdded?.()
    } catch (error) {
      setMessage(error instanceof Error ? error.message : 'Operator action failed')
    } finally {
      setPendingAction('')
    }
  }

  if (!SETUP_CONTROL_TOKEN) {
    return (
      <p className="rounded-xl border border-[#2A2A2E] bg-[#121217] p-3 text-xs text-[#7A7A85]">
        Story mark actions are hidden until a setup-control token is available.
      </p>
    )
  }

  return (
    <div className="rounded-xl border border-[#2A2A2E] bg-[#121217] p-3">
      <p className="mb-2 text-[11px] font-bold uppercase tracking-[0.06em] text-[#7A7A85]">Auditable marks</p>
      <div className="flex flex-wrap gap-2">
        {actions.map(action => (
          <button
            key={action.id}
            type="button"
            disabled={Boolean(pendingAction)}
            onClick={() => void submit(action.id, action.note, action.entityId, action.momentFpId)}
            className="rounded-lg border border-[#33333A] bg-[#1B1B1F] px-3 py-2 text-xs font-semibold text-[#EFEFF1] transition hover:border-[#A970FF]/50 disabled:cursor-not-allowed disabled:opacity-50"
          >
            {pendingAction === action.id ? 'Saving...' : action.label}
          </button>
        ))}
      </div>
      {message ? <p className="mt-2 text-xs text-[#ADADB8]">{message}</p> : null}
    </div>
  )
}

function OperatorAuditTrail({ actions }: { actions?: PulseWireOperatorAction[] }) {
  if (!actions?.length) {
    return (
      <p className="rounded-xl border border-dashed border-[#33333A] bg-[#0C0C0F] p-3 text-xs text-[#7A7A85]">
        No operator actions have been recorded for this story.
      </p>
    )
  }
  return (
    <div className="space-y-2">
      {actions.map(action => (
        <article key={action.id} className="rounded-xl border border-[#2A2A2E] bg-[#101014] p-3">
          <div className="flex flex-wrap items-center justify-between gap-2">
            <p className="text-sm font-semibold text-[#EFEFF1]">{operatorActionLabel(action.action)}</p>
            {action.createdAt ? <span className="text-[11px] text-[#7A7A85]">{formatRelativeTime(action.createdAt)}</span> : null}
          </div>
          <p className="mt-1 text-xs text-[#ADADB8]">
            by <span className="font-semibold text-[#EFEFF1]">{action.operator || 'operator'}</span>
            {auditDelta(action) ? <> - {auditDelta(action)}</> : null}
          </p>
          {action.note ? <p className="mt-2 text-xs text-[#7A7A85]">{action.note}</p> : null}
        </article>
      ))}
    </div>
  )
}

export default function PulseWireStoryDetail({ story, sourceHealth, analystMode = false, onAnalystModeChange, onAdded, onCollapse }: Props) {
  const location = useLocation()
  const gallery = story.evidenceGallery ?? []
  const receipts = storyReceipts(story) ?? []
  const timeline = storyTimeline(story)
  const matches = story.matchExplanation ?? []
  const grouped = useMemo(() => groupGallery(gallery), [gallery])
  const scores = effectiveScores(story)
  const updatedAt = storyUpdatedAt(story)

  return (
    <div className="pulse-wire-detail-enter space-y-4">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <p className="text-[11px] font-bold uppercase tracking-[0.06em] text-[#A970FF]">Expanded story</p>
        {onCollapse ? (
          <button
            type="button"
            onClick={onCollapse}
            className="rounded-lg border border-[#2A2A2E] bg-[#1B1B1F] px-3 py-1.5 text-xs font-semibold text-[#EFEFF1] transition hover:border-[#A970FF]/50 hover:text-white focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[#A970FF]"
          >
            Back to Cross-platform
          </button>
        ) : (
          <a
            href="/pulse-wire?tab=wire"
            className="rounded-lg border border-[#2A2A2E] bg-[#1B1B1F] px-3 py-1.5 text-xs font-semibold text-[#EFEFF1] transition hover:border-[#A970FF]/50 hover:text-white focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[#A970FF]"
          >
            Back to Cross-platform
          </a>
        )}
      </div>
      <nav aria-label="Story detail sections" className="flex gap-2 overflow-x-auto rounded-xl border border-[#24242B] bg-[#101014] p-2 text-xs font-semibold text-[#ADADB8]">
        {[
          ['#summary', 'Summary'],
          ['#origin', 'Origin'],
          ['#missing-evidence', 'Missing'],
          ['#evidence-gallery', 'Evidence'],
          ['#spread-timeline', 'Timeline'],
          ['#source-comparison', 'Sources'],
          ['#match-explanation', 'Match'],
          ['#operator-actions', 'Operator'],
        ].map(([href, label]) => (
          <a key={href} href={href} className="shrink-0 rounded-lg px-3 py-1.5 hover:bg-[#1B1B1F] hover:text-[#EFEFF1]">
            {label}
          </a>
        ))}
      </nav>
      <div className="pulse-wire-stagger space-y-4">
      <section id="summary" aria-labelledby="story-summary-heading" className="rounded-2xl border border-[#2A2A2E] bg-[#121217] p-4 transition-colors duration-300 hover:border-[#3A3A40]">
        <div className="mb-3 flex flex-wrap items-start justify-between gap-3">
          <div>
            <p className="text-[11px] font-bold uppercase tracking-[0.06em] text-[#7A7A85]">Story detail</p>
            <h2 id="story-summary-heading" className="text-xl font-bold text-[#F7F7F8]">{story.story.title || 'Untitled story'}</h2>
            <div className="mt-1 flex flex-wrap items-center gap-2 text-xs text-[#7A7A85]">
              {story.story.category ? <span className="capitalize">{story.story.category}</span> : null}
              {story.story.storyClass ? <span className="capitalize">Class {sourceLabel(story.story.storyClass)}</span> : null}
              {updatedAt ? <span>Updated {formatRelativeTime(updatedAt)}</span> : null}
              {story.windowScores?.computedAt ? <span>Score computed {formatRelativeTime(story.windowScores.computedAt)}</span> : null}
            </div>
            {story.story.summary ? <p className="mt-2 text-sm text-[#ADADB8]">{story.story.summary}</p> : null}
          </div>
          {story.entity?.displayName ? (
            <span className="rounded-full border border-[#2A2A2E] bg-[#1B1B1F] px-3 py-1 text-xs text-[#ADADB8]">
              {story.entity.displayName}
            </span>
          ) : null}
        </div>
        <ScoreBars scores={scores} windowScores={story.windowScores} />
        <ReceiptsRow receipts={receipts} rich className="mt-3" />
      </section>

      <OriginBlock story={story} sourceHealth={sourceHealth} />

      <ModerationContext
        story={story}
        receipts={receipts}
        gallery={gallery}
        matches={matches}
        updatedAt={updatedAt}
      />

      <MissingEvidenceBlock
        story={story}
        sourceHealth={sourceHealth}
        analystMode={analystMode}
        onAnalystModeChange={onAnalystModeChange}
        focusOnMount={location.hash === '#missing-evidence'}
      />

      <section id="evidence-gallery" aria-labelledby="evidence-gallery-heading" className="rounded-2xl border border-[#2A2A2E] bg-[#121217] p-4">
        <div className="mb-3 flex flex-wrap items-center justify-between gap-2">
          <div>
            <p id="evidence-gallery-heading" className="text-[11px] font-bold uppercase tracking-[0.06em] text-[#7A7A85]">Evidence Gallery</p>
            <p className="text-sm text-[#ADADB8]">Posts and videos that made this story spread.</p>
          </div>
          <span className="rounded-full bg-[#26262C] px-2.5 py-1 text-xs font-semibold text-[#ADADB8]">
            {gallery.length} sources
          </span>
        </div>
        {gallery.length ? (
          <div className="space-y-5">
            {grouped.map(([platform, items]) => (
              <div key={platform}>
                <p className="mb-2 text-xs font-bold uppercase tracking-[0.06em] text-[#7A7A85]">
                  {sourceLabel(platform)}
                </p>
                <div className="grid gap-3 md:grid-cols-2">
                  {items.map(item => (
                    <EvidenceCard key={`${item.id ?? item.platform}-${item.canonicalUrl}`} item={item} />
                  ))}
                </div>
              </div>
            ))}
          </div>
        ) : (
          <p className="rounded-xl border border-dashed border-[#33333A] bg-[#0C0C0F] p-4 text-sm text-[#7A7A85]">
            No hydrated evidence yet. New source URLs will appear here after ingest or manual addition.
          </p>
        )}
      </section>

      <section id="spread-timeline" aria-labelledby="spread-timeline-heading" className="rounded-2xl border border-[#2A2A2E] bg-[#121217] p-4">
        <p id="spread-timeline-heading" className="mb-3 text-[11px] font-bold uppercase tracking-[0.06em] text-[#7A7A85]">Spread timeline</p>
        {timeline?.length ? (
          <SpreadTimeline timeline={timeline} />
        ) : (
          <p className="rounded-xl border border-dashed border-[#33333A] bg-[#0C0C0F] p-4 text-sm text-[#7A7A85]">
            No timeline events are attached yet.
          </p>
        )}
      </section>

      <section id="source-comparison" aria-labelledby="source-comparison-heading" className="rounded-2xl border border-[#2A2A2E] bg-[#121217] p-4">
        <p id="source-comparison-heading" className="text-[11px] font-bold uppercase tracking-[0.06em] text-[#7A7A85]">Source comparison</p>
        <p className="mb-3 text-sm text-[#ADADB8]">Receipts table comparing risk, confidence, preview state, and source links.</p>
        {receipts.length ? <ReceiptsTable receipts={receipts} /> : <p className="text-sm text-[#7A7A85]">No receipts yet.</p>}
      </section>

      <section id="match-explanation" aria-labelledby="match-explanation-heading" className="rounded-2xl border border-[#2A2A2E] bg-[#121217] p-4">
        <p id="match-explanation-heading" className="mb-3 text-[11px] font-bold uppercase tracking-[0.06em] text-[#7A7A85]">Match explanation</p>
        {matches.length ? (
          <MatchExplanationList matches={matches} />
        ) : (
          <p className="text-sm text-[#7A7A85]">No match details yet.</p>
        )}
      </section>

      <section id="operator-actions" aria-labelledby="operator-actions-heading" className="rounded-2xl border border-[#2A2A2E] bg-[#121217] p-4">
        <p id="operator-actions-heading" className="mb-3 text-[11px] font-bold uppercase tracking-[0.06em] text-[#7A7A85]">Operator actions</p>
        <div className="space-y-3">
          <OperatorMarkActions story={story} onAdded={onAdded} />
          <AddEvidenceForm storyId={story.story.id} onAdded={onAdded} />
          <div>
            <p className="mb-2 text-xs font-bold text-[#D6D6DE]">Audit trail</p>
            <OperatorAuditTrail actions={story.operatorActions} />
          </div>
        </div>
      </section>
      </div>
    </div>
  )
}
