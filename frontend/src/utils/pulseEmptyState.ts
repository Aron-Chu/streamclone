import type { SourceStatus } from '../api.ts'

export interface LsfEmptyStateContent {
  title: string
  body: string
  showScraperAction: boolean
}

export const PULSE_ROLLUP_WINDOW = 15

function redditLsfSources(sources: SourceStatus[] | undefined): SourceStatus[] {
  return (sources ?? []).filter(source => source.source.startsWith('reddit_lsf'))
}

export function isLsfWarming(sources: SourceStatus[] | undefined): boolean {
  return redditLsfSources(sources).some(source =>
    source.provider === 'warmup'
    || source.message?.toLowerCase().includes('fetching from reddit'),
  )
}

export function isLsfPending(sources: SourceStatus[] | undefined): boolean {
  return redditLsfSources(sources).some(source => source.provider === 'pending')
}

function sourceDetail(source: SourceStatus): string {
  const msg = source.message?.trim()
  if (msg?.toLowerCase().includes('context canceled') || msg?.toLowerCase().includes('context deadline exceeded')) {
    return 'Still fetching from Reddit — the first load can take a couple of minutes.'
  }
  if (msg?.toLowerCase().includes('fetch interrupted')) {
    return 'Still fetching from Reddit — the first load can take a couple of minutes.'
  }
  if (msg?.toLowerCase() === 'provider in backoff') {
    return 'Direct Reddit fetch is cooling down — retry in about a minute.'
  }
  return msg || source.state
}

export function summarizeLsfEmptyState(
  sources: SourceStatus[] | undefined,
  options?: { scraperOffline?: boolean },
): LsfEmptyStateContent {
  const rows = redditLsfSources(sources)
  const disabled = rows.find(
    source =>
      source.provider === 'off'
      || source.message?.toLowerCase().includes('reddit provider disabled'),
  )
  if (disabled) {
    return {
      title: 'LSF threads unavailable',
      body: 'Could not reach Reddit for r/LivestreamFail highlights. Retry in a moment or check network egress from the metadata container.',
      showScraperAction: false,
    }
  }

  const warming = rows.find(source =>
    source.provider === 'warmup'
    || source.message?.toLowerCase().includes('fetching from reddit'),
  )
  if (warming) {
    return {
      title: 'LSF threads loading',
      body: 'Pulling r/LivestreamFail posts via the scraper — one quick search, usually under a minute.',
      showScraperAction: Boolean(options?.scraperOffline),
    }
  }

  const pending = rows.find(source => source.provider === 'pending')
  if (pending) {
    return {
      title: 'LSF threads',
      body: 'Search uses the Analytics scraper when you are ready — it will not start automatically so TwitchTracker sync keeps priority.',
      showScraperAction: false,
    }
  }

  const scraperMiss = rows.find(source =>
    (source.provider === 'scraper' || source.provider === 'scraper_hot')
    && source.message?.toLowerCase().includes('no recent hot posts matched'),
  )
  if (scraperMiss) {
    return {
      title: 'No LSF threads matched',
      body: 'Nothing from r/LivestreamFail mentioned this streamer in the past 7 days. Check back after a big moment.',
      showScraperAction: false,
    }
  }

  const blocked = rows.find(source => {
    if (source.state !== 'blocked') return false
    if (source.provider === 'public_json' && source.message?.includes('403')) return false
    return true
  })
  if (blocked) {
    return {
      title: 'Reddit temporarily blocked',
      body: sourceDetail(blocked),
      showScraperAction: Boolean(options?.scraperOffline),
    }
  }

  const failed = rows.find(source => source.state === 'error')
  if (failed) {
    return {
      title: 'Could not load LSF threads',
      body: sourceDetail(failed),
      showScraperAction: Boolean(options?.scraperOffline),
    }
  }

  const interrupted = rows.find(source =>
    source.state === 'unavailable'
    && (source.message?.toLowerCase().includes('fetch interrupted')
      || source.message?.toLowerCase().includes('context canceled')),
  )
  if (interrupted) {
    return {
      title: 'LSF threads loading',
      body: sourceDetail(interrupted),
      showScraperAction: false,
    }
  }

  const unavailable = rows.find(source => source.state === 'unavailable')
  if (unavailable) {
    const scraperHint = unavailable.provider === 'scraper'
      ? ' Reddit blocked direct fetch — start the Analytics scraper as a fallback.'
      : ''
    return {
      title: 'LSF source unavailable',
      body: `${sourceDetail(unavailable)}.${scraperHint}`,
      showScraperAction: Boolean(options?.scraperOffline),
    }
  }

  if (rows.some(source => source.state === 'ready')) {
    return {
      title: 'No LSF threads matched',
      body: 'Nothing from r/LivestreamFail mentioned this streamer in the past 7 days. Check back after a big moment.',
      showScraperAction: false,
    }
  }

  return {
    title: 'LSF threads loading',
    body: 'Reddit highlights for this channel are still loading.',
    showScraperAction: false,
  }
}

export function analyticsNotTrackedMessage(): string {
  return 'Turn on Track analytics to see live emote spikes and chat correlation for this stream.'
}

export function rollupsWarmingMessage(): string {
  return 'Analytics tracking is on — activity data appears after the first minute of rollups.'
}

export function slicePulseRollups<T extends { missing?: boolean }>(
  rollups: T[],
  window = PULSE_ROLLUP_WINDOW,
): T[] {
  return rollups.filter(rollup => !rollup.missing).slice(-window)
}

export function formatPeakOffset(offsetSeconds: number): string {
  const total = Math.max(0, Math.floor(offsetSeconds))
  const mm = Math.floor(total / 60)
  const ss = total % 60
  return `${mm}:${ss.toString().padStart(2, '0')}`
}

export function pickTopPeakOffsetSeconds(
  rollups: { missing?: boolean; minuteTs?: string; chatCount?: number; totalEmoteCount?: number }[],
): number | null {
  if (!rollups.length) return null
  let bestIndex = -1
  let bestScore = -1
  rollups.forEach((rollup, index) => {
    if (rollup.missing) return
    const score = (rollup.chatCount ?? 0) + (rollup.totalEmoteCount ?? 0)
    if (score > bestScore) {
      bestScore = score
      bestIndex = index
    }
  })
  if (bestIndex < 0) return null
  return bestIndex * 60
}
