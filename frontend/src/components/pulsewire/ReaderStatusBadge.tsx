import type { ReaderStatus } from '../../utils/pulseWireStoryView'

const STATUS_LABELS: Record<ReaderStatus, string> = {
  developing: 'Developing',
  corroborated: 'Corroborated',
  needs_origin: 'Needs origin',
  active: 'Active',
  settled: 'Settled',
  unverified: 'Unverified',
  insufficient_data: 'Insufficient data',
}

const STATUS_CLASS: Record<ReaderStatus, string> = {
  developing: 'bg-[#3A2A12] text-[#FFB02E]',
  corroborated: 'bg-[#16321F] text-[#3FCB7E]',
  needs_origin: 'bg-[#2B2440] text-[#A970FF]',
  active: 'bg-[#1A2D3F] text-[#68B7FF]',
  settled: 'bg-[#16321F] text-[#3FCB7E]',
  unverified: 'bg-[#2A1515] text-[#FF5C57]',
  insufficient_data: 'bg-[#26262C] text-[#ADADB8]',
}

type Props = {
  status: ReaderStatus
  compact?: boolean
}

export function readerStatusLabel(status: ReaderStatus) {
  return STATUS_LABELS[status]
}

export default function ReaderStatusBadge({ status, compact = false }: Props) {
  return (
    <span className={`inline-flex items-center rounded-full font-semibold ${STATUS_CLASS[status]} ${compact ? 'px-2 py-0.5 text-[11px]' : 'px-2.5 py-1 text-xs'}`}>
      {STATUS_LABELS[status]}
    </span>
  )
}
