// Clips tab empty-state copy decision for the Analytics Right Rail.
//
// Pure, dependency-free decision of which empty state (if any) the Clips tab
// should render, per Requirement 35 of the moment-timeline spec. Kept pure so
// it can be unit tested and reused wherever the Clips tab is rendered.
//
//  - 35.1 sync-first: the stream has zero minute-level rollup rows carrying chat
//          or emote data. Instruct the user to sync chat/emotes first, then clip
//          from ranked Moments or heatmap peaks.
//  - 35.2 use-moments: rollup data exists but no clip jobs are queued. Point the
//          user at the Moments tab or heatmap peak selection rather than an
//          unqualified "click the graph".
//  - When clip jobs already exist, no empty state is shown (returns null) so the
//          tab can render its job list.

/** Minimal rollup shape needed to decide the Clips empty state. */
export interface ClipsRollup {
  chatCount?: number
  totalEmoteCount?: number
  missing?: boolean
}

export type ClipsEmptyStateVariant = 'sync-first' | 'use-moments'

export interface ClipsEmptyStateInput {
  /** Per-minute rollups for the selected stream. */
  rollups: ClipsRollup[]
  /** Number of clip jobs currently queued/known for this stream. */
  clipJobCount: number
}

export interface ClipsEmptyStateContent {
  variant: ClipsEmptyStateVariant
  title: string
  body: string
  /**
   * True when the recommended next action is to run a sync (35.1); false when
   * the user should pick a moment/peak instead (35.2). Lets the presentational
   * component decide whether to surface a sync CTA or a Moments-tab hint.
   */
  showSyncAction: boolean
}

const SYNC_FIRST: ClipsEmptyStateContent = {
  variant: 'sync-first',
  title: 'Sync chat & emotes to find clips',
  body: 'This stream has no chat or emote activity yet. Sync chat & emotes first, then clip from ranked moments or heatmap peaks.',
  showSyncAction: true,
}

const USE_MOMENTS: ClipsEmptyStateContent = {
  variant: 'use-moments',
  title: 'No clips queued yet',
  body: 'Open the Moments tab or select a heatmap peak to queue a clip from a ranked moment.',
  showSyncAction: false,
}

function hasChatOrEmoteData(r: ClipsRollup): boolean {
  return !r.missing && ((r.chatCount ?? 0) > 0 || (r.totalEmoteCount ?? 0) > 0)
}

/**
 * Decide the Clips tab empty state per Requirement 35.
 *
 * Returns `null` when clip jobs already exist for the stream, in which case the
 * Clips tab should render its job list rather than an empty state.
 */
export function clipsEmptyState(input: ClipsEmptyStateInput): ClipsEmptyStateContent | null {
  if (input.clipJobCount > 0) return null

  const hasRollupData = (input.rollups ?? []).some(hasChatOrEmoteData)
  return hasRollupData ? USE_MOMENTS : SYNC_FIRST
}
