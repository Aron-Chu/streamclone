import assert from 'node:assert/strict'
import { describe, it } from 'node:test'
import { isBrokenLocalEmotePath, resolveEmoteImageUrl } from '../src/utils/emoteImageUrl.ts'

describe('resolveEmoteImageUrl', () => {
  it('uses Twitch CDN for native Twitch ids', () => {
    const cases = [
      ['304894101', 'https://static-cdn.jtvnw.net/emoticons/v2/304894101/default/dark/2.0'],
      ['22639', 'https://static-cdn.jtvnw.net/emoticons/v2/22639/default/dark/2.0'],
      [
        'emotesv2_34dda6b8341e46d0b2118a9cabbe6a2e',
        'https://static-cdn.jtvnw.net/emoticons/v2/emotesv2_34dda6b8341e46d0b2118a9cabbe6a2e/default/dark/2.0',
      ],
    ] as const
    for (const [id, want] of cases) {
      assert.equal(resolveEmoteImageUrl({ provider: 'twitch', id, scale: '1x' }), want)
    }
  })

  it('uses local path for synced UUID emotes', () => {
    const localID = '75f49395-d5fc-41da-998c-880c6d8fddcb'
    const want = `/emotes/${localID}/1x.webp`
    for (const provider of ['seventv', 'ffz', 'bttv', 'custom']) {
      assert.equal(resolveEmoteImageUrl({ provider, id: localID, scale: '1x' }), want)
    }
  })

  it('uses 7TV CDN for legacy provider ids', () => {
    assert.equal(
      resolveEmoteImageUrl({ provider: 'seventv', id: '62a3bf572b964d6cc2766004', scale: '1x' }),
      'https://cdn.7tv.app/emote/62a3bf572b964d6cc2766004/4x.webp',
    )
  })

  it('returns empty url for empty id', () => {
    assert.equal(resolveEmoteImageUrl({ provider: 'twitch', id: '', scale: '1x' }), '')
  })

  it('keeps valid local UUID paths from imageUrl', () => {
    const localID = '75f49395-d5fc-41da-998c-880c6d8fddcb'
    const url = `/emotes/${localID}/2x.webp`
    assert.equal(resolveEmoteImageUrl({ provider: 'seventv', id: localID, imageUrl: url, scale: '2x' }), url)
  })

  it('replaces broken local paths for Twitch ids', () => {
    assert.equal(
      resolveEmoteImageUrl({ provider: 'twitch', id: '304894101', imageUrl: '/emotes/304894101/1x.webp', scale: '1x' }),
      'https://static-cdn.jtvnw.net/emoticons/v2/304894101/default/dark/2.0',
    )
    assert.equal(isBrokenLocalEmotePath('/emotes/304894101/1x.webp'), true)
    assert.equal(isBrokenLocalEmotePath('/emotes/75f49395-d5fc-41da-998c-880c6d8fddcb/1x.webp'), false)
  })
})
