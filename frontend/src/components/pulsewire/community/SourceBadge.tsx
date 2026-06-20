type Props = {
  label: string
  className?: string
  variant?: 'tile' | 'inline'
}

export default function SourceBadge({ label, className = '', variant = 'tile' }: Props) {
  const text = label.replace(/^r\//i, '').trim() || 'Source'
  if (variant === 'inline') {
    return (
      <span
        className={`inline-flex rounded-full border border-[#A970FF]/25 bg-[#9147FF]/10 px-2 py-0.5 text-[10px] font-bold uppercase tracking-wide text-[#A970FF] ${className}`}
      >
        {text}
      </span>
    )
  }
  return (
    <div
      className={`flex h-14 w-14 shrink-0 items-center justify-center rounded-lg border border-[#A970FF]/25 bg-[#9147FF]/10 ${className}`}
      aria-hidden="true"
    >
      <span className="px-1 text-center text-[9px] font-bold uppercase leading-tight tracking-wide text-[#A970FF]">
        {text.slice(0, 12)}
      </span>
    </div>
  )
}
