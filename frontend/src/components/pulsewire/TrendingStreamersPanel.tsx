import type { PulseWireTrendingStreamer } from '../../pulseWireApi'



type Props = {

  items: PulseWireTrendingStreamer[]

  windowLabel?: string

  activeLogin?: string

  onSelect: (login: string) => void

}



export default function TrendingStreamersPanel({ items, windowLabel = '24h', activeLogin = '', onSelect }: Props) {

  return (

    <div className="rounded-xl border border-[#2A2A2E] bg-[#161619] p-4">

      <div className="mb-3 flex items-center justify-between gap-2">

        <h3 className="text-[15px] font-bold text-[#F7F7F8]">Trending streamers</h3>

        <span className="text-[11px] font-semibold text-[#7A7A85]">{windowLabel}</span>

      </div>

      {!items.length ? (

        <p className="text-xs text-[#7A7A85]">No trending streamers in this window yet.</p>

      ) : null}

      <div className="flex flex-wrap gap-2">

        {items.slice(0, 8).map(item => {

          const active = activeLogin === item.login

          const meta = [

            item.storyCount ? `${item.storyCount} stories` : null,

            item.evidenceCount ? `${item.evidenceCount} receipts` : null,

          ].filter(Boolean).join(' · ')

          return (

            <button

              key={item.login}

              type="button"

              title={meta || undefined}

              aria-pressed={active}

              onClick={() => onSelect(active ? '' : item.login)}

              className={`rounded-full border px-3 py-1.5 text-xs font-black transition focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[#A970FF] ${

                active

                  ? 'border-[#A970FF] bg-[#A970FF]/20 text-[#EFEFF1]'

                  : 'border-[#2A2A2E] bg-[#1B1B1F] text-[#ADADB8] hover:border-[#A970FF]/50 hover:text-[#EFEFF1]'

              }`}

            >

              {item.displayName || item.login}

            </button>

          )

        })}

      </div>

    </div>

  )

}
