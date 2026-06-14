import { useId, useRef, useState, type KeyboardEvent, type ReactNode } from 'react'
import {
  RIGHT_RAIL_DEFAULT_TAB,
  RIGHT_RAIL_TABS,
  type RightRailTabId,
} from './rightRailTabs.ts'

/**
 * RightRail is the tabbed container to the right of the analytics chart
 * (Requirement 3). It owns tab order, the default/active selection, and the
 * Moments empty state, but stays agnostic about the concrete panel content:
 * callers pass the Moments/Emotes/Clips/Sync panels as ReactNode props (or
 * render props), so the real wiring into Analytics.tsx lives elsewhere.
 *
 * The JSX-free tab contract (`RIGHT_RAIL_TABS`, `RIGHT_RAIL_DEFAULT_TAB`, and
 * the `RightRailTab`/`RightRailTabId` types) lives in `./rightRailTabs.ts` so
 * the Node test runner can import it; it is re-exported here so existing
 * consumers can keep importing from `RightRail`.
 */
export {
  RIGHT_RAIL_DEFAULT_TAB,
  RIGHT_RAIL_TABS,
  type RightRailTab,
  type RightRailTabId,
} from './rightRailTabs.ts'

// A render prop receives whether the active stream has rollup data so panels
// can decide their own empty states; the Moments empty state is handled by the
// container itself per Requirement 3.4.
export type RightRailPanelRender = (ctx: { hasRollupData: boolean }) => ReactNode

export type RightRailPanel = ReactNode | RightRailPanelRender

export interface RightRailProps {
  /**
   * Identifies the current stream. When this value changes, the rail resets
   * to the Moments tab (Requirement 3.2). A full page reload remounts the
   * component and therefore also resets to Moments (Requirement 3.3).
   */
  streamId?: string | null
  /**
   * Whether minute-level rollup data is available for the current stream.
   * When false and the Moments tab is active, the rail shows the
   * sync-needed empty state (Requirement 3.4).
   */
  hasRollupData?: boolean
  moments?: RightRailPanel
  emotes?: RightRailPanel
  clips?: RightRailPanel
  sync?: RightRailPanel
  /** Optional override for the Moments empty state (Requirement 3.4). */
  momentsEmptyState?: ReactNode
  className?: string
}

function renderPanel(panel: RightRailPanel | undefined, hasRollupData: boolean): ReactNode {
  if (typeof panel === 'function') {
    return (panel as RightRailPanelRender)({ hasRollupData })
  }
  return panel ?? null
}

function MomentsEmptyState({ override }: { override?: ReactNode }) {
  if (override !== undefined) return <>{override}</>
  return (
    <div className="flex flex-col items-center gap-2 px-4 py-10 text-center">
      <p className="text-sm font-black text-white">No moments yet</p>
      <p className="text-xs leading-relaxed text-zinc-400">
        Sync chat for this stream to surface its strongest moments.
      </p>
    </div>
  )
}

export function RightRail({
  streamId,
  hasRollupData = false,
  moments,
  emotes,
  clips,
  sync,
  momentsEmptyState,
  className,
}: RightRailProps) {
  const baseId = useId()
  const [activeTab, setActiveTab] = useState<RightRailTabId>(RIGHT_RAIL_DEFAULT_TAB)

  // Reset to Moments whenever the stream changes (Requirement 3.2). Tracking
  // the previous streamId in state and reconciling during render avoids an
  // extra commit + effect pass and keeps the active panel correct on the first
  // render after a stream switch.
  const [prevStreamId, setPrevStreamId] = useState<string | null | undefined>(streamId)
  if (streamId !== prevStreamId) {
    setPrevStreamId(streamId)
    setActiveTab(RIGHT_RAIL_DEFAULT_TAB)
  }

  const tabRefs = useRef<Record<RightRailTabId, HTMLButtonElement | null>>({
    moments: null,
    emotes: null,
    clips: null,
    sync: null,
  })

  const focusTab = (id: RightRailTabId) => {
    setActiveTab(id)
    tabRefs.current[id]?.focus()
  }

  const onTabKeyDown = (event: KeyboardEvent<HTMLButtonElement>, index: number) => {
    let nextIndex: number | null = null
    switch (event.key) {
      case 'ArrowRight':
      case 'ArrowDown':
        nextIndex = (index + 1) % RIGHT_RAIL_TABS.length
        break
      case 'ArrowLeft':
      case 'ArrowUp':
        nextIndex = (index - 1 + RIGHT_RAIL_TABS.length) % RIGHT_RAIL_TABS.length
        break
      case 'Home':
        nextIndex = 0
        break
      case 'End':
        nextIndex = RIGHT_RAIL_TABS.length - 1
        break
      default:
        return
    }
    if (nextIndex !== null) {
      event.preventDefault()
      focusTab(RIGHT_RAIL_TABS[nextIndex].id)
    }
  }

  const tabId = (id: RightRailTabId) => `${baseId}-tab-${id}`
  const panelId = (id: RightRailTabId) => `${baseId}-panel-${id}`

  const renderActivePanel = (): ReactNode => {
    switch (activeTab) {
      case 'moments':
        return hasRollupData ? (
          renderPanel(moments, hasRollupData)
        ) : (
          <MomentsEmptyState override={momentsEmptyState} />
        )
      case 'emotes':
        return renderPanel(emotes, hasRollupData)
      case 'clips':
        return renderPanel(clips, hasRollupData)
      case 'sync':
        return renderPanel(sync, hasRollupData)
      default:
        return null
    }
  }

  return (
    <div
      className={
        className ??
        'flex flex-col overflow-hidden rounded-xl border border-white/10 bg-zinc-950/60'
      }
    >
      <div
        role="tablist"
        aria-label="Analytics moment timeline"
        aria-orientation="horizontal"
        className="flex shrink-0 border-b border-white/10"
      >
        {RIGHT_RAIL_TABS.map((tab, index) => {
          const selected = tab.id === activeTab
          return (
            <button
              key={tab.id}
              ref={node => {
                tabRefs.current[tab.id] = node
              }}
              role="tab"
              type="button"
              id={tabId(tab.id)}
              aria-selected={selected}
              aria-controls={panelId(tab.id)}
              tabIndex={selected ? 0 : -1}
              onClick={() => setActiveTab(tab.id)}
              onKeyDown={event => onTabKeyDown(event, index)}
              className={`flex-1 px-3 py-2 text-xs font-black transition focus:outline-none focus-visible:ring-2 focus-visible:ring-violet-400 ${
                selected
                  ? 'border-b-2 border-violet-400 text-white'
                  : 'border-b-2 border-transparent text-zinc-400 hover:text-zinc-200'
              }`}
            >
              {tab.label}
            </button>
          )
        })}
      </div>
      <div
        role="tabpanel"
        id={panelId(activeTab)}
        aria-labelledby={tabId(activeTab)}
        tabIndex={0}
        className="min-h-0 flex-1 overflow-auto focus:outline-none"
      >
        {renderActivePanel()}
      </div>
    </div>
  )
}

export default RightRail
