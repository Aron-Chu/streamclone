import type { PulseWireScores, PulseWireWindowScores } from '../../pulseWireApi'

const CONF_LABELS: Record<string, string> = {
  single_source: 'Single source',
  corroborated: 'Corroborated',
  widely_reported: 'Widely reported',
}

type Props = {
  scores: PulseWireScores
  windowScores?: PulseWireWindowScores
  compact?: boolean
  className?: string
}

function barWidth(value?: number | null): number {
  if (value == null || !Number.isFinite(value)) return 0
  return Math.max(4, Math.min(100, Math.round(value)))
}

function ScoreBar({
  label,
  value,
  tone = '#9147FF',
}: {
  label: string
  value?: number | null
  tone?: string
}) {
  const width = barWidth(value)
  return (
    <div>
      <div className="mb-1 flex items-center justify-between gap-2 text-[11px] font-semibold text-[#ADADB8]">
        <span>{label}</span>
        <span className="text-[#EFEFF1]">{value != null ? Math.round(value) : '—'}</span>
      </div>
      <div className="h-1.5 overflow-hidden rounded-full bg-[#26262C]">
        <div
          className="h-full rounded-full transition-all"
          style={{ width: `${width}%`, backgroundColor: tone }}
        />
      </div>
    </div>
  )
}

function empty(label: string) {
  return <span className="text-xs text-[#7A7A85]">{label} — gathering data</span>
}

export default function ScoreBars({ scores, windowScores, compact, className = '' }: Props) {
  const confLabel = scores.confidence ? CONF_LABELS[scores.confidence] ?? scores.confidence : null

  if (windowScores) {
    const rankReady = windowScores.rankScore != null && windowScores.rankScore > 0
    const rankLabel = rankReady ? String(Math.round(windowScores.rankScore!)) : '—'
    const bars = [
      { label: 'Velocity', value: windowScores.velocityScore, tone: '#FFB02E' },
      { label: 'Credibility', value: windowScores.credibilityScore, tone: '#3FCB7E' },
      { label: 'Impact', value: windowScores.impactScore, tone: '#FF5C57' },
      { label: 'Momentum', value: windowScores.momentumScore, tone: '#A970FF' },
      { label: 'Freshness', value: windowScores.freshnessScore, tone: '#1D9BF0' },
    ]
    if (compact) {
      const rank = rankReady ? windowScores.rankScore : scores.trend
      return (
        <p className={`text-xs text-[#7A7A85] ${className}`}>
          Rank {rank != null && rank > 0 ? Math.round(rank) : '—'}
          {' · '}
          {windowScores.sourceCount != null && windowScores.sourceCount > 0
            ? `${windowScores.sourceCount} sources`
            : 'Sources gathering'}
          {' · '}
          {confLabel ?? 'Confidence n/a'}
        </p>
      )
    }
    return (
      <div className={`space-y-3 ${className}`}>
        <div className="flex flex-wrap items-center gap-3 text-[11px] font-semibold text-[#7A7A85]">
          <span className="rounded-full border border-[#A970FF]/30 bg-[#9147FF]/10 px-2 py-0.5 text-[#A970FF]">
            Rank {rankLabel}
          </span>
          {windowScores.evidenceCount != null ? <span>{windowScores.evidenceCount} evidence</span> : null}
          {windowScores.sourceCount != null ? <span>{windowScores.sourceCount} sources</span> : null}
          {confLabel ? <span className="text-[#3FCB7E]">{confLabel}</span> : null}
        </div>
        <div className="grid gap-3 sm:grid-cols-2">
          {bars.map(bar => (
            <ScoreBar key={bar.label} {...bar} />
          ))}
        </div>
      </div>
    )
  }

  if (compact) {
    return (
      <p className={`text-xs text-[#7A7A85] ${className}`}>
        {scores.trend != null ? `Trend ${Math.round(scores.trend)}` : 'Trend gathering data'}
        {' · '}
        {scores.volatility != null ? `Vol ${Math.round(scores.volatility)}` : 'Volatility n/a'}
        {' · '}
        {confLabel ?? 'Confidence n/a'}
      </p>
    )
  }

  return (
    <div className={`grid gap-2 sm:grid-cols-3 ${className}`}>
      <div className="rounded-lg bg-[#1B1B1F] p-3">
        <p className="text-[11px] font-bold uppercase tracking-wider text-[#7A7A85]">Trend</p>
        {scores.trend != null ? (
          <>
            <p className="text-xl font-bold text-[#F7F7F8]">{Math.round(scores.trend)}</p>
            <ScoreBar label="" value={scores.trend} tone="#9147FF" />
          </>
        ) : empty('Trend')}
      </div>
      <div className="rounded-lg bg-[#1B1B1F] p-3">
        <p className="text-[11px] font-bold uppercase tracking-wider text-[#7A7A85]">Volatility</p>
        {scores.volatility != null ? (
          <>
            <p className="text-xl font-bold text-[#FFB02E]">{Math.round(scores.volatility)}</p>
            <ScoreBar label="" value={scores.volatility} tone="#FFB02E" />
          </>
        ) : empty('Volatility')}
      </div>
      <div className="rounded-lg bg-[#1B1F1F] p-3">
        <p className="text-[11px] font-bold uppercase tracking-wider text-[#7A7A85]">Confidence</p>
        {confLabel ? <p className="text-sm font-semibold text-[#3FCB7E]">{confLabel}</p> : empty('Confidence')}
      </div>
    </div>
  )
}
