/** Analytics-aligned palette for Clip Studio (matches chartTheme.ts). */
export const STUDIO_THEME = {
  bg: '#0d0d12',
  surface: 'rgba(18, 19, 26, 0.92)',
  card: 'rgba(22, 23, 32, 0.95)',
  border: 'rgba(255, 255, 255, 0.08)',
  cyan: '#22d3ee',
  violet: '#a78bfa',
  emerald: '#34d399',
  rose: '#fb7185',
  muted: '#a1a1aa',
  text: '#f4f4f5',
} as const

export const FORMAT_OPTIONS = [
  { id: 'tiktok' as const, label: '9:16', hint: 'TikTok' },
  { id: 'youtube_short' as const, label: '9:16', hint: 'Shorts' },
  { id: 'instagram_reel' as const, label: '9:16', hint: 'Reels' },
  { id: 'youtube' as const, label: '16:9', hint: 'YouTube' },
  { id: 'twitter' as const, label: '1:1', hint: 'Square' },
]

export const TEMPLATE_PREVIEW_COLORS: Record<string, string> = {
  tiktok_pop: 'linear-gradient(135deg,#fbbf24,#ef4444)',
  gaming_punch: 'linear-gradient(135deg,#22d3ee,#6366f1)',
  cinematic: 'linear-gradient(135deg,#1e293b,#475569)',
  clean_vertical: 'linear-gradient(135deg,#a78bfa,#ec4899)',
  subtitle_bar: 'linear-gradient(135deg,#0f172a,#334155)',
  stacked_reaction: 'linear-gradient(135deg,#f97316,#8b5cf6)',
  karaoke_highlight: 'linear-gradient(135deg,#fde047,#a855f7)',
  react_cam: 'linear-gradient(135deg,#06b6d4,#f43f5e)',
  minimal_subs: 'linear-gradient(135deg,#18181b,#3f3f46)',
  hype_moment: 'linear-gradient(135deg,#ef4444,#eab308)',
  podcast_clip: 'linear-gradient(135deg,#334155,#94a3b8)',
  streamer_rant: 'linear-gradient(135deg,#dc2626,#7c2d12)',
  hype_zoom: 'linear-gradient(135deg,#fde047,#f97316)',
  meme_impact: 'linear-gradient(135deg,#4ade80,#166534)',
  podcast_clean: 'linear-gradient(135deg,#94a3b8,#1e293b)',
  horror_vignette: 'linear-gradient(135deg,#450a0a,#18181b)',
  sports_energy: 'linear-gradient(135deg,#38bdf8,#dc2626)',
  minimal_white: 'linear-gradient(135deg,#f4f4f5,#d4d4d8)',
  vtuber_pastel: 'linear-gradient(135deg,#fbcfe8,#c4b5fd)',
  rage_clip: 'linear-gradient(135deg,#ef4444,#7f1d1d)',
  news_lower_third: 'linear-gradient(135deg,#1d4ed8,#0f172a)',
  slowmo_cinematic: 'linear-gradient(135deg,#312e81,#64748b)',
}
