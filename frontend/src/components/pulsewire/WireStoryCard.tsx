import { memo, useState } from 'react'
import { Link } from 'react-router-dom'
import { SETUP_CONTROL_TOKEN } from '../../config'
import { createClipStory, effectiveScores, type PulseWireSourceHealth, type PulseWireStory } from '../../pulseWireApi'
import { storyThumbnail } from '../../utils/pulseWireReceiptThumb'
import { toWireStoryView, type WireStoryView } from '../../utils/pulseWireStoryView'
import { EvidenceSpreadStrip } from './EvidenceSpread'
import ReaderStatusBadge from './ReaderStatusBadge'
import { WhyTrendingLine } from './WhyTrending'
import ScoreBars from './ScoreBars'

type Props = {
  story: PulseWireStory
  view?: WireStoryView
  sourceHealth?: PulseWireSourceHealth
  analystMode?: boolean
  detailSearch?: string
  compact?: boolean
}

export default memo(function WireStoryCard({ story, view, sourceHealth, analystMode = false, detailSearch = '', compact = false }: Props) {
  const storyView = view ?? toWireStoryView(story, sourceHealth, { analystMode })
  const [clipPending, setClipPending] = useState(false)
  const [clipError, setClipError] = useState('')
  const [expanded, setExpanded] = useState(false)
  const [missingExpanded, setMissingExpanded] = useState(false)
  const thumb = storyThumbnail(story)
  const hasEntity = Boolean(story.entity?.login || story.entity?.displayName)

  async function handleCreateClip() {
    if (!storyView.canCreateClip) return
    try {
      setClipError('')
      setClipPending(true)
      await createClipStory(storyView.id)
    } catch (error) {
      setClipError(error instanceof Error ? error.message : 'Create clip failed')
    } finally {
      setClipPending(false)
    }
  }

  return (
    <article className={`rounded-[14px] border border-[#2A2A2E] bg-[#161619] ${compact ? 'p-4' : 'p-5'}`}>
      <div className="mb-3 flex items-start justify-between gap-3">
        <div className="flex min-w-0 items-center gap-3">
          {thumb ? (
            <img src={thumb} alt="" className="h-10 w-10 shrink-0 rounded-lg object-cover" loading="lazy" />
          ) : (
            <div className="grid h-10 w-10 shrink-0 place-items-center rounded-full bg-[#26262C] text-sm font-bold text-[#ADADB8]">
              {(hasEntity ? storyView.entityLabel : storyView.title).slice(0, 1).toUpperCase()}
            </div>
          )}
          <div className="min-w-0">
            {hasEntity ? (
              <>
                <p className="truncate text-sm font-bold text-[#F7F7F8]">{storyView.entityLabel}</p>
                <p className="text-xs text-[#7A7A85]">
                  {storyView.category ?? 'Streamer culture'}
                  {storyView.lastUpdatedLabel ? ` · ${storyView.lastUpdatedLabel}` : ''}
                </p>
              </>
            ) : (
              <>
                <h3 className="line-clamp-2 text-sm font-bold leading-snug text-[#F7F7F8]">{storyView.title}</h3>
                {storyView.entitySublabel ? (
                  <p className="text-xs text-[#7A7A85]">{storyView.entitySublabel}</p>
                ) : null}
              </>
            )}
          </div>
        </div>
        <ReaderStatusBadge status={storyView.readerStatus} compact />
      </div>

      {hasEntity ? (
        <h3 className="mb-2 text-base font-semibold leading-snug text-[#F7F7F8]">{storyView.title}</h3>
      ) : (
        <p className="mb-2 text-xs text-[#7A7A85]">
          {storyView.category ?? 'Streamer culture'}
          {storyView.lastUpdatedLabel ? ` · ${storyView.lastUpdatedLabel}` : ''}
        </p>
      )}
      <WhyTrendingLine view={storyView} compact />
      <EvidenceSpreadStrip view={storyView} compact className="mt-3" />

      {analystMode && storyView.missingEvidence.length ? (
        <div className="mt-3">
          <button
            type="button"
            onClick={() => setMissingExpanded(value => !value)}
            className="rounded-full border border-[#3A2A12] bg-[#21190D] px-2.5 py-1 text-xs font-semibold text-[#FFE0A3] hover:border-[#A970FF]/35"
          >
            {missingExpanded
              ? storyView.missingEvidence.join(' · ')
              : `${storyView.missingEvidence.length} signal${storyView.missingEvidence.length === 1 ? '' : 's'} pending`}
          </button>
        </div>
      ) : null}

      <div className="mt-4 flex flex-wrap items-center justify-between gap-3">
        <div className="text-xs text-[#7A7A85]">
          <span className="font-semibold text-[#D6D6DE]">{storyView.confidenceLabel}</span>
          {' confidence · '}
          {storyView.sourceCount} source{storyView.sourceCount === 1 ? '' : 's'} · {storyView.evidenceCount} evidence
        </div>
        <div className="flex flex-wrap gap-2">
          <button
            type="button"
            onClick={() => setExpanded(value => !value)}
            className="rounded-lg border border-[#33333A] bg-[#1F1F23] px-3 py-2 text-xs font-semibold text-[#ADADB8] transition hover:border-[#A970FF]/40 hover:text-[#EFEFF1] focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[#A970FF]"
          >
            {expanded ? 'Hide details' : 'View receipts'}
          </button>
          {SETUP_CONTROL_TOKEN && storyView.canCreateClip ? (
            <button
              type="button"
              onClick={() => void handleCreateClip()}
              disabled={clipPending}
              className="rounded-lg border border-[#33333A] bg-[#1F1F23] px-3 py-2 text-xs font-semibold text-[#EFEFF1] transition hover:border-[#A970FF]/40 focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[#A970FF] disabled:cursor-not-allowed disabled:opacity-50"
            >
              {clipPending ? 'Creating...' : 'Create clip'}
            </button>
          ) : null}
          <Link
            to={`/pulse-wire/${storyView.id}${detailSearch}`}
            className="rounded-lg bg-[#9147FF] px-3 py-2 text-xs font-semibold text-white transition hover:bg-[#A970FF] focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[#A970FF]"
          >
            Open story
          </Link>
        </div>
      </div>

      {expanded ? (
        <div className="mt-4 border-t border-[#2A2A2E] pt-4">
          <ScoreBars scores={effectiveScores(story)} windowScores={story.windowScores} compact />
        </div>
      ) : null}
      {clipError ? <p className="mt-2 text-xs text-red-300">{clipError}</p> : null}
    </article>
  )
})
