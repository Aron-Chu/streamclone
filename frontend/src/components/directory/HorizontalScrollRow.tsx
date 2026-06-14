import { useCallback, useEffect, useRef, useState, type ReactNode } from 'react'

interface HorizontalScrollRowProps {
  children: ReactNode
  className?: string
  gapClassName?: string
  stepRatio?: number
}

function ScrollChevron({ direction }: { direction: 'left' | 'right' }) {
  return (
    <svg
      viewBox="0 0 24 24"
      aria-hidden
      className={`h-5 w-5 ${direction === 'right' ? '' : 'rotate-180'}`}
      fill="none"
      stroke="currentColor"
      strokeWidth="2.5"
      strokeLinecap="round"
      strokeLinejoin="round"
    >
      <path d="M9 6l6 6-6 6" />
    </svg>
  )
}

export function HorizontalScrollRow({
  children,
  className = '',
  gapClassName = 'gap-4',
  stepRatio = 0.85,
}: HorizontalScrollRowProps) {
  const scrollerRef = useRef<HTMLDivElement>(null)
  const [canScrollLeft, setCanScrollLeft] = useState(false)
  const [canScrollRight, setCanScrollRight] = useState(false)
  const [hasOverflow, setHasOverflow] = useState(false)

  const updateScrollEdges = useCallback(() => {
    const el = scrollerRef.current
    if (!el) return
    const maxScroll = Math.max(0, el.scrollWidth - el.clientWidth)
    setHasOverflow(maxScroll > 4)
    setCanScrollLeft(el.scrollLeft > 4)
    setCanScrollRight(el.scrollLeft < maxScroll - 4)
  }, [])

  useEffect(() => {
    const el = scrollerRef.current
    if (!el) return
    updateScrollEdges()
    el.addEventListener('scroll', updateScrollEdges, { passive: true })
    const observer = new ResizeObserver(updateScrollEdges)
    observer.observe(el)
    for (const child of el.children) {
      if (child instanceof Element) observer.observe(child)
    }
    return () => {
      el.removeEventListener('scroll', updateScrollEdges)
      observer.disconnect()
    }
  }, [updateScrollEdges, children])

  const scrollByStep = (direction: -1 | 1) => {
    const el = scrollerRef.current
    if (!el) return
    const first = el.children[0] instanceof HTMLElement ? el.children[0] : null
    const second = el.children[1] instanceof HTMLElement ? el.children[1] : null
    const itemStep = first && second
      ? Math.max(1, second.offsetLeft - first.offsetLeft)
      : Math.max(1, first?.getBoundingClientRect().width ?? el.clientWidth * stepRatio)
    const visibleItems = Math.max(1, Math.floor(el.clientWidth / itemStep))
    const pageItems = Math.max(1, visibleItems - 1)
    el.scrollBy({ left: direction * itemStep * pageItems, behavior: 'smooth' })
  }

  const arrowClass =
    'absolute top-[42%] z-20 grid h-10 w-10 -translate-y-1/2 place-items-center rounded-full border border-white/15 bg-[#0e0e10]/95 text-zinc-100 shadow-lg shadow-black/40 backdrop-blur-sm transition hover:border-white/30 hover:bg-[#18181b] hover:text-white disabled:pointer-events-none disabled:opacity-0 focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[#9147ff]'

  return (
    <div className={`relative w-full overflow-hidden ${className}`}>
      {hasOverflow ? (
        <>
          <div
            className={`pointer-events-none absolute inset-y-0 left-0 z-10 w-16 bg-gradient-to-r from-[#0e0e10] via-[#0e0e10]/75 to-transparent transition-opacity ${canScrollLeft ? 'opacity-100' : 'opacity-0'}`}
            aria-hidden
          />
          <button
            type="button"
            aria-label="Scroll left"
            onClick={() => scrollByStep(-1)}
            disabled={!canScrollLeft}
            className={`${arrowClass} left-2`}
          >
            <ScrollChevron direction="left" />
          </button>
        </>
      ) : null}
      {hasOverflow ? (
        <>
          <div
            className={`pointer-events-none absolute inset-y-0 right-0 z-10 w-16 bg-gradient-to-l from-[#0e0e10] via-[#0e0e10]/75 to-transparent transition-opacity ${canScrollRight ? 'opacity-100' : 'opacity-0'}`}
            aria-hidden
          />
          <button
            type="button"
            aria-label="Scroll right"
            onClick={() => scrollByStep(1)}
            disabled={!canScrollRight}
            className={`${arrowClass} right-2`}
          >
            <ScrollChevron direction="right" />
          </button>
        </>
      ) : null}
      <div
        ref={scrollerRef}
        className={`horizontal-scroll-row flex snap-x snap-mandatory ${gapClassName} overflow-x-auto scroll-smooth pb-3`}
      >
        {children}
      </div>
    </div>
  )
}
