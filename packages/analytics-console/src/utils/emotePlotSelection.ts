export type EmotePlotViewMode = 'overview' | 'emotes' | 'spikes'

export type EmotePlotSelection = 'auto' | 'none' | Set<string>

export const MAX_PLOTTED_EMOTES = 6

export function defaultEmotePlotKeys(
  topEmotes: Array<{ key: string }>,
  viewMode: EmotePlotViewMode,
): Set<string> {
  const keys = topEmotes.map((emote) => emote.key).filter(Boolean)
  if (viewMode === 'overview') {
    return keys[0] ? new Set([keys[0]]) : new Set()
  }
  if (viewMode === 'emotes') {
    return new Set(keys.slice(0, 4))
  }
  return new Set(keys.slice(0, 3))
}

export function resolveChartEmoteKeys(
  selection: EmotePlotSelection,
  topEmotes: Array<{ key: string }>,
  viewMode: EmotePlotViewMode,
): Set<string> {
  if (selection === 'none') return new Set()
  if (selection instanceof Set) return selection
  return defaultEmotePlotKeys(topEmotes, viewMode)
}

export function toggleEmotePlotSelection(
  selection: EmotePlotSelection,
  key: string,
  topEmotes: Array<{ key: string }>,
  viewMode: EmotePlotViewMode,
): EmotePlotSelection {
  const current = resolveChartEmoteKeys(selection, topEmotes, viewMode)

  if (current.has(key)) {
    const next = new Set(current)
    next.delete(key)
    return next.size === 0 ? 'none' : next
  }

  if (current.size >= MAX_PLOTTED_EMOTES) return selection

  const next = new Set(current)
  next.add(key)
  return next
}

export function activityZoneFraction(expanded: boolean): number {
  return expanded ? 0.56 : 0.36
}

export function activityBandFractions(
  expanded: boolean,
  hasPlottedEmotes = false,
): {
  chat: number
  trace: number
  bars: number
} {
  if (expanded) {
    if (hasPlottedEmotes) {
      return { chat: 0.34, trace: 0.28, bars: 0.38 }
    }
    return { chat: 0.36, trace: 0.24, bars: 0.4 }
  }
  if (hasPlottedEmotes) {
    return { chat: 0.4, trace: 0.32, bars: 0.28 }
  }
  return { chat: 0.48, trace: 0.18, bars: 0.34 }
}

export interface ActivityLayout {
  zoneFraction: number
  chat: number
  trace: number
  bars: number
}

/** Interpolate collapsed (0) → expanded (1) activity band layout. */
export function lerpActivityLayout(progress: number, hasPlottedEmotes = false): ActivityLayout {
  const t = Math.max(0, Math.min(1, progress))
  const collapsed = activityBandFractions(false, hasPlottedEmotes)
  const expanded = activityBandFractions(true, hasPlottedEmotes)
  const zoneCollapsed = activityZoneFraction(false)
  const zoneExpanded = activityZoneFraction(true)
  return {
    zoneFraction: zoneCollapsed + (zoneExpanded - zoneCollapsed) * t,
    chat: collapsed.chat + (expanded.chat - collapsed.chat) * t,
    trace: collapsed.trace + (expanded.trace - collapsed.trace) * t,
    bars: collapsed.bars + (expanded.bars - collapsed.bars) * t,
  }
}
