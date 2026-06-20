import { useMemo, useState } from 'react'
import { pulseWireDisplayThumbnail } from '../../../utils/twitchClipThumb'

type Props = {
  displayThumbnailUrl?: string
  title: string
  className?: string
}

export default function ClipThumbnail({ displayThumbnailUrl, title, className = '' }: Props) {
  const [failed, setFailed] = useState(false)
  const src = useMemo(() => {
    if (failed) return undefined
    return pulseWireDisplayThumbnail(displayThumbnailUrl)
  }, [failed, displayThumbnailUrl])

  if (!src || failed) {
    return (
      <div className={`flex h-full w-full items-center justify-center bg-gradient-to-br from-[#1B1B1F] to-[#0C0C0F] ${className}`}>
        <div className="rounded-full border border-[#A970FF]/30 bg-[#9147FF]/15 px-3 py-1 text-[11px] font-semibold uppercase tracking-wide text-[#A970FF]">
          Clip
        </div>
      </div>
    )
  }

  return (
    <img
      data-testid="clip-thumb"
      src={src}
      alt={title}
      className={className}
      loading="lazy"
      onError={() => setFailed(true)}
    />
  )
}
