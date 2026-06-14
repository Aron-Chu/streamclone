export const SYNC_CHAT_AND_VIEWERS_LABEL = 'Sync chat & viewers'
export const SYNC_CHAT_AND_EMOTES_LABEL = 'Sync chat & emotes'
export const SYNCING_LABEL = 'Syncing…'
export const RESYNC_LABEL = 'Re-sync'

export type SyncCtaPlacement = 'header' | 'chartEmpty' | 'rightRail' | 'syncPanel'

export const SYNC_CTA_PLACEMENTS: readonly SyncCtaPlacement[] = [
  'header',
  'chartEmpty',
  'rightRail',
  'syncPanel',
] as const

export interface SyncStreamState {
  hasViewerSamples: boolean
  hasChatRollups: boolean
  syncing: boolean
}

export interface SyncCtaResult {
  label: string | null
  visible: boolean
  disabled: boolean
}

export function syncCtaLabel(state: SyncStreamState): string {
  if (state.syncing) return SYNCING_LABEL
  if (!state.hasChatRollups && !state.hasViewerSamples) return SYNC_CHAT_AND_VIEWERS_LABEL
  if (!state.hasChatRollups && state.hasViewerSamples) return SYNC_CHAT_AND_EMOTES_LABEL
  return RESYNC_LABEL
}

export function syncCtaForPlacement(
  state: SyncStreamState,
  placement: SyncCtaPlacement,
): SyncCtaResult {
  if (state.syncing) {
    return { label: SYNCING_LABEL, visible: true, disabled: true }
  }

  const syncComplete = state.hasChatRollups
  if (syncComplete) {
    if (placement === 'syncPanel') {
      return { label: RESYNC_LABEL, visible: true, disabled: false }
    }
    return { label: null, visible: false, disabled: false }
  }

  return { label: syncCtaLabel(state), visible: true, disabled: false }
}
