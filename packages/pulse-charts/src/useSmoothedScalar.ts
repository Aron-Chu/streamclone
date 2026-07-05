import { useEffect, useRef, useState } from 'react'

const SNAP_EPSILON = 0.05
const LERP_ALPHA = 0.35

/** Exponential ease step toward `target` (0 < alpha <= 1). */
export function lerpScalar(current: number, target: number, alpha: number): number {
  return current + (target - current) * alpha
}

/**
 * RAF-smoothed scalar for chart crosshairs and band layout.
 * Snaps instantly when `enabled` is false.
 */
export function useSmoothedScalar(target: number, enabled = true): number {
  const [display, setDisplay] = useState(target)
  const displayRef = useRef(target)
  const targetRef = useRef(target)
  const rafRef = useRef<number | null>(null)

  targetRef.current = target

  useEffect(() => {
    if (rafRef.current != null) {
      cancelAnimationFrame(rafRef.current)
      rafRef.current = null
    }

    if (!enabled) {
      displayRef.current = target
      setDisplay(target)
      return
    }

    const step = () => {
      const t = targetRef.current
      const c = displayRef.current
      if (Math.abs(c - t) < SNAP_EPSILON) {
        displayRef.current = t
        setDisplay(t)
        rafRef.current = null
        return
      }
      const next = lerpScalar(c, t, LERP_ALPHA)
      displayRef.current = next
      setDisplay(next)
      rafRef.current = requestAnimationFrame(step)
    }

    if (Math.abs(displayRef.current - target) < SNAP_EPSILON) {
      displayRef.current = target
      setDisplay(target)
      return
    }

    rafRef.current = requestAnimationFrame(step)

    return () => {
      if (rafRef.current != null) {
        cancelAnimationFrame(rafRef.current)
        rafRef.current = null
      }
    }
  }, [target, enabled])

  return enabled ? display : target
}
