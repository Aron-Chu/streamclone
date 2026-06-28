// Stat-card placeholder classification for the Analytics stat cards.
//
// Pure, dependency-free classification of which stat cards should render a
// source-appropriate placeholder ("Stats only" / "Needs sync" / "Collecting")
// instead of a misleading numeric value, per Requirement 6 of the
// moment-timeline spec. Kept pure so it can be property-tested (task 5.4) and
// reused across every stat-card placement in Analytics.tsx.

/** Collection state of a stream detail response. Mirrors AnalyticsStreamDetail.state. */
export type StreamCollectionState = 'live' | 'historical' | 'not_collected' | 'syncing'

/** The stat cards shown on the analytics page. */
export type StatCardKey = 'current' | 'average' | 'peak' | 'chat' | 'emoteUses' | 'duration'

/** Placeholder labels that replace numeric values when data coverage is incomplete. */
export type StatCardPlaceholder = 'Stats only' | 'Needs sync' | 'Collecting'

/** Minimal rollup shape needed for classification. */
export interface StatCardRollup {
  viewerSamples?: number
  chatCount?: number
  totalEmoteCount?: number
  missing?: boolean
}

export interface StatCardInput {
  state: StreamCollectionState
  /** TwitchTracker / stream-level average viewers (0 when unknown). */
  avgViewers?: number
  /** TwitchTracker / stream-level peak viewers (0 when unknown). */
  peakViewers?: number
  /** Per-minute rollups for the selected stream. */
  rollups: StatCardRollup[]
}

export interface StatCardClassification {
  /** Placeholder text to render, or null when a numeric value should be shown. */
  placeholder: StatCardPlaceholder | null
  /** True when a placeholder is shown; callers render the muted style (Req 6.5). */
  muted: boolean
}

export type StatCardClassifications = Record<StatCardKey, StatCardClassification>

/** Suggested muted class for placeholder text so every placement renders identically (Req 6.5). */
export const STAT_PLACEHOLDER_MUTED_CLASS = 'text-zinc-600 font-semibold'

const NUMERIC: StatCardClassification = { placeholder: null, muted: false }

function placeholder(text: StatCardPlaceholder): StatCardClassification {
  return { placeholder: text, muted: true }
}

function allNumeric(): StatCardClassifications {
  return {
    current: { ...NUMERIC },
    average: { ...NUMERIC },
    peak: { ...NUMERIC },
    chat: { ...NUMERIC },
    emoteUses: { ...NUMERIC },
    duration: { ...NUMERIC },
  }
}

function isLiveViewerRollup(r: StatCardRollup): boolean {
  return !r.missing && (r.viewerSamples ?? 0) > 0
}

function hasViewerOrChatData(r: StatCardRollup): boolean {
  return !r.missing && ((r.viewerSamples ?? 0) > 0 || (r.chatCount ?? 0) > 0)
}

function hasAnyData(r: StatCardRollup): boolean {
  return (
    !r.missing &&
    ((r.viewerSamples ?? 0) > 0 || (r.chatCount ?? 0) > 0 || (r.totalEmoteCount ?? 0) > 0)
  )
}

/**
 * Classify each analytics stat card as numeric or a placeholder per Requirement 6.
 *
 * Precedence (only one rule applies per call):
 *  - 6.3 Collecting: live stream with fewer than 2 non-missing rollups that have
 *        viewer samples. Chat and Emote Uses show "Collecting"; Current, Average,
 *        and Peak show available live viewer counts (numeric).
 *  - 6.2 Needs sync: state "not_collected" with no tracker averages and no rollup
 *        data. Current, Average, Peak, Chat, and Emote Uses show "Needs sync".
 *  - 6.1 Stats only: tracker averages exist but no rollup row has viewer samples
 *        or chat counts. Chat and Emote Uses show "Stats only"; Average, Peak,
 *        and Duration show the tracker-sourced numeric values.
 *  - Otherwise every card is numeric.
 *
 * Requirement 6.4 (Collecting → numeric once >= 2 rollups exist) holds naturally:
 * recomputing with more rollups simply falls through to the numeric default.
 * Requirement 6.5 is satisfied by the `muted` flag on every placeholder result.
 */
export function classifyStatCards(input: StatCardInput): StatCardClassifications {
  const { state } = input
  const avgViewers = input.avgViewers ?? 0
  const peakViewers = input.peakViewers ?? 0
  const rollups = input.rollups ?? []

  const hasTrackerAverages = avgViewers > 0 || peakViewers > 0
  const liveViewerRollupCount = rollups.filter(isLiveViewerRollup).length
  const hasViewerOrChatRollup = rollups.some(hasViewerOrChatData)
  const hasAnyRollupData = rollups.some(hasAnyData)

  const cards = allNumeric()

  // 6.3 Collecting — live stream still warming up (fewer than 2 sampled minutes).
  if (state === 'live' && liveViewerRollupCount < 2) {
    cards.chat = placeholder('Collecting')
    cards.emoteUses = placeholder('Collecting')
    return cards
  }

  // 6.2 Needs sync — nothing collected and no tracker fallback.
  if (state === 'not_collected' && !hasTrackerAverages && !hasAnyRollupData) {
    cards.current = placeholder('Needs sync')
    cards.average = placeholder('Needs sync')
    cards.peak = placeholder('Needs sync')
    cards.chat = placeholder('Needs sync')
    cards.emoteUses = placeholder('Needs sync')
    return cards
  }

  // 6.1 Stats only — tracker averages without any minute-level chat/viewer rollups.
  if (hasTrackerAverages && !hasViewerOrChatRollup) {
    cards.chat = placeholder('Stats only')
    cards.emoteUses = placeholder('Stats only')
    // Average, Peak, Duration (and Current) remain numeric using tracker values.
    return cards
  }

  return cards
}

/** Convenience guard for callers wiring placeholder rendering. */
export function isPlaceholder(card: StatCardClassification): boolean {
  return card.placeholder !== null
}
