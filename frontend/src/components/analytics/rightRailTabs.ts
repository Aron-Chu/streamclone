/**
 * Pure, JSX-free tab contract for the analytics Right Rail (Requirement 3).
 *
 * These constants live in a standalone `.ts` module (not `RightRail.tsx`) so
 * they can be imported by the Node test runner, which loads tests via
 * `node --experimental-strip-types --test` and cannot parse `.tsx` (JSX is not
 * stripped and the `.tsx` extension is not a recognized module type). The
 * `RightRail.tsx` component re-exports everything here so existing consumers can
 * keep importing from either location.
 */

export type RightRailTabId = 'moments' | 'emotes' | 'clips' | 'sync'

export interface RightRailTab {
  id: RightRailTabId
  label: string
}

// Tab order is fixed: Moments, Emotes, Clips, Sync (Requirement 3.1).
// Additional tabs may be appended after Sync in future iterations.
export const RIGHT_RAIL_TABS: readonly RightRailTab[] = [
  { id: 'moments', label: 'Moments' },
  { id: 'emotes', label: 'Emotes' },
  { id: 'clips', label: 'Clips' },
  { id: 'sync', label: 'Sync' },
] as const

// The rail always opens on Moments and resets to Moments on stream change
// (Requirement 3.2).
export const RIGHT_RAIL_DEFAULT_TAB: RightRailTabId = 'moments'
