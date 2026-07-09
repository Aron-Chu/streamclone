import { useMemo } from 'react'
import { formatHeatOffset } from '@streamclone/pulse-core'
import type { AnalyticsMinuteRollup, AnalyticsTopEmote, PulseRecapEmote, PulseStreamRecap } from '../../apiTypes.ts'
import {
  enrichRecapEmotesFromCatalog,
  resolveRecapBurstHighlight,
  resolveRecapDisplayEmotes,
} from '../../utils/recapEmoteEnrich.ts'
import { count, getEmoteImageUrl } from '../../utils/consoleFormat.ts'
import { ConsoleEmoteImg } from './ConsoleEmoteImg.tsx'
import { EmoteProviderBadge } from './ConsoleBits.tsx'

export function StreamRecapPanel({
  recap,
  topEmotesCatalog,
  rollups,
  streamStartedAt,
  onJumpToOffset,
  onPreviewOffset,
}: {
  recap: PulseStreamRecap
  topEmotesCatalog?: AnalyticsTopEmote[]
  rollups?: AnalyticsMinuteRollup[]
  streamStartedAt?: string
  onJumpToOffset?: (offsetSeconds: number) => void
  onPreviewOffset?: (offsetSeconds: number | null) => void
}) {
  const topMoments = recap.topMoments ?? []
  const topEmotes = useMemo(
    () => resolveRecapDisplayEmotes(recap.topEmotes ?? [], topEmotesCatalog, 10),
    [recap.topEmotes, topEmotesCatalog],
  )
  const clipCandidates = recap.clipCandidates ?? []
  const topMoment = topMoments[0]
  const hasHeadlineMetric =
    (recap.totalMessages ?? 0) > 0 || (recap.peakChatPerMin ?? 0) > 0
  const burstHighlight = useMemo(() => {
    if (!recap.funniestEmoteBurst) return null
    return resolveRecapBurstHighlight({
      burst: recap.funniestEmoteBurst,
      rollups,
      streamStartedAt,
      topEmotesCatalog,
    })
  }, [recap.funniestEmoteBurst, rollups, streamStartedAt, topEmotesCatalog])
  const burstEmote = burstHighlight?.emote

  if (
    !hasHeadlineMetric
    && topMoments.length === 0
    && topEmotes.length === 0
    && !recap.funniestEmoteBurst
  ) {
    return null
  }

  return (
    <section className="w-full rounded border border-white/[0.07] bg-white/[0.025] p-3">
      <div className="mb-2 flex items-start justify-between gap-3">
        <div>
          <h3 className="text-xs font-black uppercase text-zinc-400">Stream Recap</h3>
          <p className="text-[10px] font-semibold leading-snug text-zinc-500">
            Session highlights from Pulse rollups
          </p>
        </div>
        {topMoment ? (
          <span className="shrink-0 rounded border border-amber-400/20 bg-amber-500/10 px-2 py-1 text-[10px] font-black text-amber-200">
            Top {topMoment.score}
          </span>
        ) : null}
      </div>
      {hasHeadlineMetric ? (
        <div className="grid grid-cols-2 gap-2 text-xs">
          <div className="min-h-[3.25rem] rounded border border-white/5 bg-white/[0.022] p-2 leading-snug">
            <div className="text-[10px] font-black uppercase text-zinc-500">Messages</div>
            <div className="mt-1 font-black text-zinc-200">{count(recap.totalMessages)}</div>
          </div>
          <div className="min-h-[3.25rem] rounded border border-white/5 bg-white/[0.022] p-2 leading-snug">
            <div className="text-[10px] font-black uppercase text-zinc-500">Peak Chat</div>
            <div className="mt-1 font-black text-zinc-200">{count(recap.peakChatPerMin)}/min</div>
          </div>
        </div>
      ) : null}
      {recap.biggestChatSpike || recap.funniestEmoteBurst ? (
        <div className="mt-2 grid gap-1.5 text-xs leading-snug">
          {recap.biggestChatSpike ? (
            <div className="rounded border border-white/5 bg-white/[0.022] px-2 py-1.5 font-semibold text-zinc-400">
              Biggest spike at{' '}
              <strong className="text-cyan-200">
                {formatHeatOffset(recap.biggestChatSpike.offsetSeconds)}
              </strong>{' '}
              ({count(recap.biggestChatSpike.chatPerMin)}/min)
            </div>
          ) : null}
          {recap.funniestEmoteBurst ? (
            <div className="flex flex-wrap items-center gap-2 rounded border border-white/5 bg-white/[0.022] px-2 py-1.5 font-semibold text-zinc-400">
              {burstEmote ? <RecapEmoteChip emote={burstEmote} /> : null}
              <span className="min-w-0">
                Emote burst at{' '}
                <strong className="text-cyan-200">
                  {formatHeatOffset(burstHighlight?.offsetSeconds ?? recap.funniestEmoteBurst.offsetSeconds)}
                </strong>
                {recap.funniestEmoteBurst.code ? ` · ${recap.funniestEmoteBurst.code}` : ''} (
                {count(recap.funniestEmoteBurst.count)})
              </span>
            </div>
          ) : null}
        </div>
      ) : null}
      {topEmotes.length > 0 ? (
        <div className="mt-3 overflow-hidden rounded border border-white/[0.07]">
          <div className="grid grid-cols-[minmax(0,1fr)_auto_auto] gap-2 border-b border-white/[0.07] bg-white/[0.015] px-2 py-1.5 text-[10px] font-black uppercase text-zinc-500">
            <span>Top 7TV</span>
            <span>Provider</span>
            <span className="text-right">Uses</span>
          </div>
          <ul>
            {topEmotes.slice(0, 5).map((emote) => (
              <li key={emote.code}>
                <RecapEmoteRow emote={emote} />
              </li>
            ))}
          </ul>
        </div>
      ) : null}
      {topMoment && onJumpToOffset ? (
        <button
          type="button"
          onClick={() => onJumpToOffset(topMoment.offsetSeconds)}
          onMouseEnter={() => onPreviewOffset?.(topMoment.offsetSeconds)}
          onMouseLeave={() => onPreviewOffset?.(null)}
          className="mt-3 w-full rounded border border-white/[0.07] bg-white/[0.022] px-2 py-1.5 text-left text-[10px] font-semibold leading-snug text-zinc-400 transition hover:bg-white/[0.04]"
        >
          Top moment at{' '}
          <strong className="font-black text-amber-200">{formatHeatOffset(topMoment.offsetSeconds)}</strong>
          {' · '}
          score <strong className="font-black text-amber-200">{topMoment.score}</strong>
          <span className="text-zinc-500"> — jump on chart</span>
        </button>
      ) : null}
      {clipCandidates.length > 0 ? (
        <p className="mt-2 text-[10px] font-semibold leading-snug text-zinc-500">
          {clipCandidates.length} clip candidate{clipCandidates.length === 1 ? '' : 's'} from Pulse scores
          {' · '}
          see Moments tab for the full ranked list
        </p>
      ) : null}
    </section>
  )
}

function RecapEmoteChip({ emote }: { emote: PulseRecapEmote }) {
  const imageUrl = getEmoteImageUrl({
    provider: emote.provider,
    id: emote.id,
    imageUrl: emote.imageUrl,
  })
  return (
    <span className="inline-flex shrink-0 items-center gap-1.5 rounded border border-white/[0.07] bg-white/[0.028] px-1.5 py-0.5 text-[10px] font-bold text-zinc-400">
      <span className="grid h-5 w-5 shrink-0 place-items-center rounded bg-black/30 p-0.5">
        <ConsoleEmoteImg
          src={imageUrl}
          name={emote.code}
          className="max-h-full max-w-full object-contain"
          fallbackClassName="inline-flex h-3.5 w-3.5 items-center justify-center rounded bg-white/[0.06] text-[8px] font-black text-zinc-500"
        />
      </span>
      <span className="max-w-[6rem] truncate">{emote.code}</span>
      <span className="font-mono text-zinc-500">{count(emote.count)}</span>
    </span>
  )
}

function RecapEmoteRow({ emote }: { emote: PulseRecapEmote }) {
  const imageUrl = getEmoteImageUrl({
    provider: emote.provider,
    id: emote.id,
    imageUrl: emote.imageUrl,
  })
  const provider = emote.provider ?? 'seventv'
  return (
    <div className="grid grid-cols-[minmax(0,1fr)_auto_auto] items-center gap-2 border-b border-white/5 px-2 py-1.5 text-xs last:border-b-0">
      <span className="flex min-w-0 items-center gap-1.5">
        <span className="grid h-6 w-6 shrink-0 place-items-center rounded bg-black/30 p-0.5">
          <ConsoleEmoteImg
            src={imageUrl}
            name={emote.code}
            className="max-h-full max-w-full object-contain"
            fallbackClassName="inline-flex h-4 w-4 items-center justify-center rounded bg-white/[0.06] text-[8px] font-black text-zinc-500"
          />
        </span>
        <span className="truncate font-semibold text-zinc-300" title={emote.code}>
          {emote.code}
        </span>
      </span>
      <span className="flex shrink-0 items-center self-center">
        <EmoteProviderBadge provider={provider} />
      </span>
      <span className="shrink-0 text-right font-mono text-[11px] font-semibold tabular-nums text-zinc-400">
        {count(emote.count)}
      </span>
    </div>
  )
}
