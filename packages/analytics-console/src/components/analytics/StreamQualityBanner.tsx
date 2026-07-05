import type { StreamQualityDiagnosis } from '../../utils/streamQuality.ts'

export function StreamQualityBanner({
  diagnosis,
  syncing,
  canSync,
  onSync,
  onSyncViewers,
}: {
  diagnosis: StreamQualityDiagnosis | null
  syncing?: boolean
  canSync?: boolean
  onSync?: () => void
  onSyncViewers?: () => void
}) {
  if (!diagnosis) return null

  const tone =
    diagnosis.issues.includes('syncing')
      ? 'border-violet-500/25 bg-violet-500/[0.08] text-violet-100'
      : diagnosis.issues.includes('stats_only') || diagnosis.issues.includes('viewer_resync')
        ? 'border-amber-500/25 bg-amber-500/[0.08] text-amber-100'
        : diagnosis.issues.includes('refresh_only_hint')
          ? 'border-white/10 bg-white/[0.03] text-zinc-400'
          : 'border-cyan-500/20 bg-cyan-500/[0.06] text-cyan-100'

  const showAction =
    canSync
    && !syncing
    && diagnosis.suggestedAction !== 'wait_sync'
    && diagnosis.suggestedAction !== 'none'
    && diagnosis.actionLabel

  const handleAction = () => {
    if (diagnosis.suggestedAction === 'sync_viewers') {
      onSyncViewers?.()
      return
    }
    onSync?.()
  }

  return (
    <div
      className={`flex flex-col gap-2 rounded border px-3 py-2.5 text-[12px] font-semibold leading-snug sm:flex-row sm:items-center sm:justify-between ${tone}`}
      role="status"
    >
      <p>{diagnosis.message}</p>
      {showAction ? (
        <button
          type="button"
          onClick={handleAction}
          disabled={syncing}
          className="shrink-0 rounded border border-violet-400/30 bg-violet-500/10 px-3 py-1.5 text-[10px] font-black uppercase tracking-wide text-violet-200 transition hover:bg-violet-500/20 disabled:opacity-50"
        >
          {diagnosis.actionLabel}
        </button>
      ) : null}
    </div>
  )
}
