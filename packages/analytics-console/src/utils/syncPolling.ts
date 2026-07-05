import { getSyncStatus } from '../api.ts'
import type { SyncPhase, SyncStatus } from '../apiTypes.ts'

export function isTerminalSyncPhase(phase: SyncPhase | undefined): boolean {
  return phase === 'completed' || phase === 'export_pending' || phase === 'failed'
}

export async function pollSyncUntilDone(
  streamId: string,
  onUpdate: (status: SyncStatus | null) => void,
  opts?: { onProgress?: (status: SyncStatus) => void },
): Promise<SyncStatus | null> {
  let lastGood: SyncStatus | null = null
  let consecutiveFailures = 0
  let lastRollupsWritten = -1

  for (;;) {
    let status: SyncStatus | null = null
    try {
      status = await getSyncStatus(streamId)
      consecutiveFailures = 0
    } catch {
      consecutiveFailures++
      if (lastGood) {
        onUpdate({ ...lastGood, message: 'Reconnecting to sync status…' })
      } else {
        onUpdate(null)
      }
      if (consecutiveFailures > 10) return lastGood
      await new Promise((resolve) => setTimeout(resolve, 2000))
      continue
    }

    if (status) lastGood = status
    onUpdate(status)

    const rollupsWritten = (status as SyncStatus & { rollupsWritten?: number })?.rollupsWritten ?? 0
    if (status && rollupsWritten !== lastRollupsWritten) {
      lastRollupsWritten = rollupsWritten
      opts?.onProgress?.(status)
    }

    if (!status || isTerminalSyncPhase(status.phase) || status.stale) {
      return status
    }
    await new Promise((resolve) => setTimeout(resolve, 2000))
  }
}
