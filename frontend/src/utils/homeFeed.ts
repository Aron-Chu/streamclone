import type { FollowedChannel, Stream } from '../api'

export interface HomeFeedInput {
  followedChannels?: FollowedChannel[]
  topStreams?: Stream[]
  recommendationLimit?: number
}

export interface HomeFeed {
  followingLive: FollowedChannel[]
  recommendedLive: Stream[]
}

function loginKey(login?: string) {
  return (login ?? '').trim().toLowerCase()
}

function uniqueByLogin<T extends { login: string }>(items: T[]): T[] {
  const seen = new Set<string>()
  const out: T[] = []
  for (const item of items) {
    const key = loginKey(item.login)
    if (!key || seen.has(key)) continue
    seen.add(key)
    out.push(item)
  }
  return out
}

export function buildHomeFeed({
  followedChannels = [],
  topStreams = [],
  recommendationLimit = 12,
}: HomeFeedInput): HomeFeed {
  const followingLive = uniqueByLogin(followedChannels.filter(channel => channel.isLive))
  const followedLogins = new Set(followedChannels.map(channel => loginKey(channel.login)).filter(Boolean))
  const recommendedLive = uniqueByLogin(topStreams)
    .filter(stream => !followedLogins.has(loginKey(stream.login)))
    .slice(0, Math.max(0, recommendationLimit))

  return {
    followingLive,
    recommendedLive,
  }
}

export function followedChannelToStream(channel: FollowedChannel): Stream {
  return {
    id: channel.id,
    login: channel.login,
    displayName: channel.displayName,
    title: channel.title || channel.displayName || channel.login,
    category: channel.category || 'Live',
    viewers: channel.viewers ?? 0,
    thumbnailUrl: channel.thumbnailUrl || '',
    isLive: channel.isLive,
    profileImageUrl: channel.profileImage,
  }
}
