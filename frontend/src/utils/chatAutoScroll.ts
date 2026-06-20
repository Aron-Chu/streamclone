/** Distance from bottom (px) treated as "following live chat". */
export const CHAT_AT_BOTTOM_THRESHOLD_PX = 32

/** Minimum upward scroll delta (px) to treat as intentional scroll-up. */
export const CHAT_SCROLL_UP_DELTA_PX = 2

export function isChatAtBottom(
  scrollTop: number,
  scrollHeight: number,
  clientHeight: number,
  threshold = CHAT_AT_BOTTOM_THRESHOLD_PX,
): boolean {
  return scrollHeight - scrollTop - clientHeight < threshold
}

export function isChatScrollUp(
  scrollTop: number,
  previousScrollTop: number,
  delta = CHAT_SCROLL_UP_DELTA_PX,
): boolean {
  return scrollTop < previousScrollTop - delta
}

/** Wheel/trackpad upward intent should pause auto-follow immediately. */
export function shouldPauseAutoFollowOnWheel(deltaY: number): boolean {
  return deltaY < 0
}

/** Latched pause: only explicit scroll-up should pause auto-follow. */
export function shouldPauseAutoFollowOnScroll(
  scrollTop: number,
  previousScrollTop: number,
): boolean {
  return isChatScrollUp(scrollTop, previousScrollTop)
}

/** Latched pause never resumes from reaching the bottom via scroll alone. */
export function shouldResumeAutoFollowAtBottom(_atBottom: boolean): boolean {
  return false
}

/** Whether scroll-up should latch auto-follow into a paused state. */
export function shouldLatchPauseOnScrollUp(scrolledUp: boolean): boolean {
  return scrolledUp
}
