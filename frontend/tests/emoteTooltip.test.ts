import assert from 'node:assert/strict'
import test from 'node:test'

import {
  formatEmoteProviderLabel,
  formatEmoteStackTooltipLabel,
  formatEmoteTooltipLabel,
} from '../src/utils/emoteTooltip.ts'

test('formatEmoteTooltipLabel includes provider when known', () => {
  assert.equal(formatEmoteTooltipLabel('KEKW', 'seventv'), 'KEKW · 7TV')
  assert.equal(formatEmoteTooltipLabel('Kappa', 'twitch'), 'Kappa · Twitch')
})

test('formatEmoteTooltipLabel falls back to id then generic label', () => {
  assert.equal(formatEmoteTooltipLabel('', 'twitch', '304894101'), '304894101 · Twitch')
  assert.equal(formatEmoteTooltipLabel('', undefined, ''), 'Emote')
})

test('formatEmoteStackTooltipLabel joins stacked emotes', () => {
  assert.equal(
    formatEmoteStackTooltipLabel('BASE', 'seventv', [{ name: 'ZW1', provider: 'seventv' }]),
    'BASE · 7TV ZW1 · 7TV',
  )
})

test('formatEmoteProviderLabel normalizes aliases', () => {
  assert.equal(formatEmoteProviderLabel('7tv'), '7TV')
  assert.equal(formatEmoteProviderLabel('frankerfacez'), 'FFZ')
  assert.equal(formatEmoteProviderLabel('custom'), undefined)
})
