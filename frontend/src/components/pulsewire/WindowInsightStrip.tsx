import { useEffect, useMemo, useState } from 'react'
import { Link } from 'react-router-dom'
import {
  fetchPulseWireEdition,
  PulseWireApiError,
  type PulseWireEditionResponse,
  type PulseWireEditionSection,
  type PulseWireRisingStreamer,
  type PulseWireWindow,
} from '../../pulseWireApi'
import { deltaTone, formatDeltaPct, formatViewers, windowShortLabel } from '../../utils/pulseWireFormat'
import NewsSection from './NewsSection'
import StoryCompactCard from './StoryCompactCard'

type Props = {
  window: PulseWireWindow
  refreshKey?: number
  detailSearch?: string
  className?: string
}

function KpiTile({ label, value, hint }: { label: string; value: string; hint?: string }) {
  return (
    <div className="min-w-[120px] flex-1 rounded-xl border border-[#26262C] bg-[#161619] px-4 py-3">
      <p className="text-[11px] font-bold uppercase tracking-[0.06em] text-[#7A7A85]">{label}</p>
      <p className="mt-1 text-xl font-black text-[#F7F7F8]">{value}</p>
      {hint ? <p className="text-[11px] text-[#7A7A85]">{hint}</p> : null}
    </div>
  )
}

function MoverChip({ mover, window }: { mover: PulseWireRisingStreamer; window: PulseWireWindow }) {
  const name = mover.displayName || mover.login
  return (
    <Link
      to={`/pulse-wire/streamer/${encodeURIComponent(mover.login)}?window=${window}`}
      className="group inline-flex min-w-[180px] flex-1 items-center gap-3 rounded-xl border border-[#26262C] bg-[#161619] px-4 py-3 transition hover:border-[#A970FF]/50 focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[#A970FF]"
    >
      {mover.avatarUrl ? (
        <img src={mover.avatarUrl} alt="" className="h-9 w-9 shrink-0 rounded-full object-cover" loading="lazy" />
      ) : (
        <span className="grid h-9 w-9 shrink-0 place-items-center rounded-full bg-[#1B1B1F] text-sm font-bold text-[#ADADB8]">
          {name.slice(0, 1).toUpperCase()}
        </span>
      )}
      <span className="min-w-0 flex-1">
        <span className="block truncate text-sm font-bold text-[#F7F7F8] group-hover:text-white">{name}</span>
        <span className={`text-xs font-semibold ${deltaTone(mover.viewerDeltaPct)}`}>
          {formatDeltaPct(mover.viewerDeltaPct)} · {formatViewers(mover.viewersNow)} viewers
        </span>
      </span>
    </Link>
  )
}

function SectionBody({
  section,
  detailSearch,
  window,
}: {
  section: PulseWireEditionSection
  detailSearch: string
  window: PulseWireWindow
}) {
  if (section.kpis?.length) {
    return (
      <div className="flex flex-wrap gap-3">
        {section.kpis.map(kpi => (
          <KpiTile key={kpi.label} label={kpi.label} value={kpi.value} hint={kpi.hint} />
        ))}
      </div>
    )
  }
  if (section.movers?.length) {
    return (
      <div className="flex flex-wrap gap-3">
        {section.movers.map(mover => (
          <MoverChip key={mover.login} mover={mover} window={window} />
        ))}
      </div>
    )
  }
  if (section.stories?.length) {
    return (
      <div className="space-y-3">
        {section.stories.map(story => (
          <StoryCompactCard
            key={story.story.id}
            story={story}
            variant="editorial"
            detailSearch={detailSearch}
          />
        ))}
      </div>
    )
  }
  return null
}

export default function WindowInsightStrip({
  window,
  refreshKey = 0,
  detailSearch = '',
  className = '',
}: Props) {
  const [edition, setEdition] = useState<PulseWireEditionResponse | null>(null)
  const [loading, setLoading] = useState(true)
  const [disabled, setDisabled] = useState(false)

  useEffect(() => {
    let cancelled = false
    setLoading(true)
    fetchPulseWireEdition(window)
      .then(res => {
        if (cancelled) return
        setEdition(res)
        setDisabled(false)
      })
      .catch(err => {
        if (cancelled) return
        setEdition(null)
        if (err instanceof PulseWireApiError && err.code === 'pulse_wire_disabled') {
          setDisabled(true)
        }
      })
      .finally(() => {
        if (!cancelled) setLoading(false)
      })
    return () => {
      cancelled = true
    }
  }, [window, refreshKey])

  const headlineKpis = useMemo(() => {
    if (!edition) return []
    const banCount = edition.bansOfTheDay?.length ?? edition.bans?.length
    return [
      { label: 'Live channels', value: formatViewers(edition.totalLive), hint: 'sampled' },
      { label: 'Total viewers', value: formatViewers(edition.totalViewers) },
      { label: 'New entrants', value: edition.newEntrants?.length != null ? String(edition.newEntrants.length) : '—' },
      { label: 'Bans', value: banCount != null ? String(banCount) : '—' },
    ]
  }, [edition])

  const sections = edition?.sections ?? []

  if (disabled) return null

  return (
    <div className={`space-y-6 ${className}`} aria-label={`${windowShortLabel(window)} insight`}>
      {loading ? (
        <div className="flex flex-wrap gap-3">
          {Array.from({ length: 4 }).map((_, index) => (
            <div key={index} className="h-[74px] min-w-[120px] flex-1 animate-pulse rounded-xl border border-[#26262C] bg-[#161619]" />
          ))}
        </div>
      ) : (
        <>
          {headlineKpis.length ? (
            <div className="flex flex-wrap gap-3">
              {headlineKpis.map(kpi => (
                <KpiTile key={kpi.label} {...kpi} />
              ))}
            </div>
          ) : null}
          {sections.map(section => {
            const hasContent = Boolean(
              section.kpis?.length || section.movers?.length || section.stories?.length,
            )
            return (
              <NewsSection
                key={section.id}
                title={section.title}
                subtitle={section.subtitle}
                isEmpty={!hasContent}
                emptyMessage={`No ${section.title.toLowerCase()} in ${windowShortLabel(window)} yet.`}
              >
                <SectionBody section={section} detailSearch={detailSearch} window={window} />
              </NewsSection>
            )
          })}
          {!sections.length ? (
            <p className="rounded-xl border border-[#2A2A2E] bg-[#121217] p-4 text-xs text-[#7A7A85]">
              Window insights are gathering data. Edition sections populate after ingest and directory sampling.
            </p>
          ) : null}
        </>
      )}
    </div>
  )
}
