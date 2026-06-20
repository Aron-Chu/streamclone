import { memo, useState } from 'react'

import { Link } from 'react-router-dom'

import { SETUP_CONTROL_TOKEN } from '../../config'

import {
  createClipStory,
  effectiveScores,
  formatChannelMatchReason,
  matchConfidenceLabel,
  storyReceipts,
  storyUpdatedAt,
  type PulseWireStory,
} from '../../pulseWireApi'

import { formatRelativeTime } from '../../utils/pulseWireFormat'

import { storyThumbnail } from '../../utils/pulseWireReceiptThumb'
import { hasClipReadyOrigin, toWireStoryView } from '../../utils/pulseWireStoryView'

import { COMMUNITY_CARD_LINK_CLASS_CHANNEL } from './community/communityPostCardLink'
import ScoreBars from './ScoreBars'
import ReceiptsRow from './ReceiptsRow'

type Props = {
  story: PulseWireStory
  variant?: 'default' | 'channel' | 'editorial'
  detailSearch?: string
  wireFriendly?: boolean
  subdued?: boolean
}

function editorialVariant(story: PulseWireStory): 'breaking' | 'settled' | 'unverified' | 'default' {
  if (story.story.state === 'unverified') return 'unverified'
  if (story.story.state === 'published') return 'breaking'
  if (story.story.state === 'settled') return 'settled'
  return 'default'
}

export default memo(function StoryCompactCard({
  story,
  variant = 'default',
  detailSearch = '',
  wireFriendly = false,
  subdued = false,
}: Props) {
  const [clipPending, setClipPending] = useState(false)
  const [clipError, setClipError] = useState('')
  const [showDetails, setShowDetails] = useState(false)
  const isChannelVariant = variant === 'channel'
  const isEditorial = variant === 'editorial'
  const editorial = editorialVariant(story)
  const storyView = toWireStoryView(story)
  const hasEntity = Boolean(story.entity?.login || story.entity?.displayName)
  const isUnverified = editorial === 'unverified' || !story.entity?.login
  const settled = editorial === 'settled' || (!isUnverified && story.scores.confidence !== 'single_source')
  const badgeClass = editorial === 'breaking'
    ? 'bg-[#2A1515] text-[#FF5C57]'
    : isUnverified
      ? 'bg-[#3A2A12] text-[#FFB02E]'
      : settled
        ? 'bg-[#16321F] text-[#3FCB7E]'
        : 'bg-[#26262C] text-[#ADADB8]'
  const badgeLabel = editorial === 'breaking'
    ? 'Breaking'
    : isUnverified
      ? 'Unverified'
      : settled
        ? 'Settled'
        : story.story.state
  const thumb = storyThumbnail(story)
  const canClip = hasClipReadyOrigin(story)
  const updatedAt = storyUpdatedAt(story)
  const receipts = storyReceipts(story)
  const scores = effectiveScores(story)
  const sourceCount = story.windowScores?.sourceCount ?? receipts?.length ?? 0
  const primaryMatch = story.matchExplanation?.[0]
  const channelMatchReason = isChannelVariant ? formatChannelMatchReason(primaryMatch) : undefined
  const channelMatchConfidence = isChannelVariant ? matchConfidenceLabel(primaryMatch?.confidence) : undefined
  const borderClass = isChannelVariant
    ? subdued
      ? 'border-[#2A2A2E] bg-[#101014]'
      : 'border-white/10 bg-white/[0.035]'
    : editorial === 'breaking'
      ? 'border-[#FF5C57]/25 bg-[#161619]'
      : isUnverified
        ? 'border-[#3A2A12] bg-[#161619]'
        : 'border-[#26262C] bg-[#161619]'
  const detailHref = `/pulse-wire/${story.story.id}${detailSearch}`

  async function handleCreateClip() {
    if (!canClip) return
    try {
      setClipError('')
      setClipPending(true)
      await createClipStory(story.story.id)
    } catch (error) {
      setClipError(error instanceof Error ? error.message : 'Create clip failed')
    } finally {
      setClipPending(false)
    }
  }

  const cardBody = (
    <>
      <div className="mb-2 flex items-center justify-between gap-3">
        <div className="flex min-w-0 items-center gap-3">
          {thumb ? (
            <img src={thumb} alt="" className="h-[34px] w-[34px] shrink-0 rounded-full object-cover" loading="lazy" />
          ) : (
            <div className={`grid h-[34px] w-[34px] shrink-0 place-items-center rounded-full text-sm font-bold ${
              isUnverified ? 'bg-[#3A3A40] text-[#ADADB8]' : 'bg-gradient-to-br from-[#3FCB7E] to-[#1F6E4A] text-white'
            }`}>
              {(story.entity?.displayName || story.entity?.login || '?').slice(0, 1).toUpperCase()}
            </div>
          )}
          <div className="min-w-0">
            {hasEntity ? (
              <>
                <p className="text-[15px] font-bold text-[#F7F7F8]">{storyView.entityLabel}</p>
                <p className="text-xs text-[#ADADB8]">
                  {story.story.category || 'Entity match pending'}
                  {updatedAt ? <> · {formatRelativeTime(updatedAt)}</> : null}
                </p>
              </>
            ) : (
              <>
                <p className="line-clamp-2 text-[15px] font-bold leading-snug text-[#F7F7F8]">{storyView.title}</p>
                <p className="text-xs text-[#ADADB8]">
                  {storyView.entitySublabel ?? 'Entity match pending'}
                  {updatedAt ? <> · {formatRelativeTime(updatedAt)}</> : null}
                </p>
              </>
            )}
          </div>
        </div>
        <div className="flex shrink-0 flex-wrap items-center justify-end gap-2">
          <span className={`rounded-full px-2.5 py-1 text-xs font-semibold ${badgeClass}`}>{badgeLabel}</span>
          {story.story.category && !isUnverified && !isEditorial ? (
            <span className="rounded-full bg-[#26262C] px-2.5 py-1 text-xs font-semibold capitalize text-[#ADADB8]">
              {story.story.category}
            </span>
          ) : null}
        </div>
      </div>

      <p className="mb-3 text-base font-semibold text-[#F7F7F8]">
        {hasEntity ? (story.story.title || 'Story developing') : storyView.entityLabel}
      </p>

      {channelMatchReason ? (
        <div className="mb-3 flex flex-wrap items-center gap-2">
          <p className="text-xs text-[#ADADB8]">{channelMatchReason}</p>
          {channelMatchConfidence ? (
            <span className="rounded-full bg-[#26262C] px-2 py-0.5 text-[10px] font-semibold uppercase text-[#ADADB8]">
              {channelMatchConfidence}
            </span>
          ) : null}
        </div>
      ) : null}

      {wireFriendly ? (
        <p className="mb-3 text-xs font-semibold text-[#7A7A85]">
          {sourceCount > 0 ? `${sourceCount} source${sourceCount === 1 ? '' : 's'}` : 'Sources gathering'}
          {updatedAt ? <> · {formatRelativeTime(updatedAt)}</> : null}
        </p>
      ) : null}

      {isUnverified && !story.origin ? (
        <div className="mb-3 rounded-lg bg-[#2A2212] px-3 py-2 text-xs text-[#C9B98A]">
          <span className="font-semibold text-[#FFB02E]">Moment unlinked</span>
          {' — '}
          No Pulse moment matched — searching quotes + entity hints.
        </div>
      ) : null}

      <div className={`flex flex-wrap items-center gap-3 ${isChannelVariant ? '' : 'justify-between'}`}>
        <div className="min-w-0 space-y-2">
          <ReceiptsRow receipts={receipts} compact rich={isEditorial} linkable={!isChannelVariant} />
          {wireFriendly ? (
            showDetails ? <ScoreBars scores={scores} windowScores={story.windowScores} compact /> : null
          ) : (
            !isChannelVariant ? <ScoreBars scores={scores} windowScores={story.windowScores} compact /> : null
          )}
        </div>

        {!isChannelVariant ? (
          <div className="flex shrink-0 items-center gap-2">
            {wireFriendly ? (
              <button
                type="button"
                onClick={() => setShowDetails(value => !value)}
                className="rounded-lg border border-[#33333A] bg-[#1F1F23] px-3 py-2 text-xs font-semibold text-[#ADADB8] transition hover:border-[#A970FF]/40 hover:text-[#EFEFF1] focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[#A970FF]"
              >
                {showDetails ? 'Hide details' : 'Details'}
              </button>
            ) : null}

            {SETUP_CONTROL_TOKEN && canClip ? (
              <button
                type="button"
                onClick={() => void handleCreateClip()}
                disabled={clipPending}
                className="rounded-lg border border-[#33333A] bg-[#1F1F23] px-3 py-2 text-xs font-semibold text-[#EFEFF1] transition hover:border-[#A970FF]/40 focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[#A970FF] disabled:cursor-not-allowed disabled:opacity-50"
              >
                {clipPending ? 'Creating…' : 'Create clip'}
              </button>
            ) : null}

            <Link
              to={detailHref}
              className="rounded-lg bg-[#9147FF] px-3 py-2 text-xs font-semibold text-white hover:bg-[#A970FF] focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[#A970FF]"
            >
              View
            </Link>
          </div>
        ) : null}
      </div>
    </>
  )

  if (isChannelVariant) {
    return (
      <div className="space-y-2">
        <Link
          to={detailHref}
          data-testid="spread-story-card"
          className={`${COMMUNITY_CARD_LINK_CLASS_CHANNEL} px-5 py-4 ${borderClass}`}
        >
          {cardBody}
        </Link>
        {SETUP_CONTROL_TOKEN && canClip ? (
          <button
            type="button"
            onClick={() => void handleCreateClip()}
            disabled={clipPending}
            className="rounded-lg border border-[#33333A] bg-[#1F1F23] px-3 py-2 text-xs font-semibold text-[#EFEFF1] transition hover:border-[#A970FF]/40 focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[#A970FF] disabled:cursor-not-allowed disabled:opacity-50"
          >
            {clipPending ? 'Creating…' : 'Create clip'}
          </button>
        ) : null}
        {clipError ? <p className="text-xs text-red-300">{clipError}</p> : null}
      </div>
    )
  }

  return (
    <article className={`rounded-[14px] border px-5 py-4 ${borderClass}`}>
      {cardBody}
      {clipError ? <p className="mt-2 text-xs text-red-300">{clipError}</p> : null}
    </article>
  )
})
