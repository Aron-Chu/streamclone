import assert from 'node:assert/strict'
import test from 'node:test'

import { vodThumbnailUrl } from '../src/utils/vodThumbnail.ts'

test('VOD thumbnail helper fills Twitch Helix width and height placeholders', () => {
  const url = vodThumbnailUrl('https://static-cdn.jtvnw.net/cf_vods/thumb-%{width}x%{height}.jpg')
  assert.equal(url, 'https://static-cdn.jtvnw.net/cf_vods/thumb-160x90.jpg')
})

test('VOD thumbnail helper supports brace-only placeholders', () => {
  const url = vodThumbnailUrl('https://example.test/{width}/{height}/thumb.jpg', 320, 180)
  assert.equal(url, 'https://example.test/320/180/thumb.jpg')
})

test('VOD thumbnail helper returns empty string when no thumbnail exists', () => {
  assert.equal(vodThumbnailUrl(undefined), '')
  assert.equal(vodThumbnailUrl(''), '')
})
