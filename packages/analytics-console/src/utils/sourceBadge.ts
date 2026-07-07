export type ViewerSourceBadgeTone = 'strong' | 'info' | 'muted'

export interface ViewerSourceBadgeInfo {
  label: string
  tone: ViewerSourceBadgeTone
}

/** Map backend viewerSource to portal badge copy (design §13A.3). */
export function mapViewerSourceBadge(source?: string): ViewerSourceBadgeInfo | null {
  switch (source) {
    case 'live':
      return { label: 'Live samples', tone: 'strong' }
    case 'tt':
    case 'twitchtracker':
      return { label: 'TwitchTracker filled', tone: 'info' }
    case 'merged':
      return { label: 'Merged coverage', tone: 'info' }
    case 'restored':
      return { label: 'Restored from archive', tone: 'muted' }
    case '':
    case undefined:
    case 'unknown':
      return null
    default:
      return { label: 'Viewer data unavailable', tone: 'muted' }
  }
}

export function viewerSourceBadgeClass(tone: ViewerSourceBadgeTone): string {
  switch (tone) {
    case 'strong':
      return 'border-cyan-400/25 bg-cyan-500/10 text-cyan-200'
    case 'info':
      return 'border-violet-400/20 bg-violet-500/10 text-violet-200'
    default:
      return 'border-white/10 bg-white/[0.04] text-zinc-400'
  }
}
