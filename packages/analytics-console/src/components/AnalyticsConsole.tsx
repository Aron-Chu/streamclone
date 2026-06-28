import { useEffect, useMemo, useState } from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { useQuery } from '@tanstack/react-query'
import AnalyticsChart, { type AnalyticsViewMode } from './analytics/AnalyticsChart.tsx'
import {
  getAnalyticsLive,
  getAnalyticsStream,
  getAnalyticsStreams,
  getStreamGameSegments,
  getSyncStatus,
  startHistoricalSync,
  watchAnalyticsChannel,
  type AnalyticsMinuteRollup,
} from '../api.ts'
import { useAnalyticsLive } from '../hooks/useAnalyticsLive.ts'
import { syncCtaLabel } from '../utils/syncLabel.ts'

export function AnalyticsConsole() {
  const navigate = useNavigate()
  const { login = '', streamId = '' } = useParams<{ login: string; streamId?: string }>()
  const channelLogin = login.trim().toLowerCase()
  const [selectedEmotes, setSelectedEmotes] = useState<Set<string>>(new Set())
  const [selectedRollup, setSelectedRollup] = useState<AnalyticsMinuteRollup | null>(null)
  const [viewMode, setViewMode] = useState<AnalyticsViewMode>('overview')
  const [syncing, setSyncing] = useState(false)
  const [syncError, setSyncError] = useState<string | null>(null)
  const [syncNotice, setSyncNotice] = useState<string | null>(null)

  useEffect(() => {
    if (!channelLogin) return
    watchAnalyticsChannel(channelLogin).catch(() => undefined)
  }, [channelLogin])

  const liveQuery = useAnalyticsLive(channelLogin, { enabled: !streamId })
  const streamsQuery = useQuery({
    queryKey: ['analytics-console-streams', channelLogin],
    queryFn: () => getAnalyticsStreams(channelLogin, 20),
    enabled: Boolean(channelLogin),
    staleTime: 30_000,
  })

  const activeStreamId = streamId || liveQuery.data?.stream?.streamId || streamsQuery.data?.items?.[0]?.streamId || ''

  const detailQuery = useQuery({
    queryKey: ['analytics-console-detail', activeStreamId, channelLogin],
    queryFn: async () => {
      if (!activeStreamId) {
        if (!streamId && channelLogin) {
          const live = await getAnalyticsLive(channelLogin)
          return live
        }
        return null
      }
      const detail = await getAnalyticsStream(activeStreamId, { sparse: false, channel: channelLogin })
      return detail
    },
    enabled: Boolean(channelLogin) && (Boolean(activeStreamId) || !streamId),
    staleTime: 15_000,
  })

  const gamesQuery = useQuery({
    queryKey: ['analytics-console-games', activeStreamId],
    queryFn: () => getStreamGameSegments(activeStreamId),
    enabled: Boolean(activeStreamId),
    staleTime: 60_000,
  })

  const syncQuery = useQuery({
    queryKey: ['analytics-console-sync', activeStreamId],
    queryFn: () => getSyncStatus(activeStreamId),
    enabled: Boolean(activeStreamId),
    refetchInterval: syncing ? 2_000 : false,
  })

  const detail = detailQuery.data ?? undefined
  const isLive = !streamId && (detail?.state === 'live' || Boolean(liveQuery.data?.stream?.streamId))

  const syncLabel = useMemo(() => {
    const rollups = detail?.rollups ?? []
    const hasChat = rollups.some((row) => row.chatCount > 0 || row.totalEmoteCount > 0)
    const hasViewers = rollups.some((row) => row.viewerSamples > 0 || row.viewerAvg > 0)
    return syncCtaLabel({
      syncing,
      hasChatRollups: hasChat,
      hasViewerSamples: hasViewers,
    })
  }, [detail?.rollups, syncing])

  async function handleSync() {
    if (!activeStreamId) return
    setSyncing(true)
    setSyncError(null)
    setSyncNotice(null)
    try {
      await startHistoricalSync(activeStreamId, channelLogin)
      setSyncNotice('Sync started')
      await syncQuery.refetch()
    } catch (err) {
      setSyncError(err instanceof Error ? err.message : 'Sync failed')
    } finally {
      setSyncing(false)
    }
  }

  if (!channelLogin) {
    return (
      <section className="panel" aria-label="Streamclone analytics console">
        <p className="muted">Missing channel login.</p>
      </section>
    )
  }

  return (
    <section className="panel analytics-console" aria-label={`Analytics for ${channelLogin}`}>
      <div className="analytics-console__toolbar">
        <div className="analytics-console__streams">
          {(streamsQuery.data?.items ?? []).slice(0, 8).map((item) => {
            const href = `/analytics/${encodeURIComponent(channelLogin)}/s/${encodeURIComponent(item.streamId)}`
            const active = item.streamId === activeStreamId
            return (
              <Link
                key={item.streamId}
                to={href}
                className={active ? 'analytics-console__stream-link is-active' : 'analytics-console__stream-link'}
              >
                {item.title?.trim() || item.startedAt.slice(0, 10)}
              </Link>
            )
          })}
        </div>
        {!streamId && liveQuery.data?.stream?.streamId ? (
          <button
            type="button"
            className="btn btn-secondary"
            onClick={() =>
              navigate(`/analytics/${encodeURIComponent(channelLogin)}/s/${encodeURIComponent(liveQuery.data!.stream!.streamId)}`)
            }
          >
            Open live session
          </button>
        ) : null}
      </div>

      <AnalyticsChart
        detail={detail}
        selectedEmotes={selectedEmotes}
        onSelectEmote={(key) => {
          setSelectedEmotes((prev) => {
            const next = new Set(prev)
            if (next.has(key)) next.delete(key)
            else next.add(key)
            return next
          })
        }}
        selectedRollup={selectedRollup}
        onSelectRollup={setSelectedRollup}
        syncing={syncing}
        syncError={syncError}
        syncNotice={syncNotice}
        onSync={handleSync}
        onRefresh={() => {
          void detailQuery.refetch()
          void liveQuery.refetch()
        }}
        refreshing={detailQuery.isFetching}
        loading={detailQuery.isLoading || liveQuery.isLoading}
        games={gamesQuery.data ?? []}
        canSync={Boolean(activeStreamId)}
        isLive={isLive}
        syncCtaLabel={syncLabel}
        syncViewerStatus={syncQuery.data?.viewerStatus}
        viewMode={viewMode}
        onViewModeChange={setViewMode}
      />
    </section>
  )
}
