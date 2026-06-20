import { platformEntries, compactPlatformEntries, type PlatformKey, type PlatformPresenceState, type WireStoryView } from '../../utils/pulseWireStoryView'

const PLATFORM_META: Record<PlatformKey, { label: string; dot: string; short: string }> = {
  pulse: { label: 'Pulse', dot: '#A970FF', short: 'P' },
  twitch: { label: 'Twitch', dot: '#9147FF', short: 'T' },
  reddit: { label: 'Reddit', dot: '#FF4500', short: 'R' },
  youtube: { label: 'YouTube', dot: '#FF0000', short: 'YT' },
  x: { label: 'X', dot: '#1D9BF0', short: 'X' },
  tiktok: { label: 'TikTok', dot: '#00D7B0', short: 'TT' },
  bans: { label: 'Bans', dot: '#FF5C57', short: 'B' },
}

const STATE_META: Record<PlatformPresenceState, { label: string; className: string }> = {
  matched: { label: 'Matched', className: 'border-[#3FCB7E]/35 bg-[#16321F]/80 text-[#DDF7E8]' },
  linked: { label: 'Linked', className: 'border-[#A970FF]/30 bg-[#1B1B1F] text-[#EFEFF1]' },
  missing: { label: 'Missing', className: 'border-[#3A3A40] bg-[#121217] text-[#ADADB8]' },
  pending: { label: 'Pending', className: 'border-[#3A2A12] bg-[#2A2212] text-[#FFCF7A]' },
  disabled: { label: 'Off', className: 'border-[#26262C] bg-[#161619] text-[#7A7A85]' },
  degraded: { label: 'Degraded', className: 'border-[#FF5C57]/30 bg-[#2A1515] text-[#FFB8B5]' },
  not_applicable: { label: 'N/A', className: 'border-[#26262C] bg-[#161619] text-[#7A7A85]' },
}

type Props = {
  view: WireStoryView
  compact?: boolean
  className?: string
}

export function EvidenceSpreadStrip({ view, compact = false, className = '' }: Props) {
  const entries = compact ? compactPlatformEntries(view) : platformEntries(view)
  return (
    <div className={`flex flex-wrap gap-2 ${className}`} aria-label="Evidence spread">
      {entries.map(item => {
        const meta = PLATFORM_META[item.platform]
        const state = STATE_META[item.state]
        return (
          <span
            key={item.platform}
            title={`${meta.label}: ${item.label}`}
            className={`inline-flex items-center gap-1.5 rounded-full border font-medium ${state.className} ${compact ? 'px-2 py-1 text-[11px]' : 'px-2.5 py-1.5 text-xs'}`}
          >
            <span className="grid h-4 min-w-4 place-items-center rounded bg-black/20 px-1 text-[9px] font-black" style={{ color: meta.dot }}>
              {meta.short}
            </span>
            <span>{meta.label}</span>
            <span className="text-[#7A7A85]">{item.count > 0 ? item.count : state.label}</span>
          </span>
        )
      })}
    </div>
  )
}

export function EvidenceSpreadCards({ view, className = '' }: Props) {
  return (
    <div className={`grid gap-2 sm:grid-cols-2 lg:grid-cols-3 2xl:grid-cols-6 ${className}`} aria-label="Evidence spread cards">
      {platformEntries(view).map(item => {
        const meta = PLATFORM_META[item.platform]
        const state = STATE_META[item.state]
        const body = (
          <>
            <div className="mb-2 flex items-start justify-between gap-2">
              <span className="inline-flex min-w-0 items-center gap-2 text-xs font-black text-[#F7F7F8]">
                <span className="grid h-7 min-w-7 place-items-center rounded bg-[#222229] px-1 text-[11px]" style={{ color: meta.dot }}>
                  {meta.short}
                </span>
                <span className="truncate">{meta.label}</span>
              </span>
              <span className={`rounded-full border px-2 py-0.5 text-[10px] font-bold uppercase ${state.className}`}>
                {state.label}
              </span>
            </div>
            <p className="line-clamp-2 min-h-[34px] text-xs font-semibold leading-snug text-[#D6D6DE]">{item.label}</p>
            <p className="mt-2 text-[11px] text-[#7A7A85]">
              {item.count > 0 ? `${item.count} attached` : item.role ? `${item.role} signal` : 'No evidence'}
              {item.metricLabel ? ` - ${item.metricLabel}` : ''}
            </p>
            {(item.platform === 'x' || item.platform === 'tiktok') && item.state !== 'linked' ? (
              <p className="mt-2 text-[11px] text-[#7A7A85]">Link-only unless a URL is attached.</p>
            ) : null}
          </>
        )
        if (item.href) {
          return (
            <a
              key={item.platform}
              href={item.href}
              target="_blank"
              rel="noopener noreferrer"
              className="rounded-lg border border-[#2A2A2E] bg-[#15151B] p-3 transition hover:border-[#A970FF]/40 focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[#A970FF]"
            >
              {body}
            </a>
          )
        }
        return (
          <div key={item.platform} className="rounded-lg border border-[#2A2A2E] bg-[#15151B] p-3">
            {body}
          </div>
        )
      })}
    </div>
  )
}
