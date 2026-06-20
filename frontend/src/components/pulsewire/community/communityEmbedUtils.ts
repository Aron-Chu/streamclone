import type { PulseWireCommunityPost } from '../../../pulseWireApi'

/** Reddit oEmbed often returns a link-only blockquote — skip it when we already show the title. */
export function isUsefulCommunityEmbed(post: PulseWireCommunityPost): boolean {
  const embedUrl = post.embedUrl?.trim()
  const embedHtml = post.embedHtml?.trim()
  if (embedUrl) return true
  if (!embedHtml) return false
  if (post.linkedPlatform === 'reddit' && !/<iframe\b/i.test(embedHtml)) return false
  return true
}
