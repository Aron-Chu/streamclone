/** Map absolute VOD time to seconds on the current relay timeline. */
export function vodRelativeSeekSeconds(absoluteOffset: number, relayBaseSeconds: number): number {
  const absolute = Number.isFinite(absoluteOffset) ? Math.max(0, Math.floor(absoluteOffset)) : 0
  const base = Number.isFinite(relayBaseSeconds) ? Math.max(0, relayBaseSeconds) : 0
  return Math.max(0, absolute - base)
}

export interface SeekableRange {
  start: number
  end: number
}

/**
 * True when relativeSec lies inside any seekable range (or within known duration).
 * VOD relays expose a short sliding HLS window — far jumps need a relay restart.
 */
export function isRelativeSeekInWindow(
  relativeSec: number,
  seekableRanges: SeekableRange[],
  durationSec?: number | null,
): boolean {
  if (!Number.isFinite(relativeSec) || relativeSec < 0) return false
  for (const range of seekableRanges) {
    if (relativeSec >= range.start - 0.5 && relativeSec <= range.end - 0.5) return true
  }
  if (durationSec != null && Number.isFinite(durationSec) && durationSec > 0) {
    return relativeSec <= durationSec - 0.5
  }
  return false
}

export function readVideoSeekableRanges(video: Pick<HTMLVideoElement, 'seekable'>): SeekableRange[] {
  const ranges: SeekableRange[] = []
  for (let i = 0; i < video.seekable.length; i++) {
    ranges.push({ start: video.seekable.start(i), end: video.seekable.end(i) })
  }
  return ranges
}

/**
 * True when the target lies outside the relay's seekable HLS window and needs a
 * backend relay restart instead of an in-player seek.
 */
export function needsVodRelayRestart(
  absoluteSec: number,
  relayBaseSec: number,
  seekableRanges: SeekableRange[],
  padSeconds = 30,
  durationSec?: number | null,
): boolean {
  const relative = vodRelativeSeekSeconds(absoluteSec, relayBaseSec)
  if (isRelativeSeekInWindow(relative, seekableRanges, durationSec)) {
    return false
  }
  if (!seekableRanges.length) {
    return relative >= padSeconds
  }
  return true
}

/** Index of the moment nearest the playhead (for highlight + prev/next). */
export function nearestMomentIndex(
  offsets: number[],
  currentOffsetSec: number,
): number {
  if (!offsets.length) return -1
  let best = 0
  let bestDist = Math.abs(offsets[0] - currentOffsetSec)
  for (let i = 1; i < offsets.length; i++) {
    const dist = Math.abs(offsets[i] - currentOffsetSec)
    if (dist < bestDist) {
      bestDist = dist
      best = i
    }
  }
  return best
}
