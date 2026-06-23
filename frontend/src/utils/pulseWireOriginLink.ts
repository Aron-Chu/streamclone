import type { PulseWireStory } from '../pulseWireApi.ts'
import { buildVodDeepLink } from '@streamclone/pulse-core'

export function buildPulseWireOriginHref(story: PulseWireStory): string | undefined {
  const login = story.entity?.login?.trim()
  const streamId = story.origin?.streamId?.trim()
  const vodId = story.origin?.vodId?.trim()
  if (!login || !streamId || !vodId) return undefined
  return buildVodDeepLink(login, vodId, story.origin?.vodOffsetS ?? 0, streamId)
}
