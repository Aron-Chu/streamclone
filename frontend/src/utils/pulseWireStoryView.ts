import type {
  PulseWireReceipt,
  PulseWireSourceHealth,
  PulseWireStory,
  PulseWireTimelineStep,
} from '../pulseWireApi.ts'
import { formatRelativeTime } from './pulseWireFormat.ts'

export type ReaderStatus =
  | 'developing'
  | 'corroborated'
  | 'needs_origin'
  | 'active'
  | 'settled'
  | 'unverified'
  | 'insufficient_data'

export type PlatformKey = 'pulse' | 'twitch' | 'reddit' | 'youtube' | 'x' | 'tiktok' | 'bans'

export type PlatformPresenceState =
  | 'matched'
  | 'linked'
  | 'missing'
  | 'pending'
  | 'disabled'
  | 'degraded'
  | 'not_applicable'

export type SourceRole = 'origin' | 'amplification' | 'context' | 'authority'

export type WireStoryView = {
  id: number
  title: string
  entityLabel: string
  entitySublabel?: string
  category?: string
  readerStatus: ReaderStatus
  confidenceLabel: 'High' | 'Medium' | 'Low' | 'Insufficient data'
  displayReason: string
  displayReasonBullets: string[]
  platformPresence: Record<PlatformKey, {
    state: PlatformPresenceState
    label: string
    count: number
    role?: SourceRole
    href?: string
    metricLabel?: string
  }>
  missingEvidence: string[]
  sourceCount: number
  evidenceCount: number
  lastUpdatedLabel?: string
  primaryActionLabel: 'Open story' | 'View receipts'
  canCreateClip: boolean
  hasPulseOrigin: boolean
}

const PLATFORM_ORDER: PlatformKey[] = ['pulse', 'twitch', 'reddit', 'youtube', 'x', 'tiktok', 'bans']

const SOURCE_TO_PLATFORM: Array<{ test: RegExp; platform: PlatformKey; role: SourceRole; label: string }> = [
  { test: /pulse|moment|origin/i, platform: 'pulse', role: 'origin', label: 'Pulse matched' },
  { test: /twitch|clip/i, platform: 'twitch', role: 'origin', label: 'Twitch clip' },
  { test: /reddit|lsf/i, platform: 'reddit', role: 'amplification', label: 'Reddit' },
  { test: /youtube|shorts?/i, platform: 'youtube', role: 'context', label: 'YouTube' },
  { test: /\bx\b|x_post|twitter/i, platform: 'x', role: 'amplification', label: 'X link' },
  { test: /tiktok/i, platform: 'tiktok', role: 'amplification', label: 'TikTok link' },
  { test: /ban|streamerbans/i, platform: 'bans', role: 'authority', label: 'StreamerBans' },
]

const PLATFORM_LABELS: Record<PlatformKey, string> = {
  pulse: 'Pulse',
  twitch: 'Twitch',
  reddit: 'Reddit',
  youtube: 'YouTube',
  x: 'X',
  tiktok: 'TikTok',
  bans: 'Bans',
}

function storyReceipts(story: PulseWireStory): PulseWireReceipt[] | undefined {
  return story.windowReceipts?.length ? story.windowReceipts : story.receipts
}

function storyTimeline(story: PulseWireStory): PulseWireTimelineStep[] | undefined {
  return story.windowTimeline?.length ? story.windowTimeline : story.timeline
}

function storyUpdatedAt(story: PulseWireStory): string | undefined {
  return story.lastSeenAt ?? story.story.updatedAt ?? story.scores.updatedAt ?? story.windowScores?.computedAt
}

const DEFAULT_ROLES: Record<PlatformKey, SourceRole> = {
  pulse: 'origin',
  twitch: 'origin',
  reddit: 'amplification',
  youtube: 'context',
  x: 'amplification',
  tiktok: 'amplification',
  bans: 'authority',
}

const YOUTUBE_AMPLIFICATION_PATTERN = /\brepost\b|\breupload\b|\breaction\b|\breacts?\b|\bclip\b|\bshorts?\b|\bmirror\b|\bhighlight\b|\brecap\b/i
const EXTRACTED_LINK_PATTERN = /url|link|comment|manual/i

function sourcePlatform(sourceType: string): PlatformKey | undefined {
  return SOURCE_TO_PLATFORM.find(item => item.test.test(sourceType))?.platform
}

function classifyYouTubeRole(...values: Array<string | undefined>): SourceRole {
  const text = values.filter(Boolean).join(' ')
  if (YOUTUBE_AMPLIFICATION_PATTERN.test(text) || EXTRACTED_LINK_PATTERN.test(text)) return 'amplification'
  return 'context'
}

function sourceRole(receipt: PulseWireReceipt, platform: PlatformKey): SourceRole {
  if (platform === 'youtube') {
    return classifyYouTubeRole(receipt.sourceType, receipt.label, receipt.url, receipt.risk, receipt.previewStatus)
  }
  const sourceType = receipt.sourceType
  return SOURCE_TO_PLATFORM.find(item => item.test.test(sourceType))?.role ?? DEFAULT_ROLES[platform]
}

function receiptMetric(receipt: PulseWireReceipt): string | undefined {
  if (receipt.label && /\d/.test(receipt.label)) return receipt.label
  if (receipt.pct > 0) return `${Math.round(receipt.pct)}%`
  return undefined
}

function sourceModeState(mode?: string): PlatformPresenceState | undefined {
  if (mode === 'off') return 'disabled'
  if (mode === 'error') return 'degraded'
  if (mode === 'degraded') return 'degraded'
  if (mode === 'deferred') return 'pending'
  return undefined
}

function emptyPresence(platform: PlatformKey, sourceHealth?: PulseWireSourceHealth, story?: PulseWireStory): WireStoryView['platformPresence'][PlatformKey] {
  const health = sourceHealth?.[platform]
  const healthState = sourceModeState(health?.mode)
  if (healthState) {
    return {
      state: healthState,
      label: health?.mode === 'off' ? `${PLATFORM_LABELS[platform]} off` : `${PLATFORM_LABELS[platform]} ${healthState}`,
      count: 0,
      role: DEFAULT_ROLES[platform],
    }
  }

  const receipts = story ? storyReceipts(story) ?? [] : []
  const onlyTwitchClip =
    receipts.length > 0 &&
    receipts.every(r => sourcePlatform(r.sourceType) === 'twitch')
  const ingestActive =
    platform === 'reddit'
      ? sourceHealth?.reddit?.mode === 'active' || sourceHealth?.reddit?.mode === 'degraded'
      : platform === 'youtube'
        ? sourceHealth?.youtube?.mode === 'active' || sourceHealth?.youtube?.mode === 'degraded'
        : false

  if (onlyTwitchClip && ingestActive && (platform === 'reddit' || platform === 'youtube')) {
    return {
      state: 'pending',
      label: 'Corroboration pending',
      count: 0,
      role: DEFAULT_ROLES[platform],
    }
  }

  if (platform === 'pulse') {
    return { state: 'pending', label: 'Pulse match pending', count: 0, role: 'origin' }
  }
  if (platform === 'x' || platform === 'tiktok') {
    return { state: 'missing', label: `No ${PLATFORM_LABELS[platform]} link`, count: 0, role: 'amplification' }
  }
  if (platform === 'bans') {
    return { state: 'not_applicable', label: 'No ban signal', count: 0, role: 'authority' }
  }
  return { state: 'missing', label: `No ${PLATFORM_LABELS[platform]}`, count: 0, role: DEFAULT_ROLES[platform] }
}

function buildPlatformPresence(story: PulseWireStory, sourceHealth?: PulseWireSourceHealth): WireStoryView['platformPresence'] {
  const presence = Object.fromEntries(
    PLATFORM_ORDER.map(platform => [platform, emptyPresence(platform, sourceHealth, story)]),
  ) as WireStoryView['platformPresence']

  if (story.origin) {
    presence.pulse = { state: 'matched', label: 'Pulse matched', count: 1, role: 'origin' }
    presence.twitch = { state: 'matched', label: 'Twitch origin', count: 1, role: 'origin' }
  }

  for (const receipt of storyReceipts(story) ?? []) {
    const platform = sourcePlatform(receipt.sourceType)
    if (!platform) continue
    const current = presence[platform]
    const role = sourceRole(receipt, platform)
    const count = current.state === 'matched' || current.state === 'linked'
      ? current.count + 1
      : 1
    const state: PlatformPresenceState = current.state === 'matched' || (platform === 'pulse' && story.origin) ? 'matched' : 'linked'
    presence[platform] = {
      state,
      label: receipt.label ?? SOURCE_TO_PLATFORM.find(item => item.platform === platform)?.label ?? PLATFORM_LABELS[platform],
      count,
      role,
      href: receipt.url ?? current.href,
      metricLabel: receiptMetric(receipt) ?? current.metricLabel,
    }
  }

  for (const preview of story.evidenceGallery ?? []) {
    const platform = sourcePlatform(preview.platform)
    if (!platform) continue
    const current = presence[platform]
    if (current.state === 'matched' || current.state === 'linked') continue
    presence[platform] = {
      state: 'linked',
      label: preview.providerName ?? PLATFORM_LABELS[platform],
      count: 1,
      role: platform === 'youtube'
        ? classifyYouTubeRole(preview.platform, preview.providerName, preview.title, preview.matchKind, preview.note)
        : DEFAULT_ROLES[platform],
      href: preview.canonicalUrl,
      metricLabel: preview.previewStatus,
    }
  }

  return presence
}

function countSources(presence: WireStoryView['platformPresence'], story: PulseWireStory): number {
  return story.windowScores?.sourceCount
    ?? PLATFORM_ORDER.filter(platform => {
      const state = presence[platform].state
      return state === 'matched' || state === 'linked'
    }).length
}

function evidenceCount(story: PulseWireStory, sourceCount: number): number {
  return story.windowScores?.evidenceCount
    ?? Math.max(storyReceipts(story)?.length ?? 0, story.evidenceGallery?.length ?? 0, sourceCount)
}

function hasOrigin(presence: WireStoryView['platformPresence']): boolean {
  return presence.pulse.state === 'matched' || presence.twitch.state === 'matched'
}

export function hasSearchedMissingOrigin(story: PulseWireStory): boolean {
  return story.story.originSearchStatus === 'searched_missing'
}

export function hasClipReadyOrigin(story: PulseWireStory): boolean {
  return Boolean(
    story.origin?.streamId?.trim() &&
    story.origin?.vodId?.trim() &&
    story.origin?.vodOffsetS != null,
  )
}

const OFFICIAL_RESPONSE_PATTERN = /\bofficial\b|\bstatement\b|\bresponse\b|creator site|creator-domain|creator domain|manual[_ -]?curation/i

function hasOfficialResponseEvidence(story: PulseWireStory): boolean {
  const receipts = storyReceipts(story) ?? []
  if (receipts.some(receipt => OFFICIAL_RESPONSE_PATTERN.test([
    receipt.sourceType,
    receipt.label,
    receipt.url,
  ].filter(Boolean).join(' ')))) {
    return true
  }

  if ((story.evidenceGallery ?? []).some(item => OFFICIAL_RESPONSE_PATTERN.test([
    item.platform,
    item.providerName,
    item.title,
    item.author,
    item.canonicalUrl,
    item.matchKind,
    item.note,
  ].filter(Boolean).join(' ')))) {
    return true
  }

  return (story.matchExplanation ?? []).some(match => OFFICIAL_RESPONSE_PATTERN.test([
    match.sourceType,
    match.matchedBy,
    match.sourceUrl,
    ...(match.factors ?? []),
  ].filter(Boolean).join(' ')))
}

function expectsOfficialResponse(story: PulseWireStory, sourceCount: number, evidence: number): boolean {
  return (sourceCount >= 2 || evidence >= 2) && !hasOfficialResponseEvidence(story)
}

function readerStatus(story: PulseWireStory, sourceCount: number, originMatched: boolean): ReaderStatus {
  if (story.story.state === 'unverified') return 'unverified'
  if (story.story.state === 'settled') return 'settled'
  if (sourceCount === 0) return 'insufficient_data'
  if (!originMatched && sourceCount >= 2) return 'needs_origin'
  if (story.scores.confidence === 'single_source' || sourceCount <= 1) return 'developing'
  if (story.scores.confidence === 'corroborated' || story.scores.confidence === 'widely_reported' || sourceCount >= 2) {
    return 'corroborated'
  }
  if (story.story.state === 'published') return 'active'
  return 'developing'
}

function confidenceLabel(story: PulseWireStory, sourceCount: number): WireStoryView['confidenceLabel'] {
  if (sourceCount <= 0) return 'Insufficient data'
  if (story.scores.confidence === 'widely_reported') return 'High'
  if (story.scores.confidence === 'corroborated' || sourceCount >= 2) return 'Medium'
  if (story.scores.confidence === 'single_source' || sourceCount === 1) return 'Low'
  return 'Insufficient data'
}

function platformSummary(presence: WireStoryView['platformPresence'], platform: PlatformKey) {
  const item = presence[platform]
  if (item.state === 'matched' || item.state === 'linked') return item
  return undefined
}

function buildReasonBullets(story: PulseWireStory, view: Pick<WireStoryView, 'platformPresence' | 'sourceCount' | 'evidenceCount' | 'lastUpdatedLabel'>): string[] {
  const bullets: string[] = []
  const reddit = platformSummary(view.platformPresence, 'reddit')
  const youtube = platformSummary(view.platformPresence, 'youtube')
  const bans = platformSummary(view.platformPresence, 'bans')
  const twitch = platformSummary(view.platformPresence, 'twitch')

  if (view.sourceCount >= 2) {
    bullets.push(`${view.sourceCount} sources are attached inside this window.`)
  } else if (view.sourceCount === 1) {
    bullets.push('One source is attached; corroboration is still missing.')
  }
  if (reddit) bullets.push(`${reddit.label}${reddit.metricLabel ? ` shows ${reddit.metricLabel}` : ' is linked as community pickup'}.`)
  if (youtube) bullets.push(`${youtube.label} evidence is linked for context.`)
  if (bans) bullets.push('StreamerBans or moderation evidence is attached as an authority source.')
  if (twitch && twitch.state === 'matched') bullets.push('A Twitch origin or clip is attached.')
  if (!hasOrigin(view.platformPresence)) {
    bullets.push(hasSearchedMissingOrigin(story) ? 'Origin search ran; no Twitch origin was found.' : 'No Pulse or Twitch origin is matched yet.')
  }
  if (view.lastUpdatedLabel) bullets.push(`Updated ${view.lastUpdatedLabel}.`)
  if (!bullets.length && story.story.summary) bullets.push(story.story.summary)
  return bullets.slice(0, 4)
}

export type WireStoryViewOptions = {
  analystMode?: boolean
}

function platformLinked(presence: WireStoryView['platformPresence'], platform: PlatformKey): boolean {
  const state = presence[platform].state
  return state === 'matched' || state === 'linked'
}

function buildMissingEvidence(
  presence: WireStoryView['platformPresence'],
  story: PulseWireStory,
  sourceCount: number,
  evidence: number,
): string[] {
  const missing: string[] = []
  if (!hasOrigin(presence)) missing.push(hasSearchedMissingOrigin(story) ? 'No Twitch origin found' : 'No Pulse or Twitch origin matched')
  if (presence.reddit.state === 'missing') missing.push('No Reddit pickup found')
  if (presence.youtube.state === 'missing') missing.push('No YouTube repost found')
  if (presence.x.state === 'missing') missing.push('No X link found')
  if (presence.tiktok.state === 'missing') missing.push('No TikTok link found')
  if (expectsOfficialResponse(story, sourceCount, evidence)) missing.push('No official response found')
  if (story.story.category === 'bans' && presence.bans.state !== 'matched' && presence.bans.state !== 'linked') {
    missing.push('No StreamerBans authority evidence attached')
  }
  return missing
}

function buildReaderMissingEvidence(
  presence: WireStoryView['platformPresence'],
  story: PulseWireStory,
  sourceCount: number,
  evidence: number,
): string[] {
  if (sourceCount < 2) return []
  const full = buildMissingEvidence(presence, story, sourceCount, evidence)
  if (!full.length) return []

  const hasReddit = platformLinked(presence, 'reddit')
  const hasYoutube = platformLinked(presence, 'youtube')
  const hasTwitchOrigin = platformLinked(presence, 'twitch') || platformLinked(presence, 'pulse')
  const spreadStarted = hasReddit || hasYoutube || hasTwitchOrigin
  if (!spreadStarted) return []

  return full.filter(gap => {
    if (gap.includes('X link') || gap.includes('TikTok')) {
      return hasReddit && hasYoutube
    }
    if (gap.includes('Pulse') || gap.includes('Twitch origin') || gap.includes('Twitch clip')) {
      return hasReddit || hasYoutube
    }
    if (gap.includes('Reddit pickup')) {
      return hasYoutube || hasTwitchOrigin
    }
    if (gap.includes('YouTube')) {
      return hasReddit || hasTwitchOrigin
    }
    return true
  })
}

function truncateLabel(value: string, max = 56): string {
  const trimmed = value.trim()
  if (trimmed.length <= max) return trimmed
  return `${trimmed.slice(0, max - 3)}...`
}

function entityPresentation(story: PulseWireStory): { label: string; sublabel?: string } {
  if (story.entity?.displayName || story.entity?.login) {
    return { label: story.entity.displayName || story.entity.login || 'Story developing' }
  }
  const title = (story.story.title || '').trim()
  if (title && title !== 'Story developing') {
    return {
      label: truncateLabel(title),
      sublabel: 'Streamer not matched yet',
    }
  }
  return { label: 'Story developing', sublabel: 'Streamer not matched yet' }
}

export function storySourceCount(story: PulseWireStory, sourceHealth?: PulseWireSourceHealth): number {
  if (story.windowScores?.sourceCount != null) return story.windowScores.sourceCount
  const presence = buildPlatformPresence(story, sourceHealth)
  return countSources(presence, story)
}

export function isCrossPlatformStory(story: PulseWireStory, sourceHealth?: PulseWireSourceHealth): boolean {
  return storySourceCount(story, sourceHealth) >= 2
}

export function toWireStoryView(
  story: PulseWireStory,
  sourceHealth?: PulseWireSourceHealth,
  options?: WireStoryViewOptions,
): WireStoryView {
  const platformPresence = buildPlatformPresence(story, sourceHealth)
  const sourceCount = countSources(platformPresence, story)
  const evidence = evidenceCount(story, sourceCount)
  const originMatched = hasOrigin(platformPresence)
  const updatedAt = storyUpdatedAt(story)
  const baseView = {
    platformPresence,
    sourceCount,
    evidenceCount: evidence,
    lastUpdatedLabel: updatedAt ? formatRelativeTime(updatedAt) : undefined,
  }
  const displayReasonBullets = buildReasonBullets(story, baseView)
  const timeline = storyTimeline(story)
  const entity = entityPresentation(story)

  return {
    id: story.story.id,
    title: story.story.title || 'Story developing',
    entityLabel: entity.label,
    entitySublabel: entity.sublabel,
    category: story.story.category,
    readerStatus: readerStatus(story, sourceCount, originMatched),
    confidenceLabel: confidenceLabel(story, sourceCount),
    displayReason: displayReasonBullets[0]
      ?? (timeline?.[0]?.label ? `${timeline[0].label} is the earliest attached evidence.` : 'Evidence is still gathering.'),
    displayReasonBullets,
    platformPresence,
    missingEvidence: options?.analystMode
      ? buildMissingEvidence(platformPresence, story, sourceCount, evidence)
      : buildReaderMissingEvidence(platformPresence, story, sourceCount, evidence),
    sourceCount,
    evidenceCount: evidence,
    lastUpdatedLabel: baseView.lastUpdatedLabel,
    primaryActionLabel: evidence > 1 ? 'View receipts' : 'Open story',
    canCreateClip: hasClipReadyOrigin(story),
    hasPulseOrigin: platformPresence.pulse.state === 'matched',
  }
}

export function platformEntries(view: WireStoryView) {
  return PLATFORM_ORDER.map(platform => ({ platform, ...view.platformPresence[platform] }))
}

export function compactPlatformEntries(view: WireStoryView) {
  const entries = platformEntries(view)
  const linked = entries.filter(item => item.state === 'matched' || item.state === 'linked')
  const missing = entries.filter(item => item.state === 'missing' || item.state === 'pending')
  if (!missing.length) return linked
  return [...linked, missing[0]]
}
