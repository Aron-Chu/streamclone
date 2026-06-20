import type { InsightCard } from '../api.ts'
import type { PulseWireCommunityPost } from '../pulseWireApi.ts'
import { pulseWireDisplayThumbnail } from './twitchClipThumb.ts'

/** Map metadata LSF insight cards to Pulse Wire community post shape. */
export function insightToCommunityPost(post: InsightCard): PulseWireCommunityPost {
  const thumb = post.thumbnail?.trim()
  const displayThumbnailUrl = thumb ? pulseWireDisplayThumbnail(thumb) : undefined
  return {
    id: Number(post.id) || 0,
    title: post.title,
    url: post.url,
    permalink: post.permalink,
    source: 'reddit',
    subreddit: post.subreddit,
    score: post.score,
    comments: post.comments,
    thumbnailUrl: thumb,
    displayThumbnailUrl,
    previewKind: displayThumbnailUrl ? 'reddit' : 'none',
    flair: post.flairText,
    streamerLogin: post.streamerTags?.[0],
    streamerDisplayName: post.streamerTags?.[0],
  }
}
