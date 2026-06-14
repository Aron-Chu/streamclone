import { Link } from 'react-router-dom'
import type { Stream } from '../../api'
import { useStreamPrewarm } from '../../hooks/useStreamPrewarm'

const W = 440
const H = 248

function thumb(url: string | undefined, w = W, h = H) {
  return (url ?? '').replace('{width}', String(w)).replace('{height}', String(h))
}

function formatViewers(count: number): string {
  if (count >= 1000) {
    return `${(count / 1000).toFixed(1).replace(/\.0$/, '')}K`
  }
  return count.toLocaleString()
}

interface DirectoryStreamCardProps {
  stream: Stream
  index?: number
}

export function DirectoryStreamCard({ stream, index = 0 }: DirectoryStreamCardProps) {
  const isLive = stream.isLive ?? Boolean(stream.thumbnailUrl && (stream.viewers ?? 0) > 0)
  const title = stream.title || stream.displayName || stream.login
  const previewUrl = stream.thumbnailUrl || stream.profileImageUrl
  const avatarUrl = stream.profileImageUrl || stream.thumbnailUrl
  const { prewarm, cancelPrewarm } = useStreamPrewarm()

  return (
    <Link
      to={`/c/${stream.login}`}
      onMouseEnter={() => prewarm(stream.login, isLive)}
      onMouseLeave={cancelPrewarm}
      onFocus={() => prewarm(stream.login, isLive)}
      onBlur={cancelPrewarm}
      className="group block overflow-hidden rounded-md transition duration-200 hover:bg-[#1f1f23]"
      style={{ animationDelay: `${Math.min(index, 10) * 35}ms` }}
    >
      <div className="relative aspect-video overflow-hidden rounded-md bg-[#18181b]">
        {previewUrl ? (
          isLive ? (
            <img
              src={thumb(stream.thumbnailUrl || previewUrl)}
              alt={title}
              className="h-full w-full object-cover transition duration-300 group-hover:brightness-110"
            />
          ) : (
            <div className="flex h-full w-full items-center justify-center bg-[#18181b]">
              <img
                src={thumb(stream.profileImageUrl, 140, 140)}
                alt={stream.displayName || stream.login}
                className="h-20 w-20 rounded-full object-cover"
              />
            </div>
          )
        ) : (
          <div className="h-full w-full bg-[#18181b]" />
        )}
        <div
          className={`absolute left-2 top-2 rounded px-1.5 py-0.5 text-[10px] font-bold uppercase tracking-wide text-white ${
            isLive ? 'bg-red-600' : 'bg-zinc-600'
          }`}
        >
          {isLive ? 'Live' : 'Offline'}
        </div>
        {isLive ? (
          <div className="absolute bottom-2 left-2 rounded bg-black/75 px-1.5 py-0.5 text-xs font-semibold text-white backdrop-blur-sm">
            {formatViewers(stream.viewers ?? 0)} viewers
          </div>
        ) : null}
      </div>
      <div className="flex gap-2 p-2">
        <div className="h-9 w-9 shrink-0 overflow-hidden rounded-full bg-[#26262c]">
          {avatarUrl ? (
            <img
              src={thumb(avatarUrl, 72, 72)}
              alt={stream.displayName || stream.login}
              className="h-full w-full object-cover"
            />
          ) : (
            <div className="grid h-full w-full place-items-center text-xs font-bold text-zinc-400">
              {(stream.displayName || stream.login).slice(0, 1).toUpperCase()}
            </div>
          )}
        </div>
        <div className="min-w-0 flex-1">
          <div className="line-clamp-2 text-sm font-semibold leading-tight text-zinc-100 group-hover:text-white">
            {title}
          </div>
          <div className="truncate text-xs text-zinc-400">{stream.displayName || stream.login}</div>
          <div className="truncate text-xs text-zinc-500">
            {isLive ? (stream.category || 'Live') : 'Offline'}
          </div>
        </div>
      </div>
    </Link>
  )
}
