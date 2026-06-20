import { useQuery } from '@tanstack/react-query'
import { Link, useLocation } from 'react-router-dom'
import { getCategories, getFollowedChannels, getStreams } from '../api'
import type { Category, FollowedChannel } from '../api'
import { useAuth } from '../auth'
import { useStreamPrewarm } from '../hooks/useStreamPrewarm'
import { useUiSettings } from '../settings'
import { formatCategoryViewers } from '../utils/categorySort'
import BrandLogo from './BrandLogo'

interface ChannelRailProps {
  collapsed: boolean
  mobileOpen: boolean
  onToggleCollapsed: () => void
  onCloseMobile: () => void
  viewerOverrides?: Record<string, number | undefined>
}

function fromLiveDirectory(streams: Awaited<ReturnType<typeof getStreams>> | undefined): FollowedChannel[] {
  return (streams ?? []).map(stream => ({
    id: stream.id ?? stream.login,
    login: stream.login,
    displayName: stream.displayName || stream.login,
    isLive: true,
    title: stream.title,
    category: stream.category,
    viewers: stream.viewers,
    thumbnailUrl: stream.thumbnailUrl,
  }))
}

function thumb(url: string | undefined, w = 56, h = 74) {
  return (url ?? '').replace('{width}', String(w)).replace('{height}', String(h))
}

function Avatar({ channel, collapsed }: { channel: FollowedChannel; collapsed: boolean }) {
  const img = channel.profileImage || channel.thumbnailUrl
  return (
    <div className={`${collapsed ? 'h-9 w-9' : 'h-10 w-10'} grid shrink-0 place-items-center overflow-hidden rounded-full bg-white/10 text-sm font-black text-violet-100`}>
      {img ? <img src={img.replace('{width}', '80').replace('{height}', '80')} alt={channel.displayName || channel.login} className="h-full w-full object-cover" /> : (channel.displayName || channel.login).slice(0, 1).toUpperCase()}
    </div>
  )
}

function formatViewers(val: number): string {
  if (val >= 1000) {
    return (val / 1000).toFixed(1).replace(/\.0$/, '') + 'k'
  }
  return val.toString()
}

function ChannelItem({ channel, collapsed, active, onClick, viewerOverrides }: { channel: FollowedChannel; collapsed: boolean; active: boolean; onClick?: () => void; viewerOverrides?: Record<string, number | undefined> }) {
  const viewers = viewerOverrides?.[channel.login] ?? channel.viewers ?? 0
  const { prewarm, cancelPrewarm } = useStreamPrewarm()
  return (
    <Link
      to={`/c/${channel.login}`}
      onClick={onClick}
      onMouseEnter={() => prewarm(channel.login, Boolean(channel.isLive) && !active)}
      onMouseLeave={cancelPrewarm}
      onFocus={() => prewarm(channel.login, Boolean(channel.isLive) && !active)}
      onBlur={cancelPrewarm}
      className={`group flex items-center gap-3 rounded px-2 py-1.5 transition ${active ? 'bg-violet-500/20 text-white' : 'text-zinc-300 hover:bg-white/[0.07] hover:text-white'}`}
      title={collapsed ? `${channel.displayName || channel.login}${channel.isLive ? ` - ${channel.category || 'Live'}` : ' - Offline'}` : undefined}
    >
      <div className="relative">
        <Avatar channel={channel} collapsed={collapsed} />
        {channel.isLive ? <span className="absolute -bottom-0.5 -right-0.5 h-3 w-3 rounded-full border-2 border-[#111117] bg-red-500" /> : null}
      </div>
      {!collapsed ? (
        <div className="flex flex-1 items-center justify-between min-w-0">
          <div className="min-w-0 flex-1">
            <div className="truncate text-sm font-black">{channel.displayName || channel.login}</div>
            <div className="truncate text-xs font-semibold text-zinc-500">
              {channel.isLive ? (channel.category || 'Just Chatting') : 'Offline'}
            </div>
          </div>
          {channel.isLive ? (
            <div className="ml-2 flex items-center gap-1 text-[10px] font-black text-red-500">
              <span className="h-1.5 w-1.5 rounded-full bg-red-500" />
              <span>{formatViewers(viewers)}</span>
            </div>
          ) : null}
        </div>
      ) : null}
    </Link>
  )
}

function SectionHeader({
  label,
  count,
  open,
  onToggle,
  collapsed,
}: {
  label: string
  count: number
  open: boolean
  onToggle: () => void
  collapsed: boolean
}) {
  if (collapsed) return null
  return (
    <button
      type="button"
      onClick={onToggle}
      className="mb-1 flex w-full items-center justify-between rounded px-2 py-1 text-left text-[11px] font-black uppercase text-zinc-500 transition hover:bg-white/[0.06] hover:text-zinc-200"
    >
      <span>{label}</span>
      <span className="flex items-center gap-2">
        <span>{count}</span>
        <span>{open ? '-' : '+'}</span>
      </span>
    </button>
  )
}

function ChannelList({
  channels,
  collapsed,
  onCloseMobile,
  viewerOverrides,
}: {
  channels: FollowedChannel[]
  collapsed: boolean
  onCloseMobile?: () => void
  viewerOverrides?: Record<string, number | undefined>
}) {
  const location = useLocation()
  return (
    <div className="space-y-1">
      {channels.map(channel => (
        <ChannelItem
          key={`${channel.login}-${channel.id}`}
          channel={channel}
          collapsed={collapsed}
          active={location.pathname === `/c/${channel.login}`}
          onClick={onCloseMobile}
          viewerOverrides={viewerOverrides}
        />
      ))}
    </div>
  )
}

function CategoryItem({
  category,
  collapsed,
  onClick,
}: {
  category: Category
  collapsed: boolean
  onClick?: () => void
}) {
  return (
    <Link
      to={`/browse/category/${category.id}?name=${encodeURIComponent(category.name)}`}
      onClick={onClick}
      className="group flex items-center gap-3 rounded px-2 py-1.5 text-zinc-300 transition hover:bg-white/[0.07] hover:text-white"
      title={collapsed ? category.name : undefined}
    >
      <div className={`${collapsed ? 'h-9 w-9' : 'h-10 w-8'} grid shrink-0 place-items-center overflow-hidden rounded bg-white/10 text-xs font-black text-violet-100`}>
        {category.thumbnailUrl ? (
          <img src={thumb(category.thumbnailUrl)} alt="" className="h-full w-full object-cover" />
        ) : (
          category.name.slice(0, 1).toUpperCase()
        )}
      </div>
      {!collapsed ? (
        <div className="min-w-0 flex-1">
          <div className="truncate text-sm font-black">{category.name}</div>
          <div className="truncate text-xs font-semibold text-zinc-500">
            {category.viewers ? formatCategoryViewers(category.viewers) : 'Browse live channels'}
          </div>
        </div>
      ) : null}
    </Link>
  )
}

function CategoryList({
  categories,
  collapsed,
  onCloseMobile,
}: {
  categories: Category[]
  collapsed: boolean
  onCloseMobile?: () => void
}) {
  return (
    <div className="space-y-1">
      {categories.map(category => (
        <CategoryItem
          key={`${category.id}-${category.name}`}
          category={category}
          collapsed={collapsed}
          onClick={onCloseMobile}
        />
      ))}
    </div>
  )
}

function RailContent({
  collapsed,
  onCloseMobile,
  viewerOverrides,
}: {
  collapsed: boolean
  onCloseMobile?: () => void
  viewerOverrides?: Record<string, number | undefined>
}) {
  const settings = useUiSettings(s => s.settings)
  const toggleRailSection = useUiSettings(s => s.toggleRailSection)
  const auth = useAuth()
  const followed = useQuery({
    queryKey: ['followed', auth.isAuthenticated],
    queryFn: () => getFollowedChannels(auth.isAuthenticated),
    retry: false,
    staleTime: 30_000,
  })
  const live = useQuery({
    queryKey: ['streams'],
    queryFn: getStreams,
    staleTime: 30_000,
  })
  const categories = useQuery({
    queryKey: ['categories'],
    queryFn: getCategories,
    staleTime: 60_000,
  })
  const followedChannels = followed.data ?? []
  const fallback = fromLiveDirectory(live.data)
  const liveFollowing = followedChannels.filter(channel => channel.isLive)
  const offlineFollowing = followedChannels.filter(channel => !channel.isLive)
  const showFollowing = followedChannels.length > 0
  const topLive = showFollowing ? fallback.filter(channel => !followedChannels.some(followedChannel => followedChannel.login === channel.login)).slice(0, 20) : fallback
  const recommendedCategories = (categories.data ?? []).slice(0, 8)
  const sections = [
    { id: 'live' as const, label: showFollowing ? 'Following live' : 'Live channels', channels: showFollowing ? liveFollowing : topLive },
    { id: 'offline' as const, label: 'Following offline', channels: showFollowing ? offlineFollowing : [] },
    { id: 'top' as const, label: 'Top live', channels: showFollowing ? topLive : [] },
  ].filter(section => section.channels.length > 0)

  return (
    <>
      <div className={`flex h-16 items-center border-b border-white/10 px-3 ${collapsed ? 'justify-center' : ''}`}>
        <Link to="/" onClick={onCloseMobile} className={`flex items-center ${collapsed ? 'justify-center' : ''}`}>
          <BrandLogo size="sm" showText={!collapsed} />
        </Link>
      </div>

      <div className="min-h-0 flex-1 overflow-y-auto px-2 py-3">
        {sections.map(section => {
          const open = settings.railSections[section.id]
          return (
            <div key={section.id} className={collapsed ? 'mb-2' : 'mb-4'}>
              <SectionHeader
                label={section.label}
                count={section.channels.length}
                open={open}
                onToggle={() => toggleRailSection(section.id)}
                collapsed={collapsed}
              />
              {open ? <ChannelList channels={section.channels} collapsed={collapsed} onCloseMobile={onCloseMobile} viewerOverrides={viewerOverrides} /> : null}
            </div>
          )
        })}
        {recommendedCategories.length ? (
          <div className={collapsed ? 'mb-2' : 'mb-4'}>
            <SectionHeader
              label="Recommended categories"
              count={recommendedCategories.length}
              open={settings.railSections.categories}
              onToggle={() => toggleRailSection('categories')}
              collapsed={collapsed}
            />
            {settings.railSections.categories ? (
              <CategoryList
                categories={recommendedCategories}
                collapsed={collapsed}
                onCloseMobile={onCloseMobile}
              />
            ) : null}
          </div>
        ) : null}
      </div>

    </>
  )
}

export default function ChannelRail({
  collapsed,
  mobileOpen,
  onToggleCollapsed,
  onCloseMobile,
  viewerOverrides,
}: ChannelRailProps) {
  return (
    <>
      <aside className={`hidden min-h-screen shrink-0 flex-col border-r border-white/10 bg-[#0E0E11] text-white lg:flex ${collapsed ? 'w-16' : 'w-64'}`}>
        <RailContent
          collapsed={collapsed}
          viewerOverrides={viewerOverrides}
        />
        <button onClick={onToggleCollapsed} className="border-t border-white/10 px-2 py-3 text-xs font-black text-zinc-400 transition hover:bg-white/[0.06] hover:text-white">
          {collapsed ? '>>' : 'Collapse'}
        </button>
      </aside>

      {mobileOpen ? (
        <div className="fixed inset-0 z-50 lg:hidden">
          <button aria-label="Close navigation" onClick={onCloseMobile} className="absolute inset-0 bg-black/70" />
          <aside className="relative flex h-full w-72 flex-col border-r border-white/10 bg-[#0E0E11] text-white shadow-2xl">
            <RailContent
              collapsed={false}
              onCloseMobile={onCloseMobile}
              viewerOverrides={viewerOverrides}
            />
          </aside>
        </div>
      ) : null}
    </>
  )
}
