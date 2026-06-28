/** Muted chart palette for the Analytics dashboard (cyan / violet / emerald). */
export const CHART_THEME = {
  background: '#0d0d12',
  viewer: {
    color: '#22d3ee',
    fillTop: 0.16,
    fillBottom: 0,
    line: 0.85,
    dot: 0.7,
    guide: 0.15,
  },
  emote: {
    color: '#34d399',
    bar: 0.4,
    barBaseline: 0.15,
    barSpike: 0.55,
    line: 0.55,
    guide: 0.28,
  },
  chat: {
    color: '#a78bfa',
    line: '#d4d4d8',
    lineOpacity: 0.72,
    whisperBar: 0.12,
    guide: 0.22,
  },
  spike: {
    color: '#fb7185',
    opacity: 0.5,
    dotRadius: 2.5,
  },
  emoteOverlay: 0.13,
  legendSwatch: 0.7,
  perEmotePalette: ['#f59e0b', '#fb7185', '#60a5fa', '#f472b6', '#facc15'],
} as const

export function hexToRgba(hex: string, opacity: number): string {
  const normalized = hex.replace('#', '')
  const r = parseInt(normalized.slice(0, 2), 16)
  const g = parseInt(normalized.slice(2, 4), 16)
  const b = parseInt(normalized.slice(4, 6), 16)
  return `rgba(${r}, ${g}, ${b}, ${opacity})`
}

export function legendDotStyle(color: string): { background: string } {
  return { background: hexToRgba(color, CHART_THEME.legendSwatch) }
}
