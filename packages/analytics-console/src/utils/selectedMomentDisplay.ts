import {
  buildMomentScoreModel,
  computeMomentScore100,
  computeStreamBaselines,
  detectPickReason,
  heatmapEmotesFromRollup,
  topEmotesFromRollup,
} from '@streamclone/pulse-core'
import type { AnalyticsMinuteRollup, AnalyticsTopEmote } from '../apiTypes.ts'
import type { ReplayHeatmapDetailPoint, ReplayHeatmapPoint } from '../types/heatmap.ts'
import { minuteEmoteTotal } from '../components/analytics/chartRollupUtils.ts'
import { buildTwitchVodUrl, type VodLinkState } from './twitchVodUrl.ts'
import { formatVodOffset, rollupOffsetSeconds } from './consoleFormat.ts'

export interface SelectedMomentDisplay {
  offsetSeconds: number
  offsetStr: string
  vodUrl?: string
  scoreModel: ReturnType<typeof buildMomentScoreModel>
  momentEmotes: ReturnType<typeof topEmotesFromRollup>
  activityLine: string
}

export function buildSelectedMomentDisplay({
  rollup,
  rollups,
  startedAt,
  vodLinkState,
  topEmotesCatalog,
  heatmapPoint,
  heatmapDetail,
}: {
  rollup: AnalyticsMinuteRollup
  rollups: AnalyticsMinuteRollup[]
  startedAt?: string
  vodLinkState: VodLinkState
  topEmotesCatalog?: AnalyticsTopEmote[]
  heatmapPoint?: ReplayHeatmapPoint | null
  heatmapDetail?: ReplayHeatmapDetailPoint | null
}): SelectedMomentDisplay {
  const baselines = computeStreamBaselines(rollups)
  let offsetSeconds = 0
  let offsetStr = ''
  if (startedAt) {
    offsetSeconds = rollupOffsetSeconds(rollup, startedAt)
    offsetStr = formatVodOffset(offsetSeconds)
  }

  const fallbackReason = detectPickReason(rollup, baselines, topEmotesCatalog)
  const scoreModel = buildMomentScoreModel({
    heatmapPoint,
    heatmapDetail,
    fallbackScore100: computeMomentScore100(rollup, baselines, rollups),
    fallbackReason,
    fallbackTopEmotes: heatmapEmotesFromRollup(rollup, 5, topEmotesCatalog),
  })
  const vodUrl =
    vodLinkState.status === 'linked' && vodLinkState.vodId
      ? buildTwitchVodUrl(vodLinkState.vodId, offsetSeconds)
      : undefined
  const chatCount = rollup.chatCount ?? 0
  const emoteCount = minuteEmoteTotal(rollup)

  return {
    offsetSeconds,
    offsetStr,
    vodUrl,
    scoreModel,
    momentEmotes: topEmotesFromRollup(rollup, 3, topEmotesCatalog),
    activityLine: `${chatCount} chat · ${emoteCount} emotes`,
  }
}
