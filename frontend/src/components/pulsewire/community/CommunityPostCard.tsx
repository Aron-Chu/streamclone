import { useMemo, useState } from 'react'
import type { PulseWireCommunityPost } from '../../../pulseWireApi'
import { formatEngagementCount, hasEngagementCounts } from '../../../utils/pulseWireFormat'
import { pulseWireDisplayThumbnail } from '../../../utils/twitchClipThumb'
import { isUsefulCommunityEmbed } from './communityEmbedUtils'
import CommunityPostEmbed from './CommunityPostEmbed'
import SourceBadge from './SourceBadge'
import {
  COMMUNITY_CARD_LINK_CLASS,
  COMMUNITY_CARD_LINK_CLASS_CHANNEL,
  COMMUNITY_CARD_LINK_CLASS_COMPACT,
  COMMUNITY_EMBED_META_LINK_CLASS,
  communityPostThreadUrl,
} from './communityPostCardLink'

type Props = {
  post: PulseWireCommunityPost
  variant?: 'channel' | 'wire'
}

export default function CommunityPostCard({ post, variant = 'wire' }: Props) {
  const threadUrl = communityPostThreadUrl(post)
  const subreddit = post.subreddit ? `r/${post.subreddit.replace(/^r\//, '')}` : 'Reddit'
  const isChannel = variant === 'channel'
  const previewKind = post.previewKind ?? ((post.displayThumbnailUrl || post.thumbnailUrl) ? 'fallback' : 'none')
  const hasImagePreview = previewKind !== 'none'
  const hasEmbed = isUsefulCommunityEmbed(post)
  const hasSelfText = Boolean(post.selfText?.trim())
  const showEngagement = hasEngagementCounts(post.score, post.comments)
  const [previewFailed, setPreviewFailed] = useState(false)
  const previewSrc = useMemo(() => {
    if (previewFailed || !hasImagePreview) return undefined
    return pulseWireDisplayThumbnail(post.displayThumbnailUrl ?? post.thumbnailUrl)
  }, [post.displayThumbnailUrl, post.thumbnailUrl, previewFailed, hasImagePreview])

  const metaRow = showEngagement ? (
    <div className="mt-2 flex flex-wrap items-center gap-2 text-[11px] font-semibold text-[#7A7A85]">
      <span>{formatEngagementCount(post.score)} upvotes</span>
      <span>·</span>
      <span>{formatEngagementCount(post.comments)} comments</span>
    </div>
  ) : null

  const selfTextBlock = hasSelfText ? (
    <p className="mt-2 line-clamp-4 text-xs leading-relaxed text-[#ADADB8]">{post.selfText}</p>
  ) : null

  const streamerTag = post.streamerLogin || post.streamerDisplayName ? (
    <div className="mt-2 flex flex-wrap gap-1.5">
      <span className="rounded-full border border-[#A970FF]/25 bg-[#9147FF]/10 px-2 py-0.5 text-[10px] font-semibold uppercase text-[#A970FF]">
        Matched: {post.streamerDisplayName || post.streamerLogin}
      </span>
    </div>
  ) : null

  const flairTag = post.flair?.trim() ? (
    <span
      data-testid="community-flair-badge"
      className="rounded-full border border-[#FF7447]/35 bg-[#2A1710] px-2 py-0.5 text-[10px] font-semibold uppercase text-[#FFB199]"
    >
      {post.flair}
    </span>
  ) : null

  const embedBlock = hasEmbed ? (
    <CommunityPostEmbed
      embedUrl={post.embedUrl}
      embedHtml={post.embedHtml}
      linkedPlatform={post.linkedPlatform}
      title={post.title}
      className={!isChannel && (previewSrc || hasImagePreview) ? 'rounded-t-[14px]' : ''}
    />
  ) : null

  const linkProps = {
    href: threadUrl,
    target: '_blank' as const,
    rel: 'noreferrer' as const,
    'data-testid': 'community-post-card',
  }

  if (!isChannel && previewSrc) {
    return (
      <a {...linkProps} className={COMMUNITY_CARD_LINK_CLASS}>
        <div className="relative aspect-video bg-[#1B1B1F]">
          <img
            data-testid="community-preview"
            src={previewSrc}
            alt=""
            className="h-full w-full object-cover transition duration-300 group-hover:scale-105"
            loading="lazy"
            onError={() => setPreviewFailed(true)}
          />
          <span className="absolute left-3 top-3 rounded-full border border-[#FF4500]/35 bg-black/70 px-2 py-0.5 text-[10px] font-bold uppercase text-[#FF8B60]">
            {subreddit}
          </span>
        </div>
        <div className="p-4">
          <div className="flex flex-wrap items-center gap-2">
            {flairTag}
          </div>
          <h4 className="line-clamp-2 text-sm font-semibold leading-5 text-[#F7F7F8]">{post.title}</h4>
          {selfTextBlock}
          {metaRow}
          {streamerTag}
        </div>
      </a>
    )
  }

  if (!isChannel && hasEmbed && !previewSrc) {
    return (
      <article className="overflow-hidden rounded-[14px] border border-[#2A2A2E] bg-[#121217]">
        {embedBlock}
        <a {...linkProps} className={COMMUNITY_EMBED_META_LINK_CLASS}>
          <div className="flex flex-wrap items-center gap-2">
            <SourceBadge label={subreddit} variant="inline" />
            {flairTag}
            {post.linkedPlatform ? (
              <span className="text-[10px] font-semibold uppercase tracking-wide text-[#7A7A85]">
                Linked {post.linkedPlatform}
              </span>
            ) : (
              <span className="text-[10px] font-semibold uppercase tracking-wide text-[#7A7A85]">
                Discussion thread
              </span>
            )}
          </div>
          <h4 className="mt-2 line-clamp-2 text-sm font-semibold leading-5 text-[#F7F7F8]">{post.title}</h4>
          {selfTextBlock}
          {metaRow}
          {streamerTag}
        </a>
      </article>
    )
  }

  const hasCompactThumb = Boolean(previewSrc && hasImagePreview)
  const baseLinkClass = isChannel ? COMMUNITY_CARD_LINK_CLASS_CHANNEL : COMMUNITY_CARD_LINK_CLASS_COMPACT
  const wireLinkClass = hasCompactThumb ? `${baseLinkClass} flex gap-3` : baseLinkClass

  return (
    <a {...linkProps} className={wireLinkClass}>
      {!hasImagePreview || !previewSrc ? (
        <>
          <div className="flex flex-wrap items-center gap-2">
            <SourceBadge label={subreddit} variant="inline" />
            {flairTag}
            {!isChannel ? (
              <span className="text-[10px] font-semibold uppercase tracking-wide text-[#7A7A85]">
                {hasSelfText ? 'Text post' : 'Discussion thread'}
              </span>
            ) : (
              <span className="text-[10px] font-semibold uppercase tracking-wide text-[#7A7A85]">
                Text post · No preview
              </span>
            )}
          </div>
          {embedBlock}
          <h4 className="mt-2 line-clamp-2 text-sm font-semibold leading-5 text-[#F7F7F8]">{post.title}</h4>
          {selfTextBlock}
          {metaRow}
          {streamerTag}
        </>
      ) : (
        <>
          <img
            data-testid="community-preview"
            src={previewSrc}
            alt=""
            className="h-14 w-14 shrink-0 rounded-lg object-cover"
            loading="lazy"
            onError={() => setPreviewFailed(true)}
          />
          <div className="min-w-0 flex-1">
            <div className="flex flex-wrap items-center gap-2">
              <div className={`text-[10px] font-bold uppercase tracking-[0.08em] ${
                isChannel ? 'text-violet-300' : 'text-[#A970FF]'
              }`}>
                {subreddit}
              </div>
              {flairTag}
            </div>
            <h4 className="mt-1 line-clamp-2 text-sm font-semibold leading-5 text-[#F7F7F8]">{post.title}</h4>
            {selfTextBlock}
            {metaRow}
            {streamerTag}
          </div>
        </>
      )}
    </a>
  )
}
