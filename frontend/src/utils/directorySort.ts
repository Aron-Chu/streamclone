import type { Stream } from '../api'

export type DirectorySort = 'viewers' | 'title' | 'category'

export const DIRECTORY_SORT_STORAGE_KEY = 'streamclone:directorySort'

export const SORT_OPTIONS: { value: DirectorySort; label: string }[] = [
  { value: 'viewers', label: 'Most viewed' },
  { value: 'title', label: 'Title (A–Z)' },
  { value: 'category', label: 'Category (A–Z)' },
]

export function getStoredDirectorySort(): DirectorySort {
  if (typeof sessionStorage === 'undefined') return 'viewers'
  try {
    const stored = sessionStorage.getItem(DIRECTORY_SORT_STORAGE_KEY)
    if (stored === 'viewers' || stored === 'title' || stored === 'category') return stored
  } catch {
    // sessionStorage unavailable
  }
  return 'viewers'
}

export function setStoredDirectorySort(sort: DirectorySort): void {
  try {
    sessionStorage.setItem(DIRECTORY_SORT_STORAGE_KEY, sort)
  } catch {
    // sessionStorage unavailable
  }
}

export function sortDirectoryStreams(streams: Stream[], sort: DirectorySort): Stream[] {
  if (sort === 'viewers') return streams
  const copy = [...streams]
  if (sort === 'title') {
    copy.sort((a, b) => {
      const titleA = (a.title || a.displayName || a.login).toLowerCase()
      const titleB = (b.title || b.displayName || b.login).toLowerCase()
      return titleA.localeCompare(titleB)
    })
  } else if (sort === 'category') {
    copy.sort((a, b) =>
      (a.category || '').toLowerCase().localeCompare((b.category || '').toLowerCase()),
    )
  }
  return copy
}

export function isLocalSort(sort: DirectorySort): boolean {
  return sort !== 'viewers'
}
