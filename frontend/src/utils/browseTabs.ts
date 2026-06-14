export type BrowseTab = 'categories' | 'live'

export const BROWSE_TAB_STORAGE_KEY = 'streamclone:browseTab'

export function isBrowseTab(value: string | null): value is BrowseTab {
  return value === 'categories' || value === 'live'
}

export function getStoredBrowseTab(): BrowseTab {
  if (typeof sessionStorage === 'undefined') return 'categories'
  try {
    const stored = sessionStorage.getItem(BROWSE_TAB_STORAGE_KEY)
    if (isBrowseTab(stored)) return stored
  } catch {
    // sessionStorage unavailable
  }
  return 'categories'
}

export function setStoredBrowseTab(tab: BrowseTab): void {
  try {
    sessionStorage.setItem(BROWSE_TAB_STORAGE_KEY, tab)
  } catch {
    // sessionStorage unavailable
  }
}

export function browseTabFromPathname(pathname: string): BrowseTab | null {
  if (pathname === '/browse/live') return 'live'
  if (pathname === '/browse') return 'categories'
  return null
}

export function getBrowseTabFromPathname(pathname: string): BrowseTab {
  if (pathname === '/browse/live') return 'live'
  return 'categories'
}

export function browseTabPath(tab: BrowseTab): string {
  return tab === 'live' ? '/browse/live' : '/browse'
}
