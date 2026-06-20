import type { PlaybackLatencyMode } from './settings'

export interface LiveLatencyControllerInput {
  behindLiveSec: number | null
  targetLatencySec: number | null
  bufferSizeSec: number | null
  stalls: number
  fetchRatio: number | null
  userMode: PlaybackLatencyMode
  effectiveMode: PlaybackLatencyMode
  levelCount: number
  currentLevel: number
}

export interface LiveLatencyControllerOutput {
  effectiveMode: PlaybackLatencyMode
  maxLiveSyncPlaybackRate: number
  shouldJumpLive: boolean
  playbackRate: number
  levelCap: number | null
}

const HARD_JUMP_SEC = 8
const TARGET_SLACK_SEC = 1.5
const STABLE_UPGRADE_SEC = 30
const STALL_DOWNGRADE_THRESHOLD = 3
const FETCH_RATIO_CAP = 1.35
const MODE_TRANSITION_COOLDOWN_MS = 4000

const modeOrder: PlaybackLatencyMode[] = ['instant', 'fast', 'stable']

function modeIndex(mode: PlaybackLatencyMode) {
  return modeOrder.indexOf(mode)
}

function downgradeMode(mode: PlaybackLatencyMode): PlaybackLatencyMode {
  const idx = modeIndex(mode)
  return idx >= modeOrder.length - 1 ? mode : modeOrder[idx + 1]
}

function upgradeMode(mode: PlaybackLatencyMode): PlaybackLatencyMode {
  const idx = modeIndex(mode)
  return idx <= 0 ? mode : modeOrder[idx - 1]
}

function targetBehindSec(mode: PlaybackLatencyMode, llHls: boolean) {
  if (mode === 'instant') return llHls ? 1.5 : 2.5
  if (mode === 'fast') return llHls ? 2.5 : 4
  return llHls ? 4 : 6
}

function maxCatchUpRate(mode: PlaybackLatencyMode) {
  if (mode === 'instant') return 1.3
  if (mode === 'fast') return 1.2
  return 1.05
}

export function createLiveLatencyController(userMode: PlaybackLatencyMode, llHls = false) {
  let effectiveMode = userMode
  let stableSince = 0
  let lastModeChangeAt = -MODE_TRANSITION_COOLDOWN_MS
  let lastStallCount = 0
  let levelCap: number | null = null

  const tick = (input: LiveLatencyControllerInput): LiveLatencyControllerOutput => {
    const now = performance.now()
    const target = targetBehindSec(effectiveMode, llHls)
    const behind = input.behindLiveSec ?? 0
    const stallDelta = Math.max(0, input.stalls - lastStallCount)
    lastStallCount = input.stalls

    let shouldJumpLive = false
    let playbackRate = 1
    let maxLiveSyncPlaybackRate = maxCatchUpRate(effectiveMode)

    if (behind > HARD_JUMP_SEC) {
      shouldJumpLive = true
      playbackRate = 1
      stableSince = 0
    } else if (behind > target + TARGET_SLACK_SEC) {
      playbackRate = Math.min(maxCatchUpRate(effectiveMode), 1 + Math.min(0.3, (behind - target) / 10))
      stableSince = 0
    } else if (behind <= target) {
      playbackRate = 1
      if (stableSince === 0) stableSince = now
    } else {
      playbackRate = 1
      stableSince = 0
    }

    const fetchPressure = input.fetchRatio !== null && input.fetchRatio >= FETCH_RATIO_CAP
    if (fetchPressure && input.levelCount > 1 && input.currentLevel > 0) {
      levelCap = Math.max(0, input.currentLevel - 1)
    } else if (!fetchPressure && levelCap !== null && stableSince > 0 && now - stableSince >= STABLE_UPGRADE_SEC * 1000) {
      levelCap = null
    }

    const canChangeMode = now - lastModeChangeAt >= MODE_TRANSITION_COOLDOWN_MS
    if (canChangeMode && stallDelta >= STALL_DOWNGRADE_THRESHOLD && effectiveMode !== 'stable') {
      effectiveMode = downgradeMode(effectiveMode)
      lastModeChangeAt = now
      stableSince = 0
    } else if (
      canChangeMode
      && effectiveMode !== userMode
      && stableSince > 0
      && now - stableSince >= STABLE_UPGRADE_SEC * 1000
      && (input.bufferSizeSec ?? 0) >= target
      && stallDelta === 0
    ) {
      const next = upgradeMode(effectiveMode)
      if (modeIndex(next) < modeIndex(userMode)) {
        effectiveMode = next
        lastModeChangeAt = now
        stableSince = 0
      }
    }

    if (modeIndex(effectiveMode) < modeIndex(userMode)) {
      effectiveMode = userMode
    }

    maxLiveSyncPlaybackRate = maxCatchUpRate(effectiveMode)

    return {
      effectiveMode,
      maxLiveSyncPlaybackRate,
      shouldJumpLive,
      playbackRate,
      levelCap,
    }
  }

  const reset = (mode: PlaybackLatencyMode) => {
    effectiveMode = mode
    stableSince = 0
    lastModeChangeAt = -MODE_TRANSITION_COOLDOWN_MS
    lastStallCount = 0
    levelCap = null
  }

  return { tick, reset }
}
