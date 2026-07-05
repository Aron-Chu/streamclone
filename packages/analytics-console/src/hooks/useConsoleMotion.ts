import { useEffect, useState } from 'react'

function readPrefersReducedMotion(): boolean {
  if (typeof window === 'undefined') return false
  return window.matchMedia('(prefers-reduced-motion: reduce)').matches
}

/** Console motion gate — respects OS reduced-motion preference. */
export function useConsoleMotion(): { motionEnabled: boolean } {
  const [reduced, setReduced] = useState(readPrefersReducedMotion)

  useEffect(() => {
    const mq = window.matchMedia('(prefers-reduced-motion: reduce)')
    const onChange = () => setReduced(mq.matches)
    mq.addEventListener('change', onChange)
    return () => mq.removeEventListener('change', onChange)
  }, [])

  return { motionEnabled: !reduced }
}
