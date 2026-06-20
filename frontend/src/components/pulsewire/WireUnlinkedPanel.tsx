import type { PulseWireUnlinkedEvidence } from '../../pulseWireApi'
import { formatCompactCount } from '../../utils/pulseWireFormat'
import { pulseWireDisplayThumbnail } from '../../utils/twitchClipThumb'
import SourceBadge from './community/SourceBadge'

type Props = {
  items: PulseWireUnlinkedEvidence[]
  className?: string
}

export default function WireUnlinkedPanel({ items, className = '' }: Props) {
  if (!items.length) return null

  return (
    <section className={className} aria-labelledby="unlinked-evidence-heading">
      <h2 id="unlinked-evidence-heading" className="text-lg font-semibold text-[#EFEFF1]">
        Unlinked evidence
      </h2>
      <p className="mt-1 text-sm text-[#ADADB8]">
        Ingested items not yet clustered into Wire stories for this window.
      </p>
      <div className="mt-4 space-y-3">
        {items.map(item => {
          const previewSrc = pulseWireDisplayThumbnail(item.displayThumbnailUrl)
          const label = item.source === 'reddit' ? 'LSF' : item.source
          return (
            <article key={item.id} className="rounded-[14px] border border-[#2A2A2E] bg-[#121217] p-4">
              <div className="flex gap-3">
                {previewSrc ? (
                  <img
                    src={previewSrc}
                    alt=""
                    className="h-14 w-14 shrink-0 rounded-lg object-cover"
                    loading="lazy"
                  />
                ) : (
                  <SourceBadge label={label} />
                )}
                <div className="min-w-0 flex-1">
                  <div className="text-[10px] font-bold uppercase tracking-[0.08em] text-[#A970FF]">
                    {item.source}
                    {item.category ? ` · ${item.category}` : ''}
                  </div>
                  <h3 className="mt-1 line-clamp-2 text-sm font-semibold leading-5 text-[#F7F7F8]">{item.title}</h3>
                  <div className="mt-2 flex flex-wrap gap-2 text-[11px] font-semibold text-[#7A7A85]">
                    {item.score ? <span>{formatCompactCount(item.score)} upvotes</span> : null}
                    {item.viewCount ? <span>{formatCompactCount(item.viewCount)} views</span> : null}
                    {item.comments ? <span>{formatCompactCount(item.comments)} comments</span> : null}
                  </div>
                </div>
              </div>
              {item.url ? (
                <a
                  href={item.url}
                  target="_blank"
                  rel="noreferrer"
                  className="mt-3 inline-flex rounded-lg border border-[#33333A] bg-[#1B1B1F] px-3 py-1.5 text-[11px] font-semibold uppercase tracking-wide text-[#EFEFF1] transition hover:border-[#A970FF]/40"
                >
                  Open source
                </a>
              ) : null}
            </article>
          )
        })}
      </div>
    </section>
  )
}
