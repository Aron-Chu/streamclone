import { storyTimeline, storyUpdatedAt, type PulseWireBanEvent, type PulseWireSourceHealth, type PulseWireStory, type PulseWireTrendingStreamer, type PulseWireWatchEntry, type PulseWireWindow } from '../../pulseWireApi'
import { useState, type ReactNode } from 'react'
import { toWireStoryView } from '../../utils/pulseWireStoryView'
import SourceHealthPanel from './SourceHealthPanel'

type Props = {
  sourceHealth: PulseWireSourceHealth
  trending: PulseWireTrendingStreamer[]
  stories: PulseWireStory[]
  followedStories?: PulseWireStory[]
  watchEntries?: PulseWireWatchEntry[]
  bans: PulseWireBanEvent[]
  banError?: string
  window: PulseWireWindow
  activeLogin: string
  activeCategory?: string
  searchQuery?: string
  onSelectLogin: (login: string) => void
  onSelectWatchEntry?: (entry: PulseWireWatchEntry) => void
  onAddWatchEntry?: (kind: PulseWireWatchEntry['kind'], value: string, label?: string) => Promise<void> | void
  onDeleteWatchEntry?: (id: number) => Promise<void> | void
  savedOnly?: boolean
  onShowSaved?: () => void
  onOpenOperatorTools?: () => void
}

function statusTone(sourceHealth: PulseWireSourceHealth) {
  const entries = Object.values(sourceHealth)
  if (!entries.length) return { label: 'All systems warming', color: '#FFCF7A' }
  const unhealthy = entries.filter(item => item.mode === 'error' || item.healthy === false)
  if (unhealthy.length) return { label: `${unhealthy.length} source issue${unhealthy.length === 1 ? '' : 's'}`, color: '#FF5C57' }
  return { label: 'All systems healthy', color: '#3FCB7E' }
}

function RailPanel({ title, action, children }: { title: string; action?: string; children: ReactNode }) {
  return (
    <section className="rounded-lg border border-[#24242B] bg-[#101014] p-3">
      <div className="mb-3 flex items-center justify-between gap-2">
        <h3 className="text-sm font-black text-[#F7F7F8]">{title}</h3>
        {action ? <span className="text-[11px] font-semibold text-[#A970FF]">{action}</span> : null}
      </div>
      {children}
    </section>
  )
}

function alertFreshnessLabel(value?: string) {
  const label = shortRelativeTime(value)
  return label === 'time unknown' ? 'recent activity' : label
}

function alertLatestAt(story: PulseWireStory) {
  const timelineLatest = (storyTimeline(story) ?? [])
    .map(item => Date.parse(item.at))
    .filter(Number.isFinite)
    .sort((a, b) => b - a)[0]
  const updatedAt = Date.parse(storyUpdatedAt(story) ?? '')
  return timelineLatest ?? (Number.isFinite(updatedAt) ? updatedAt : 0)
}

function alertSortScore(story: PulseWireStory, sourceCount: number) {
  return {
    velocity: story.windowScores?.velocityScore ?? story.scores.volatility ?? 0,
    latestAt: alertLatestAt(story),
    sourceCount,
  }
}

function LiveSpreadAlertsPanel({
  stories,
  sourceHealth,
  trending,
  onSelectLogin,
}: {
  stories: PulseWireStory[]
  sourceHealth: PulseWireSourceHealth
  trending: PulseWireTrendingStreamer[]
  onSelectLogin: (login: string) => void
}) {
  const storyAlerts = stories
    .map(story => {
      const view = toWireStoryView(story, sourceHealth)
      const hasYouTube = ['linked', 'matched'].includes(view.platformPresence.youtube.state)
      const hasReddit = ['linked', 'matched'].includes(view.platformPresence.reddit.state)
      const recentSourceDelta = Math.max(0, Math.trunc(story.windowScores?.recentSourceDelta ?? 0))
      const parts = [
        recentSourceDelta > 0 ? `+${recentSourceDelta} source${recentSourceDelta === 1 ? '' : 's'} in last hour` : '',
        view.sourceCount > 1 ? `${view.sourceCount} sources attached` : '',
        hasYouTube ? 'YouTube evidence attached' : '',
        hasReddit ? 'Reddit thread active' : '',
      ].filter(Boolean)
      if (!parts.length) return null
      return {
        id: story.story.id,
        title: view.title,
        login: story.entity?.login ?? '',
        entity: view.entityLabel,
        label: parts.slice(0, 3).join(' - '),
        metric: alertFreshnessLabel(storyUpdatedAt(story)),
        sort: alertSortScore(story, view.sourceCount),
      }
    })
    .filter((item): item is NonNullable<typeof item> => item != null)
    .sort((a, b) => (
      (b.sort.velocity - a.sort.velocity) ||
      (b.sort.latestAt - a.sort.latestAt) ||
      (b.sort.sourceCount - a.sort.sourceCount)
    ))
    .slice(0, 3)

  return (
    <RailPanel title="Live spread alerts">
      {storyAlerts.length ? (
        <div className="space-y-2">
          {storyAlerts.map(item => (
            <button
              key={item.id}
              type="button"
              onClick={() => item.login ? onSelectLogin(item.login) : undefined}
              className="flex w-full items-center justify-between gap-2 rounded-lg bg-[#17171D] px-2 py-2 text-left transition hover:bg-[#1D1D24]"
            >
              <div className="min-w-0">
                <p className="truncate text-xs font-bold text-[#F7F7F8]">{item.entity}</p>
                <p className="mt-0.5 text-[11px] text-[#7A7A85]">
                  {item.label}
                </p>
              </div>
              <span className="shrink-0 rounded bg-[#21190D] px-2 py-1 text-[11px] font-bold text-[#FFE0A3]">
                {item.metric}
              </span>
            </button>
          ))}
        </div>
      ) : trending.length ? (
        <div className="space-y-2">
          {trending.slice(0, 3).map(item => (
            <button
              key={item.login}
              type="button"
              onClick={() => onSelectLogin(item.login)}
              className="flex w-full items-center justify-between gap-2 rounded-lg bg-[#17171D] px-2 py-2 text-left transition hover:bg-[#1D1D24]"
            >
              <div className="min-w-0">
                <p className="truncate text-xs font-bold text-[#F7F7F8]">{item.displayName}</p>
                <p className="mt-0.5 text-[11px] text-[#7A7A85]">
                  {item.storyCount} active stor{item.storyCount === 1 ? 'y' : 'ies'}
                  {item.evidenceCount ? ` - ${item.evidenceCount} evidence` : ''}
                </p>
              </div>
              <span className="shrink-0 rounded bg-[#21190D] px-2 py-1 text-[11px] font-bold text-[#FFE0A3]">
                {item.evidenceCount ? `${item.evidenceCount} evidence` : `${item.storyCount} stories`}
              </span>
            </button>
          ))}
        </div>
      ) : (
        <p className="text-xs leading-relaxed text-[#7A7A85]">
          Alerts appear after story-level deltas or trending streamer activity arrive.
        </p>
      )}
    </RailPanel>
  )
}

function WatchlistMini({
  followedStories,
  watchEntries = [],
  activeLogin,
  activeCategory = '',
  searchQuery = '',
  onSelectLogin,
  onSelectWatchEntry,
  onAddWatchEntry,
  onDeleteWatchEntry,
}: {
  followedStories: PulseWireStory[]
  watchEntries?: PulseWireWatchEntry[]
  activeLogin: string
  activeCategory?: string
  searchQuery?: string
  onSelectLogin: (login: string) => void
  onSelectWatchEntry?: (entry: PulseWireWatchEntry) => void
  onAddWatchEntry?: (kind: PulseWireWatchEntry['kind'], value: string, label?: string) => Promise<void> | void
  onDeleteWatchEntry?: (id: number) => Promise<void> | void
}) {
  const [pending, setPending] = useState('')
  const streamers = Array.from(followedStories.reduce((acc, story) => {
    const login = story.entity?.login?.trim()
    if (!login) return acc
    const existing = acc.get(login)
    acc.set(login, {
      login,
      displayName: story.entity?.displayName?.trim() || login,
      storyCount: (existing?.storyCount ?? 0) + 1,
      latestAt: Math.max(existing?.latestAt ?? 0, alertLatestAt(story)),
    })
    return acc
  }, new Map<string, { login: string; displayName: string; storyCount: number; latestAt: number }>()).values())
    .sort((a, b) => b.storyCount - a.storyCount || b.latestAt - a.latestAt)
    .slice(0, 3)
  const categoryEntries = watchEntries.filter(item => item.kind === 'category')
  const keywordEntries = watchEntries.filter(item => item.kind === 'keyword')
  const normalizedSearch = searchQuery.trim()
  const canWatchCategory = activeCategory && !categoryEntries.some(item => item.value === activeCategory)
  const canWatchKeyword = normalizedSearch.length >= 2 && !keywordEntries.some(item => item.value.toLowerCase() === normalizedSearch.toLowerCase())

  async function add(kind: PulseWireWatchEntry['kind'], value: string, label?: string) {
    if (!onAddWatchEntry) return
    try {
      setPending(`${kind}:${value}`)
      await onAddWatchEntry(kind, value, label)
    } finally {
      setPending('')
    }
  }

  async function remove(id: number) {
    if (!onDeleteWatchEntry) return
    try {
      setPending(`delete:${id}`)
      await onDeleteWatchEntry(id)
    } finally {
      setPending('')
    }
  }

  return (
    <RailPanel title="Your watchlist" action={watchEntries.length ? 'Saved filters' : streamers.length ? 'Streamers' : undefined}>
      {streamers.length ? (
        <div className="space-y-1.5">
          {streamers.map(item => {
            const active = activeLogin === item.login
            return (
              <button
                key={item.login}
                type="button"
                onClick={() => onSelectLogin(active ? '' : item.login)}
                className={`flex w-full items-center justify-between gap-2 rounded-lg px-2 py-1.5 text-left transition ${
                  active ? 'bg-[#2B194C] text-[#F7F7F8]' : 'bg-[#17171D] text-[#D6D6DE] hover:bg-[#1D1D24]'
                }`}
              >
                <span className="truncate text-xs font-semibold">{item.displayName}</span>
                <span className="text-[11px] text-[#7A7A85]">{item.storyCount} followed</span>
              </button>
            )
          })}
        </div>
      ) : (
        <p className="text-xs leading-relaxed text-[#7A7A85]">
          Follow stories to build a streamer watchlist for this window.
        </p>
      )}
      {watchEntries.length ? (
        <div className="mt-3 space-y-2 border-t border-[#24242B] pt-3">
          {categoryEntries.length ? (
            <div>
              <p className="mb-1 text-[10px] font-bold uppercase tracking-[0.06em] text-[#7A7A85]">Categories</p>
              <div className="space-y-1">
                {categoryEntries.map(item => (
                  <div key={item.id} className="flex items-center gap-1">
                    <button
                      type="button"
                      onClick={() => onSelectWatchEntry?.(item)}
                      className={`min-w-0 flex-1 rounded-lg px-2 py-1.5 text-left text-xs font-semibold transition ${
                        activeCategory === item.value ? 'bg-[#2B194C] text-[#F7F7F8]' : 'bg-[#17171D] text-[#D6D6DE] hover:bg-[#1D1D24]'
                      }`}
                    >
                      {item.label || item.value.replace(/_/g, ' ')}
                    </button>
                    {onDeleteWatchEntry ? (
                      <button
                        type="button"
                        disabled={pending === `delete:${item.id}`}
                        onClick={() => void remove(item.id)}
                        className="rounded-lg px-2 py-1.5 text-[11px] font-semibold text-[#7A7A85] hover:bg-[#1D1D24] hover:text-[#EFEFF1] disabled:opacity-50"
                        aria-label={`Remove ${item.label || item.value} watch`}
                      >
                        x
                      </button>
                    ) : null}
                  </div>
                ))}
              </div>
            </div>
          ) : null}
          {keywordEntries.length ? (
            <div>
              <p className="mb-1 text-[10px] font-bold uppercase tracking-[0.06em] text-[#7A7A85]">Keywords</p>
              <div className="space-y-1">
                {keywordEntries.map(item => (
                  <div key={item.id} className="flex items-center gap-1">
                    <button
                      type="button"
                      onClick={() => onSelectWatchEntry?.(item)}
                      className={`min-w-0 flex-1 rounded-lg px-2 py-1.5 text-left text-xs font-semibold transition ${
                        searchQuery.trim().toLowerCase() === item.value.toLowerCase()
                          ? 'bg-[#2B194C] text-[#F7F7F8]'
                          : 'bg-[#17171D] text-[#D6D6DE] hover:bg-[#1D1D24]'
                      }`}
                    >
                      {item.label || item.value}
                    </button>
                    {onDeleteWatchEntry ? (
                      <button
                        type="button"
                        disabled={pending === `delete:${item.id}`}
                        onClick={() => void remove(item.id)}
                        className="rounded-lg px-2 py-1.5 text-[11px] font-semibold text-[#7A7A85] hover:bg-[#1D1D24] hover:text-[#EFEFF1] disabled:opacity-50"
                        aria-label={`Remove ${item.label || item.value} watch`}
                      >
                        x
                      </button>
                    ) : null}
                  </div>
                ))}
              </div>
            </div>
          ) : null}
        </div>
      ) : null}
      {(canWatchCategory || canWatchKeyword) && onAddWatchEntry ? (
        <div className="mt-3 flex flex-wrap gap-2 border-t border-[#24242B] pt-3">
          {canWatchCategory ? (
            <button
              type="button"
              disabled={pending === `category:${activeCategory}`}
              onClick={() => void add('category', activeCategory, activeCategory.replace(/_/g, ' '))}
              className="rounded-lg border border-[#33333A] bg-[#17171D] px-2.5 py-1.5 text-[11px] font-semibold text-[#EFEFF1] transition hover:border-[#A970FF]/50 disabled:opacity-50"
            >
              Watch {activeCategory.replace(/_/g, ' ')}
            </button>
          ) : null}
          {canWatchKeyword ? (
            <button
              type="button"
              disabled={pending === `keyword:${normalizedSearch}`}
              onClick={() => void add('keyword', normalizedSearch)}
              className="rounded-lg border border-[#33333A] bg-[#17171D] px-2.5 py-1.5 text-[11px] font-semibold text-[#EFEFF1] transition hover:border-[#A970FF]/50 disabled:opacity-50"
            >
              Watch "{normalizedSearch}"
            </button>
          ) : null}
        </div>
      ) : null}
      <button
        type="button"
        onClick={() => onSelectLogin('')}
        className="mt-3 text-[11px] font-semibold text-[#A970FF] hover:text-[#CDB4FF]"
      >
        Clear streamer filter
      </button>
    </RailPanel>
  )
}

function FollowedStoriesMini({
  stories,
  window,
  savedOnly,
  onShowSaved,
}: {
  stories: PulseWireStory[]
  window: PulseWireWindow
  savedOnly?: boolean
  onShowSaved?: () => void
}) {
  return (
    <RailPanel title="Followed stories" action={savedOnly ? 'Filtering' : 'Saved'}>
      {stories.length ? (
        <div className="space-y-1.5">
          {stories.slice(0, 4).map(story => {
            const view = toWireStoryView(story)
            return (
              <a
                key={story.story.id}
                href={`/pulse-wire/${story.story.id}?tab=wire&window=${window}`}
                className="block rounded-lg bg-[#17171D] px-2 py-1.5 transition hover:bg-[#1D1D24]"
              >
                <p className="truncate text-xs font-semibold text-[#F7F7F8]">{view.title}</p>
                <p className="mt-0.5 text-[11px] text-[#7A7A85]">
                  {view.entityLabel} - {view.readerStatus.replace(/_/g, ' ')}
                </p>
              </a>
            )
          })}
        </div>
      ) : (
        <p className="text-xs leading-relaxed text-[#7A7A85]">
          Follow stories from the lead desk to keep them here for this window.
        </p>
      )}
      <button
        type="button"
        onClick={onShowSaved}
        disabled={savedOnly}
        className="mt-3 text-[11px] font-semibold text-[#A970FF] hover:text-[#CDB4FF] disabled:cursor-default disabled:text-[#7A7A85]"
      >
        {savedOnly ? 'Showing saved stories' : 'Show saved stories'}
      </button>
    </RailPanel>
  )
}

function eventLabel(eventType: string) {
  return eventType.replace(/_/g, ' ')
}

function shortRelativeTime(value?: string) {
  if (!value) return 'time unknown'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return 'time unknown'
  const delta = Date.now() - date.getTime()
  const minutes = Math.max(0, Math.round(delta / 60000))
  if (minutes < 60) return `${minutes}m ago`
  const hours = Math.round(minutes / 60)
  if (hours < 48) return `${hours}h ago`
  return `${Math.round(hours / 24)}d ago`
}

function ModerationMini({ bans, error }: { bans: PulseWireBanEvent[]; error?: string }) {
  return (
    <RailPanel title="Bans & moderation" action="View all">
      {error ? (
        <div className="rounded-lg border border-[#3A2A12] bg-[#21190D] p-3 text-xs leading-relaxed text-[#FFE0A3]">
          Ban feed unavailable: {error}
        </div>
      ) : bans.length ? (
        <div className="space-y-2">
          {bans.slice(0, 4).map(item => {
            const body = (
              <>
                <div className="min-w-0">
                  <p className="truncate text-xs font-bold text-[#F7F7F8]">
                    {item.streamerDisplayName || item.streamerLogin}
                  </p>
                  <p className="mt-0.5 line-clamp-2 text-[11px] leading-snug text-[#ADADB8]">{item.headline}</p>
                  <p className="mt-1 text-[10px] capitalize text-[#7A7A85]">
                    {eventLabel(item.eventType)} - {item.platform || item.source} - {shortRelativeTime(item.occurredAt)}
                  </p>
                </div>
                {typeof item.confidence === 'number' ? (
                  <span className="shrink-0 rounded bg-[#1B1B21] px-1.5 py-0.5 text-[10px] font-bold text-[#CDB4FF]">
                    {Math.round(item.confidence <= 1 ? item.confidence * 100 : item.confidence)}%
                  </span>
                ) : null}
              </>
            )
            if (item.sourceUrl) {
              return (
                <a
                  key={item.id}
                  href={item.sourceUrl}
                  target="_blank"
                  rel="noopener noreferrer"
                  className="flex items-start justify-between gap-2 rounded-lg bg-[#17171D] px-2 py-2 transition hover:bg-[#1D1D24]"
                >
                  {body}
                </a>
              )
            }
            return (
              <div key={item.id} className="flex items-start justify-between gap-2 rounded-lg bg-[#17171D] px-2 py-2">
                {body}
              </div>
            )
          })}
        </div>
      ) : (
        <p className="text-xs leading-relaxed text-[#7A7A85]">
          No ban or moderation events found in this window.
        </p>
      )}
    </RailPanel>
  )
}

export default function WireReaderRail({
  sourceHealth,
  trending,
  stories,
  followedStories = [],
  watchEntries = [],
  bans,
  banError,
  window,
  activeLogin,
  activeCategory,
  searchQuery,
  onSelectLogin,
  onSelectWatchEntry,
  onAddWatchEntry,
  onDeleteWatchEntry,
  savedOnly,
  onShowSaved,
  onOpenOperatorTools,
}: Props) {
  const status = statusTone(sourceHealth)
  return (
    <div className="space-y-3">
      <div className="rounded-lg border border-[#24242B] bg-[#101014] p-3">
        <div className="flex items-center justify-between gap-2">
          <div>
            <p className="text-[11px] font-black uppercase tracking-[0.06em] text-[#7A7A85]">Stack status</p>
            <p className="mt-1 text-xs font-semibold text-[#D6D6DE]">{status.label}</p>
          </div>
          <span className="h-2.5 w-2.5 rounded-full" style={{ backgroundColor: status.color }} />
        </div>
        <p className="mt-2 text-[11px] text-[#7A7A85]">Window: {window}</p>
      </div>
      <LiveSpreadAlertsPanel stories={stories} sourceHealth={sourceHealth} trending={trending} onSelectLogin={onSelectLogin} />
      <SourceHealthPanel sources={sourceHealth} compact onViewAll={onOpenOperatorTools} />
      <FollowedStoriesMini stories={followedStories} window={window} savedOnly={savedOnly} onShowSaved={onShowSaved} />
      <WatchlistMini
        followedStories={followedStories}
        watchEntries={watchEntries}
        activeLogin={activeLogin}
        activeCategory={activeCategory}
        searchQuery={searchQuery}
        onSelectLogin={onSelectLogin}
        onSelectWatchEntry={onSelectWatchEntry}
        onAddWatchEntry={onAddWatchEntry}
        onDeleteWatchEntry={onDeleteWatchEntry}
      />
      <ModerationMini bans={bans} error={banError} />
    </div>
  )
}
