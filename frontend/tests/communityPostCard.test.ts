import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import test from 'node:test'

import { communityPostThreadUrl, COMMUNITY_CARD_LINK_CLASS } from '../src/components/pulsewire/community/communityPostCardLink.ts'

const here = dirname(fileURLToPath(import.meta.url))
const communityCardSource = readFileSync(
  join(here, '../src/components/pulsewire/community/CommunityPostCard.tsx'),
  'utf8',
)
const pulseWirePageSource = readFileSync(
  join(here, '../src/components/pulsewire/PulseWirePage.tsx'),
  'utf8',
)

test('communityPostThreadUrl prefers permalink over url', () => {
  assert.equal(
    communityPostThreadUrl({
      permalink: 'https://reddit.com/r/LivestreamFail/comments/abc123/thread/',
      url: 'https://reddit.com/r/LivestreamFail/comments/abc123',
    }),
    'https://reddit.com/r/LivestreamFail/comments/abc123/thread/',
  )
})

test('communityPostThreadUrl falls back to url when permalink is missing', () => {
  assert.equal(
    communityPostThreadUrl({
      url: 'https://reddit.com/r/LivestreamFail/comments/xyz789',
    }),
    'https://reddit.com/r/LivestreamFail/comments/xyz789',
  )
})

test('CommunityPostCard uses full-card anchor link without Open thread button', () => {
  assert.match(communityCardSource, /'data-testid':\s*'community-post-card'/)
  assert.match(communityCardSource, /href:\s*threadUrl/)
  assert.match(communityCardSource, /COMMUNITY_CARD_LINK_CLASS/)
  assert.match(COMMUNITY_CARD_LINK_CLASS, /hover:border-\[#A970FF\]\/40/)
  assert.doesNotMatch(communityCardSource, /Open thread/)
})

test('TrendingLead community hero is a full-card anchor without Open thread button', () => {
  assert.match(pulseWirePageSource, /data-testid="trending-lead"/)
  assert.match(pulseWirePageSource, /group block overflow-hidden rounded-xl/)
  assert.doesNotMatch(pulseWirePageSource, /Open thread/)
})
