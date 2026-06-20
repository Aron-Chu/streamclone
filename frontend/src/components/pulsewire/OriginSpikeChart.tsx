import type { PulseWireStory } from '../../pulseWireApi'

type Origin = NonNullable<PulseWireStory['origin']>

function clampPercent(value: number) {
  if (!Number.isFinite(value)) return 0
  return Math.max(8, Math.min(100, Math.round(value)))
}

function offsetLabel(relativeS: number) {
  if (!Number.isFinite(relativeS) || relativeS === 0) return '0s'
  const abs = Math.abs(Math.round(relativeS))
  const minutes = Math.floor(abs / 60)
  const seconds = abs % 60
  const value = minutes > 0 && seconds === 0 ? `${minutes}m` : `${abs}s`
  return `${relativeS > 0 ? '+' : '-'}${value}`
}

export default function OriginSpikeChart({ origin, compact = false }: { origin?: Origin; compact?: boolean }) {
  if (!origin) return null

  const points = (origin.originSpikePoints ?? [])
    .filter(point => Number.isFinite(point.relativeS))
    .sort((a, b) => a.relativeS - b.relativeS)
  if (!points.length) return null

  const maxChat = Math.max(...points.map(point => point.chatCount ?? 0), 1)
  const maxEmotes = Math.max(...points.map(point => point.totalEmoteCount ?? 0), 1)
  const maxViewers = Math.max(...points.map(point => point.viewerMax ?? 0), 1)
  const visiblePoints = compact && points.length > 7
    ? points.filter((_, index) => index % Math.ceil(points.length / 5) === 0).slice(0, 5)
    : points

  return (
    <div className={`${compact ? 'mt-3' : ''} rounded-lg border border-[#26352C] bg-[#111B15] p-3`} aria-label="Chat activity spike chart">
      <div className="mb-2 flex items-center justify-between gap-2">
        <p className="text-[11px] font-black uppercase tracking-[0.06em] text-[#A8F0C2]">Chat activity spike</p>
        <span className="text-[10px] font-semibold text-[#6FB986]">{points.length} real rollups</span>
      </div>
      <div className="grid h-28 grid-flow-col items-end gap-2" role="img" aria-label="Real Analytics chat, emote, and viewer rollups around the matched origin timestamp">
        {visiblePoints.map(point => {
          const chatHeight = clampPercent(((point.chatCount ?? 0) / maxChat) * 100)
          const emoteHeight = clampPercent(((point.totalEmoteCount ?? 0) / maxEmotes) * 100)
          const viewerHeight = clampPercent(((point.viewerMax ?? 0) / maxViewers) * 100)
          const isOrigin = Math.abs(point.relativeS) < 30
          return (
          <div key={`${point.offsetS}-${point.relativeS}`} className="flex h-full min-w-0 flex-col justify-end gap-1">
            <span className={`text-center text-[10px] font-bold ${isOrigin ? 'text-[#FFFFFF]' : 'text-[#D6F7DF]'}`}>
              {point.chatCount}
            </span>
            <div className={`flex h-16 items-end gap-0.5 rounded ${isOrigin ? 'bg-[#203724] ring-1 ring-[#72F0A3]/50' : 'bg-[#17221B]'}`}>
              <div
                className="w-full rounded-t bg-[#72F0A3]"
                title={`${point.chatCount} chat messages`}
                style={{ height: `${chatHeight}%` }}
              />
              <div
                className="w-full rounded-t bg-[#A970FF]"
                title={`${point.totalEmoteCount} emotes`}
                style={{ height: `${emoteHeight}%` }}
              />
              <div
                className="w-full rounded-t bg-[#56A8FF]"
                title={`${point.viewerMax ?? 0} peak viewers`}
                style={{ height: `${viewerHeight}%` }}
              />
            </div>
            <span className={`truncate text-center text-[10px] font-semibold ${isOrigin ? 'text-[#A8F0C2]' : 'text-[#83C999]'}`}>
              {offsetLabel(point.relativeS)}
            </span>
          </div>
        )})}
      </div>
      <div className="mt-2 flex flex-wrap gap-2 text-[10px] font-semibold text-[#83C999]">
        <span><span className="mr-1 inline-block h-2 w-2 rounded-sm bg-[#72F0A3]" />chat</span>
        <span><span className="mr-1 inline-block h-2 w-2 rounded-sm bg-[#A970FF]" />emotes</span>
        <span><span className="mr-1 inline-block h-2 w-2 rounded-sm bg-[#56A8FF]" />viewers</span>
      </div>
    </div>
  )
}
