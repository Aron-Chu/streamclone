import type { PulseWireReceipt, PulseWireStory } from '../pulseWireApi'
import { storyReceipts } from '../pulseWireApi'

export function receiptThumbnail(url?: string): string | undefined {
  if (!url) return undefined
  if (/\.(jpg|jpeg|png|webp|gif)(\?|$)/i.test(url)) return url

  const youtube = url.match(/(?:youtube\.com\/watch\?v=|youtu\.be\/|youtube\.com\/shorts\/)([\w-]+)/)
  if (youtube?.[1]) return `https://img.youtube.com/vi/${youtube[1]}/mqdefault.jpg`

  const clip = url.match(/clips\.twitch\.tv\/([^/?]+)|twitch\.tv\/[^/]+\/clip\/([^/?]+)/)
  if (clip) {
    const slug = clip[1] || clip[2]
    if (slug) return `https://clips-media-assets2.twitch.tv/${slug}-preview-480x272.jpg`
  }

  return undefined
}

export function storyEntityAvatar(story: PulseWireStory): string | undefined {
  return story.entity?.avatarUrl
}

export function storyThumbnail(story: PulseWireStory): string | undefined {
  if (story.entity?.avatarUrl) return story.entity.avatarUrl
  const timeline = story.windowTimeline?.length ? story.windowTimeline : story.timeline
  for (const step of timeline ?? []) {
    const thumb = receiptThumbnail(step.sourceUrl)
    if (thumb) return thumb
  }
  for (const receipt of storyReceipts(story) ?? []) {
    if (receipt.thumbnailUrl) return receipt.thumbnailUrl
    const thumb = receiptThumbnail(receipt.url)
    if (thumb) return thumb
  }
  for (const preview of story.evidenceGallery ?? []) {
    if (preview.thumbnailUrl) return preview.thumbnailUrl
  }
  return undefined
}

export function receiptThumb(receipt: PulseWireReceipt): string | undefined {
  if (receipt.thumbnailUrl) return receipt.thumbnailUrl
  return receiptThumbnail(receipt.url)
}
