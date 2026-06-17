import { useMemo, useState } from 'react'
import { Link } from 'react-router-dom'
import { stopSetupService, stopStream } from '../../api'
import {
  networkTaskCategoryLabel,
  summarizeNetworkTasks,
  type NetworkTask,
  type NetworkTaskDisableAction,
} from '../../utils/networkTaskManager'

const IMPACT_STYLES = {
  high: 'border-rose-400/30 bg-rose-500/10 text-rose-100',
  medium: 'border-amber-300/25 bg-amber-500/10 text-amber-50',
  low: 'border-emerald-400/20 bg-emerald-500/10 text-emerald-100',
  unknown: 'border-white/10 bg-white/[0.03] text-zinc-400',
} as const

export interface NetworkTaskManagerPanelProps {
  tasks: NetworkTask[]
  onPauseMonitoring?: () => void
  onActionComplete?: () => void
}

export default function NetworkTaskManagerPanel({
  tasks,
  onPauseMonitoring,
  onActionComplete,
}: NetworkTaskManagerPanelProps) {
  const summary = useMemo(() => summarizeNetworkTasks(tasks), [tasks])
  const [pending, setPending] = useState<NetworkTask | null>(null)
  const [busyId, setBusyId] = useState<string | null>(null)
  const [actionError, setActionError] = useState<string | null>(null)

  const runAction = async (task: NetworkTask, action: NetworkTaskDisableAction) => {
    setBusyId(task.id)
    setActionError(null)
    try {
      if (action.kind === 'stop-relay') {
        await stopStream(action.channel)
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

  return (
    <section className="rounded-xl border border-white/10 bg-[#0e0e10] p-4">
      <div className="mb-3 flex flex-wrap items-center justify-between gap-2">
        <div>
          <div className="text-[11px] font-black uppercase tracking-wide text-zinc-500">Network task manager</div>
          <p className="mt-1 text-xs font-semibold text-zinc-500">
            What is using bandwidth right now — streams, site services, and this tab.
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

      {actionError ? (
        <div className="mb-3 rounded-lg border border-rose-400/30 bg-rose-500/10 px-3 py-2 text-xs font-semibold text-rose-50">
          {actionError}
        </div>
      ) : null}

      {!tasks.length ? (
        <div className="text-sm font-semibold text-zinc-500">No network tasks detected yet.</div>
      ) : (
        <div className="space-y-2">
          {tasks.map(task => (
            <article
              key={task.id}
              className="rounded-lg border border-white/10 bg-white/[0.03] px-3 py-3"
            >
              <div className="flex flex-wrap items-start justify-between gap-3">
                <div className="min-w-0 flex-1">
                  <div className="flex flex-wrap items-center gap-2">
                    <span className="text-sm font-black text-white">{task.name}</span>
                    <span className="rounded border border-white/10 px-1.5 py-0.5 text-[10px] font-black uppercase text-zinc-500">
                      {networkTaskCategoryLabel(task.category)}
                    </span>
                    <span className={`rounded border px-1.5 py-0.5 text-[10px] font-black uppercase ${IMPACT_STYLES[task.impact]}`}>
                      {task.impact} impact
                    </span>
                    {task.status === 'idle' ? (
                      <span className="text-[10px] font-black uppercase text-zinc-600">Paused</span>
                    ) : null}
                  </div>
                  <p className="mt-1 text-xs font-semibold leading-5 text-zinc-500">{task.detail}</p>
                  {task.throughputHint ? (
                    <p className="mt-1 text-[11px] font-mono text-zinc-400">{task.throughputHint}</p>
                  ) : null}
                  {task.category === 'stream' && task.disableAction?.kind === 'stop-relay' ? (
                    <Link
                      to={`/c/${encodeURIComponent(task.disableAction.channel)}`}
                      className="mt-2 inline-block text-[11px] font-black uppercase text-violet-300 transition hover:text-violet-100"
                    >
                      Open channel →
                    </Link>
                  ) : null}
                </div>
                {task.canDisable && task.disableAction && task.disableLabel ? (
                  <button
                    type="button"
                    disabled={busyId === task.id}
                    onClick={() => setPending(task)}
                    className="shrink-0 rounded border border-white/10 px-2.5 py-1.5 text-[11px] font-black uppercase text-zinc-200 transition hover:bg-white/10 disabled:opacity-50"
                  >
                    {busyId === task.id ? 'Working…' : task.disableLabel}
                  </button>
                ) : null}
              </div>
            </article>
          ))}
        </div>
      )}

      {pending?.disableWarning ? (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/70 p-4">
          <div className="w-full max-w-md rounded-xl border border-white/10 bg-[#12121a] p-5 shadow-2xl">
            <div className="text-sm font-black text-white">Stop {pending.name}?</div>
            <p className="mt-2 text-sm font-semibold leading-6 text-zinc-400">{pending.disableWarning}</p>
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
