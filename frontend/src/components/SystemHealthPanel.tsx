import { useState } from 'react'
import { useSystemHealth } from '../hooks/useSystemHealth'
import OptionalServicesPanel from './OptionalServicesPanel'

const INSTALL_GUIDE_URL = 'https://github.com/Aron-Chu/streamclone/blob/master/docs/install-desktop.md'

function StatusChip({ label, good, loading }: { label: string; good: boolean; loading?: boolean }) {
  return (
    <span className={`rounded px-2 py-1 text-[10px] font-black uppercase tracking-wide ${
      loading ? 'bg-zinc-500/20 text-zinc-300' : good ? 'bg-emerald-400/15 text-emerald-100' : 'bg-amber-400/15 text-amber-100'
    }`}>
      {label}
    </span>
  )
}

type SystemHealthPanelProps = {
  variant?: 'full' | 'compact'
  onDismiss?: () => void
  onBrowse?: () => void
}

export default function SystemHealthPanel({ variant = 'full', onDismiss, onBrowse }: SystemHealthPanelProps) {
  const full = variant === 'full'
  const health = useSystemHealth({ probeHost: full, probeControl: full })
  const [copied, setCopied] = useState(false)

  const copyDiagnostics = async () => {
    const text = JSON.stringify(health.diagnosticsBundle, null, 2)
    try {
      await navigator.clipboard.writeText(text)
      setCopied(true)
      window.setTimeout(() => setCopied(false), 2000)
    } catch {
      setCopied(false)
    }
  }

  if (variant === 'compact') {
    return (
      <div className="space-y-3">
        <div className="flex flex-wrap gap-1.5">
          <StatusChip label="Core" good={health.coreReady} loading={health.statusLoading} />
        </div>
      </div>
    )
  }

  return (
    <div className="space-y-5">
      <OptionalServicesPanel variant="overlay" onDismiss={onDismiss} onBrowse={onBrowse} />

      <div className="rounded-lg border border-white/10 bg-white/[0.03] p-4">
        <div className="mb-3 flex flex-wrap items-center justify-between gap-2">
          <div className="text-sm font-black text-white">System status</div>
          <button
            type="button"
            onClick={() => void health.refreshAll()}
            className="rounded border border-white/10 bg-white/[0.06] px-3 py-1.5 text-xs font-black text-zinc-200 transition hover:bg-white/10"
          >
            Refresh diagnostics
          </button>
        </div>
        <div className="mb-4 flex flex-wrap gap-1.5">
          <StatusChip label="Core" good={health.coreReady} loading={health.metadata.isLoading && !health.metadata.data} />
          <StatusChip label="Install helper" good={health.installHelperReady} loading={health.control.isLoading} />
          <StatusChip label="Docker" good={health.dockerReady} loading={health.host.isLoading} />
        </div>

        {health.host.data?.containers?.length ? (
          <div className="mb-4 overflow-x-auto">
            <div className="mb-2 text-[11px] font-black uppercase text-zinc-500">Containers</div>
            <table className="min-w-full text-left text-xs">
              <thead>
                <tr className="text-zinc-500">
                  <th className="pb-2 pr-4 font-black uppercase">Name</th>
                  <th className="pb-2 font-black uppercase">Status</th>
                </tr>
              </thead>
              <tbody>
                {health.host.data.containers.map((c) => (
                  <tr key={c.name} className="border-t border-white/5 text-zinc-300">
                    <td className="py-1.5 pr-4 font-mono text-[11px]">{c.name}</td>
                    <td className="py-1.5 text-[11px]">{c.status}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        ) : null}

        <div className="flex flex-wrap items-center gap-2">
          <button
            type="button"
            onClick={() => void copyDiagnostics()}
            className="rounded border border-white/10 bg-white/[0.06] px-3 py-1.5 text-xs font-black text-zinc-200 transition hover:bg-white/10"
          >
            {copied ? 'Copied' : 'Copy diagnostics'}
          </button>
          <a
            href={INSTALL_GUIDE_URL}
            target="_blank"
            rel="noreferrer"
            className="rounded border border-white/10 px-3 py-1.5 text-xs font-black text-zinc-400 transition hover:bg-white/10 hover:text-zinc-200"
          >
            Install guide
          </a>
        </div>
      </div>
    </div>
  )
}
