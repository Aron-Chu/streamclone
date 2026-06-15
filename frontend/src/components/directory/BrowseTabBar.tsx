import { Link, useLocation } from 'react-router-dom'
import type { BrowseTab } from '../../utils/browseTabs'

function BrowseTabLink({
  to,
  active,
  children,
}: {
  to: string
  active: boolean
  children: string
}) {
  return (
    <Link
      to={to}
      role="tab"
      aria-selected={active}
      className={`rounded-md px-3 py-1.5 text-sm font-bold transition ${
        active
          ? 'bg-[#9147ff] text-white'
          : 'border border-[#3a3a3d] bg-[#18181b] text-zinc-300 hover:border-[#53535a] hover:bg-[#1f1f23] hover:text-white'
      }`}
    >
      {children}
    </Link>
  )
}

interface BrowseTabBarProps {
  activeTab: BrowseTab
}

export function BrowseTabBar({ activeTab }: BrowseTabBarProps) {
  const location = useLocation()
  const search = location.search

  return (
    <div className="flex items-center gap-2" role="tablist" aria-label="Browse">
      <BrowseTabLink to={`/browse${search}`} active={activeTab === 'categories'}>
        Categories
      </BrowseTabLink>
      <BrowseTabLink to={`/browse/live${search}`} active={activeTab === 'live'}>
        Live Channels
      </BrowseTabLink>
    </div>
  )
}
