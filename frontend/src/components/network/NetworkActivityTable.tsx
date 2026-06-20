import { useMemo, useState } from 'react'
import { Link } from 'react-router-dom'
import { useQueryClient } from '@tanstack/react-query'
import { setAlwaysTracked, stopSetupService, stopStream } from '../../api'
import {
  filterNodesByTab,
  formatBytes,
  formatRate,
  summarizeActivityNodes,
  type NetworkActivityNode,
  type NetworkActivityTab,
} from '../../utils/networkActivityModel'
import type { NetworkTaskDisableAction } from '../../utils/networkTaskManager'
import AnalyticsSyncBreakdownRow from './AnalyticsSyncBreakdownRow'
import NetworkSparkline from './NetworkSparkline'

const IMPACT_STYLES = {
  high: 'border-rose-400/30 bg-rose-500/10 text-rose-100',
  medium: 'border-amber-300/25 bg-amber-500/10 text-amber-50',
  low: 'border-emerald-400/20 bg-emerald-500/10 text-emerald-100',
  unknown: 'border-white/10 bg-white/[0.03] text-zinc-400',
} as const

type SortKey = 'rate' | 'bytes' | 'impact' | 'name'

export interface NetworkActivityTableProps {
  nodes: NetworkActivityNode[]
  activeTab: NetworkActivityTab
  onPauseMonitoring?: () => void
  onActionComplete?: () => void
}

export default function NetworkActivityTable({
  nodes,
  activeTab,
  onPauseMonitoring,
  onActionComplete,
}: NetworkActivityTableProps) {
  const [sortKey, setSortKey] = useState<SortKey>('rate')
  const [expanded, setExpanded] = useState<Record<string, boolean>>({})
  const [pending, setPending] = useState<NetworkActivityNode | null>(null)
  const [busyId, setBusyId] = useState<string | null>(null)
  const [actionError, setActionError] = useState<string | null>(null)
  const queryClient = useQueryClient()

  const summary = useMemo(() => summarizeActivityNodes(nodes), [nodes])

  const filtered = useMemo(() => filterNodesByTab(nodes, activeTab), [nodes, activeTab])

  const topLevel = useMemo(() => {
    const parents = filtered.filter(node => !node.parentId)
    const impactRank = { high: 0, medium: 1, low: 2, unknown: 3 }
    return [...parents].sort((a, b) => {
      if (sortKey === 'name') return a.name.localeCompare(b.name)
      if (sortKey === 'bytes') return (b.bytesTotal ?? 0) - (a.bytesTotal ?? 0)
      if (sortKey === 'impact') return impactRank[a.impact] - impactRank[b.impact]
      return (b.bytesPerSec ?? 0) - (a.bytesPerSec ?? 0)
    })
  }, [filtered, sortKey])

  const childrenByParent = useMemo(() => {
    const map = new Map<string, NetworkActivityNode[]>()
    for (const node of filtered) {
      if (!node.parentId) continue
      const list = map.get(node.parentId) ?? []
      list.push(node)
      map.set(node.parentId, list)
    }
    return map
  }, [filtered])

  const runAction = async (node: NetworkActivityNode, action: NetworkTaskDisableAction) => {
    setBusyId(node.id)
    setActionError(null)
    try {
      if (action.kind === 'stop-relay') {
        await stopStream(action.channel)
      } else if (action.kind === 'untrack-channel') {
        await setAlwaysTracked(action.channel, false)
        void queryClient.invalidateQueries({ queryKey: ['always-tracked'] })
        void queryClient.invalidateQueries({ queryKey: ['analytics-always-tracked'] })
      } else if (action.kind === 'stop-optional') {
        await stopSetupService(action.service)
      } else if (action.kind === 'pause-page-monitoring') {
        onPauseMonitoring?.()
      }
      setPending(null)
      onActionComplete?.()
    } catch (err) {
      setActionError(err instanceof Error ? err.message : 'Action failed')
    } finally {
      setBusyId(null)
    }
  }

  const toggleExpanded = (id: string) => {
    setExpanded(current => ({ ...current, [id]: !current[id] }))
  }

  return (
    <section className="rounded-xl border border-white/10 bg-[#0e0e10] p-4">
      <div className="mb-3 flex flex-wrap items-center justify-between gap-2">
        <div>
          <div className="text-[11px] font-black uppercase tracking-wide text-zinc-500">Activity table</div>
          <p className="mt-1 text-xs font-semibold text-zinc-500">
            Measured bytes and live rates — expand analytics sync rows for sub-steps.
          </p>
        </div>
        <div className="flex flex-wrap gap-2 text-[10px] font-black uppercase">
          <span className="rounded border border-white/10 px-2 py-1 text-zinc-400">{summary.activeCount} active</span>
          {summary.highImpact ? (
            <span className="rounded border border-rose-400/30 bg-rose-500/10 px-2 py-1 text-rose-100">
              {summary.highImpact} high impact
            </span>
          ) : null}
        </div>
      </div>

      <div className="mb-3 flex flex-wrap gap-2">
        {([
          ['rate', 'Rate'],
          ['bytes', 'Total bytes'],
          ['impact', 'Impact'],
          ['name', 'Name'],
        ] as const).map(([key, label]) => (
          <button
            key={key}
            type="button"
            onClick={() => setSortKey(key)}
            className={`rounded border px-2 py-1 text-[10px] font-black uppercase ${
              sortKey === key ? 'border-white/20 bg-white/10 text-white' : 'border-white/10 text-zinc-500'
            }`}
          >
            Sort: {label}
          </button>
        ))}
      </div>

      {actionError ? (
        <div className="mb-3 rounded-lg border border-rose-400/30 bg-rose-500/10 px-3 py-2 text-xs font-semibold text-rose-50">
          {actionError}
        </div>
      ) : null}

      {!topLevel.length ? (
        <div className="text-sm font-semibold text-zinc-500">No activity in this category yet.</div>
      ) : (
        <div className="overflow-x-auto">
          <table className="w-full min-w-[720px] border-collapse text-left text-sm">
            <thead>
              <tr className="border-b border-white/10 text-[10px] font-black uppercase text-zinc-500">
                <th className="px-2 py-2">Name</th>
                <th className="px-2 py-2">Rate</th>
                <th className="px-2 py-2">Total</th>
                <th className="px-2 py-2 w-28">60s</th>
                <th className="px-2 py-2">Impact</th>
                <th className="px-2 py-2 text-right">Action</th>
              </tr>
            </thead>
            <tbody>
              {topLevel.map(node => {
                const children = childrenByParent.get(node.id) ?? []
                const isAnalytics = node.category === 'analytics' && children.length > 0
                const isExpanded = expanded[node.id] ?? isAnalytics

                return (
                  <tr key={node.id} className="border-b border-white/5 align-top">
                    <td className="px-2 py-3">
                      <div className="font-black text-white">{node.name}</div>
                      {node.detail ? (
                        <p className="mt-1 max-w-md text-xs font-semibold text-zinc-500">{node.detail}</p>
                      ) : null}
                      {node.throughputHint ? (
                        <p className="mt-1 font-mono text-[11px] text-zinc-400">{node.throughputHint}</p>
                      ) : null}
                      {node.category === 'stream' && node.disableAction?.kind === 'stop-relay' ? (
                        <Link
                          to={`/c/${encodeURIComponent(node.disableAction.channel)}`}
                          className="mt-2 inline-block text-[11px] font-black uppercase text-violet-300 transition hover:text-violet-100"
                        >
                          Open channel →
                        </Link>
                      ) : null}
                      {isAnalytics ? (
                        <div className="mt-2">
                          <AnalyticsSyncBreakdownRow
                            parent={node}
                            children={children}
                            expanded={isExpanded}
                            onToggle={() => toggleExpanded(node.id)}
                          />
                        </div>
                      ) : null}
                    </td>
                    <td className="px-2 py-3 font-mono text-xs text-zinc-200">{formatRate(node.bytesPerSec)}</td>
                    <td className="px-2 py-3 font-mono text-xs text-zinc-200">{formatBytes(node.bytesTotal)}</td>
                    <td className="px-2 py-3">
                      <NetworkSparkline series={node.sparkline ?? []} height={36} />
                    </td>
                    <td className="px-2 py-3">
                      <span className={`rounded border px-1.5 py-0.5 text-[10px] font-black uppercase ${IMPACT_STYLES[node.impact]}`}>
                        {node.impact}
                      </span>
                    </td>
                    <td className="px-2 py-3 text-right">
                      {node.canDisable && node.disableAction && node.disableLabel ? (
                        <button
                          type="button"
                          disabled={busyId === node.id}
                          onClick={() => setPending(node)}
                          className="rounded border border-white/10 px-2.5 py-1.5 text-[11px] font-black uppercase text-zinc-200 transition hover:bg-white/10 disabled:opacity-50"
                        >
                          {busyId === node.id ? 'Working…' : node.disableLabel}
                        </button>
                      ) : null}
                    </td>
                  </tr>
                )
              })}
            </tbody>
          </table>
        </div>
      )}

      {pending?.disableWarning ? (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/70 p-4">
          <div className="w-full max-w-md rounded-xl border border-white/10 bg-[#12121a] p-5 shadow-2xl">
            <div className="text-sm font-black text-white">Stop {pending.name}?</div>
            <p className="mt-2 text-sm font-semibold leading-6 text-zinc-400">{pending.disableWarning}</p>
            {pending.bytesPerSec ? (
              <p className="mt-2 font-mono text-xs text-zinc-500">
                Current rate: {formatRate(pending.bytesPerSec)}
              </p>
            ) : null}
            <div className="mt-4 flex flex-wrap justify-end gap-2">
              <button
                type="button"
                onClick={() => setPending(null)}
                className="rounded border border-white/10 px-3 py-2 text-xs font-black text-zinc-300 transition hover:bg-white/10"
              >
                Cancel
              </button>
              <button
                type="button"
                disabled={busyId === pending.id}
                onClick={() => pending.disableAction && void runAction(pending, pending.disableAction)}
                className="rounded border border-rose-400/40 bg-rose-500/15 px-3 py-2 text-xs font-black text-rose-100 transition hover:bg-rose-500/25 disabled:opacity-50"
              >
                {pending.disableLabel ?? 'Confirm'}
              </button>
            </div>
          </div>
        </div>
      ) : null}
    </section>
  )
}
