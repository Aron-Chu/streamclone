import type { PulseWireReceipt } from '../../pulseWireApi'
import { receiptThumb } from '../../utils/pulseWireReceiptThumb'
import { formatRelativeTime } from '../../utils/pulseWireFormat'

const DOT: Record<string, string> = {
  pulse_origin: '#A970FF',
  reddit_thread: '#FF4500',
  youtube_video: '#FF0000',
  twitch_clip: '#9147FF',
  x_post: '#1D9BF0',
  tiktok_video: '#00D7B0',
  streamerbans: '#FF5C57',
  kick_clip: '#53FC18',
}

const SOURCE_LABELS: Record<string, string> = {
  pulse_origin: 'Pulse origin',
  reddit_thread: 'Reddit',
  youtube_video: 'YouTube',
  twitch_clip: 'Twitch clip',
  x_post: 'X post',
  tiktok_video: 'TikTok',
  streamerbans: 'StreamerBans',
}

function sourceLabel(sourceType: string): string {
  return SOURCE_LABELS[sourceType] ?? sourceType.replace(/_/g, ' ')
}

type Props = {
  receipts?: PulseWireReceipt[]
  compact?: boolean
  rich?: boolean
  className?: string
  /** When false, receipt chips render as spans (for cards wrapped in a parent link). */
  linkable?: boolean
}

export default function ReceiptsRow({ receipts, compact, rich, className = '', linkable = true }: Props) {
  if (!receipts?.length) {
    return compact ? null : <p className={`text-xs text-[#7A7A85] ${className}`}>No receipts yet</p>
  }

  return (
    <div className={`flex flex-wrap gap-2 ${className}`}>
      {receipts.map((receipt, index) => {
        const thumb = receiptThumb(receipt)
        const label = receipt.label ?? sourceLabel(receipt.sourceType)
        const content = (
          <>
            {thumb ? (
              <img src={thumb} alt="" className={`${rich ? 'h-5 w-5' : 'h-4 w-4'} shrink-0 rounded-full object-cover`} loading="lazy" />
            ) : (
              <span className="h-2 w-2 shrink-0 rounded-full" style={{ background: DOT[receipt.sourceType] || '#7A7A85' }} />
            )}
            <span className="truncate">{label}</span>
            <span className="font-bold text-[#EFEFF1]">{receipt.pct}%</span>
            {receipt.occurredAt && rich ? (
              <span className="text-[10px] text-[#7A7A85]">{formatRelativeTime(receipt.occurredAt)}</span>
            ) : null}
            {receipt.risk ? (
              <span className="text-[10px] uppercase text-[#7A7A85]">{receipt.risk.replace(/_/g, ' ')}</span>
            ) : null}
            {receipt.previewStatus && rich ? (
              <span className="text-[10px] capitalize text-[#7A7A85]">{receipt.previewStatus}</span>
            ) : null}
          </>
        )
        const chipClass = `inline-flex max-w-full items-center gap-2 rounded-full border border-[#2A2A2E] bg-[#1B1B1F] ${
          rich ? 'px-3 py-1.5 text-xs' : 'px-2.5 py-1 text-xs'
        } font-medium text-[#D6D6DE] transition hover:border-[#A970FF]/30`

        if (receipt.url && linkable) {
          return (
            <a
              key={`${receipt.sourceType}-${receipt.url}-${index}`}
              href={receipt.url}
              target="_blank"
              rel="noopener noreferrer"
              className={chipClass}
            >
              {content}
            </a>
          )
        }

        return (
          <span key={`${receipt.sourceType}-${index}`} className={chipClass}>
            {content}
          </span>
        )
      })}
    </div>
  )
}
