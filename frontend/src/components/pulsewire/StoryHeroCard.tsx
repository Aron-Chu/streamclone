import { useState } from 'react'

import { Link } from 'react-router-dom'

import { SETUP_CONTROL_TOKEN } from '../../config'

import {

  createClipStory,

  effectiveScores,

  followStory,

  storyReceipts,

  storyTimeline,

  storyUpdatedAt,

  unfollowStory,

  type PulseWireStory,

} from '../../pulseWireApi'

import { formatRelativeTime } from '../../utils/pulseWireFormat'

import { storyEntityAvatar, storyThumbnail } from '../../utils/pulseWireReceiptThumb'
import { hasClipReadyOrigin } from '../../utils/pulseWireStoryView'

import ScoreBars from './ScoreBars'

import ReceiptsRow from './ReceiptsRow'

import SpreadTimeline from './SpreadTimeline'



type EditorialVariant = 'breaking' | 'settled' | 'unverified' | 'default'



type Props = {

  story: PulseWireStory

  variant?: EditorialVariant

  onView?: () => void

  onTrackedChange?: (tracked: boolean) => void

  wireFriendly?: boolean

}



const WIRE_SUMMARY_BOILERPLATE = 'Wire-native social evidence grouped from global source ingest.'



function resolveVariant(story: PulseWireStory, variant?: EditorialVariant): EditorialVariant {

  if (variant && variant !== 'default') return variant

  if (story.story.state === 'unverified') return 'unverified'

  if (story.story.state === 'published') return 'breaking'

  if (story.story.state === 'settled') return 'settled'

  return 'default'

}



function badgeClass(variant: EditorialVariant) {

  if (variant === 'breaking') return 'bg-[#2A1515] text-[#FF5C57]'

  if (variant === 'unverified') return 'bg-[#3A2A12] text-[#FFB02E]'

  if (variant === 'settled') return 'bg-[#16321F] text-[#3FCB7E]'

  return 'bg-[#26262C] text-[#ADADB8]'

}



function badgeLabel(variant: EditorialVariant, state: string) {

  if (variant === 'breaking') return 'Breaking'

  if (variant === 'settled') return 'Settled'

  if (variant === 'unverified') return 'Unverified'

  return state

}



function borderClass(variant: EditorialVariant) {

  if (variant === 'breaking') return 'border-[#FF5C57]/30'

  if (variant === 'unverified') return 'border-[#3A2A12]'

  if (variant === 'settled') return 'border-[#3FCB7E]/25'

  return 'border-[#2A2A2E]'

}



export default function StoryHeroCard({ story, variant, onView, onTrackedChange, wireFriendly = false }: Props) {

  const [tracked, setTracked] = useState(Boolean(story.tracked))

  const [trackPending, setTrackPending] = useState(false)

  const [clipPending, setClipPending] = useState(false)

  const [actionError, setActionError] = useState('')

  const [showDetails, setShowDetails] = useState(false)

  const editorial = resolveVariant(story, variant)

  const entity = story.entity?.displayName || story.entity?.login || 'Streamer'

  const thumb = storyThumbnail(story)

  const entityAvatar = storyEntityAvatar(story)

  const thumbIsEntityAvatar = Boolean(entityAvatar && thumb === entityAvatar)

  const canClip = hasClipReadyOrigin(story)

  const updatedAt = storyUpdatedAt(story)

  const receipts = storyReceipts(story)

  const timeline = storyTimeline(story)

  const scores = effectiveScores(story)

  const sourceCount = story.windowScores?.sourceCount ?? receipts?.length ?? 0



  async function toggleTrack() {

    try {

      setActionError('')

      setTrackPending(true)

      if (tracked) {

        await unfollowStory(story.story.id)

        setTracked(false)

        onTrackedChange?.(false)

      } else {

        await followStory(story.story.id)

        setTracked(true)

        onTrackedChange?.(true)

      }

    } catch (error) {

      setActionError(error instanceof Error ? error.message : 'Track failed')

    } finally {

      setTrackPending(false)

    }

  }



  async function handleCreateClip() {

    if (!canClip) return

    try {

      setActionError('')

      setClipPending(true)

      await createClipStory(story.story.id)

    } catch (error) {

      setActionError(error instanceof Error ? error.message : 'Create clip failed')

    } finally {

      setClipPending(false)

    }

  }



  return (

    <article className={`rounded-2xl border bg-[#161619] p-5 shadow-[0_8px_24px_-6px_rgba(0,0,0,.35)] ${borderClass(editorial)}`}>

      <header className="mb-3 flex flex-wrap items-start justify-between gap-3">

        <div className="flex min-w-0 items-center gap-3">

          {thumb ? (

            <img

              src={thumb}

              alt=""

              className={`h-12 w-12 shrink-0 object-cover ${thumbIsEntityAvatar ? 'rounded-full' : 'rounded-lg'}`}

              loading="lazy"

            />

          ) : (

            <div className={`grid h-12 w-12 shrink-0 place-items-center bg-[#1B1B1F] text-sm font-bold text-[#ADADB8] ${entityAvatar ? 'rounded-full' : 'rounded-lg'}`}>

              {entity.slice(0, 1).toUpperCase()}

            </div>

          )}

          <div className="min-w-0">

            <div className="flex flex-wrap items-center gap-2">

              <h2 className="text-lg font-bold text-[#F7F7F8]">{entity}</h2>

              <span className={`rounded-full px-2 py-0.5 text-xs font-semibold uppercase tracking-wide ${badgeClass(editorial)}`}>

                {badgeLabel(editorial, story.story.state)}

              </span>

              {story.story.category ? (

                <span className="rounded-full bg-[#26262C] px-2 py-0.5 text-xs font-semibold text-[#ADADB8]">

                  {story.story.category}

                </span>

              ) : null}

            </div>

            <div className="flex flex-wrap items-center gap-2 text-xs text-[#7A7A85]">

              {story.origin?.streamId ? <span>Origin stream · {story.origin.vodOffsetS}s</span> : null}

              {updatedAt ? <span>Updated {formatRelativeTime(updatedAt)}</span> : null}

            </div>

          </div>

        </div>

      </header>

      <h3 className="mb-2 text-xl font-semibold text-[#F7F7F8]">{story.story.title || 'Developing story'}</h3>

      {wireFriendly ? (

        <p className="mb-4 text-xs font-semibold text-[#7A7A85]">

          {sourceCount > 0 ? `${sourceCount} source${sourceCount === 1 ? '' : 's'}` : 'Sources gathering'}

          {updatedAt ? <> · {formatRelativeTime(updatedAt)}</> : null}

        </p>

      ) : null}

      {story.story.summary && story.story.summary !== WIRE_SUMMARY_BOILERPLATE ? (

        <p className="mb-4 text-sm leading-relaxed text-[#ADADB8]">{story.story.summary}</p>

      ) : null}

      {story.origin?.quotes?.length ? (

        <div className="mb-4 rounded-xl border border-[#2A2A2E] bg-[#1B1B1F] p-3">

          <p className="mb-1 text-[11px] font-bold uppercase tracking-[0.06em] text-[#7A7A85]">Origin moment</p>

          <p className="text-sm text-[#EFEFF1]">{story.origin.quotes[0]}</p>

        </div>

      ) : null}

      {wireFriendly ? (

        showDetails ? (

          <>

            <ScoreBars scores={scores} windowScores={story.windowScores} className="mb-4" />

            <SpreadTimeline timeline={timeline} className="mb-4" />

          </>

        ) : null

      ) : (

        <>

          <ScoreBars scores={scores} windowScores={story.windowScores} className="mb-4" />

          <SpreadTimeline timeline={timeline} className="mb-4" />

        </>

      )}

      <ReceiptsRow receipts={receipts} rich className="mb-4" />

      {actionError ? <p className="mb-3 text-xs text-red-300">{actionError}</p> : null}

      <div className="flex flex-wrap gap-2">

        {wireFriendly ? (

          <button

            type="button"

            onClick={() => setShowDetails(value => !value)}

            className="rounded-lg border border-[#33333A] bg-[#1F1F23] px-4 py-2 text-sm font-semibold text-[#ADADB8] transition hover:border-[#A970FF]/40 hover:text-[#EFEFF1] focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[#A970FF]"

          >

            {showDetails ? 'Hide details' : 'Details'}

          </button>

        ) : null}

        <button

          type="button"

          onClick={() => void toggleTrack()}

          disabled={trackPending}

          className="rounded-lg border border-[#33333A] bg-[#1F1F23] px-4 py-2 text-sm font-semibold text-[#EFEFF1] transition hover:border-[#A970FF]/40 focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[#A970FF] disabled:opacity-50"

        >

          {trackPending ? 'Saving…' : tracked ? 'Untrack story' : 'Track story'}

        </button>

        {SETUP_CONTROL_TOKEN && canClip ? (

          <button

            type="button"

            onClick={() => void handleCreateClip()}

            disabled={clipPending}

            className="rounded-lg border border-[#33333A] bg-[#1F1F23] px-4 py-2 text-sm font-semibold text-[#EFEFF1] transition hover:border-[#A970FF]/40 focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[#A970FF] disabled:cursor-not-allowed disabled:opacity-50"

          >

            {clipPending ? 'Creating…' : 'Create clip'}

          </button>

        ) : null}

        {onView ? (

          <button

            type="button"

            onClick={onView}

            className="rounded-lg bg-[#9147FF] px-4 py-2 text-sm font-semibold text-white hover:bg-[#A970FF] focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[#A970FF]"

          >

            View story

          </button>

        ) : (

          <Link

            to={`/pulse-wire/${story.story.id}`}

            className="rounded-lg bg-[#9147FF] px-4 py-2 text-sm font-semibold text-white hover:bg-[#A970FF] focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[#A970FF]"

          >

            View story

          </Link>

        )}

      </div>

    </article>

  )

}
