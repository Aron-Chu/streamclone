import type { ReactNode } from 'react'

type Props = {
  eyebrow?: string
  title: string
  subtitle?: string
  aside?: ReactNode
  action?: ReactNode
  emptyMessage?: string
  isEmpty?: boolean
  className?: string
  children: ReactNode
}

export default function NewsSection({
  eyebrow,
  title,
  subtitle,
  aside,
  action,
  emptyMessage,
  isEmpty,
  className = '',
  children,
}: Props) {
  const trailing = aside ?? action
  return (
    <section className={className}>
      <div className="mb-3 flex flex-wrap items-end justify-between gap-3">
        <div>
          {eyebrow ? (
            <p className="text-[11px] font-bold uppercase tracking-[0.08em] text-[#A970FF]">{eyebrow}</p>
          ) : null}
          <h2 className="text-[18px] font-bold text-[#F7F7F8]">{title}</h2>
          {subtitle ? <p className="mt-1 text-xs text-[#7A7A85]">{subtitle}</p> : null}
        </div>
        {trailing}
      </div>
      {isEmpty && emptyMessage ? (
        <p className="rounded-xl border border-[#2A2A2E] bg-[#121217] p-4 text-xs text-[#7A7A85]">{emptyMessage}</p>
      ) : (
        children
      )}
    </section>
  )
}
