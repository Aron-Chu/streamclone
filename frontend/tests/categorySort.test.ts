import assert from 'node:assert/strict'
import { describe, it } from 'node:test'
import type { Category } from '../src/api.ts'
import { formatCategoryViewers, sortCategories } from '../src/utils/categorySort.ts'

function category(partial: Partial<Category> & Pick<Category, 'id' | 'name'>): Category {
  return {
    thumbnailUrl: partial.thumbnailUrl ?? '',
    ...partial,
  }
}

describe('sortCategories', () => {
  const categories = [
    category({ id: 'a', name: 'Zelda', viewers: 100 }),
    category({ id: 'b', name: 'Apex Legends', viewers: 200 }),
    category({ id: 'c', name: 'Minecraft', viewers: 200 }),
    category({ id: 'd', name: 'Chess' }),
  ]

  it('sorts by viewers descending with name tiebreaker', () => {
    const sorted = sortCategories(categories, 'viewers')
    assert.deepEqual(sorted.map(item => item.name), ['Apex Legends', 'Minecraft', 'Zelda', 'Chess'])
  })

  it('sorts by name A-Z', () => {
    const sorted = sortCategories(categories, 'name')
    assert.deepEqual(sorted.map(item => item.name), ['Apex Legends', 'Chess', 'Minecraft', 'Zelda'])
  })

  it('does not mutate the input array', () => {
    const copy = [...categories]
    sortCategories(categories, 'name')
    assert.deepEqual(categories, copy)
  })
})

describe('formatCategoryViewers', () => {
  it('formats category viewer totals compactly', () => {
    assert.equal(formatCategoryViewers(612), '612 viewers')
    assert.equal(formatCategoryViewers(612_400), '612.4K viewers')
    assert.equal(formatCategoryViewers(1_200_000), '1.2M viewers')
  })

  it('falls back to zero when viewers are missing', () => {
    assert.equal(formatCategoryViewers(undefined), '0 viewers')
  })
})
