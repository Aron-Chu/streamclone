import { describe, expect, it } from 'vitest'
import {
  activityBandFractions,
  activityZoneFraction,
  lerpActivityLayout,
} from '../src/emotePlotSelection.ts'

describe('activityBandFractions', () => {
  it('allocates more trace rail when emotes are plotted (collapsed)', () => {
    const base = activityBandFractions(false, false)
    const plotted = activityBandFractions(false, true)
    expect(plotted.trace).toBeGreaterThan(base.trace)
    expect(plotted.chat).toBeLessThan(base.chat)
    expect(plotted.chat + plotted.trace + plotted.bars).toBeCloseTo(1, 5)
  })

  it('expands activity zone fraction when expanded', () => {
    expect(activityZoneFraction(true)).toBeGreaterThan(activityZoneFraction(false))
    expect(activityBandFractions(true).trace).toBeGreaterThan(activityBandFractions(false).trace)
  })

  it('grows emote bar band area more than collapsed on expand', () => {
    const collapsedArea = activityZoneFraction(false) * activityBandFractions(false).bars
    const expandedArea = activityZoneFraction(true) * activityBandFractions(true).bars
    expect(expandedArea).toBeGreaterThan(collapsedArea * 1.35)
  })
})

describe('lerpActivityLayout', () => {
  it('uses plotted-emote band layout when flagged', () => {
    const plottedCollapsed = lerpActivityLayout(0, true)
    const defaultCollapsed = lerpActivityLayout(0, false)
    expect(plottedCollapsed.trace).toBeGreaterThan(defaultCollapsed.trace)
  })

  it('interpolates midpoints at progress 0.5', () => {
    const half = lerpActivityLayout(0.5, true)
    const collapsed = lerpActivityLayout(0, true)
    const expanded = lerpActivityLayout(1, true)
    expect(half.zoneFraction).toBeCloseTo(
      (collapsed.zoneFraction + expanded.zoneFraction) / 2,
      5,
    )
  })
})
