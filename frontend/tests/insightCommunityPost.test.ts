import assert from 'node:assert/strict'
import test from 'node:test'

import type { InsightCard } from '../src/api.ts'
import { insightToCommunityPost } from '../src/utils/insightCommunityPost.ts'

test('insightToCommunityPost maps LSF thumbnail fields for CommunityPostCard', () => {
  const post: InsightCard = {
    id: 'abc123',
    title: 'Streamer fails at boss',
    url: 'https://reddit.com/r/LivestreamFail/comments/abc123',
    permalink: '/r/LivestreamFail/comments/abc123',
    thumbnail: 'https://i.redd.it/preview.jpg',
    score: 4200,
    comments: 88,
    createdUtc: 1_700_000_000,
    subreddit: 'LivestreamFail',
    flairText: 'Fail',
    streamerTags: ['ninja'],
  }

  const mapped = insightToCommunityPost(post)

  assert.equal(mapped.displayThumbnailUrl, 'https://i.redd.it/preview.jpg')
  assert.equal(mapped.previewKind, 'reddit')
  assert.equal(mapped.flair, 'Fail')
  assert.equal(mapped.thumbnailUrl, 'https://i.redd.it/preview.jpg')
  assert.equal(mapped.streamerLogin, 'ninja')
})

test('insightToCommunityPost sets previewKind none when thumbnail is missing', () => {
  const mapped = insightToCommunityPost({
    id: '1',
    title: 'Text-only thread',
    url: 'https://reddit.com/r/LivestreamFail/comments/1',
    permalink: '/r/LivestreamFail/comments/1',
    score: 10,
    comments: 2,
    createdUtc: 1_700_000_000,
  })

  assert.equal(mapped.previewKind, 'none')
  assert.equal(mapped.displayThumbnailUrl, undefined)
})
