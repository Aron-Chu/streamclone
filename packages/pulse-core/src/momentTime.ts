import { formatHeatOffset } from './liveHeat.ts'

export interface MomentTimeLabel {
  primary: string
  secondary?: string
}

/** Local wall-clock primary label with VOD offset secondary when stream start is known. */
export function formatMomentTimeLabel(args: {
  startedAtIso?: string
  offsetSeconds: number
}): MomentTimeLabel {
  const offsetSeconds = Math.max(0, Math.floor(args.offsetSeconds))
  const offset = formatHeatOffset(offsetSeconds)
  const startedAtIso = args.startedAtIso?.trim()
  if (!startedAtIso) {
    return { primary: offset }
  }
  const startMs = Date.parse(startedAtIso)
  if (!Number.isFinite(startMs)) {
    return { primary: offset }
  }
  const primary = new Date(startMs + offsetSeconds * 1000).toLocaleString(undefined, {
    weekday: 'short',
    hour: 'numeric',
    minute: '2-digit',
  })
  return {
    primary,
    secondary: `${offset} into stream`,
  }
}
