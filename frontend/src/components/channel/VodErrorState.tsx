import { useEffect, useRef } from 'react'
import {
  describeVodError,
  HLS_NOT_READY_MAX_AUTO_RETRIES,
  isVodErrorRetryable,
  type VodErrorAction,
  type VodErrorActionKind,
  type VodErrorCode,
  type VodErrorContext,
  type VodErrorDescriptor,
  type VodErrorInput,
} from './vodError.ts'

// Re-export the pure error helpers and types so existing importers of this
// component module keep working. The implementation lives in vodError.ts
// (React-free) so it can be imported by the Node test runner, which cannot
// load .tsx files.
export {
  describeVodError,
  HLS_NOT_READY_MAX_AUTO_RETRIES,
  isVodErrorRetryable,
}
export type {
  VodErrorAction,
  VodErrorActionKind,
  VodErrorCode,
  VodErrorContext,
  VodErrorDescriptor,
  VodErrorInput,
}

export interface VodErrorStateProps {
  error: VodErrorInput
  channelLogin?: string | null
  vodId?: string | null
  fromAnalytics?: boolean
  onRetry?: () => void
  className?: string
}

/**
 * VodErrorState renders an actionable VOD relay failure panel. For the
 * hls_not_ready code it auto-retries up to HLS_NOT_READY_MAX_AUTO_RETRIES
 * times before falling back to a manual retry (Requirement 2.5). Auto-retry
 * state is keyed by code + vodId so switching VODs or error codes resets it.
 */
export function VodErrorState({
  error,
  channelLogin,
  vodId,
  fromAnalytics,
  onRetry,
  className,
}: VodErrorStateProps) {
  const descriptor = describeVodError(error, {
    channelLogin,
    vodId,
    fromAnalytics,
  })
  const autoRetryKey = `${descriptor.code}:${vodId ?? ''}`
  const autoRetryStateRef = useRef<{ key: string; count: number }>({ key: autoRetryKey, count: 0 })

  if (autoRetryStateRef.current.key !== autoRetryKey) {
    autoRetryStateRef.current = { key: autoRetryKey, count: 0 }
  }

  const isHlsNotReady = descriptor.code === 'hls_not_ready'

  useEffect(() => {
    if (!isHlsNotReady || !onRetry) return
    const state = autoRetryStateRef.current
    if (state.count >= HLS_NOT_READY_MAX_AUTO_RETRIES) return
    const timer = window.setTimeout(() => {
      autoRetryStateRef.current = { key: autoRetryKey, count: state.count + 1 }
      onRetry()
    }, 1500)
    return () => window.clearTimeout(timer)
  }, [autoRetryKey, isHlsNotReady, onRetry])

  const autoRetriesUsed = autoRetryStateRef.current.count
  const autoRetrying = isHlsNotReady && autoRetriesUsed < HLS_NOT_READY_MAX_AUTO_RETRIES

  const handleAction = (action: VodErrorAction) => {
    switch (action.kind) {
      case 'retry':
        onRetry?.()
        break
      case 'hard-refresh':
        window.location.reload()
        break
      case 'twitch':
        // handled by anchor element
        break
    }
  }

  return (
    <div
      role="alert"
      aria-live="assertive"
      className={
        className ??
        'flex max-w-md flex-col gap-3 rounded-xl border border-white/10 bg-zinc-950/90 p-5 text-left shadow-2xl shadow-black/60 backdrop-blur-xl'
      }
    >
      <div className="flex flex-col gap-1">
        <h3 className="text-sm font-black text-white">{descriptor.title}</h3>
        <p className="text-xs leading-relaxed text-zinc-300">{descriptor.description}</p>
        {autoRetrying ? (
          <p className="mt-1 flex items-center gap-2 text-xs font-semibold text-violet-300">
            <span className="h-3 w-3 animate-spin rounded-full border-2 border-violet-300/30 border-t-violet-300" />
            Retrying automatically ({autoRetriesUsed + 1} of {HLS_NOT_READY_MAX_AUTO_RETRIES})…
          </p>
        ) : null}
      </div>
      <div className="flex flex-wrap gap-2">
        {descriptor.actions.map(action => {
          const base =
            'rounded px-3 py-1.5 text-xs font-black transition focus:outline-none focus-visible:ring-2 focus-visible:ring-violet-400'
          const tone = action.primary
            ? 'bg-violet-500 text-white hover:bg-violet-400'
            : 'border border-white/15 bg-white/5 text-zinc-200 hover:bg-white/10'
          if (action.kind === 'twitch' && action.href) {
            return (
              <a
                key={action.kind}
                href={action.href}
                target="_blank"
                rel="noreferrer noopener"
                className={`${base} ${tone}`}
              >
                {action.label}
              </a>
            )
          }
          return (
            <button
              key={action.kind}
              type="button"
              onClick={() => handleAction(action)}
              className={`${base} ${tone}`}
            >
              {action.label}
            </button>
          )
        })}
      </div>
    </div>
  )
}

export default VodErrorState
