import assert from 'node:assert/strict'
import { describe, it } from 'node:test'
import { hrefForLink, splitLinkifyParts } from '../src/utils/linkifyText.ts'

function plainText(parts: ReturnType<typeof splitLinkifyParts>): string {
  return parts.map(part => part.type === 'url' ? part.value + (part.suffix ?? '') : part.value).join('')
}

describe('splitLinkifyParts', () => {
  it('returns plain text unchanged when no URLs', () => {
    const parts = splitLinkifyParts('hello world')
    assert.equal(plainText(parts), 'hello world')
    assert.equal(parts.filter(part => part.type === 'url').length, 0)
  })

  it('linkifies https URLs', () => {
    const parts = splitLinkifyParts('see https://twitch.tv/xqc now')
    assert.equal(parts.filter(part => part.type === 'url').length, 1)
    assert.ok(plainText(parts).includes('https://twitch.tv/xqc'))
  })

  it('linkifies www URLs', () => {
    const parts = splitLinkifyParts('visit www.example.com/path')
    assert.equal(parts.filter(part => part.type === 'url').length, 1)
  })

  it('linkifies www-prefixed hosts in prose', () => {
    const parts = splitLinkifyParts('see also www.example.com for details')
    assert.equal(parts.filter(part => part.type === 'url').length, 1)
  })

  it('keeps trailing punctuation outside the link', () => {
    const parts = splitLinkifyParts('go to https://example.com.')
    assert.equal(parts.filter(part => part.type === 'url').length, 1)
    assert.equal(parts.find(part => part.type === 'url')?.suffix, '.')
  })

  it('handles mixed text and multiple URLs', () => {
    const parts = splitLinkifyParts('a https://a.com b www.b.tv c')
    assert.equal(parts.filter(part => part.type === 'url').length, 2)
  })
})

describe('hrefForLink', () => {
  it('prefixes bare domains with https', () => {
    assert.equal(hrefForLink('twitch.tv/xqc'), 'https://twitch.tv/xqc')
  })
})
