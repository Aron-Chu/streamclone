import { clipsEmptyState, type ClipsRollup } from '../../utils/clipsEmptyState'

export interface ClipsTabEmptyStateProps {
  /** Per-minute rollups for the selected stream. */
  rollups: ClipsRollup[]
  /** Number of clip jobs currently queued/known for this stream. */
  clipJobCount: number
  /** Invoked when the user activates the sync action on the sync-first state (35.1). */
  onSync?: () => void
  /** Invoked when the user chooses to open the Moments tab on the use-moments state (35.2). */
  onOpenMoments?: () => void
}

/**
 * Honest empty state for the Clips tab (Requirement 35).
 *
 * Renders nothing when clip jobs already exist; otherwise shows either a
 * sync-first prompt (no chat/emote rollups) or a Moments/heatmap hint (rollups
 * exist but no clip jobs). Never directs the user to an empty graph.
 */
export function ClipsTabEmptyState({
  rollups,
  clipJobCount,
  onSync,
  onOpenMoments,
}: ClipsTabEmptyStateProps) {
  const content = clipsEmptyState({ rollups, clipJobCount })
  if (!content) return null

  return (
    <div
      className="grid place-items-center rounded-lg border border-white/10 bg-white/[0.035] px-4 py-8 text-center"
      data-variant={content.variant}
    >
      <div className="max-w-xs">
        <div className="text-sm font-black text-zinc-200">{content.title}</div>
        <p className="mt-1.5 text-[11px] font-semibold leading-snug text-zinc-500">{content.body}</p>
        {content.showSyncAction ? (
          onSync ? (
            <button
              type="button"
              onClick={onSync}
              className="mt-3 inline-flex items-center justify-center rounded-md bg-violet-600 px-3 py-1.5 text-[11px] font-black text-white transition-colors hover:bg-violet-500"
            >
              Sync chat &amp; emotes
            </button>
          ) : null
        ) : onOpenMoments ? (
          <button
            type="button"
            onClick={onOpenMoments}
            className="mt-3 inline-flex items-center justify-center rounded-md border border-white/10 bg-white/[0.04] px-3 py-1.5 text-[11px] font-black text-zinc-200 transition-colors hover:bg-white/[0.08]"
          >
            Open Moments
          </button>
        ) : null}
      </div>
    </div>
  )
}

export default ClipsTabEmptyState
