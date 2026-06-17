import type { NetworkActivityTab } from '../../utils/networkActivityModel'
import { NETWORK_ACTIVITY_TABS } from '../../utils/networkActivityModel'

export interface NetworkCategoryTabsProps {
  activeTab: NetworkActivityTab
  onTabChange: (tab: NetworkActivityTab) => void
  counts?: Partial<Record<NetworkActivityTab, number>>
}

export default function NetworkCategoryTabs({
  activeTab,
  onTabChange,
  counts,
}: NetworkCategoryTabsProps) {
  return (
    <div className="flex flex-wrap gap-2 border-b border-white/10 pb-3">
      {NETWORK_ACTIVITY_TABS.map(tab => {
        const count = counts?.[tab.id]
        const active = activeTab === tab.id
        return (
          <button
            key={tab.id}
            type="button"
            onClick={() => onTabChange(tab.id)}
            className={`rounded-lg border px-3 py-2 text-xs font-black uppercase transition ${
              active
                ? 'border-violet-400/40 bg-violet-500/15 text-violet-100'
                : 'border-white/10 text-zinc-400 hover:bg-white/5 hover:text-zinc-200'
            }`}
          >
            {tab.label}
            {count != null && count > 0 ? (
              <span className="ml-1.5 rounded bg-white/10 px-1.5 py-0.5 text-[10px] font-mono text-zinc-300">
                {count}
              </span>
            ) : null}
          </button>
        )
      })}
    </div>
  )
}
