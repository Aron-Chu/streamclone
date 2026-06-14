import assert from 'node:assert/strict'
import { describe, it } from 'node:test'
import type { FollowedChannel, Stream } from '../src/api.ts'
import { buildHomeFeed, followedChannelToStream } from '../src/utils/homeFeed.ts'

function stream(login: string, partial: Partial<Stream> = {}): Stream {
  return {
    login,
    title: partial.title ?? login,
    category: partial.category ?? 'Just Chatting',
    viewers: partial.viewers ?? 100,
    thumbnailUrl: partial.thumbnailUrl ?? '',
    ...partial,
  }
}

function followed(login: string, partial: Partial<FollowedChannel> = {}): FollowedChannel {
  return {
    id: partial.id ?? login,
    login,
    displayName: partial.displayName ?? login,
    isLive: partial.isLive ?? false,
    ...partial,
  }
}

describe('buildHomeFeed', () => {
  it('excludes followed channels from recommended live streams', () => {
    const feed = buildHomeFeed({
      followedChannels: [
        followed('HasanAbi', { isLive: true }),
        followed('OfflineChan'),
      ],
      topStreams: [
        stream('hasanabi'),
        stream('newlive'),
        stream('offlinechan'),
        stream('another'),
      ],
    })

    assert.deepEqual(feed.followingLive.map(channel => channel.login), ['HasanAbi'])
    assert.deepEqual(feed.recommendedLive.map(item => item.login), ['newlive', 'another'])
  })

  it('handles signed-out users by recommending top streams', () => {
    const feed = buildHomeFeed({
      topStreams: [stream('one'), stream('two')],
    })

    assert.deepEqual(feed.followingLive, [])
    assert.deepEqual(feed.recommendedLive.map(item => item.login), ['one', 'two'])
  })

  it('handles empty inputs without placeholder rows', () => {
    const feed = buildHomeFeed({})

    assert.deepEqual(feed.followingLive, [])
    assert.deepEqual(feed.recommendedLive, [])
  })
})

describe('followedChannelToStream', () => {
  it('maps live followed channels into stream cards', () => {
    const mapped = followedChannelToStream(followed('livechan', {
      displayName: 'LiveChan',
      isLive: true,
      category: 'Chess',
      viewers: 123,
      thumbnailUrl: 'https://thumb/{width}x{height}.jpg',
      profileImage: 'https://avatar.jpg',
    }))

    assert.equal(mapped.login, 'livechan')
    assert.equal(mapped.title, 'LiveChan')
    assert.equal(mapped.category, 'Chess')
    assert.equal(mapped.viewers, 123)
    assert.equal(mapped.isLive, true)
    assert.equal(mapped.profileImageUrl, 'https://avatar.jpg')
  })
})
