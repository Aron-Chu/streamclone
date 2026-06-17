import { Link } from 'react-router-dom'

import type {
  AnalyticsStreamDetail,
  AnalyticsTopEmote,
  ChannelInsights,
  ClipCard,
  InsightCard,
} from '../../api.ts'
import { normalizeBrowserOriginUrl } from '../../config.ts'
import { useOptionalServices } from '../../hooks/useOptionalServices.ts'
import { resolveEmoteImageUrl } from '../../utils/emoteImageUrl.ts'
import {
  analyticsNotTrackedMessage,
  isLsfPending,
  isLsfWarming,
  summarizeLsfEmptyState,
} from '../../utils/pulseEmptyState.ts'
import { LiveStatsBand } from '../analytics/LiveStatsBand.tsx'
import TrackAnalyticsToggle from './TrackAnalyticsToggle.tsx'

function fullCount(value: number | null | undefined) {
  if (value == null || Number.isNaN(value)) return '—'
  return new Intl.NumberFormat(undefined, { notation: value >= 10_000 ? 'compact' : 'standard', maximumFractionDigits: 1 }).format(value)
}

function formatClipDuration(seconds?: number) {
  if (seconds == null || !Number.isFinite(seconds)) return null
  const total = Math.max(0, Math.floor(seconds))
  const mm = Math.floor(total / 60)
  const ss = total % 60
  return `${mm}:${ss.toString().padStart(2, '0')}`
}

function emoteImageUrl(emote: AnalyticsTopEmote): string | undefined {
  const url = resolveEmoteImageUrl({
    provider: emote.provider,
    id: emote.id,
    imageUrl: emote.imageUrl,
    scale: '2x',
  })
  return url ? normalizeBrowserOriginUrl(url, ['/emotes/']) : undefined
}

function PulseLsfLoadingSkeleton({ subtitle }: { subtitle: string }) {
  return (
    <div className="rounded-lg border border-violet-400/20 bg-white/[0.035] px-4 py-5">
      <div className="flex items-center justify-center gap-2">
        <span className="relative flex h-2.5 w-2.5">
          <span className="absolute inline-flex h-full w-full animate-ping rounded-full bg-violet-400/70 opacity-75" />
          <span className="relative inline-flex h-2.5 w-2.5 rounded-full bg-violet-500" />
        </span>
        <div className="text-sm font-black text-violet-200">LSF threads loading</div>
      </div>
      <p className="mt-2 text-center text-xs font-semibold leading-relaxed text-zinc-500">{subtitle}</p>
      <div className="mt-4 space-y-3">
        {[0, 1].map(key => (
          <div key={key} className="flex gap-3 rounded-lg border border-white/5 bg-black/20 p-3">
            <div className="h-14 w-14 shrink-0 animate-pulse rounded bg-gradient-to-br from-violet-500/20 to-zinc-800/80" />
            <div className="min-w-0 flex-1 space-y-2">
              <div className="h-3 w-16 animate-pulse rounded bg-violet-500/20" />
              <div className="h-4 w-full animate-pulse rounded bg-white/10" />
              <div className="h-4 w-4/5 animate-pulse rounded bg-white/10" />
              <div className="h-3 w-24 animate-pulse rounded bg-white/5" />
            </div>
          </div>
        ))}
      </div>
      <p className="mt-3 text-center text-[11px] font-semibold text-zinc-600">Checking again automatically…</p>
    </div>
  )
}

function PulseLsfCard({ post }: { post: InsightCard }) {
  const threadUrl = post.permalink || post.url
  const subreddit = post.subreddit ? `r/${post.subreddit.replace(/^r\//, '')}` : 'Reddit'
  return (
    <article className="rounded-lg border border-white/10 bg-white/[0.035] p-3">
      <div className="flex gap-3">
        {post.thumbnail ? (
          <img
            src={post.thumbnail}
            alt=""
            className="h-14 w-14 shrink-0 rounded object-cover"
            loading="lazy"
          />
        ) : null}
        <div className="min-w-0 flex-1">
          <div className="text-[10px] font-black uppercase tracking-wide text-violet-300">{subreddit}</div>
          <h4 className="mt-1 line-clamp-2 text-sm font-black leading-5 text-white">{post.title}</h4>
          <div className="mt-2 flex flex-wrap items-center gap-2 text-[11px] font-semibold text-zinc-500">
            <span>{fullCount(post.score)} upvotes</span>
            <span>·</span>
            <span>{fullCount(post.comments)} comments</span>
          </div>
          {post.streamerTags?.length ? (
            <div className="mt-2 flex flex-wrap gap-1.5">
              {post.streamerTags.map(tag => (
                <span
                  key={tag}
                  className="rounded border border-violet-400/20 bg-violet-500/10 px-2 py-0.5 text-[10px] font-black uppercase text-violet-200"
                >
                  Matched: {tag}
                </span>
              ))}
            </div>
          ) : null}
        </div>
      </div>
      <a
        href={threadUrl}
        target="_blank"
        rel="noreferrer"
        className="mt-3 inline-flex rounded border border-white/10 bg-black/30 px-3 py-1.5 text-[11px] font-black uppercase tracking-wide text-zinc-200 transition hover:border-violet-300/40 hover:text-white"
      >
        Open thread
      </a>
    </article>
  )
}

function ClipSpikeCard({ clip }: { clip: ClipCard }) {
  const duration = formatClipDuration(clip.durationSeconds)
  return (
    <a
      href={clip.url}
      target="_blank"
      rel="noreferrer"
      className="group block overflow-hidden rounded-lg border border-white/10 bg-white/[0.035] transition hover:border-violet-300/50"
    >
      <div className="relative aspect-video bg-zinc-900">
        {clip.thumbnailUrl ? (
          <img
            src={clip.thumbnailUrl}
            alt={clip.title}
            className="h-full w-full object-cover transition duration-300 group-hover:scale-105"
            loading="lazy"
          />
        ) : null}
        {duration ? (
          <div className="absolute bottom-2 right-2 rounded bg-black/75 px-2 py-0.5 text-[11px] font-bold text-zinc-100">
            {duration}
          </div>
        ) : null}
      </div>
      <div className="p-3">
        <div className="line-clamp-2 text-sm font-black leading-5 text-white">{clip.title}</div>
        <div className="mt-2 text-[11px] font-semibold text-zinc-500">
          {fullCount(clip.viewCount)} views
        </div>
      </div>
    </a>
  )
}

export interface StreamPulsePanelProps {
  channelLogin: string
  insights?: ChannelInsights
  insightsLoading: boolean
  insightsFetching: boolean
  insightsError: boolean
  lsfLoadPending: boolean
  onLoadLsf: () => void
  liveAnalytics?: AnalyticsStreamDetail
  liveAnalyticsLoading: boolean
  trackLiveAnalytics: boolean
  trackAnalyticsPending: boolean
  onTrackAnalytics: (track: boolean) => void
  autoUpdate: boolean
  onAutoUpdateChange: (next: boolean) => void
}

function StreamPulsePanel({
  channelLogin,
  insights,
  insightsLoading,
  insightsFetching,
  insightsError,
  lsfLoadPending,
  onLoadLsf,
  liveAnalytics,
  liveAnalyticsLoading,
  trackLiveAnalytics,
  trackAnalyticsPending,
  onTrackAnalytics,
  autoUpdate,
  onAutoUpdateChange,
}: StreamPulsePanelProps) {
  const { scraperOffline, startService, isStarting } = useOptionalServices({ probeControl: false })

  const lsfPosts = insights?.lsf ?? []
  const lsfEmpty = summarizeLsfEmptyState(insights?.sources, { scraperOffline })
  const lsfWarming = isLsfWarming(insights?.sources)
  const lsfPending = isLsfPending(insights?.sources)
  const clipSpike = insights?.clips?.[0]
  const topEmotes = liveAnalytics?.topEmotes?.slice(0, 8) ?? []

  return (
    <div className="flex h-full flex-col">
      <div className="space-y-4 p-4">
        <div className="flex items-start justify-between gap-3">
          <div>
            <div className="flex flex-wrap items-center gap-2">
              <h2 className="text-sm font-black uppercase tracking-wide text-white">Stream Pulse</h2>
              <span className="rounded bg-red-600/90 px-2 py-0.5 text-[10px] font-black uppercase text-white">
                Live
              </span>
              {insightsFetching && autoUpdate ? (
                <span className="text-[10px] font-semibold text-zinc-500">Updating…</span>
              ) : null}
            </div>
            <p className="mt-1 text-xs font-semibold text-zinc-500">Top r/LivestreamFail threads about this streamer from the past 7 days.</p>
          </div>
          <div className="flex shrink-0 flex-col items-end gap-2">
            <TrackAnalyticsToggle
              tracked={trackLiveAnalytics}
              pending={trackAnalyticsPending}
              onToggle={onTrackAnalytics}
              trackLabel="Track streamer"
              trackingLabel="Tracking"
            />
            <label className="flex items-center gap-2 text-[11px] font-semibold text-zinc-400">
              <span>Auto-updating</span>
              <button
                type="button"
                role="switch"
                aria-checked={autoUpdate}
                onClick={() => onAutoUpdateChange(!autoUpdate)}
                className={`relative h-5 w-9 rounded-full transition ${autoUpdate ? 'bg-violet-600' : 'bg-zinc-700'}`}
              >
                <span
                  className={`absolute top-0.5 h-4 w-4 rounded-full bg-white transition ${autoUpdate ? 'left-4' : 'left-0.5'}`}
                />
              </button>
            </label>
          </div>
        </div>

        <section>
          <LiveStatsBand
            login={channelLogin}
            enabled
            className="rounded-lg border border-white/10 bg-zinc-950/60 p-3"
          />
        </section>

        <section className="space-y-3">
          <div className="flex items-center justify-between gap-2">
            <h3 className="text-xs font-black uppercase text-zinc-400">Top past 7 days</h3>
            {lsfPosts.length ? (
              <span className="rounded bg-violet-600/20 px-2 py-0.5 text-[10px] font-black uppercase text-violet-200">
                {lsfPosts.length} thread{lsfPosts.length === 1 ? '' : 's'}
              </span>
            ) : null}
          </div>
          {insightsLoading && !insights ? (
            <div className="rounded-lg border border-white/10 bg-white/[0.035] px-4 py-6 text-center text-xs font-semibold text-zinc-500">
              Loading LSF threads…
            </div>
          ) : insightsError ? (
            <div className="rounded-lg border border-white/10 bg-white/[0.035] px-4 py-6 text-center text-xs font-semibold text-zinc-500">
              LSF threads are temporarily unavailable. Retrying…
            </div>
          ) : lsfWarming || (lsfLoadPending && !lsfPosts.length) ? (
            <PulseLsfLoadingSkeleton subtitle={lsfEmpty.body} />
          ) : lsfPosts.length ? (
            <div className="space-y-3">
              {lsfPosts.map(post => (
                <PulseLsfCard key={post.id} post={post} />
              ))}
            </div>
          ) : (
            <div className="rounded-lg border border-white/10 bg-white/[0.035] px-4 py-5 text-center">
              <div className="text-sm font-black text-zinc-200">{lsfEmpty.title}</div>
              <p className="mt-2 text-xs font-semibold leading-relaxed text-zinc-500">{lsfEmpty.body}</p>
              {lsfPending ? (
                <button
                  type="button"
                  disabled={lsfLoadPending || insightsFetching}
                  onClick={onLoadLsf}
                  className="mt-3 rounded bg-violet-600 px-3 py-1.5 text-[11px] font-black uppercase text-white transition hover:bg-violet-500 disabled:opacity-60"
                >
                  {lsfLoadPending || insightsFetching ? 'Searching Reddit…' : 'Search r/LivestreamFail'}
                </button>
              ) : null}
              {lsfEmpty.showScraperAction ? (
                <button
                  type="button"
                  disabled={isStarting('scraper')}
                  onClick={() => void startService('scraper')}
                  className="mt-3 rounded bg-violet-600 px-3 py-1.5 text-[11px] font-black uppercase text-white transition hover:bg-violet-500 disabled:opacity-60"
                >
                  {isStarting('scraper') ? 'Starting…' : 'Start Analytics'}
                </button>
              ) : null}
              <p className="mt-3 text-[11px] font-semibold text-zinc-600">
                Recent r/LivestreamFail threads load automatically from Reddit.
              </p>
            </div>
          )}
        </section>

        <section className="space-y-3">
          <h3 className="text-xs font-black uppercase text-zinc-400">Top emotes</h3>
          {!trackLiveAnalytics ? (
            <div className="rounded-lg border border-white/10 bg-white/[0.035] px-4 py-4 text-xs font-semibold text-zinc-500">
              {analyticsNotTrackedMessage()} Use <span className="text-violet-200">Track streamer</span> above to enable live emote spikes.
            </div>
          ) : liveAnalyticsLoading && !topEmotes.length ? (
            <div className="text-xs font-semibold text-zinc-500">Loading live emotes…</div>
          ) : topEmotes.length ? (
            <div className="flex flex-wrap gap-2">
              {topEmotes.map(emote => {
                const image = emoteImageUrl(emote)
                return (
                  <div
                    key={emote.key}
                    title={`${emote.name} · ${fullCount(emote.count)}`}
                    className="flex items-center gap-2 rounded-lg border border-white/10 bg-white/[0.035] px-2 py-1.5"
                  >
                    {image ? (
                      <img src={image} alt={emote.name} className="h-6 w-6 object-contain" loading="lazy" />
                    ) : null}
                    <span className="max-w-[5rem] truncate text-[11px] font-black text-zinc-200">{emote.name}</span>
                    <span className="text-[10px] font-semibold text-zinc-500">{fullCount(emote.count)}</span>
                  </div>
                )
              })}
            </div>
          ) : (
            <div className="text-xs font-semibold text-zinc-500">No emote spikes yet this stream.</div>
          )}
        </section>

        <section className="space-y-3">
          <h3 className="text-xs font-black uppercase text-zinc-400">Clip spike</h3>
          {clipSpike ? (
            <ClipSpikeCard clip={clipSpike} />
          ) : (
            <div className="rounded-lg border border-white/10 bg-white/[0.035] px-4 py-4 text-xs font-semibold text-zinc-500">
              No recent clips loaded for this channel.
            </div>
          )}
        </section>

        <div className="pb-2 text-center">
          <Link
            to={`/analytics/${encodeURIComponent(channelLogin)}`}
            className="text-[11px] font-black uppercase tracking-wide text-violet-300 transition hover:text-violet-200"
          >
            Open full analytics →
          </Link>
        </div>
      </div>
    </div>
  )
}

export default StreamPulsePanel
