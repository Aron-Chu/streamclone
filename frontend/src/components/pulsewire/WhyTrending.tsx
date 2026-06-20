import type { WireStoryView } from '../../utils/pulseWireStoryView'

type Props = {
  view: WireStoryView
  compact?: boolean
}

export function WhyTrendingLine({ view, compact = false }: Props) {
  return (
    <p className={`${compact ? 'text-xs' : 'text-sm'} leading-relaxed text-[#ADADB8]`}>
      {view.displayReason}
    </p>
  )
}

export function WhyTrendingBullets({ view }: Props) {
  const bullets = view.displayReasonBullets.length ? view.displayReasonBullets : [view.displayReason]
  return (
    <ul className="space-y-2">
      {bullets.map((item, index) => (
        <li key={`${index}-${item}`} className="flex gap-2 text-sm leading-relaxed text-[#D6D6DE]">
          <span className="mt-2 h-1.5 w-1.5 shrink-0 rounded-full bg-[#A970FF]" />
          <span>{item}</span>
        </li>
      ))}
    </ul>
  )
}
