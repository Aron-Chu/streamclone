import type { PulseWireSourceHealth, PulseWireStory } from '../../src/pulseWireApi.ts'
import type { ReaderStatus, WireStoryView } from '../../src/utils/pulseWireStoryView.ts'

export const pulseWireFixtureNow = '2026-06-17T20:00:00Z'

export type ExpectedWireStoryViewFixture = {
  readerStatus: ReaderStatus
  confidenceLabel: WireStoryView['confidenceLabel']
  entityLabel: string
  hasPulseOrigin: boolean
  platformStates: Partial<Record<keyof WireStoryView['platformPresence'], WireStoryView['platformPresence'][keyof WireStoryView['platformPresence']]['state']>>
  missingEvidence?: string[]
}

export function pulseWireStoryFixture(partial: Partial<PulseWireStory>): PulseWireStory {
  return {
    story: {
      id: 1,
      title: 'Streamer clip spreads on Reddit',
      state: 'published',
      updatedAt: pulseWireFixtureNow,
      ...partial.story,
    },
    entity: partial.entity,
    origin: partial.origin,
    scores: {
      trend: null,
      volatility: null,
      confidence: 'single_source',
      sentiment: null,
      ...partial.scores,
    },
    windowScores: partial.windowScores,
    receipts: partial.receipts ?? [],
    windowReceipts: partial.windowReceipts ?? [],
    timeline: partial.timeline ?? [],
    windowTimeline: partial.windowTimeline ?? [],
    evidenceGallery: partial.evidenceGallery ?? [],
    matchExplanation: partial.matchExplanation ?? [],
    operatorActions: partial.operatorActions,
    tracked: partial.tracked ?? false,
    matchTier: partial.matchTier,
    firstSeenAt: partial.firstSeenAt,
    lastSeenAt: partial.lastSeenAt,
  }
}

export const wireTwitchOnlyDeveloping = pulseWireStoryFixture({
  story: { id: 910, title: 'Twitch clip is still developing', category: 'clips' },
  entity: { login: 'caseoh', displayName: 'CaseOh' },
  windowScores: { sourceCount: 1, evidenceCount: 1, velocityScore: 24, rankScore: 31 },
  windowReceipts: [
    { sourceType: 'twitch_clip', pct: 92, label: 'Twitch clip', url: 'https://clips.twitch.tv/example' },
  ],
  windowTimeline: [
    { at: pulseWireFixtureNow, sourceType: 'twitch_clip', label: 'Twitch clip posted' },
  ],
  evidenceGallery: [
    {
      id: 9101,
      canonicalUrl: 'https://clips.twitch.tv/example',
      platform: 'twitch',
      providerName: 'Twitch',
      title: 'Twitch clip receipt',
      previewStatus: 'ready',
    },
  ],
})

export const wireRedditYoutubeCorrelated = pulseWireStoryFixture({
  story: { id: 911, title: 'LSF thread spreads to YouTube before origin is found', category: 'drama' },
  entity: { login: 'caseoh', displayName: 'CaseOh' },
  scores: { trend: null, volatility: null, confidence: 'corroborated', sentiment: null },
  windowScores: { sourceCount: 2, evidenceCount: 3, velocityScore: 62, rankScore: 74 },
  windowReceipts: [
    { sourceType: 'reddit_thread', pct: 88, label: 'LSF thread', url: 'https://reddit.com/r/LivestreamFail/example' },
    { sourceType: 'youtube_video', pct: 65, label: 'YouTube repost', url: 'https://youtube.com/watch?v=abc123def45' },
  ],
  windowTimeline: [
    { at: pulseWireFixtureNow, sourceType: 'reddit_thread', label: 'LSF pickup' },
    { at: pulseWireFixtureNow, sourceType: 'youtube_video', label: 'YouTube repost' },
  ],
  evidenceGallery: [
    {
      id: 9111,
      canonicalUrl: 'https://reddit.com/r/LivestreamFail/example',
      platform: 'reddit',
      providerName: 'Reddit / LSF',
      title: 'LSF discussion',
      previewStatus: 'ready',
    },
    {
      id: 9112,
      canonicalUrl: 'https://youtube.com/watch?v=abc123def45',
      platform: 'youtube',
      providerName: 'YouTube',
      title: 'YouTube repost',
      previewStatus: 'fallback',
    },
  ],
  matchExplanation: [
    { sourceType: 'reddit_thread', matchedBy: 'entity', confidence: 0.82, factors: ['streamer name match'] },
    { sourceType: 'youtube_video', matchedBy: 'title', confidence: 0.66, factors: ['title overlap'] },
  ],
})

export const wireBanEventAuthority = pulseWireStoryFixture({
  story: {
    id: 912,
    title: 'HasanAbi Twitch ban confirmed by StreamerBans',
    summary: 'Authority receipt and discussion source are attached.',
    category: 'bans',
  },
  entity: { login: 'hasanabi', displayName: 'HasanAbi' },
  scores: { trend: null, volatility: null, confidence: 'corroborated', sentiment: null },
  windowScores: { sourceCount: 2, evidenceCount: 2, velocityScore: 38, rankScore: 64 },
  windowReceipts: [
    {
      sourceType: 'streamerbans',
      pct: 92,
      label: 'StreamerBans confirmation',
      url: 'https://streamerbans.example/hasanabi',
      previewStatus: 'ready',
      occurredAt: pulseWireFixtureNow,
    },
    { sourceType: 'reddit_thread', pct: 70, label: 'LSF discussion', url: 'https://reddit.com/r/LivestreamFail/hasan-ban' },
  ],
  windowTimeline: [
    { at: pulseWireFixtureNow, sourceType: 'streamerbans', label: 'StreamerBans event', sourceUrl: 'https://streamerbans.example/hasanabi' },
  ],
  evidenceGallery: [
    {
      id: 9121,
      canonicalUrl: 'https://streamerbans.example/hasanabi',
      platform: 'streamerbans',
      providerName: 'StreamerBans',
      title: 'HasanAbi Twitch ban confirmed',
      previewStatus: 'ready',
      matchKind: 'authority_feed',
      createdAtSrc: pulseWireFixtureNow,
    },
  ],
})

export const wireNeedsOrigin = pulseWireStoryFixture({
  ...wireRedditYoutubeCorrelated,
  story: { ...wireRedditYoutubeCorrelated.story, id: 913, title: 'Corroborated spread still needs an origin' },
})

export const wirePulseMatched = pulseWireStoryFixture({
  story: { id: 914, title: 'Pulse matched story can open the origin VOD moment', category: 'drama' },
  entity: { login: 'caseoh', displayName: 'CaseOh' },
  origin: {
    streamId: '316955094498',
    vodId: '2371095470',
    vodOffsetS: 420,
    quotes: ['chat spike matched the story origin'],
    chatSpikeSummary: 'Chat jumped 3.2x near the matched quote window.',
    originConfidence: 0.87,
    originSpikePoints: [
      { offsetS: 360, relativeS: -60, chatCount: 26, totalEmoteCount: 18, sevenTvEmoteCount: 12, viewerMax: 1500 },
      { offsetS: 420, relativeS: 0, chatCount: 81, totalEmoteCount: 66, sevenTvEmoteCount: 44, viewerMax: 2100 },
      { offsetS: 480, relativeS: 60, chatCount: 34, totalEmoteCount: 20, sevenTvEmoteCount: 11, viewerMax: 1700 },
    ],
    topEmotes: [
      { name: 'OMEGALUL', provider: '7tv', count: 42 },
      { name: 'Aware', provider: 'bttv', count: 28 },
    ],
  },
  scores: { trend: null, volatility: null, confidence: 'corroborated', sentiment: null },
  windowScores: { sourceCount: 2, evidenceCount: 2, velocityScore: 52, rankScore: 84 },
  windowReceipts: [
    { sourceType: 'pulse_origin', pct: 100, label: 'Pulse origin' },
    { sourceType: 'reddit_thread', pct: 77, label: 'LSF thread', url: 'https://reddit.com/r/LivestreamFail/pulse-origin' },
  ],
  windowTimeline: [
    { at: pulseWireFixtureNow, sourceType: 'pulse_origin', label: 'Pulse origin moment' },
    { at: pulseWireFixtureNow, sourceType: 'reddit_thread', label: 'LSF pickup' },
  ],
  evidenceGallery: [],
  matchExplanation: [
    { sourceType: 'pulse_origin', matchedBy: 'analytics_moment', confidence: 1, factors: ['origin timestamp matched'] },
  ],
})

export const wireUnverified = pulseWireStoryFixture({
  story: { id: 915, title: 'Unconfirmed streamer rumor', state: 'unverified' },
  entity: undefined,
  scores: { trend: null, volatility: null, confidence: null, sentiment: null },
  windowScores: { sourceCount: 0, evidenceCount: 0 },
  evidenceGallery: [],
})

export const wireSettled = pulseWireStoryFixture({
  story: { id: 916, title: 'Settled streamer story', state: 'settled' },
  entity: { login: 'lacy', displayName: 'Lacy' },
  scores: { trend: null, volatility: null, confidence: 'widely_reported', sentiment: null },
  windowScores: { sourceCount: 3, evidenceCount: 5, velocityScore: 12, rankScore: 49 },
  windowReceipts: [
    { sourceType: 'reddit_thread', pct: 70, label: 'LSF thread', url: 'https://reddit.com/r/LivestreamFail/settled' },
    { sourceType: 'youtube_video', pct: 60, label: 'YouTube recap', url: 'https://youtube.com/watch?v=settled12345' },
    { sourceType: 'x_post', pct: 35, label: 'X post', url: 'https://x.com/creator/status/123' },
  ],
})

export const wireEmptyCorrob = pulseWireStoryFixture({
  story: { id: 917, title: 'Corroborated shell with no usable receipts', state: 'published' },
  scores: { trend: null, volatility: null, confidence: 'corroborated', sentiment: null },
  windowScores: { sourceCount: 0, evidenceCount: 0 },
  windowReceipts: [],
  windowTimeline: [],
  evidenceGallery: [],
})

export const sourceHealthDegraded: PulseWireSourceHealth = {
  reddit: { mode: 'degraded', healthy: false, last_error: 'BrowserContext.new_page failed' },
  youtube: { mode: 'error', healthy: false, last_error: 'no videos in ytInitialData' },
  streamerbans: { mode: 'active', healthy: true, last_items: 1 },
}

export const sourceHealthLinkOnly: PulseWireSourceHealth = {
  instagram: { mode: 'link_only', healthy: true, hint: 'Instagram is link-only evidence.' },
  x: { mode: 'link_only', healthy: true, hint: 'X appears through extracted links/oEmbed unless optional x-ingest is enabled.' },
  tiktok: { mode: 'link_only', healthy: true, hint: 'TikTok links render as evidence previews; no direct discovery source is enabled.' },
  kick: { mode: 'deferred', healthy: false, hint: 'Kick discovery is planned after Evidence Gallery proves useful.' },
}

export const pulseWireStoryFixtures = {
  wireTwitchOnlyDeveloping,
  wireRedditYoutubeCorrelated,
  wireBanEventAuthority,
  wireNeedsOrigin,
  wirePulseMatched,
  wireUnverified,
  wireSettled,
  wireEmptyCorrob,
}

export const expectedWireStoryViews: Record<keyof typeof pulseWireStoryFixtures, ExpectedWireStoryViewFixture> = {
  wireTwitchOnlyDeveloping: {
    readerStatus: 'developing',
    confidenceLabel: 'Low',
    entityLabel: 'CaseOh',
    hasPulseOrigin: false,
    platformStates: { twitch: 'linked' },
  },
  wireRedditYoutubeCorrelated: {
    readerStatus: 'needs_origin',
    confidenceLabel: 'Medium',
    entityLabel: 'CaseOh',
    hasPulseOrigin: false,
    platformStates: { reddit: 'linked', youtube: 'linked', pulse: 'pending', twitch: 'missing' },
    missingEvidence: ['No Pulse or Twitch origin matched'],
  },
  wireBanEventAuthority: {
    readerStatus: 'needs_origin',
    confidenceLabel: 'Medium',
    entityLabel: 'HasanAbi',
    hasPulseOrigin: false,
    platformStates: { bans: 'linked', reddit: 'linked' },
    missingEvidence: ['No Pulse or Twitch origin matched'],
  },
  wireNeedsOrigin: {
    readerStatus: 'needs_origin',
    confidenceLabel: 'Medium',
    entityLabel: 'CaseOh',
    hasPulseOrigin: false,
    platformStates: { reddit: 'linked', youtube: 'linked' },
    missingEvidence: ['No Pulse or Twitch origin matched'],
  },
  wirePulseMatched: {
    readerStatus: 'corroborated',
    confidenceLabel: 'Medium',
    entityLabel: 'CaseOh',
    hasPulseOrigin: true,
    platformStates: { pulse: 'matched', reddit: 'linked' },
  },
  wireUnverified: {
    readerStatus: 'unverified',
    confidenceLabel: 'Insufficient data',
    entityLabel: 'Unconfirmed streamer rumor',
    hasPulseOrigin: false,
    platformStates: {},
  },
  wireSettled: {
    readerStatus: 'settled',
    confidenceLabel: 'High',
    entityLabel: 'Lacy',
    hasPulseOrigin: false,
    platformStates: { reddit: 'linked', youtube: 'linked', x: 'linked' },
  },
  wireEmptyCorrob: {
    readerStatus: 'insufficient_data',
    confidenceLabel: 'Insufficient data',
    entityLabel: 'Corroborated shell with no usable receipts',
    hasPulseOrigin: false,
    platformStates: {},
  },
}
