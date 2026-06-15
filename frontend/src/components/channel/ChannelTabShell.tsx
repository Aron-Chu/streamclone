import type { ReactNode } from 'react'

type ChannelTab = 'about' | 'stats' | 'clips' | 'vods' | 'diagnostics' | 'emotes'

type ChannelTabShellProps = {
  activeTab: ChannelTab
  onTab: (tab: ChannelTab) => void
  dense?: boolean
  children: ReactNode
}

const tabs: Array<{ id: ChannelTab; label: string }> = [
  { id: 'about', label: 'About' },
  { id: 'stats', label: 'Stats' },
  { id: 'clips', label: 'Clips' },
  { id: 'vods', label: 'Videos' },
  { id: 'emotes', label: 'Emotes' },
]

export default function ChannelTabShell({ activeTab, onTab, dense, children }: ChannelTabShellProps) {
  return (
    <section className="shrink-0 bg-[#0e0e10]">
      <div className="sticky top-0 z-10 border-b border-white/10 bg-[#0e0e10]/95 px-4 backdrop-blur-sm lg:px-6">
        <div className="flex items-center gap-1 overflow-x-auto">
          {tabs.map(tab => (
            <button
              key={tab.id}
              type="button"
              onClick={() => onTab(tab.id)}
              className={`shrink-0 border-b-2 px-3 py-3 text-sm font-semibold transition ${activeTab === tab.id ? 'border-[#bf94ff] text-white' : 'border-transparent text-zinc-400 hover:border-zinc-600 hover:text-zinc-200'}`}
            >
              {tab.label}
            </button>
          ))}
          <button
            type="button"
            onClick={() => onTab('diagnostics')}
            className={`shrink-0 border-b-2 px-3 py-3 text-sm font-semibold transition ${
              activeTab === 'diagnostics'
                ? 'border-amber-300 text-amber-100'
                : 'border-transparent text-zinc-500 hover:border-zinc-600 hover:text-zinc-300'
            }`}
          >
            Advanced
          </button>
        </div>
      </div>

      <div className={`w-full max-w-none ${dense ? 'space-y-5 px-4 py-3 lg:px-6' : 'space-y-6 px-4 py-4 lg:px-6'}`}>
        {children}
      </div>
    </section>
  )
}
