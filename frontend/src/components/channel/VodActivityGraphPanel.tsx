import { useCallback, useMemo, useState } from 'react'

import { getSyncStatus, startHistoricalSync, type AnalyticsStream, type SyncStatus } from '../../api'
import { normalizeVodId } from '../../utils/vodId'

type VodActivityGraphPanelProps = {
  channelLogin: string
  vodId: string
  streamId: string | null
  streams: AnalyticsStream[]
  onStreamLinked: (streamId: string) => void
  onSyncComplete?: () => void
  className?: string
}

function isTerminalSyncPhase(phase: string | undefined): boolean {
  return phase === 'completed' || phase === 'failed' || phase === 'export_pending'
}

function syncStatusLabel(status: SyncStatus | null): string {
  if (!status) return 'Starting sync…'
  switch (status.phase) {
    case 'parsing_tracker':
      return 'Parsing TwitchTracker…'
    case 'resolving_vod':
      return 'Resolving VOD…'
    case 'fetching_comments':
      return 'Fetching chat…'
    case 'writing_rollups':
      return 'Writing minute rollups…'
    case 'exporting_archive':
      return 'Exporting archive…'
    case 'completed':
      return 'Sync complete'
    case 'failed':
      return status.error || 'Sync failed'
    default:
      return 'Syncing analytics…'
  }
}

export default function VodActivityGraphPanel({
  channelLogin,
  vodId,
  streamId,
  streams,
  onStreamLinked,
  onSyncComplete,
  className,
}: VodActivityGraphPanelProps) {
  const [syncing, setSyncing] = useState(false)
  const [status, setStatus] = useState<SyncStatus | null>(null)
  const [error, setError] = useState('')

  const resolvedStreamId = useMemo(() => {
    if (streamId) return streamId
    const normalizedVod = normalizeVodId(vodId)
    if (!normalizedVod) return null
    const match = streams.find(item => normalizeVodId(item.vodId) === normalizedVod)
    return match?.streamId ?? null
  }, [streamId, streams, vodId])

  const handleBuildGraph = useCallback(async () => {
    if (!channelLogin || !resolvedStreamId) {
      setError('No analytics stream is linked to this VOD yet. Open Analytics and sync the stream first.')
      return
    }
    setSyncing(true)
    setError('')
    setStatus(null)
    try {
      const start = await startHistoricalSync(resolvedStreamId, channelLogin, {
        vodId,
        forceChat: true,
      })
      if (start.status) setStatus(start.status)
      if (!streamId) onStreamLinked(resolvedStreamId)
      for (let attempt = 0; attempt < 180; attempt += 1) {
        await new Promise(resolve => window.setTimeout(resolve, 2000))
        const next = await getSyncStatus(resolvedStreamId).catch(() => null)
        if (!next) break
        setStatus(next)
        if (isTerminalSyncPhase(next.phase)) break
      }
      onSyncComplete?.()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to build activity graph')
    } finally {
      setSyncing(false)
    }
  }, [channelLogin, onStreamLinked, onSyncComplete, resolvedStreamId, streamId, vodId])

  return (
    <div
      className={`rounded-lg border border-white/10 bg-black/50 px-3 py-2 text-[11px] text-zinc-300 backdrop-blur-sm ${className ?? ''}`}
      data-testid="vod-activity-graph-panel"
    >
      <p className="font-semibold text-zinc-200">Activity chart unavailable</p>
      <p className="mt-1 text-zinc-500">
        Sync minute-level chat and emote rollups to unlock the activity chart in Pulse and moments.
      </p>
      {error ? <p className="mt-2 text-amber-200">{error}</p> : null}
      {syncing && status ? (
        <p className="mt-2 text-zinc-400">{syncStatusLabel(status)}</p>
      ) : null}
      <button
        type="button"
        disabled={syncing || !resolvedStreamId}
        onClick={() => void handleBuildGraph()}
        className="mt-2 rounded border border-violet-400/35 bg-violet-500/15 px-2.5 py-1 text-[10px] font-bold uppercase tracking-wide text-violet-100 transition hover:bg-violet-500/25 disabled:cursor-not-allowed disabled:opacity-50"
      >
        {syncing ? 'Building activity graph…' : 'Build activity graph'}
      </button>
      {!resolvedStreamId ? (
        <p className="mt-2 text-zinc-500">
          Track this channel in Analytics first so Streamclone can match this VOD to a stream id.
        </p>
      ) : null}
    </div>
  )
}
