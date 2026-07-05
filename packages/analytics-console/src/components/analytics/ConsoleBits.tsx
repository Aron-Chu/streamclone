import { useEffect, useRef, useState } from 'react'
import type { AnalyticsStreamDetail, SourceStatus } from '../../apiTypes.ts'
import { emoteProviderLabel, emoteProviderTone } from '../../emoteUtils.ts'
import { sourceTone } from '../../utils/consoleFormat.ts'
import { useConsoleMotion } from '../../hooks/useConsoleMotion.ts'

export function StatCard({ label, value, tone }: { label: string; value: string; tone?: string }) {
  const { motionEnabled } = useConsoleMotion()
  const [pulse, setPulse] = useState(false)
  const prevValue = useRef(value)

  useEffect(() => {
    if (prevValue.current === value) return
    prevValue.current = value
    if (!motionEnabled) return
    setPulse(true)
    const timer = window.setTimeout(() => setPulse(false), 220)
    return () => window.clearTimeout(timer)
  }, [value, motionEnabled])

  return (
    <div className="sc-stat-card rounded border border-white/10 bg-white/[0.035] p-3" data-value-changed={pulse ? 'true' : undefined}>
      <div className="text-[11px] font-black uppercase text-zinc-500">{label}</div>
      <div className={`sc-stat-card__value mt-1 truncate text-xl font-black ${tone || 'text-white'}`}>{value}</div>
    </div>
  )
}

export function ChatCoverageBadge({ detail }: { detail?: AnalyticsStreamDetail }) {
  const pct = detail?.chatCoveragePct ?? detail?.chatCoverage?.coveragePct
  if (pct === undefined || pct <= 0) return null
  const partial = detail?.chatCoverage?.partial
  const title = partial
    ? `Chat spans ${detail?.chatCoverage?.chatSpanMinutes ?? 0} of ${detail?.chatCoverage?.streamSpanMinutes ?? 0} stream minutes — re-sync later for more`
    : 'Chat rollups cover most of the stream timeline'
  return (
    <span
      title={title}
      className={`rounded border px-2 py-1 text-[10px] font-black uppercase ${
        partial
          ? 'border-amber-400/25 bg-amber-500/10 text-amber-200'
          : 'border-emerald-400/20 bg-emerald-500/10 text-emerald-300'
      }`}
    >
      {Math.round(pct)}% chat coverage
    </span>
  )
}

const SOURCE_PILL_LABELS: Record<string, string> = {
  analytics_db: 'Tracked',
  live: 'Live',
  gql: 'Imported VOD',
  ivr: 'Legacy',
  mixed: 'Mixed',
}

function formatSourcePillLabel(source: SourceStatus): string {
  const custom = source.label?.trim()
  if (custom && !/^analytics\s*db$/i.test(custom)) return custom
  return SOURCE_PILL_LABELS[source.source] ?? source.source.replace(/_/g, ' ')
}

function formatSourcePillState(state: string): string {
  if (state === 'ready') return ''
  return state.replace(/_/g, ' ')
}

export function SourcePills({ sources }: { sources?: SourceStatus[] }) {
  if (!sources?.length) return null
  return (
    <div className="flex flex-wrap gap-2">
      {sources.map((source) => {
        const label = formatSourcePillLabel(source)
        const stateSuffix = formatSourcePillState(source.state)
        const text = stateSuffix ? `${label} ${stateSuffix}` : label
        return (
          <span
            key={`${source.source}-${source.state}-${source.label ?? ''}`}
            title={source.label}
            className={`rounded border px-2 py-1 text-[10px] font-black uppercase ${sourceTone(source.state)}`}
          >
            {text}
          </span>
        )
      })}
    </div>
  )
}

export function EmoteProviderBadge({ provider }: { provider?: string }) {
  if (!provider) return null
  return (
    <span className={`rounded border px-1.5 py-0.5 text-[9px] font-black uppercase ${emoteProviderTone(provider)}`}>
      {emoteProviderLabel(provider)}
    </span>
  )
}
