import assert from 'node:assert/strict'
import { describe, it } from 'node:test'
import { BROWSE_TAB_STORAGE_KEY, browseTabFromPathname, browseTabPath, getStoredBrowseTab, isBrowseTab, setStoredBrowseTab } from '../src/utils/browseTabs.ts'

describe('isBrowseTab', () => {
  it('accepts supported browse tabs', () => {
    assert.equal(isBrowseTab('categories'), true)
    assert.equal(isBrowseTab('live'), true)
  })

  it('rejects unsupported tab values', () => {
    assert.equal(isBrowseTab('top'), false)
    assert.equal(isBrowseTab(null), false)
  })

  it('persists the selected browse tab in sessionStorage', () => {
    const values = new Map<string, string>()
    const previous = globalThis.sessionStorage
    Object.defineProperty(globalThis, 'sessionStorage', {
      configurable: true,
      value: {
        getItem: (key: string) => values.get(key) ?? null,
        setItem: (key: string, value: string) => values.set(key, value),
      },
    })

    try {
      assert.equal(getStoredBrowseTab(), 'categories')
      setStoredBrowseTab('live')
      assert.equal(values.get(BROWSE_TAB_STORAGE_KEY), 'live')
      assert.equal(getStoredBrowseTab(), 'live')
      values.set(BROWSE_TAB_STORAGE_KEY, 'unknown')
      assert.equal(getStoredBrowseTab(), 'categories')
    } finally {
      Object.defineProperty(globalThis, 'sessionStorage', {
        configurable: true,
        value: previous,
      })
    }
  })
})

describe('browseTabFromPathname', () => {
  it('maps browse routes to tabs', () => {
    assert.equal(browseTabFromPathname('/browse'), 'categories')
    assert.equal(browseTabFromPathname('/browse/live'), 'live')
    assert.equal(browseTabFromPathname('/browse/category/123'), null)
  })
})

describe('browseTabPath', () => {
  it('returns browse routes for each tab', () => {
    assert.equal(browseTabPath('categories'), '/browse')
    assert.equal(browseTabPath('live'), '/browse/live')
  })
})
