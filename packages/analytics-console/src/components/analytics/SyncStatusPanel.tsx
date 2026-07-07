import type { AnalyticsStreamDetail, SyncStatus } from '../../apiTypes.ts'
import { relativeTime } from '../../utils/consoleFormat.ts'
import { SourcePills } from './ConsoleBits.tsx'

function phaseLabel(phase: string): string {
  switch (phase) {
    case 'completed':
      return 'Collection complete'
    case 'failed':
      return 'Collection failed'
    case 'fetching_comments':
      return 'Fetching VOD chat'
    case 'writing_rollups':
      return 'Writing minute rollups'
    case 'scraping_tracker':
    case 'parsing_tracker':
      return 'Resolving viewer minutes'
    case 'resolving_vod':
      return 'Resolving VOD'
    case 'starting':
      return 'Starting'
    default:
      return phase.replace(/_/g, ' ')
  }
}

export function SyncStatusPanel({
  detail,
  syncStatus,
}: {
  detail?: AnalyticsStreamDetail
  syncStatus?: SyncStatus | null
}) {
  const coverage = detail?.chatCoverage
  const coveragePct = detail?.chatCoveragePct ?? coverage?.coveragePct
  const sources = detail?.sources ?? []

  return (
    <div className="space-y-3 p-3">
      {syncStatus && !syncStatus.stale ? (
        <div className="rounded border border-white/10 bg-white/[0.03] px-3 py-2.5">
          <div className="text-[11px] font-black uppercase text-zinc-500">Collection status</div>
          <div
            className={`mt-1 text-sm font-black ${
              syncStatus.phase === 'failed' ? 'text-red-300' : 'text-zinc-100'
            }`}
          >
            {phaseLabel(syncStatus.phase)}
          </div>
          {syncStatus.message ? (
            <div className="mt-1 text-[11px] font-semibold text-zinc-400">{syncStatus.message}</div>
          ) : null}
          {syncStatus.error ? (
            <div className="mt-1 text-[11px] font-bold text-red-300">{syncStatus.error}</div>
          ) : null}
          {syncStatus.updatedAt ? (
            <div className="mt-1 text-[10px] font-semibold text-zinc-600">
              Updated {relativeTime(syncStatus.updatedAt)}
            </div>
          ) : null}
        </div>
      ) : null}

      {coveragePct !== undefined && coveragePct > 0 ? (
        <div className="rounded border border-white/10 bg-white/[0.03] px-3 py-2.5">
          <div className="text-[11px] font-black uppercase text-zinc-500">Chat coverage</div>
          <div className="mt-1 text-sm font-black text-zinc-100">{Math.round(coveragePct)}% of stream minutes</div>
          {coverage?.partial ? (
            <div className="mt-1 text-[11px] font-semibold text-amber-200/90">
              Partial coverage — chat spans {coverage.chatSpanMinutes ?? 0} of {coverage.streamSpanMinutes ?? 0} minutes.
            </div>
          ) : null}
        </div>
      ) : null}

      {sources.length > 0 ? (
        <div className="rounded border border-white/10 bg-white/[0.03] px-3 py-2.5">
          <div className="mb-2 text-[11px] font-black uppercase text-zinc-500">Data sources</div>
          <SourcePills sources={sources} />
        </div>
      ) : null}

      {!syncStatus && (coveragePct === undefined || coveragePct <= 0) && sources.length === 0 ? (
        <div className="text-[11px] font-semibold text-zinc-500">
          Backend collection status appears here once this stream is tracked.
        </div>
      ) : null}
    </div>
  )
}
