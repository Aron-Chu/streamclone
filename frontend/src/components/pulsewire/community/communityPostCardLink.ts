import type { PulseWireCommunityPost } from '../../../pulseWireApi'

export function communityPostThreadUrl(post: Pick<PulseWireCommunityPost, 'permalink' | 'url'>): string {
  return post.permalink || post.url
}

export const COMMUNITY_CARD_LINK_CLASS =
  'group block overflow-hidden rounded-[14px] border border-[#2A2A2E] bg-[#121217] transition hover:border-[#A970FF]/40'

export const COMMUNITY_CARD_LINK_CLASS_COMPACT =
  'group block rounded-[14px] border border-[#2A2A2E] bg-[#121217] p-4 transition hover:border-[#A970FF]/40'

export const COMMUNITY_CARD_LINK_CLASS_CHANNEL =
  'group block rounded-[14px] border border-white/10 bg-white/[0.035] p-4 transition hover:border-violet-300/40'

export const COMMUNITY_EMBED_META_LINK_CLASS =
  'group block p-4 transition hover:bg-[#16161B]'
