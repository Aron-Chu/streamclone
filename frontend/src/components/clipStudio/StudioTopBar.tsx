import { Link } from 'react-router-dom'
import type { ClipperJob } from '../../api'
import StackStatusButton from '../StackStatusButton'
import type { FormatPreset, PreviewMode, RenderStatus } from './types'
import { formatHighlightRange, spikePositionInSource } from './utils'
import { FORMAT_OPTIONS } from './studioTheme'

interface StudioTopBarProps {
  job: ClipperJob
  trimStart: number
  trimEnd: number
  duration: number
  canPreviewSource: boolean
  canPreviewFinal: boolean
  sourceUrl: string
  finalUrl: string
  previewMode: PreviewMode
  formatPreset: FormatPreset
  renderStatus: RenderStatus
  onPreviewModeChange: (mode: PreviewMode) => void
  onFormatPresetChange: (preset: FormatPreset) => void
  onExport: () => void
  exportDisabled: boolean
}

export function StudioTopBar({
  job,
  trimStart,
  trimEnd,
  duration,
  canPreviewSource,
  canPreviewFinal,
  sourceUrl,
  finalUrl,
  previewMode,
  formatPreset,
  renderStatus,
  onPreviewModeChange,
  onFormatPresetChange,
  onExport,
  exportDisabled,
}: StudioTopBarProps) {
  const ctx = job.moment_context
  const streamTime = ctx?.minute_ts
    ? new Date(ctx.minute_ts).toLocaleTimeString([], { hour: 'numeric', minute: '2-digit' })
    : null
  const spikePos = spikePositionInSource(job, duration)
  const showHighlight = spikePos != null
  const reason = ctx?.pick_reason?.replace(/_/g, ' ') ?? 'moment'

  return (
    <header className="flex shrink-0 flex-wrap items-center gap-3 border-b border-white/[0.08] bg-[#0d0d12]/90 px-4 py-2.5 backdrop-blur-md">
      <div className="flex min-w-0 flex-1 items-center gap-3">
        <StackStatusButton className="!text-[10px]" />
        <Link
          to={`/analytics/${encodeURIComponent(job.channel)}`}
          className="flex items-center gap-1 text-xs text-zinc-400 transition hover:text-zinc-100"
        >
          <svg width="14" height="14" fill="currentColor" viewBox="0 0 16 16" aria-hidden="true">
            <path fillRule="evenodd" d="M15 8a.5.5 0 0 0-.5-.5H2.707l3.147-3.146a.5.5 0 1 0-.708-.708l-4 4a.5.5 0 0 0 0 .708l4 4a.5.5 0 0 0 .708-.708L2.707 8.5H14.5A.5.5 0 0 0 15 8z"/>
          </svg>
          Back
        </Link>
        <div className="flex min-w-0 items-center gap-2">
          <span className="flex h-8 w-8 shrink-0 items-center justify-center rounded-full bg-violet-500/20 text-sm font-bold text-violet-300">
            {job.channel.charAt(0).toUpperCase()}
          </span>
          <div className="min-w-0">
            <h1 className="truncate text-sm font-bold text-zinc-100">
              {job.title || `${job.channel} clip`}
            </h1>
            <p className="truncate text-[11px] text-zinc-500">
              {job.channel}
              {streamTime ? ` · ${streamTime}` : ''}
              <span className="ml-1 text-emerald-400/80">#{job.id.substring(0, 8)}</span>
            </p>
          </div>
        </div>
      </div>

      {showHighlight && (
        <div className="hidden items-center gap-1.5 rounded-full border border-rose-500/25 bg-rose-500/10 px-2.5 py-1 text-[10px] font-medium text-rose-200 sm:flex">
          <span className="h-1.5 w-1.5 rounded-full bg-rose-400" />
          {reason} · {formatHighlightRange(trimStart, trimEnd)}
        </div>
      )}

      <div className="flex flex-wrap items-center gap-2">
        <div className="flex rounded-lg border border-white/10 bg-black/30 p-0.5">
          <button
            type="button"
            disabled={!canPreviewSource}
            onClick={() => onPreviewModeChange('source')}
            className={`rounded-md px-2.5 py-1 text-[11px] font-semibold transition ${
              previewMode === 'source' ? 'bg-cyan-500/20 text-cyan-300' : 'text-zinc-500 hover:text-zinc-300'
            } disabled:opacity-40`}
          >
            Source
          </button>
          <button
            type="button"
            disabled={!canPreviewFinal}
            onClick={() => onPreviewModeChange('final')}
            className={`rounded-md px-2.5 py-1 text-[11px] font-semibold transition ${
              previewMode === 'final' ? 'bg-emerald-500/20 text-emerald-300' : 'text-zinc-500 hover:text-zinc-300'
            } disabled:opacity-40`}
          >
            Final
          </button>
        </div>

        <div className="hidden items-center gap-1 lg:flex">
          {FORMAT_OPTIONS.map(opt => (
            <button
              key={opt.id}
              type="button"
              title={opt.hint}
              onClick={() => onFormatPresetChange(opt.id)}
              className={`rounded px-2 py-0.5 text-[10px] font-semibold transition ${
                formatPreset === opt.id
                  ? 'bg-violet-500/20 text-violet-300 ring-1 ring-violet-400/30'
                  : 'text-zinc-500 hover:bg-white/5'
              }`}
            >
              {opt.hint}
            </button>
          ))}
        </div>

        {canPreviewSource && (
          <a
            href={sourceUrl}
            download
            className="hidden rounded-md border border-white/10 px-2.5 py-1 text-[11px] font-semibold text-zinc-400 hover:bg-white/5 sm:inline"
          >
            Source
          </a>
        )}
        {canPreviewFinal && (
          <a
            href={finalUrl}
            download
            className="hidden rounded-md border border-white/10 px-2.5 py-1 text-[11px] font-semibold text-zinc-400 hover:bg-white/5 sm:inline"
          >
            MP4
          </a>
        )}

        {renderStatus === 'rendering' && (
          <span className="text-[11px] font-medium text-cyan-400">Rendering…</span>
        )}

        <button
          type="button"
          onClick={onExport}
          disabled={exportDisabled}
          className="rounded-lg bg-gradient-to-r from-cyan-500 to-violet-500 px-4 py-1.5 text-xs font-bold text-white shadow-lg shadow-cyan-500/20 transition hover:brightness-110 disabled:opacity-40"
        >
          Export Short
        </button>
      </div>
    </header>
  )
}
