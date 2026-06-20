import type { PulseWireSourceHealth } from '../../pulseWireApi'

const MODE_CLASS: Record<string, string> = {
  active: 'bg-[#16321F] text-[#3FCB7E]',
  link_only: 'bg-[#1A2D3F] text-[#68B7FF]',
  off: 'bg-[#26262C] text-[#ADADB8]',
  error: 'bg-[#2A1515] text-[#FF5C57]',
  degraded: 'bg-[#332713] text-[#FFD166]',
  deferred: 'bg-[#3A2A12] text-[#FFB02E]',
}

function label(value: string) {
  return value.replace(/_/g, ' ')
}

type Props = {
  sources?: PulseWireSourceHealth
  compact?: boolean
  onViewAll?: () => void
}

export default function SourceHealthPanel({ sources, compact = false, onViewAll }: Props) {
  const entries = Object.entries(sources ?? {})
  return (
    <section className={`rounded-lg border border-[#2A2A2E] bg-[#101014] ${compact ? 'p-3' : 'p-4'}`}>
      <div className="mb-3 flex items-center justify-between gap-2">
        <h3 className="text-sm font-bold text-[#F7F7F8]">Source health</h3>
        {compact && onViewAll ? (
          <button
            type="button"
            onClick={onViewAll}
            className="text-[11px] font-semibold uppercase tracking-[0.06em] text-[#A970FF] hover:text-[#CDB4FF] focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[#A970FF]"
          >
            View all
          </button>
        ) : (
          <span className="text-[11px] uppercase tracking-[0.06em] text-[#7A7A85]">{compact ? 'View all' : 'Wire modes'}</span>
        )}
      </div>
      {entries.length ? (
        <div className={compact ? 'space-y-1.5' : 'space-y-2'}>
          {entries.map(([name, source]) => (
            <div key={name} className={`rounded-lg bg-[#17171D] ${compact ? 'px-2 py-1.5' : 'p-2'}`}>
              <div className="flex items-center justify-between gap-2">
                <span className="text-xs font-semibold capitalize text-[#D6D6DE]">{label(name)}</span>
                <span className={`rounded-full px-2 py-0.5 text-[10px] font-bold uppercase ${MODE_CLASS[source.mode] ?? MODE_CLASS.off}`}>
                  {label(source.mode)}
                </span>
              </div>
              {source.last_error ? <p className="mt-1 text-[11px] text-red-300">{source.last_error}</p> : null}
              {!source.last_error && source.hint ? <p className="mt-1 text-[11px] text-[#7A7A85]">{source.hint}</p> : null}
              {source.details ? (
                <div className="mt-1.5 space-y-1">
                  {Object.entries(source.details).map(([detailName, detail]) => (
                    <div key={detailName} className="flex items-center justify-between gap-2 text-[11px]">
                      <span className="capitalize text-[#7A7A85]">{label(detailName)}</span>
                      <span className={detail.healthy ? 'text-[#3FCB7E]' : 'text-[#FFD166]'}>
                        {detail.healthy ? `${detail.last_items ?? 0} items` : (detail.last_error || 'not ready')}
                      </span>
                    </div>
                  ))}
                </div>
              ) : null}
            </div>
          ))}
        </div>
      ) : (
        <p className="text-xs text-[#7A7A85]">Source health will appear after the first ingest poll.</p>
      )}
    </section>
  )
}
