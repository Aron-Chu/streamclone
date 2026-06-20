import { useState } from 'react'
import { SETUP_CONTROL_TOKEN } from '../../config'
import {
  createClipStory,
  followStory,
  storyReceipts,
  storyTimeline,
  type PulseWireEvidencePreview,
  type PulseWireSourceHealth,
  type PulseWireStory,
  type PulseWireTimelineStep,
} from '../../pulseWireApi'
import { hasSearchedMissingOrigin, toWireStoryView } from '../../utils/pulseWireStoryView'
import { buildPulseWireOriginHref } from '../../utils/pulseWireOriginLink'
import { receiptThumbnail } from '../../utils/pulseWireReceiptThumb'
import { EvidenceSpreadCards } from './EvidenceSpread'
import OriginSpikeChart from './OriginSpikeChart'
import ReaderStatusBadge from './ReaderStatusBadge'
import { WhyTrendingBullets } from './WhyTrending'

type Props = {
  story: PulseWireStory
  sourceHealth?: PulseWireSourceHealth
  analystMode?: boolean
  onOpen: () => void
  missingEvidenceHref?: string
  onReviewMissingEvidence?: () => void
  onTrackedChange?: (tracked: boolean) => void
}

const TIMELINE_SOURCE_CLASS: Record<string, string> = {
  twitch_clip: 'border-[#9147FF]/40 text-[#CDB4FF]',
  twitch: 'border-[#9147FF]/40 text-[#CDB4FF]',
  reddit_thread: 'border-[#FF4500]/40 text-[#FFB39A]',
  reddit: 'border-[#FF4500]/40 text-[#FFB39A]',
  youtube_video: 'border-[#FF0000]/40 text-[#FFAAAA]',
  youtube: 'border-[#FF0000]/40 text-[#FFAAAA]',
  x: 'border-[#1D9BF0]/40 text-[#A8D8FF]',
  twitter: 'border-[#1D9BF0]/40 text-[#A8D8FF]',
  tiktok: 'border-[#00D7B0]/40 text-[#9EF5E3]',
  bans: 'border-[#FF5C57]/40 text-[#FFC0BD]',
}

function shortTime(value?: string) {
  if (!value) return ''
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return ''
  return date.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })
}

function evidenceLabel(evidence: PulseWireEvidencePreview) {
  return evidence.providerName || evidence.platform || 'Source'
}

function evidenceMeta(evidence: PulseWireEvidencePreview) {
  return [evidence.author, evidence.previewStatus].filter(Boolean).map(String).join(' - ')
}

function originConfidenceLabel(value?: number) {
  if (value == null || !Number.isFinite(value)) return ''
  return `${Math.round(Math.max(0, Math.min(1, value)) * 100)}% origin confidence`
}

function PulseOriginBlock({ story }: { story: PulseWireStory }) {
  const origin = story.origin
  if (!origin) {
    return (
      <div className="mt-3 rounded-lg border border-dashed border-[#3A2A12] bg-[#181308] p-3 text-xs leading-relaxed text-[#FFE0A3]">
        Pulse origin graph and top emotes are waiting on real Analytics origin matching.
      </div>
    )
  }

  const emotes = (origin.topEmotes ?? []).slice(0, 5)
  return (
    <div className="mt-3 rounded-lg border border-[#27412F] bg-[#102016] p-3">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <p className="text-xs font-bold text-[#A8F0C2]">Pulse-origin evidence is attached.</p>
        {origin.originConfidence != null ? (
          <span className="rounded bg-[#183727] px-2 py-0.5 text-[10px] font-black uppercase text-[#72F0A3]">
            {originConfidenceLabel(origin.originConfidence)}
          </span>
        ) : null}
      </div>
      {origin.chatSpikeSummary ? (
        <p className="mt-2 text-xs leading-relaxed text-[#D6F7DF]">{origin.chatSpikeSummary}</p>
      ) : null}
      <OriginSpikeChart origin={origin} compact />
      {emotes.length ? (
        <div className="mt-3 flex flex-wrap gap-1.5" aria-label="Top origin emotes">
          {emotes.map((emote, index) => (
            <span key={`${emote.id ?? emote.name}-${index}`} className="rounded border border-[#2D5A3A] bg-[#16291D] px-2 py-1 text-[11px] font-semibold text-[#A8F0C2]">
              {emote.name}{emote.count ? ` x${emote.count}` : ''}
            </span>
          ))}
        </div>
      ) : null}
      <div className="mt-3 grid gap-2 text-[11px] text-[#83C999]">
        <span>stream {origin.streamId}</span>
        {origin.vodId ? <span>VOD {origin.vodId}</span> : null}
        <span>offset {origin.vodOffsetS}s</span>
      </div>
    </div>
  )
}

function StoryTimelineChips({ timeline }: { timeline?: PulseWireTimelineStep[] }) {
  const steps = (timeline ?? []).slice(0, 6)
  if (!steps.length) {
    return (
      <div className="rounded-lg border border-[#2A2A2E] bg-[#101014] px-3 py-2 text-xs text-[#7A7A85]">
        Timeline appears after clustered evidence has timestamps.
      </div>
    )
  }

  return (
    <div className="flex gap-2 overflow-x-auto pb-1" aria-label="Story timeline">
      {steps.map((step, index) => (
        <div key={`${step.at}-${step.label}-${index}`} className="flex shrink-0 items-center gap-2">
          {index > 0 ? <span className="h-px w-5 bg-[#33333A]" /> : null}
          <a
            href={step.sourceUrl}
            target={step.sourceUrl ? '_blank' : undefined}
            rel={step.sourceUrl ? 'noopener noreferrer' : undefined}
            className={`min-w-[150px] rounded-lg border bg-[#111116] px-3 py-2 ${TIMELINE_SOURCE_CLASS[step.sourceType] ?? 'border-[#33333A] text-[#D6D6DE]'}`}
          >
            <p className="truncate text-[11px] font-bold">{step.label}</p>
            <p className="mt-1 text-[10px] text-[#7A7A85]">{shortTime(step.at) || step.sourceType}</p>
          </a>
        </div>
      ))}
    </div>
  )
}

function EvidencePreviewGrid({ story }: { story: PulseWireStory }) {
  const galleryPreviews = (story.evidenceGallery ?? []).slice(0, 4)
  const receiptPreviews: PulseWireEvidencePreview[] = galleryPreviews.length
    ? []
    : (storyReceipts(story) ?? [])
        .filter(receipt => receipt.previewStatus === 'ready' && receipt.url)
        .slice(0, 4)
        .map((receipt, index) => ({
          id: receipt.previewId ?? index,
          canonicalUrl: receipt.url ?? '',
          platform: receipt.sourceType,
          providerName: receipt.label || receipt.sourceType,
          title: receipt.label || receipt.url,
          thumbnailUrl: receipt.thumbnailUrl || receiptThumbnail(receipt.url),
          previewStatus: receipt.previewStatus ?? 'ready',
        }))
  const previews = galleryPreviews.length ? galleryPreviews : receiptPreviews
  if (!previews.length) {
    return (
      <div className="grid gap-3 md:grid-cols-2 xl:grid-cols-4">
        <div className="rounded-lg border border-dashed border-[#33333A] bg-[#101014] p-3 text-xs text-[#7A7A85]">
          Evidence previews are available on the story page once source metadata is fetched.
        </div>
      </div>
    )
  }

  return (
    <div className="grid gap-3 md:grid-cols-2 xl:grid-cols-4" aria-label="Evidence previews">
      {previews.map((preview, index) => (
        <a
          key={preview.id ?? `${preview.canonicalUrl}-${index}`}
          href={preview.canonicalUrl}
          target="_blank"
          rel="noopener noreferrer"
          className="group overflow-hidden rounded-lg border border-[#2A2A2E] bg-[#111116] transition hover:border-[#A970FF]/45"
        >
          <div className="relative aspect-video bg-[#1B1B1F]">
            {preview.thumbnailUrl ? (
              <img
                src={preview.thumbnailUrl}
                alt=""
                loading="lazy"
                className="h-full w-full object-cover opacity-90 transition group-hover:opacity-100"
              />
            ) : (
              <div className="grid h-full place-items-center px-3 text-center text-[11px] font-semibold text-[#7A7A85]">
                Preview fallback
              </div>
            )}
            <span className="absolute left-2 top-2 rounded bg-black/70 px-1.5 py-0.5 text-[10px] font-bold text-white">
              {evidenceLabel(preview)}
            </span>
          </div>
          <div className="p-3">
            <p className="line-clamp-2 min-h-[34px] text-xs font-semibold leading-snug text-[#F7F7F8]">
              {preview.title || preview.canonicalUrl}
            </p>
            <p className="mt-2 truncate text-[11px] text-[#7A7A85]">{evidenceMeta(preview) || 'Open source'}</p>
          </div>
        </a>
      ))}
    </div>
  )
}

export default function LeadStoryDesk({ story, sourceHealth, analystMode = false, onOpen, missingEvidenceHref, onReviewMissingEvidence, onTrackedChange }: Props) {
  const [tracked, setTracked] = useState(Boolean(story.tracked))
  const [pending, setPending] = useState('')
  const [error, setError] = useState('')
  const view = toWireStoryView(story, sourceHealth, { analystMode })
  const originHref = buildPulseWireOriginHref(story)
  const searchedMissingOrigin = hasSearchedMissingOrigin(story)

  async function track() {
    try {
      setError('')
      setPending('track')
      await followStory(story.story.id)
      setTracked(true)
      onTrackedChange?.(true)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Track story failed')
    } finally {
      setPending('')
    }
  }

  async function clip() {
    if (!view.canCreateClip) return
    try {
      setError('')
      setPending('clip')
      await createClipStory(story.story.id)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Create clip failed')
    } finally {
      setPending('')
    }
  }

  return (
    <section className="rounded-lg border border-[#24242B] bg-[#101014] p-4 shadow-[0_0_0_1px_rgba(255,255,255,0.015)]">
      <header className="flex flex-wrap items-start justify-between gap-3 border-b border-[#202027] pb-3">
        <div className="min-w-0">
          <div className="flex flex-wrap items-center gap-2">
            <span className="rounded bg-[#2B194C] px-2 py-0.5 text-[10px] font-black uppercase tracking-[0.08em] text-[#D9C3FF]">
              Lead story
            </span>
            {view.lastUpdatedLabel ? <span className="text-[11px] text-[#7A7A85]">Updated {view.lastUpdatedLabel}</span> : null}
          </div>
          <h2 className="mt-3 max-w-4xl text-[22px] font-black leading-tight text-[#F7F7F8] md:text-2xl">{view.title}</h2>
          {story.story.summary ? <p className="mt-2 max-w-3xl text-sm leading-relaxed text-[#ADADB8]">{story.story.summary}</p> : null}
        </div>
        <ReaderStatusBadge status={view.readerStatus} />
      </header>

      <div className="mt-4 grid gap-4 xl:grid-cols-[minmax(0,1fr)_240px]">
        <div className="min-w-0">
          <div className="grid gap-3 lg:grid-cols-[1fr_1fr_180px]">
            <div className="rounded-lg border border-[#24242B] bg-[#15151B] p-3">
              <h3 className="text-xs font-black uppercase tracking-[0.06em] text-[#BFBFCB]">Why trending</h3>
              <div className="mt-2">
                <WhyTrendingBullets view={view} />
              </div>
            </div>

            <div className="rounded-lg border border-[#24242B] bg-[#15151B] p-3">
              <div className="mb-2 flex flex-wrap items-center justify-between gap-2">
                <h3 className="text-xs font-black uppercase tracking-[0.06em] text-[#BFBFCB]">Origin status</h3>
                <span className="rounded bg-[#213526] px-2 py-0.5 text-[10px] font-black uppercase text-[#72F0A3]">
                  {view.hasPulseOrigin ? 'Pulse matched' : view.canCreateClip ? 'Twitch origin found' : searchedMissingOrigin ? 'No Twitch origin found' : 'Origin pending'}
                </span>
              </div>
              <p className="text-xs leading-relaxed text-[#ADADB8]">
                {view.hasPulseOrigin
                  ? view.canCreateClip
                    ? 'Pulse origin evidence is VOD-backed and can feed clip workflows.'
                    : 'Pulse origin evidence is attached; clip workflows wait for a VOD-backed timestamp.'
                  : searchedMissingOrigin
                    ? 'Analytics searched the available Twitch moments for this story and did not find a matching origin yet.'
                    : 'No Pulse or Twitch origin is matched yet. The story can still be useful, but origin proof is missing.'}
              </p>
              {view.hasPulseOrigin && originHref ? (
                <a
                  href={originHref}
                  className="mt-3 rounded border border-[#33333A] bg-[#1B1B21] px-3 py-1.5 text-xs font-semibold text-[#EFEFF1] transition hover:border-[#A970FF]/40"
                >
                  View origin moment
                </a>
              ) : null}
            </div>

            <div className="rounded-lg border border-[#24242B] bg-[#15151B] p-3">
              <p className="text-xs font-black uppercase tracking-[0.06em] text-[#BFBFCB]">Confidence</p>
              <div className="mt-2 flex items-end justify-between gap-2">
                <p className="text-2xl font-black text-[#F7F7F8]">{view.confidenceLabel}</p>
                <span className="rounded bg-[#183727] px-1.5 py-0.5 text-[10px] font-bold text-[#78E7A1]">
                  {view.sourceCount > 1 ? 'Multi-source' : 'Single-source'}
                </span>
              </div>
              <div className="mt-3 h-1.5 overflow-hidden rounded-full bg-[#25252B]">
                <div
                  className="h-full rounded-full bg-[#3FCB7E]"
                  style={{ width: `${Math.min(100, Math.max(24, view.sourceCount * 22 + view.evidenceCount * 8))}%` }}
                />
              </div>
              <p className="mt-2 text-[11px] text-[#7A7A85]">
                Based on {view.sourceCount} source{view.sourceCount === 1 ? '' : 's'} and {view.evidenceCount} evidence item{view.evidenceCount === 1 ? '' : 's'}.
              </p>
            </div>
          </div>

          <EvidenceSpreadCards view={view} className="mt-4" />

          <div className="mt-4 rounded-lg border border-[#24242B] bg-[#15151B] p-3">
            <div className="mb-3 flex flex-wrap items-center justify-between gap-2">
              <h3 className="text-xs font-black uppercase tracking-[0.06em] text-[#BFBFCB]">Story timeline</h3>
              <button type="button" onClick={onOpen} className="text-[11px] font-semibold text-[#A970FF] hover:text-[#CDB4FF]">
                View all evidence
              </button>
            </div>
            <StoryTimelineChips timeline={storyTimeline(story)} />
          </div>

          <div className="mt-4">
            <div className="mb-3 flex flex-wrap items-center justify-between gap-2">
              <h3 className="text-sm font-bold text-[#F7F7F8]">Evidence previews</h3>
              <button type="button" onClick={onOpen} className="text-[11px] font-semibold text-[#A970FF] hover:text-[#CDB4FF]">
                View evidence
              </button>
            </div>
            <EvidencePreviewGrid story={story} />
          </div>
        </div>

        <aside className="min-w-0">
          <div className="rounded-lg border border-[#24242B] bg-[#15151B] p-3">
            <div className="flex items-center justify-between gap-2">
              <p className="text-xs font-black uppercase tracking-[0.06em] text-[#BFBFCB]">Pulse origin</p>
              <span className="text-[11px] font-semibold text-[#A970FF]">Window {story.windowScores?.window ?? '24h'}</span>
            </div>
            <PulseOriginBlock story={story} />
            <p className="mt-3 text-xs text-[#ADADB8]">
              {view.hasPulseOrigin ? 'Matched to a Pulse-origin moment.' : 'Pulse graph is pending until Analytics origin matching is connected.'}
            </p>
            {(analystMode || view.missingEvidence.length > 0) && view.missingEvidence.length ? (
              <div className="mt-3 rounded border border-[#3A2A12] bg-[#21190D] p-2">
                <p className="text-[11px] font-semibold text-[#FFE0A3]">{view.missingEvidence[0]}</p>
                {missingEvidenceHref ? (
                  <a
                    href={missingEvidenceHref}
                    onClick={event => {
                      if (!onReviewMissingEvidence) return
                      event.preventDefault()
                      onReviewMissingEvidence()
                    }}
                    className="mt-2 inline-flex text-[11px] font-semibold text-[#A970FF] hover:text-[#CDB4FF]"
                  >
                    Review missing evidence
                  </a>
                ) : null}
              </div>
            ) : null}
          </div>
          <div className="mt-3 flex flex-wrap gap-2">
            <button
              type="button"
              onClick={onOpen}
              className="rounded-lg bg-[#9147FF] px-4 py-2 text-sm font-semibold text-white transition hover:bg-[#A970FF] focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[#A970FF]"
            >
              Open story
            </button>
            <button
              type="button"
              onClick={() => void track()}
              disabled={tracked || pending === 'track'}
              className="rounded-lg border border-[#33333A] bg-[#1F1F23] px-4 py-2 text-sm font-semibold text-[#EFEFF1] transition hover:border-[#A970FF]/40 focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[#A970FF] disabled:opacity-50"
            >
              {tracked ? 'Tracked' : pending === 'track' ? 'Saving...' : 'Track story'}
            </button>
            {SETUP_CONTROL_TOKEN && view.canCreateClip ? (
              <button
                type="button"
                onClick={() => void clip()}
                disabled={pending === 'clip'}
                className="rounded-lg border border-[#33333A] bg-[#1F1F23] px-4 py-2 text-sm font-semibold text-[#EFEFF1] transition hover:border-[#A970FF]/40 focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[#A970FF] disabled:opacity-50"
              >
                {pending === 'clip' ? 'Creating...' : 'Create clip'}
              </button>
            ) : null}
          </div>
          {error ? <p className="mt-2 text-xs text-red-300">{error}</p> : null}
        </aside>
      </div>
    </section>
  )
}
