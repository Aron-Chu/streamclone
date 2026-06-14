import assert from 'node:assert/strict'
import { describe, it } from 'node:test'
import type { Stream } from '../src/api.ts'
import {
  isLocalSort,
  sortDirectoryStreams,
  type DirectorySort,
} from '../src/utils/directorySort.ts'

function stream(partial: Partial<Stream> & Pick<Stream, 'login'>): Stream {
  return {
    title: partial.title ?? partial.login,
    category: partial.category ?? 'Just Chatting',
    viewers: partial.viewers ?? 100,
    thumbnailUrl: partial.thumbnailUrl ?? '',
    ...partial,
    login: partial.login,
  }
}

describe('sortDirectoryStreams', () => {
  const streams = [
    stream({ login: 'a', title: 'Zebra Stream', category: 'Fortnite', viewers: 500 }),
    stream({ login: 'b', title: 'Alpha Stream', category: 'Minecraft', viewers: 1000 }),
    stream({ login: 'c', title: 'Beta Stream', category: 'Apex Legends', viewers: 200 }),
  ]

  it('preserves API order for viewers sort', () => {
    const sorted = sortDirectoryStreams(streams, 'viewers')
    assert.deepEqual(sorted.map(s => s.login), ['a', 'b', 'c'])
  })

  it('sorts by title A-Z', () => {
    const sorted = sortDirectoryStreams(streams, 'title')
    assert.deepEqual(sorted.map(s => s.title), ['Alpha Stream', 'Beta Stream', 'Zebra Stream'])
  })

  it('sorts by category A-Z', () => {
    const sorted = sortDirectoryStreams(streams, 'category')
    assert.deepEqual(sorted.map(s => s.category), ['Apex Legends', 'Fortnite', 'Minecraft'])
  })

  it('does not mutate the input array', () => {
    const copy = [...streams]
    sortDirectoryStreams(streams, 'title')
    assert.deepEqual(streams, copy)
  })

  it('uses displayName or login as title fallback', () => {
    const items = [
      stream({ login: 'z', title: '', displayName: 'Zed Channel', viewers: 10 }),
      stream({ login: 'a', title: '', displayName: 'Amy Channel', viewers: 20 }),
    ]
    const sorted = sortDirectoryStreams(items, 'title')
    assert.deepEqual(sorted.map(s => s.login), ['a', 'z'])
  })
})

describe('isLocalSort', () => {
  const cases: [DirectorySort, boolean][] = [
    ['viewers', false],
    ['title', true],
    ['category', true],
  ]

  for (const [sort, expected] of cases) {
    it(`returns ${expected} for ${sort}`, () => {
      assert.equal(isLocalSort(sort), expected)
    })
  }
})
