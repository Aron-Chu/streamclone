import { useQuery } from '@tanstack/react-query'
import { Link, useLocation } from 'react-router-dom'
import { getFollowedChannels, getStreams } from '../api'
import type { FollowedChannel } from '../api'
import { useAuth } from '../auth'
import { useUiSettings } from '../settings'

interface ChannelRailProps {
  collapsed: boolean
  mobileOpen: boolean
  onToggleCollapsed: () => void
  onCloseMobile: () => void
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

function ChannelItem({ channel, collapsed, active, onClick }: { channel: FollowedChannel; collapsed: boolean; active: boolean; onClick?: () => void }) {
  return (
    <Link
      to={`/c/${channel.login}`}
      onClick={onClick}
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
              <span>{formatViewers(channel.viewers ?? 0)}</span>
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
}: {
  channels: FollowedChannel[]
  collapsed: boolean
  onCloseMobile?: () => void
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
        />
      ))}
    </div>
  )
}

function RailContent({ collapsed, onCloseMobile }: { collapsed: boolean; onCloseMobile?: () => void }) {
  const auth = useAuth()
  const settings = useUiSettings(s => s.settings)
  const toggleRailSection = useUiSettings(s => s.toggleRailSection)
  const followed = useQuery({
    queryKey: ['followed'],
    queryFn: getFollowedChannels,
    enabled: auth.isAuthenticated,
    retry: false,
    staleTime: 30_000,
  })
  const live = useQuery({
    queryKey: ['streams'],
    queryFn: getStreams,
    staleTime: 30_000,
  })
  const followedChannels = followed.data ?? []
  const fallback = fromLiveDirectory(live.data)
  const liveFollowing = followedChannels.filter(channel => channel.isLive)
  const offlineFollowing = followedChannels.filter(channel => !channel.isLive)
  const showFollowing = auth.isAuthenticated && followedChannels.length > 0
  const topLive = showFollowing ? fallback.filter(channel => !followedChannels.some(followedChannel => followedChannel.login === channel.login)).slice(0, 20) : fallback
  const sections = [
    { id: 'live' as const, label: showFollowing ? 'Following live' : 'Live channels', channels: showFollowing ? liveFollowing : topLive },
    { id: 'offline' as const, label: 'Following offline', channels: showFollowing ? offlineFollowing : [] },
    { id: 'top' as const, label: 'Top live', channels: showFollowing ? topLive : [] },
  ].filter(section => section.channels.length > 0)

  return (
    <>
      <div className={`flex h-16 items-center border-b border-white/10 px-3 ${collapsed ? 'justify-center' : 'justify-between'}`}>
        {!collapsed ? (
          <Link to="/" onClick={onCloseMobile} className="flex items-center gap-2">
            <span className="grid h-8 w-8 place-items-center rounded bg-violet-500 text-sm font-black text-white">7</span>
            <span className="text-sm font-black">Streamclone</span>
          </Link>
        ) : (
          <Link to="/" onClick={onCloseMobile} className="grid h-9 w-9 place-items-center rounded bg-violet-500 text-sm font-black text-white">7</Link>
        )}
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
              {open ? <ChannelList channels={section.channels} collapsed={collapsed} onCloseMobile={onCloseMobile} /> : null}
            </div>
          )
        })}
      </div>

    </>
  )
}

export default function ChannelRail({ collapsed, mobileOpen, onToggleCollapsed, onCloseMobile }: ChannelRailProps) {
  return (
    <>
      <aside className={`hidden min-h-screen shrink-0 flex-col border-r border-white/10 bg-[#111117] text-white lg:flex ${collapsed ? 'w-16' : 'w-64'}`}>
        <RailContent collapsed={collapsed} />
        <button onClick={onToggleCollapsed} className="border-t border-white/10 px-2 py-3 text-xs font-black text-zinc-400 transition hover:bg-white/[0.06] hover:text-white">
          {collapsed ? '>>' : 'Collapse'}
        </button>
      </aside>

      {mobileOpen ? (
        <div className="fixed inset-0 z-50 lg:hidden">
          <button aria-label="Close navigation" onClick={onCloseMobile} className="absolute inset-0 bg-black/70" />
          <aside className="relative flex h-full w-72 flex-col border-r border-white/10 bg-[#111117] text-white shadow-2xl">
            <RailContent collapsed={false} onCloseMobile={onCloseMobile} />
          </aside>
        </div>
      ) : null}
    </>
  )
}
