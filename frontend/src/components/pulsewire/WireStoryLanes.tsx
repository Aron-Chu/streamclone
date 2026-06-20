import type { PulseWireSourceHealth, PulseWireStory } from '../../pulseWireApi'
import { toWireStoryView, type ReaderStatus } from '../../utils/pulseWireStoryView'
import WireStoryCard from './WireStoryCard'

type Props = {
  stories: PulseWireStory[]
  sourceHealth?: PulseWireSourceHealth
  analystMode?: boolean
  detailSearch?: string
}

type Lane = {
  id: string
  title: string
  subtitle: string
  statuses: ReaderStatus[]
}

const LANES: Lane[] = [
  {
    id: 'developing',
    title: 'Developing now',
    subtitle: 'Useful single-source stories that still need corroboration.',
    statuses: ['developing', 'active', 'unverified', 'insufficient_data'],
  },
  {
    id: 'confirmed',
    title: 'Confirmed across sources',
    subtitle: 'Stories with multiple attached signals or corroborated confidence.',
    statuses: ['corroborated', 'settled'],
  },
  {
    id: 'needs_origin',
    title: 'Needs origin',
    subtitle: 'Stories spreading outside Twitch before a Pulse or Twitch origin is found.',
    statuses: ['needs_origin'],
  },
]

function sortStories(stories: PulseWireStory[]) {
  return [...stories].sort((a, b) => {
    const rank = (b.windowScores?.rankScore ?? b.scores.trend ?? 0) - (a.windowScores?.rankScore ?? a.scores.trend ?? 0)
    if (rank !== 0) return rank
    const freshness = Date.parse(b.lastSeenAt ?? b.story.updatedAt ?? '') - Date.parse(a.lastSeenAt ?? a.story.updatedAt ?? '')
    if (freshness !== 0) return freshness
    return confidenceWeight(b) - confidenceWeight(a)
  })
}

function confidenceWeight(story: PulseWireStory) {
  switch (story.scores.confidence) {
    case 'widely_reported':
      return 3
    case 'corroborated':
      return 2
    case 'single_source':
      return 1
    default:
      return 0
  }
}

export default function WireStoryLanes({ stories, sourceHealth, analystMode = false, detailSearch = '' }: Props) {
  const mapped = stories.map(story => ({ story, view: toWireStoryView(story, sourceHealth, { analystMode }) }))
  const lanes = LANES.map(lane => ({
    ...lane,
    items: sortStories(mapped.filter(item => lane.statuses.includes(item.view.readerStatus)).map(item => item.story)),
  })).filter(lane => lane.items.length > 0)

  if (!lanes.length) {
    return (
      <section className="rounded-2xl border border-[#2A2A2E] bg-[#121217] p-5 text-sm text-[#ADADB8]">
        No cross-source stories yet. Showing single-source developments here once evidence arrives.
      </section>
    )
  }

  return (
    <div className="grid gap-3 xl:grid-cols-3">
      {lanes.map(lane => (
        <section key={lane.id} className="rounded-lg border border-[#24242B] bg-[#101014] p-3">
          <div className="mb-3 flex items-start justify-between gap-2">
            <div className="min-w-0">
              <h2 className="text-sm font-black text-[#F7F7F8]">{lane.title}</h2>
              <p className="mt-1 line-clamp-2 text-[11px] leading-relaxed text-[#7A7A85]">{lane.subtitle}</p>
            </div>
            <span className="rounded bg-[#1B1B21] px-2 py-0.5 text-[10px] font-bold text-[#A970FF]">{lane.items.length}</span>
          </div>
          <div className="space-y-3">
            {lane.items.slice(0, 3).map(story => (
              <WireStoryCard key={story.story.id} story={story} sourceHealth={sourceHealth} analystMode={analystMode} detailSearch={detailSearch} compact />
            ))}
          </div>
        </section>
      ))}
    </div>
  )
}
