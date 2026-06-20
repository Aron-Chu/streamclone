import { useMemo, useState } from 'react'
import { SETUP_CONTROL_TOKEN } from '../../config'
import { confirmDevelopingStory, type PulseWireStory } from '../../pulseWireApi'

function DevelopingRow({ tone, label }: { tone: string; label: string }) {
  return (
    <div className="flex items-center justify-between gap-3 text-sm">
      <div className="flex items-center gap-2">
        <span className={`h-2.5 w-2.5 rounded-full ${tone}`} />
        <span className="text-[#D6D6DE]">{label}</span>
      </div>
      <span className="text-[#7A7A85]">›</span>
    </div>
  )
}

export default function DevelopingPanel({
  items,
  onConfirmed,
}: {
  items: PulseWireStory[]
  onConfirmed?: () => void
}) {
  const [pendingId, setPendingId] = useState<number | null>(null)
  const [actionError, setActionError] = useState('')
  const counts = useMemo(() => ({
    confirmation: items.length,
    noOrigin: items.filter(item => !item.origin).length,
    bans: items.filter(item => item.story.category === 'bans').length,
  }), [items])

  return (
    <div className="rounded-xl border border-[#2A2A2E] bg-[#161619] p-4">
      <div className="mb-3 flex items-center gap-2">
        <h3 className="text-[15px] font-bold text-[#F7F7F8]">Developing</h3>
        <span className="h-2 w-2 rounded-full bg-[#FFB02E]" />
      </div>
      {!items.length ? <p className="text-xs text-[#7A7A85]">No stories need confirmation right now.</p> : null}
      {items.length ? (
        <div className="space-y-3">
          <DevelopingRow tone="bg-[#FF5C57]" label={`${counts.confirmation} stories need confirmation`} />
          <DevelopingRow tone="bg-[#FFB02E]" label={`${counts.noOrigin} clips have no Twitch origin`} />
          <DevelopingRow tone="bg-[#A970FF]" label={`${counts.bans} streamer ban stories detected`} />
          <div className="space-y-2 border-t border-[#2A2A2E] pt-3">
            {items.slice(0, 3).map(item => (
              <div key={item.story.id} className="flex items-center justify-between gap-3">
                <p className="line-clamp-2 text-xs text-[#ADADB8]">
                  {item.story.title || `Story #${item.story.id}`}
                </p>
                {SETUP_CONTROL_TOKEN ? (
                  <button
                    type="button"
                    disabled={pendingId === item.story.id}
                    onClick={async () => {
                      try {
                        setActionError('')
                        setPendingId(item.story.id)
                        await confirmDevelopingStory(item.story.id, 'confirm')
                        onConfirmed?.()
                      } catch (error) {
                        setActionError(error instanceof Error ? error.message : 'Confirm failed')
                      } finally {
                        setPendingId(null)
                      }
                    }}
                    className="shrink-0 rounded border border-[#A970FF]/40 bg-[#9147FF]/15 px-2.5 py-1 text-[11px] font-bold text-[#EFEFF1] transition hover:bg-[#9147FF]/25 disabled:opacity-50"
                  >
                    {pendingId === item.story.id ? 'Saving…' : 'Confirm'}
                  </button>
                ) : null}
              </div>
            ))}
          </div>
          {!SETUP_CONTROL_TOKEN ? (
            <p className="text-xs text-[#7A7A85]">Operator confirms use the local setup-control token path.</p>
          ) : null}
          {actionError ? <p className="text-xs text-red-300">{actionError}</p> : null}
        </div>
      ) : null}
    </div>
  )
}
