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
    barSpike: 0.72,
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
    opacity: 0.85,
    dotRadius: 4,
    hoverRadius: 5.5,
  },
  emoteOverlay: 0.13,
  legendSwatch: 0.7,
  perEmotePalette: ['#fb7185', '#fbbf24', '#38bdf8', '#c084fc', '#4ade80'],
  moment: {
    selected: '#f59e0b',
    preview: 'rgba(245,158,11,0.45)',
  },
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

/** Thin vertical line swatch for legend chips (outline style, not filled dot). */
export function emoteLegendSwatchStyle(color: string): {
  width: string
  height: string
  borderRadius: string
  backgroundColor: string
  flexShrink: number
} {
  return {
    width: '2px',
    height: '12px',
    borderRadius: '1px',
    backgroundColor: hexToRgba(color, 0.85),
    flexShrink: 0,
  }
}

export function emoteChartColor(index: number): string {
  const palette = CHART_THEME.perEmotePalette
  return palette[((index % palette.length) + palette.length) % palette.length]
}

export function emoteChartColorForKey(key: string, orderedPlottedKeys: string[]): string {
  const index = orderedPlottedKeys.indexOf(key)
  return emoteChartColor(index >= 0 ? index : 0)
}

export function orderedEmoteColors(keys: string[]): string[] {
  return keys.map((_, index) => emoteChartColor(index))
}

export function emoteChipSelectionStyle(
  color: string | undefined,
  opts: { selected?: boolean; plotted?: boolean } = {},
): {
  borderColor: string
  backgroundColor: string
  color: string
} {
  const { selected = false, plotted = false } = opts
  if (!color || (!selected && !plotted)) {
    return {
      borderColor: 'rgba(255,255,255,0.1)',
      backgroundColor: 'rgba(255,255,255,0.04)',
      color: 'rgb(212 212 216)',
    }
  }
  if (selected) {
    return {
      borderColor: hexToRgba(color, 0.55),
      backgroundColor: hexToRgba(color, 0.08),
      color: 'rgb(244 244 245)',
    }
  }
  return {
    borderColor: hexToRgba(color, 0.35),
    backgroundColor: hexToRgba(color, 0.06),
    color: 'rgb(244 244 245)',
  }
}
